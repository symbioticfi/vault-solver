package defaultstrategy

import (
	"context"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
	"gopkg.in/yaml.v3"
)

const Name = "default"

type monitorSource interface {
	run(context.Context)
	refresh(context.Context)
	snapshot() *snapshot
	candidates(auction types.AuctionSnapshot, nowTs uint64, adapter types.AdapterSnapshot) []evalItem
}

type Strategy struct {
	cfg           Config
	adapter       common.Address
	callback      common.Address
	gasAccounting bool
	reader        Reader
	signer        signer
	chainID       *big.Int
	mon           monitorSource
	engine        bundleEngine
	state         decisionStateCache
	reservations  decisionReservations
	log           logr.Logger
}

type decisionState struct {
	CallbackNative    *big.Int
	CallbackUpdatedAt time.Time
}

type decisionStateCache struct {
	v atomic.Value // stores *decisionState
}

func (c *decisionStateCache) store(st decisionState) {
	st.CallbackNative = cloneBig(st.CallbackNative)
	c.v.Store(&st)
}

func (c *decisionStateCache) load() (decisionState, bool) {
	v := c.v.Load()
	if v == nil {
		return decisionState{}, false
	}
	st := *(v.(*decisionState))
	st.CallbackNative = cloneBig(st.CallbackNative)
	return st, true
}

//nolint:gochecknoinits // solver-local strategy self-registration mirrors solver registration.
func init() {
	strategies.Register(Name, strategies.Registration{Factory: NewFromConfig, ValidateConfig: ValidateConfig})
}

func ValidateConfig(raw yaml.Node, deps strategies.ValidationDeps) error {
	cfg, err := ParseConfig(raw)
	if err != nil {
		return err
	}
	return validateConfig(cfg, deps.GasAccounting)
}

func NewFromConfig(raw yaml.Node, deps strategies.Deps) (types.Strategy, error) {
	cfg, err := ParseConfig(raw)
	if err != nil {
		return nil, err
	}
	return New(cfg, Deps{
		Reader:              newChainReader(deps.Chain, deps.Log),
		Signer:              deps.Signer,
		Log:                 deps.Log,
		ChainID:             deps.ChainID,
		Adapter:             deps.Adapter,
		Callback:            deps.Callback,
		LoadAdapterSnapshot: deps.LoadAdapterSnapshot,
		GasAccounting:       deps.GasAccounting,
	})
}

func New(cfg Config, deps Deps) (*Strategy, error) {
	if deps.Callback == (common.Address{}) {
		return nil, errors.New("callback is required")
	}
	if deps.Adapter == (common.Address{}) {
		return nil, errors.New("adapter is required")
	}
	if deps.LoadAdapterSnapshot == nil {
		return nil, errors.New("adapter snapshot source is required")
	}
	if err := validateConfig(cfg, deps.GasAccounting); err != nil {
		return nil, err
	}
	var (
		mon monitorSource
		err error
	)
	if cfg.TestMonitor != nil {
		mon, err = newTestMonitor(deps.Reader, deps.Log, cfg, deps.Callback, deps.LoadAdapterSnapshot, cfg.TestMonitor)
		if err != nil {
			return nil, err
		}
	} else {
		mon = newAPIMonitor(deps.Log, cfg, deps.ChainID, deps.LoadAdapterSnapshot)
	}
	return &Strategy{
		cfg:           cfg,
		adapter:       deps.Adapter,
		callback:      deps.Callback,
		gasAccounting: deps.GasAccounting,
		reader:        deps.Reader,
		signer:        deps.Signer,
		chainID:       big.NewInt(deps.ChainID),
		mon:           mon,
		engine:        newBundleEngine(cfg, deps.Log),
		log:           deps.Log,
	}, nil
}

func validateConfig(cfg Config, gasAccounting bool) error {
	if !gasAccounting && cfg.TotalBundleProfitBps != 0 {
		return errors.New("strategy.config.bid.totalBundleProfitBps requires gas accounting")
	}
	if !gasAccounting && cfg.MinBundleProfitBidBps != 0 {
		return errors.New("strategy.config.bid.minBundleProfitBidBps requires gas accounting")
	}
	if cfg.TestMonitor == nil && cfg.MorphoAPIURL == "" {
		return errors.New("morphoApiUrl is required unless strategy.config.testMonitor is configured")
	}
	return nil
}

func (s *Strategy) Run(ctx context.Context) {
	s.refreshState(ctx)
	s.mon.refresh(ctx)
	var wg sync.WaitGroup
	wg.Go(func() { s.stateLoop(ctx) })
	wg.Go(func() { s.mon.run(ctx) })
	wg.Wait()
}

func (s *Strategy) stateLoop(ctx context.Context) {
	interval := s.cfg.MonitorPoll
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshState(ctx)
		}
	}
}

