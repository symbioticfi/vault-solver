package defaultstrategy

import (
	"context"
	"math/big"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"

	"github.com/symbioticfi/vault-solver/internal/parse"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
	"gopkg.in/yaml.v3"
)

const Name = "default"

type monitorSource interface {
	refresh(context.Context)
	snapshot() *snapshot
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
}

type decisionState struct {
	CallbackNative    *big.Int
	CallbackUpdatedAt time.Time
}

type decisionStateCache struct {
	v atomic.Pointer[decisionState]
}

func (c *decisionStateCache) store(st decisionState) {
	st.CallbackNative = bigmath.Clone(st.CallbackNative)
	c.v.Store(&st)
}

func (c *decisionStateCache) load() (decisionState, bool) {
	v := c.v.Load()
	if v == nil {
		return decisionState{}, false
	}
	st := *v
	st.CallbackNative = bigmath.Clone(st.CallbackNative)
	return st, true
}

func NewFromConfig(raw yaml.Node, deps types.Dependencies) (types.Strategy, error) {
	testMonitor, err := parse.Bool(os.Getenv(envTestMonitor), envTestMonitor)
	if err != nil {
		return nil, err
	}
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
	var (
		mon monitorSource
		err error
	)
	if deps.TestMonitor {
		mon, err = newTestMonitor(deps.Reader, deps.Log, deps.Callback, deps.LoadAdapterSnapshot)
		if err != nil {
			return nil, err
		}
	} else {
		if cfg.MorphoAPIURL == "" {
			return nil, errors.New("morphoApiUrl is required unless test monitor is enabled")
		}
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
		engine:        bundleEngine{cfg: cfg, log: deps.Log},
		maxAge:        cfg.MaxStateAge,
		log:           deps.Log,
	}, nil
}

// Run owns both independent refresh loops; no snapshot source starts background work.
func (s *Strategy) Run(ctx context.Context) {
	interval := s.cfg.MonitorPoll
	if interval <= 0 {
		interval = 10 * time.Second
	}
	var workers sync.WaitGroup
	for _, refresh := range []func(context.Context){s.refreshState, s.mon.refresh} {
		workers.Go(func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for ctx.Err() == nil {
				refresh(ctx)
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		})
	}
	workers.Wait()
}

func (s *Strategy) refreshState(ctx context.Context) {
	if s.reader == nil {
		return
	}
	balance, err := s.reader.ReadNativeBalance(ctx, s.callback)
	if err != nil {
		s.log.Error(err, "read callback balance failed; keeping last cached balance", "callback", s.callback.Hex())
		return
	}
	s.state.store(decisionState{CallbackNative: balance, CallbackUpdatedAt: time.Now()})
}

func (s *Strategy) DecideBid(ctx context.Context, input types.BidInput) (types.BidOutput, error) {
	if (input.Adapter.Address != (common.Address{}) && input.Adapter.Address != s.adapter) ||
		input.Context.Callback != s.callback || !input.Adapter.Filler || input.Adapter.Paused {
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
	reserved := s.reservations.reconcile(input.PendingAuctions, input.Now, st.CallbackUpdatedAt)
	scored := s.scoredLegs(snap, input.Auction, input.Now, input.Adapter)
	scored = filterReservedPositions(scored, reserved.positions)
	if len(scored) == 0 {
		return skipBid(skipNoLegs), nil
	}
	priced, reason := s.selectForBid(ctx, input, scored)
	if err := ctx.Err(); err != nil {
		return types.BidOutput{}, err
	}
	if reason != "" {
		return skipBid(reason), nil
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
			"gasPriceWei", input.Context.MaxTxGasPrice)
		return skipBid(types.SkipReasonDepositLow), nil
	}
	availableCallback := new(big.Int).Sub(bigmath.OrZero(st.CallbackNative), reserved.bidNative)
	if availableCallback.Cmp(priced.bidNative) < 0 {
		s.log.Info("bid skipped: callback balance cannot cover bid",
			"auction", input.Auction.ID, "callback", s.callback.Hex(),
			"callbackWei", st.CallbackNative, "reservedBidWei", reserved.bidNative,
			"availableWei", availableCallback, "requiredWei", priced.bidNative)
		return skipBid(types.SkipReasonCallbackBalance), nil
	}
	if err := ctx.Err(); err != nil {
		return types.BidOutput{}, err
	}
	out, err := s.bidOutputFromBundle(input, priced)
	if err != nil {
		return types.BidOutput{}, err
	}
	s.reservations.reserve(input.Auction.ID, priced)
	return out, nil
}

