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
	"github.com/symbioticfi/vault-solver/internal/observability"
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
	maxAge        time.Duration
	log           logr.Logger
	tracer        *observability.Tracer
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
	strategies.Register(Name, strategies.Registration{Factory: NewFromConfig})
}

func NewFromConfig(raw yaml.Node, deps strategies.Deps) (types.Strategy, error) {
	testMonitor, err := testMonitorFromEnv()
	if err != nil {
		return nil, err
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		return nil, err
	}
	return New(cfg, Deps{
		Solver:              deps.Solver,
		Reader:              newChainReader(deps.Chain),
		Signer:              deps.Signer,
		Log:                 deps.Log,
		ChainID:             deps.ChainID,
		Adapter:             deps.Adapter,
		Callback:            deps.Callback,
		LoadAdapterSnapshot: deps.LoadAdapterSnapshot,
		GasAccounting:       deps.GasAccounting,
		TestMonitor:         testMonitor,
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
	if !deps.GasAccounting && cfg.TotalBundleProfitBps != 0 {
		return nil, errors.New("strategy.config.bid.totalBundleProfitBps requires gas accounting")
	}
	if !deps.GasAccounting && cfg.MinBundleProfitBidBps != 0 {
		return nil, errors.New("strategy.config.bid.minBundleProfitBidBps requires gas accounting")
	}
	tracer := newStrategyTracer(deps.Solver)
	var (
		mon monitorSource
		err error
	)
	if deps.TestMonitor {
		mon, err = newTestMonitor(deps.Reader, deps.Log, cfg, deps.Callback, deps.LoadAdapterSnapshot)
		if err != nil {
			return nil, err
		}
	} else {
		if cfg.MorphoAPIURL == "" {
			return nil, errors.New("morphoApiUrl is required unless test monitor is enabled")
		}
		mon = newAPIMonitor(deps.Log, cfg, deps.ChainID, deps.LoadAdapterSnapshot, tracer)
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
		engine:        newBundleEngine(cfg),
		maxAge:        cfg.MaxStateAge,
		log:           deps.Log,
		tracer:        tracer,
	}, nil
}

func (s *Strategy) Run(ctx context.Context) {
	// The strategy logger is the solver's; carry it so the state and monitor loops below log through
	// observability.Log(ctx) and pick up each stage's trace ids.
	ctx = observability.WithLogger(ctx, s.log)

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
		observability.Log(ctx).Error(
			err, "read callback balance failed; keeping last cached balance", "callback", s.callback.Hex())
		callbackNative = prev.CallbackNative
		callbackUpdatedAt = prev.CallbackUpdatedAt
	}
	s.state.store(decisionState{
		CallbackNative:    callbackNative,
		CallbackUpdatedAt: callbackUpdatedAt,
	})
}

func (s *Strategy) DecideBid(ctx context.Context, input types.BidInput) (types.BidOutput, error) {
	// The bid entry point: carry the strategy logger so every stage below logs through the context.
	ctx = observability.WithLogger(ctx, s.log)

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
	if snap == nil || (s.maxAge > 0 && input.Now.Sub(snap.updatedAt) > s.maxAge) {
		return skipBid(skipStaleState), nil
	}
	st, ok := s.state.load()
	if !ok || !freshAt(st.CallbackUpdatedAt, input.Now, s.maxAge) {
		return skipBid(skipStaleState), nil
	}
	if skip := snapshotFreshForAuction(snap, input.Auction); skip != "" {
		return skipBid(skip), nil
	}
	hasReservation := s.reservations.reconcile(input.PendingAuctions, input.Now, st.CallbackUpdatedAt)
	// Every bundle shares this adapter's routing liquidity and potentially Morpho market state.
	// Position-only reservations cannot authorize a second bundle against the same snapshot.
	if len(input.PendingAuctions) > 0 || hasReservation {
		return skipBid(types.SkipReasonInFlight), nil
	}
	nowTs := clampTsAt(input.Auction.Timestamp, input.Now)
	scored := s.sizedLegs(ctx, s.candidates(ctx, input.Auction, nowTs, input.Adapter))
	if len(scored) == 0 {
		return skipBid(skipNoLegs), nil
	}
	gasPrice := cloneBig(input.Context.MaxTxGasPrice)
	priced, skip := s.pricedBundleFor(ctx, input, scored, gasPrice)
	if skip != "" {
		return types.BidOutput{Decision: types.DecisionSkip, Reason: skip}, nil
	}
	if unaffordable := s.affordableBundle(ctx, input, priced, st, gasPrice); unaffordable != "" {
		return skipBid(unaffordable), nil
	}
	out, err := s.bidOutputFromBundle(input, priced)
	if err != nil {
		return types.BidOutput{}, err
	}
	s.reservations.reserve(input.Auction.ID)
	return out, nil
}

func (s *Strategy) candidates(
	ctx context.Context, a types.AuctionSnapshot, nowTs uint64, adapter types.AdapterSnapshot,
) []evalItem {
	_, end := s.tracer.Start(ctx, "oev.auction.candidates")
	defer end(nil)
	return s.mon.candidates(a, nowTs, adapter)
}

func (s *Strategy) sizedLegs(ctx context.Context, cands []evalItem) []scoredLeg {
	_, end := s.tracer.Start(ctx, "oev.auction.size")
	defer end(nil)
	out := make([]scoredLeg, 0, len(cands))
	for _, it := range cands {
		if sized, ok := sizeLeg(it.cand, it.price, it.quote, it.accrued, s.cfg.Sizing); ok {
			out = append(out, scoredLeg{
				selectedLeg:       sized.leg,
				settlementLoanOut: sized.settlementLoanOut,
				collateral:        it.cand.Market.Params.CollateralToken,
				profit:            sized.profit,
				maxAssets:         it.quote.MaxAssets,
				source:            it,
				replay:            true,
			})
		}
	}
	return out
}

// pricedBundleFor chooses the bundle of legs to settle together and prices it, under oev.auction.bundle.
func (s *Strategy) pricedBundleFor(
	ctx context.Context, input types.BidInput, scored []scoredLeg, gasPrice *big.Int,
) (pricedBundle, string) {
	ctx, end := s.tracer.Start(ctx, "oev.auction.bundle")
	defer end(nil)

	laneState := liquidLaneStateFromAdapter(input.Adapter)
	feedCount := auctionFeedCount(input.Auction)
	var (
		b      chosenBundle
		priced pricedBundle
		skip   string
	)
	if s.gasAccounting {
		rate := validRate(input.Context.GasPrices.TokenOutPerNative(input.Adapter.Loan))
		if rate == nil {
			observability.Log(ctx).Info("bid skipped: loan/native gas rate unavailable",
				"auctionId", input.Auction.ID, "scoredLegs", len(scored), "feedCount", feedCount)
			return pricedBundle{}, skipGasUnprofitable
		}
		b, skip = s.engine.selectNetBundle(scored, rate, laneState, gasPrice, input.Context.GasLimit, feedCount)
		if skip == "" {
			priced = s.engine.priceBundle(b, rate, laneState, gasPrice, feedCount)
		} else if skip == skipGasUnprofitable && len(b.legs) > 0 {
			s.engine.logBundleEconomics(ctx, input.Auction.ID,
				"bid skipped: bundle is not profitable after gas and bid",
				b, rate, laneState, gasPrice, input.Context.GasLimit, feedCount, len(scored))
		}
	} else {
		b, skip = s.engine.selectBundleWithGas(scored, laneState, input.Context.GasLimit, feedCount)
		if skip == "" {
			priced = s.engine.priceBundleWithoutGasAccounting(b, laneState, gasPrice, feedCount)
		}
	}
	return priced, skip
}

// affordableBundle is the economic gate on a priced bundle, under oev.auction.economics: the executor
// deposit must cover the predicted settlement gas and the callback must be able to pay the bid. It
// returns the skip reason, or "" when the bundle is affordable.
func (s *Strategy) affordableBundle(
	ctx context.Context, input types.BidInput, priced pricedBundle, st decisionState, gasPrice *big.Int,
) string {
	ctx, end := s.tracer.Start(ctx, "oev.auction.economics")
	defer end(nil)

	if !depositCoversSettlementGas(input.Context.ExecutorDeposit, input.Context.ExecutorMinDeposit, priced.gasNative) {
		observability.Log(ctx).Info("bid skipped: executor deposit cannot cover predicted settlement gas",
			"auctionId", input.Auction.ID,
			"depositWei", input.Context.ExecutorDeposit,
			"requiredWei", executorDepositRequired(input.Context.ExecutorMinDeposit, priced.gasNative),
			"minDepositWei", input.Context.ExecutorMinDeposit,
			"gasUnits", priced.gas.Units,
			"gasNative", priced.gasNative,
			"gasPriceWei", gasPrice)
		return types.SkipReasonDepositLow
	}
	availableCallback := orZero(st.CallbackNative)
	if availableCallback.Cmp(priced.bidNative) < 0 {
		observability.Log(ctx).Info("bid skipped: callback balance cannot cover bid",
			"auctionId", input.Auction.ID, "callback", s.callback.Hex(),
			"callbackWei", st.CallbackNative,
			"availableWei", availableCallback, "requiredWei", priced.bidNative)
		return types.SkipReasonCallbackBalance
	}
	return ""
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
			ExpectedLoanOut: cloneBig(leg.settlementLoanOut),
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