func (s *Strategy) refreshState(ctx context.Context) {
	if s.reader == nil {
		return
	}
	prev, _ := s.state.load()
	callbackNative, err := s.reader.ReadNativeBalance(ctx, s.callback)
	callbackUpdatedAt := time.Now()
	if err != nil {
		s.log.Error(err, "read callback balance failed; keeping last cached balance", "callback", s.callback.Hex())
		callbackNative = prev.CallbackNative
		callbackUpdatedAt = prev.CallbackUpdatedAt
	}
	s.state.store(decisionState{
		CallbackNative:    callbackNative,
		CallbackUpdatedAt: callbackUpdatedAt,
	})
}

func (s *Strategy) DecideBid(_ context.Context, input types.BidInput) (types.BidOutput, error) {
	if input.Adapter.Address != (common.Address{}) && input.Adapter.Address != s.adapter {
		return skipBid(skipNoLegs), nil
	}
	if input.Context.Callback != s.callback {
		return skipBid(skipNoLegs), nil
	}
	if !input.Adapter.Filler {
		return skipBid(skipNoLegs), nil
	}
	if input.Adapter.Paused {
		return skipBid(skipNoLegs), nil
	}
	snap := s.mon.snapshot()
	if snap == nil || (s.cfg.MaxStateAge > 0 && input.Now.Sub(snap.updatedAt) > s.cfg.MaxStateAge) {
		return skipBid(skipStaleState), nil
	}
	st, ok := s.state.load()
	if !ok || !freshAt(st.CallbackUpdatedAt, input.Now, s.cfg.MaxStateAge) {
		return skipBid(skipStaleState), nil
	}
	if skip := snapshotFreshForAuction(snap, input.Auction); skip != "" {
		return skipBid(skip), nil
	}
	reserved := s.reservations.reconcile(input.PendingAuctions, input.Now, st.CallbackUpdatedAt)
	scored := s.scoredLegs(input.Auction, input.Now, input.Adapter)
	scored = filterReservedPositions(scored, reserved.positions)
	if len(scored) == 0 {
		return skipBid(skipNoLegs), nil
	}
	laneState := liquidLaneStateFromAdapter(input.Adapter)
	gasPrice := cloneBig(input.Context.MaxTxGasPrice)
	feedCount := auctionFeedCount(input.Auction)
	priced, skip := s.selectAndPriceBundle(input, scored, laneState, gasPrice, feedCount)
	if skip != "" {
		return types.BidOutput{Decision: types.DecisionSkip, Reason: skip}, nil
	}
	reservedAndCurrentGas := new(big.Int).Add(reserved.gasNative, priced.gasNative)
	if !depositCoversSettlementGas(input.Context.ExecutorDeposit, input.Context.ExecutorMinDeposit, reservedAndCurrentGas) {
		s.log.Info("bid skipped: executor deposit cannot cover predicted settlement gas",
			"auction", input.Auction.ID,
			"depositWei", input.Context.ExecutorDeposit,
			"requiredWei", executorDepositRequired(input.Context.ExecutorMinDeposit, reservedAndCurrentGas),
			"reservedGasWei", reserved.gasNative,
			"minDepositWei", input.Context.ExecutorMinDeposit,
			"gasUnits", priced.gas.Units,
			"gasNative", priced.gasNative,
			"gasPriceWei", gasPrice)
		return skipBid(types.SkipReasonDepositLow), nil
	}
	availableCallback := new(big.Int).Sub(orZero(st.CallbackNative), reserved.bidNative)
	if availableCallback.Cmp(priced.bidNative) < 0 {
		s.log.Info("bid skipped: callback balance cannot cover bid",
			"auction", input.Auction.ID, "callback", s.callback.Hex(),
			"callbackWei", st.CallbackNative, "reservedBidWei", reserved.bidNative,
			"availableWei", availableCallback, "requiredWei", priced.bidNative)
		return skipBid(types.SkipReasonCallbackBalance), nil
	}
	out, err := s.bidOutputFromBundle(input, priced)
	if err != nil {
		return types.BidOutput{}, err
	}
	s.reservations.reserve(input.Auction.ID, priced)
	return out, nil
}