func (s *Strategy) selectForBid(ctx context.Context, input types.BidInput, scored []scoredLeg) (pricedBundle, string) {
	lane := liquidLaneStateFromAdapter(input.Adapter)
	gasPrice := bigmath.Clone(input.Context.MaxTxGasPrice)
	count := auctionFeedCount(input.Auction)
	if !s.gasAccounting {
		bundle, reason := s.engine.selectBundleWithGas(ctx, scored, lane, input.Context.GasLimit, count)
		if reason != "" {
			return pricedBundle{}, reason
		}
		return s.engine.priceBundleWithoutGasAccounting(bundle, lane, gasPrice, count), ""
	}
	rate := validRate(input.Context.GasPrices.TokenOutPerNative(input.Adapter.Loan))
	if rate == nil {
		s.log.Info("bid skipped: loan/native gas rate unavailable", "auction", input.Auction.ID, "scoredLegs", len(scored), "feedCount", count)
		return pricedBundle{}, skipGasUnprofitable
	}
	bundle, reason := s.engine.selectNetBundle(ctx, scored, rate, lane, gasPrice, input.Context.GasLimit, count)
	if reason != "" {
		if reason == skipGasUnprofitable && len(bundle.legs) > 0 {
			s.engine.logBundleEconomics(input.Auction.ID, "bid skipped: bundle is not profitable after gas and bid",
				bundle, rate, lane, gasPrice, input.Context.GasLimit, count, len(scored))
		}
		return pricedBundle{}, reason
	}
	return s.engine.priceBundle(bundle, rate, lane, gasPrice, count), ""
}

func (s *Strategy) scoredLegs(snap *snapshot, a types.AuctionSnapshot, now time.Time, adapter types.AdapterSnapshot) []scoredLeg {
	nowTs := clampTsAt(a.Timestamp, now)
	cands := candidatesFromAuctionWithAdapter(s.log, snap, a, nowTs, adapter)
	out := make([]scoredLeg, 0, len(cands))
	for _, it := range cands {
		if sized, ok := sizeLeg(it.cand, it.price, it.quote, s.cfg.Sizing); ok {
			out = append(out, scoredLeg{
				bundleLeg: bundleLeg{
					selectedLeg:     sized.leg,
					expectedLoanOut: sized.expectedLoanOut,
					collateral:      it.cand.Market.Params.CollateralToken,
				},
				profit:    sized.profit,
				maxAssets: it.quote.MaxAssets,
				source:    &it,
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

func (s *Strategy) bidOutputFromBundle(input types.BidInput, priced pricedBundle) (output types.BidOutput, err error) {
	defer func() {
		if err != nil {
			err = errors.Errorf("prepare callback authorization: %w", err)
		}
	}()
	if s.signer == nil {
		return output, errors.New("signer is required")
	}
	chainID := input.Context.ChainID
	if chainID == nil {
		chainID = s.chainID
	}
	if chainID == nil || chainID.Sign() <= 0 {
		return output, errors.New("chain id is required")
	}
	auth := operationAuth{AuctionKey: auctionKeyHash(input.Auction.ID), BidAmount: bigmath.Clone(priced.bidNative),
		MinBundleProfit: bigmath.Clone(priced.minBundleProfitLoan), Deadline: callbackAuthDeadline(input.Now, s.cfg.CallbackAuthTTL)}
	digest, err := callbackAuthDigest(chainID, input.Context.Callback, input.Context.Executor, auth, priced.selectedLegs)
	if err != nil {
		return output, err
	}
	signature, err := s.signer.SignHash(digest)
	if err != nil {
		return output, errors.Errorf("sign callback auth: %w", err)
	}
	data, err := encodeOperationData(auth, priced.selectedLegs, signature)
	if err != nil {
		return output, err
	}
	return types.BidOutput{Decision: types.DecisionBid, BidAmount: auth.BidAmount, OperationData: data}, nil
}

func gasDemands(in []bundleLeg) []liquidlanegas.Demand {
	out := make([]liquidlanegas.Demand, len(in))
	for i, leg := range in {
		out[i] = liquidlanegas.Demand{Collateral: leg.collateral, AmountOut: leg.expectedLoanOut}
	}
	return out
}

func liquidLaneStateFromAdapter(adapter types.AdapterSnapshot) *liquidLaneState {
	if adapter.FreeAssets == nil || adapter.Withdrawable == nil {
		return nil
	}
	st := &liquidLaneState{
		FreeAssets:   bigmath.Clone(adapter.FreeAssets),
		Withdrawable: bigmath.Clone(adapter.Withdrawable),
		Acquire:      make(map[common.Address]*big.Int, len(adapter.Redeemable)),
	}
	for _, r := range adapter.Redeemable {
		if r.Asset == (common.Address{}) {
			continue
		}
		st.Acquire[r.Asset] = bigmath.Clone(r.AcquireBalance)
	}
	return st
}

func freshAt(updatedAt, now time.Time, maxAge time.Duration) bool {
	return !updatedAt.IsZero() && (maxAge <= 0 || now.Sub(updatedAt) <= maxAge)
}

var _ types.Strategy = (*Strategy)(nil)

// envTestMonitor is an explicit local harness flag; production leaves it unset.
const envTestMonitor = "OEV_TEST_MONITOR"
