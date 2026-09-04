package policy

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/chain"
	internalsigner "github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/decision"
	"gopkg.in/yaml.v3"
)

const Name = "default"

type Strategy struct {
	cfg           Config
	adapter       common.Address
	callback      common.Address
	gasAccounting bool
	reader        Reader
	signer        signer
	chainID       *big.Int
	engine        bundleEngine
	log           logr.Logger
}

type FactoryDeps struct {
	Chain               *chain.Client
	Signer              internalsigner.Signer
	Log                 logr.Logger
	ChainID             int64
	Adapter             common.Address
	Callback            common.Address
	LoadAdapterSnapshot func() (decision.AdapterSnapshot, bool)
	GasAccounting       bool
}

func ValidateConfig(raw yaml.Node, gasAccounting bool) error {
	cfg, err := ParseConfig(raw)
	if err != nil {
		return err
	}
	return validateConfig(cfg, gasAccounting)
}

func NewFromConfig(raw yaml.Node, deps FactoryDeps) (decision.Planner, decision.FactSource, error) {
	cfg, err := ParseConfig(raw)
	if err != nil {
		return nil, nil, err
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

func New(cfg Config, deps Deps) (*Strategy, decision.FactSource, error) {
	if deps.Callback == (common.Address{}) {
		return nil, nil, errors.New("callback is required")
	}
	if deps.Adapter == (common.Address{}) {
		return nil, nil, errors.New("adapter is required")
	}
	if deps.LoadAdapterSnapshot == nil {
		return nil, nil, errors.New("adapter snapshot source is required")
	}
	if err := validateConfig(cfg, deps.GasAccounting); err != nil {
		return nil, nil, err
	}
	var (
		source decision.FactSource
		err    error
	)
	if !cfg.UsesAPIMonitor() {
		source, err = newTestMonitor(deps.Reader, deps.Log, cfg, deps.Callback, deps.LoadAdapterSnapshot, cfg.TestMonitor)
		if err != nil {
			return nil, nil, err
		}
	} else {
		source = newAPIMonitor(deps.Log, cfg, deps.ChainID, deps.LoadAdapterSnapshot)
	}
	return &Strategy{
		cfg:           cfg,
		adapter:       deps.Adapter,
		callback:      deps.Callback,
		gasAccounting: deps.GasAccounting,
		reader:        deps.Reader,
		signer:        deps.Signer,
		chainID:       big.NewInt(deps.ChainID),
		engine:        newBundleEngine(cfg, deps.Log),
		log:           deps.Log,
	}, source, nil
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

func (s *Strategy) DecideBid(_ context.Context, input decision.BidInput) (decision.BidOutput, error) {
	if !s.accepts(input) {
		return skipBid(skipNoLegs), nil
	}
	if input.Market.UpdatedAt.IsZero() ||
		(s.cfg.MaxStateAge > 0 && input.Now.Sub(input.Market.UpdatedAt) > s.cfg.MaxStateAge) {
		return skipBid(skipStaleState), nil
	}
	if input.Context.CallbackNative == nil {
		return skipBid(skipStaleState), nil
	}
	if skip := snapshotFreshForAuction(input.Market, input.Auction); skip != "" {
		return skipBid(skip), nil
	}
	scored := s.scoredLegs(input.Market)
	scored = filterReservedPositions(scored, input.Exposure.Positions)
	if len(scored) == 0 {
		return skipBid(skipNoLegs), nil
	}
	laneState := liquidLaneStateFromAdapter(input.Adapter)
	gasPrice := cloneBig(input.Context.MaxTxGasPrice)
	feedCount := auctionFeedCount(input.Auction)
	priced, skip := s.selectAndPriceBundle(input, scored, laneState, gasPrice, feedCount)
	if skip != "" {
		return decision.BidOutput{Decision: decision.DecisionSkip, Reason: skip}, nil
	}
	reservedAndCurrentGas := new(big.Int).Add(orZero(input.Exposure.GasNative), priced.gasNative)
	if !depositCoversSettlementGas(input.Context.ExecutorDeposit, input.Context.ExecutorMinDeposit, reservedAndCurrentGas) {
		s.log.Info("bid skipped: executor deposit cannot cover predicted settlement gas",
			"auction", input.Auction.ID,
			"depositWei", input.Context.ExecutorDeposit,
			"requiredWei", executorDepositRequired(input.Context.ExecutorMinDeposit, reservedAndCurrentGas),
			"reservedGasWei", input.Exposure.GasNative,
			"minDepositWei", input.Context.ExecutorMinDeposit,
			"gasUnits", priced.gas.Units,
			"gasNative", priced.gasNative,
			"gasPriceWei", gasPrice)
		return skipBid(decision.SkipReasonDepositLow), nil
	}
	availableCallback := new(big.Int).Sub(input.Context.CallbackNative, orZero(input.Exposure.BidNative))
	if availableCallback.Cmp(priced.bidNative) < 0 {
		s.log.Info("bid skipped: callback balance cannot cover bid",
			"auction", input.Auction.ID, "callback", s.callback.Hex(),
			"callbackWei", input.Context.CallbackNative, "reservedBidWei", input.Exposure.BidNative,
			"availableWei", availableCallback, "requiredWei", priced.bidNative)
		return skipBid(decision.SkipReasonCallbackBalance), nil
	}
	out, err := s.bidOutputFromBundle(input, priced)
	if err != nil {
		return decision.BidOutput{}, err
	}
	return out, nil
}

func (s *Strategy) accepts(input decision.BidInput) bool {
	return (input.Adapter.Address == (common.Address{}) || input.Adapter.Address == s.adapter) &&
		input.Context.Callback == s.callback && input.Adapter.Filler && !input.Adapter.Paused
}

func (s *Strategy) selectAndPriceBundle(
	input decision.BidInput,
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

func (s *Strategy) scoredLegs(facts decision.MarketFacts) []scoredLeg {
	out := make([]scoredLeg, 0, len(facts.Candidates))
	for _, candidate := range facts.Candidates {
		item := evalItemFromCandidate(candidate)
		if sized, ok := sizeLeg(item.cand, item.price, item.quote, item.accrued, s.cfg.Sizing); ok {
			out = append(out, scoredLeg{
				bundleLeg: bundleLeg{
					selectedLeg:     sized.leg,
					expectedLoanOut: sized.expectedLoanOut,
					collateral:      item.cand.Market.Params.CollateralToken,
				},
				profit:    sized.profit,
				maxAssets: item.quote.MaxAssets,
				source:    item,
				replay:    true,
			})
		}
	}
	return out
}

func auctionFeedCount(a decision.AuctionSnapshot) int {
	if a.RawPriceCount > 0 {
		return a.RawPriceCount
	}
	return len(a.Prices)
}

func skipBid(reason string) decision.BidOutput {
	return decision.BidOutput{Decision: decision.DecisionSkip, Reason: reason}
}

func (s *Strategy) bidOutputFromBundle(input decision.BidInput, priced pricedBundle) (decision.BidOutput, error) {
	if s.signer == nil {
		return decision.BidOutput{}, errors.New("signer is required")
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
		return decision.BidOutput{}, errors.New("chain id is required")
	}
	authDigest, err := callbackAuthDigest(chainID, input.Context.Callback, input.Context.Executor, auth, priced.selectedLegs)
	if err != nil {
		return decision.BidOutput{}, err
	}
	authSig, err := s.signer.SignHash(authDigest)
	if err != nil {
		return decision.BidOutput{}, errors.Errorf("sign callback auth: %w", err)
	}
	opData, err := encodeOperationData(auth, priced.selectedLegs, authSig)
	if err != nil {
		return decision.BidOutput{}, err
	}
	return decision.BidOutput{
		Decision:      decision.DecisionBid,
		BidAmount:     cloneBig(priced.bidNative),
		OperationData: opData,
		Exposure:      exposureFromBundle(priced),
	}, nil
}

func exposureFromBundle(priced pricedBundle) decision.Exposure {
	positions := make([]decision.PositionClaim, len(priced.selectedLegs))
	for index, leg := range priced.selectedLegs {
		positions[index] = decision.PositionClaim{MarketID: leg.MarketId, Borrower: leg.Borrower}
	}
	return decision.Exposure{
		BidNative: cloneBig(priced.bidNative), GasNative: cloneBig(priced.gasNative), Positions: positions,
	}
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

func liquidLaneStateFromAdapter(adapter decision.AdapterSnapshot) *liquidLaneState {
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

var _ decision.Planner = (*Strategy)(nil)