func (s *Strategy) selectAndPriceBundle(
	input types.BidInput,
	scored []scoredLeg,
	laneState *liquidLaneState,
	gasPrice *big.Int,
	feedCount int,
) (pricedBundle, string) {
	if !s.gasAccounting {
		bundle, skip := s.engine.selectBundleWithGas(scored, laneState, input.Context.GasLimit, feedCount)
		if skip != "" {
			return pricedBundle{}, skip
		}
		return s.engine.priceBundleWithoutGasAccounting(bundle, laneState, gasPrice, feedCount), ""
	}

	rate := validRate(input.Context.GasPrices.TokenOutPerNative(input.Adapter.Loan))
	if rate == nil {
		s.log.Info("bid skipped: loan/native gas rate unavailable",
			"auction", input.Auction.ID, "scoredLegs", len(scored), "feedCount", feedCount)
		return pricedBundle{}, skipGasUnprofitable
	}
	bundle, skip := s.engine.selectNetBundle(
		scored, rate, laneState, gasPrice, input.Context.GasLimit, feedCount,
	)
	if skip == "" {
		return s.engine.priceBundle(bundle, rate, laneState, gasPrice, feedCount), ""
	}
	if skip == skipGasUnprofitable && len(bundle.legs) > 0 {
		s.engine.logBundleEconomics(input.Auction.ID, "bid skipped: bundle is not profitable after gas and bid",
			bundle, rate, laneState, gasPrice, input.Context.GasLimit, feedCount, len(scored))
	}
	return pricedBundle{}, skip
}

func (s *Strategy) scoredLegs(a types.AuctionSnapshot, now time.Time, adapter types.AdapterSnapshot) []scoredLeg {
	nowTs := clampTsAt(a.Timestamp, now)
	cands := s.mon.candidates(a, nowTs, adapter)
	out := make([]scoredLeg, 0, len(cands))
	for _, it := range cands {
		if sized, ok := sizeLeg(it.cand, it.price, it.quote, it.accrued, s.cfg.Sizing); ok {
			out = append(out, scoredLeg{
				bundleLeg: bundleLeg{
					selectedLeg:     sized.leg,
					expectedLoanOut: sized.expectedLoanOut,
					collateral:      it.cand.Market.Params.CollateralToken,
				},
				profit:    sized.profit,
				maxAssets: it.quote.MaxAssets,
				source:    it,
				replay:    true,
			})
		}
	}
	return out
}

func auctionFeedCount(a types.AuctionSnapshot) int {
	if a.RawPriceCount > 0 {
		return a.RawPriceCount
	}
	return len(a.Prices)
}

func skipBid(reason string) types.BidOutput {
	return types.BidOutput{Decision: types.DecisionSkip, Reason: reason}
}

func (s *Strategy) bidOutputFromBundle(input types.BidInput, priced pricedBundle) (types.BidOutput, error) {
	if s.signer == nil {
		return types.BidOutput{}, errors.New("signer is required")
	}
	auth := operationAuth{
		AuctionKey:      auctionKeyHash(input.Auction.ID),
		BidAmount:       cloneBig(priced.bidNative),
		MinBundleProfit: cloneBig(priced.minBundleProfitLoan),
		Deadline:        callbackAuthDeadline(input.Now, s.cfg.CallbackAuthTTL),
	}
	chainID := cloneBig(input.Context.ChainID)
	if chainID == nil {
		chainID = cloneBig(s.chainID)
	}
	if chainID == nil || chainID.Sign() <= 0 {
		return types.BidOutput{}, errors.New("chain id is required")
	}
	authDigest, err := callbackAuthDigest(chainID, input.Context.Callback, input.Context.Executor, auth, priced.selectedLegs)
	if err != nil {
		return types.BidOutput{}, err
	}
	authSig, err := s.signer.SignHash(authDigest)
	if err != nil {
		return types.BidOutput{}, errors.Errorf("sign callback auth: %w", err)
	}
	opData, err := encodeOperationData(auth, priced.selectedLegs, authSig)
	if err != nil {
		return types.BidOutput{}, err
	}
	return types.BidOutput{
		Decision:      types.DecisionBid,
		BidAmount:     cloneBig(priced.bidNative),
		OperationData: opData,
	}, nil
}

func legHints(in []bundleLeg) []legHint {
	out := make([]legHint, len(in))
	for i, leg := range in {
		out[i] = legHint{
			selectedLeg:     leg.selectedLeg,
			Collateral:      leg.collateral,
			ExpectedLoanOut: cloneBig(leg.expectedLoanOut),
		}
	}
	return out
}

func liquidLaneStateFromAdapter(adapter types.AdapterSnapshot) *liquidLaneState {
	if adapter.FreeAssets == nil || adapter.Withdrawable == nil {
		return nil
	}
	st := &liquidLaneState{
		FreeAssets:   cloneBig(adapter.FreeAssets),
		Withdrawable: cloneBig(adapter.Withdrawable),
		Acquire:      make(map[common.Address]*big.Int, len(adapter.Redeemable)),
	}
	for _, r := range adapter.Redeemable {
		if r.Asset == (common.Address{}) {
			continue
		}
		st.Acquire[r.Asset] = cloneBig(r.AcquireBalance)
	}
	return st
}

func freshAt(updatedAt, now time.Time, maxAge time.Duration) bool {
	return !updatedAt.IsZero() && (maxAge <= 0 || now.Sub(updatedAt) <= maxAge)
}

var _ types.Strategy = (*Strategy)(nil)
