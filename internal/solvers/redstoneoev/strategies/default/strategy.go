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
	cfg      Config
	callback common.Address
	reader   Reader
	signer   signer
	chainID  *big.Int
	mon      monitorSource
	engine   bundleEngine
	state    decisionStateCache
	maxAge   time.Duration
	log      logr.Logger
}

type decisionState struct {
	Rate           *big.Int
	CallbackNative *big.Int
	UpdatedAt      time.Time
}

type decisionStateCache struct {
	v atomic.Value // stores *decisionState
}

func (c *decisionStateCache) store(st decisionState) {
	st.Rate = cloneBig(st.Rate)
	st.CallbackNative = cloneBig(st.CallbackNative)
	c.v.Store(&st)
}

func (c *decisionStateCache) load() (decisionState, bool) {
	v := c.v.Load()
	if v == nil {
		return decisionState{}, false
	}
	st := *(v.(*decisionState))
	st.Rate = cloneBig(st.Rate)
	st.CallbackNative = cloneBig(st.CallbackNative)
	return st, true
}

//nolint:gochecknoinits // solver-local strategy self-registration mirrors solver registration.
func init() {
	strategies.Register(Name, NewFromConfig)
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
	if deps.Adapter == (common.Address{}) {
		return nil, errors.New("adapter is required")
	}
	if deps.Callback == (common.Address{}) {
		return nil, errors.New("callback is required")
	}
	cfg.Adapter = deps.Adapter
	return New(cfg, Deps{
		Reader:      NewChainReader(deps.Chain, deps.Log),
		Signer:      deps.Signer,
		Log:         deps.Log,
		ChainID:     deps.ChainID,
		Callback:    deps.Callback,
		TestMonitor: testMonitor,
	})
}

func New(cfg Config, deps Deps) (*Strategy, error) {
	if deps.Callback == (common.Address{}) {
		return nil, errors.New("callback is required")
	}
	var (
		mon monitorSource
		err error
	)
	if deps.TestMonitor {
		mon, err = newTestMonitor(deps.Reader, deps.Log, cfg, deps.Callback)
		if err != nil {
			return nil, err
		}
	} else {
		if cfg.MorphoAPIURL == "" {
			return nil, errors.New("morphoApiUrl is required unless test monitor is enabled")
		}
		mon = newAPIMonitor(deps.Reader, deps.Log, cfg, deps.ChainID)
	}
	return &Strategy{
		cfg:      cfg,
		callback: deps.Callback,
		reader:   deps.Reader,
		signer:   deps.Signer,
		chainID:  big.NewInt(deps.ChainID),
		mon:      mon,
		engine:   newBundleEngine(cfg, deps.Log),
		maxAge:   cfg.MaxStateAge,
		log:      deps.Log,
	}, nil
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
	now := time.Now()
	rate := s.reader.ReadLoanEthRate(ctx, s.cfg.Adapter, s.cfg.LoanEthFeed, now)
	callbackNative, err := s.reader.ReadNativeBalance(ctx, s.callback)
	if err != nil {
		s.log.Error(err, "read callback balance failed; keeping last cached balance", "callback", s.callback.Hex())
		if prev, ok := s.state.load(); ok {
			callbackNative = prev.CallbackNative
		}
	}
	s.state.store(decisionState{Rate: rate, CallbackNative: callbackNative, UpdatedAt: now})
}

func (s *Strategy) DecideBid(_ context.Context, input types.BidInput) (types.BidOutput, error) {
	if input.Adapter.Address != (common.Address{}) && input.Adapter.Address != s.cfg.Adapter {
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
	if !ok || (s.maxAge > 0 && input.Now.Sub(st.UpdatedAt) > s.maxAge) {
		return skipBid(skipStaleState), nil
	}
	if skip := snapshotFreshForAuction(snap, input.Auction); skip != "" {
		return skipBid(skip), nil
	}
	if hasPendingAuctions(input.PendingAuctions, input.Now) {
		return skipBid(types.SkipReasonInFlight), nil
	}
	scored := s.scoredLegs(input.Auction, input.Now, input.Adapter)
	if len(scored) == 0 {
		return skipBid(skipNoLegs), nil
	}
	rate := validRate(st.Rate)
	if rate == nil {
		s.log.Info("bid skipped: loan/ETH rate unavailable",
			"auction", input.Auction.ID, "scoredLegs", len(scored), "feedCount", len(input.Auction.Prices))
		return skipBid(skipGasUnprofitable), nil
	}
	laneState := liquidLaneStateFromAdapter(input.Adapter)
	gasPrice := cloneBig(input.Context.MaxTxGasPrice)
	feedCount := len(input.Auction.Prices)
	b, skip := s.engine.selectNetBundle(scored, rate, laneState, gasPrice, input.Context.GasLimit, feedCount)
	if skip != "" {
		if skip == skipGasUnprofitable && len(b.legs) > 0 {
			s.engine.logBundleEconomics(input.Auction.ID, "bid skipped: bundle is not profitable after gas and bid",
				b, rate, laneState, gasPrice, input.Context.GasLimit, feedCount, len(scored))
		}
		return types.BidOutput{Decision: types.DecisionSkip, Reason: skip}, nil
	}
	priced := s.engine.priceBundle(b, rate, laneState, gasPrice, feedCount)
	if orZero(st.CallbackNative).Cmp(priced.bidNative) < 0 {
		s.log.Info("bid skipped: callback balance cannot cover bid",
			"auction", input.Auction.ID, "callback", s.callback.Hex(),
			"callbackWei", st.CallbackNative, "requiredWei", priced.bidNative)
		return skipBid(types.SkipReasonCallbackBalance), nil
	}
	return s.bidOutputFromBundle(input, priced)
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

func hasPendingAuctions(in []types.PendingAuction, now time.Time) bool {
	for _, a := range in {
		if a.ID == "" {
			continue
		}
		if a.ExpiresAt.IsZero() || now.Before(a.ExpiresAt) {
			return true
		}
	}
	return false
}

var _ types.Strategy = (*Strategy)(nil)
