package redstoneoev

// bundle.go holds the leg-selection engine that turns scored legs into one priced solve.

import (
	"cmp"
	"maps"
	"math/big"
	"slices"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/morpho"
)

const netBundleBeamWidth = 64

// scoredLeg is a liquidatable, sized leg paired with its expected profit, in the single loan token's base
// units (one adapter ⇒ one loan token, so there is nothing to group by).
type scoredLeg struct {
	leg             LiquidationLeg
	expectedLoanOut *big.Int       // solver-local loan-token output estimate; not sent to the callback
	profit          *big.Int       // loan-token base units
	collateral      common.Address // seized collateral; legs sharing it share the adapter's getMaxAssets pool
	maxAssets       *big.Int       // cached adapter getMaxAssets budget (loan units; nil ⇒ uncapped)
	source          evalItem
	replay          bool
}

// chosenBundle is the set of legs selected for one solve. Single-token by design: the on-chain callback
// runs every leg against its one immutable LiquidLaneAdapter and a single loan token.
type chosenBundle struct {
	legs             []LiquidationLeg
	expectedLoanOuts []*big.Int       // solver-local output estimates, parallel to legs
	collaterals      []common.Address // seized collateral per leg, parallel to legs; used by the gas predictor
	borrowers        []string         // lowercased borrower addresses, parallel to legs (the solve's borrowers field)
	grossLoan        *big.Int         // Σ leg profit in the loan token's units
}

type pricedBundle struct {
	gas                 gasPrediction
	gasNative           *big.Int
	bidNative           *big.Int
	minBundleProfitLoan *big.Int
	callbackLegs        []LiquidationLeg
}

// selectBundle is the gross-profit fallback for dry-run/no-rate paths. Live bidding uses selectNetBundle.
//
// Legs sharing collateral also share the adapter's getMaxAssets pool, so selection caps cumulative expected
// loan output per collateral against the cached adapter liquidity.
func (s *Solver) selectBundle(scored []scoredLeg) (chosenBundle, string) {
	return s.selectBundleWithGas(scored, nil, 0, defaultPriceUpdateFeeds)
}

func (s *Solver) selectBundleWithGas(scored []scoredLeg, gasState *gasPredictorState, gasLimit uint64, feedCount int) (chosenBundle, string) {
	if len(scored) == 0 {
		return chosenBundle{}, skipNoLegs
	}
	best, ok := s.searchBundle(scored, gasState, gasLimit, feedCount, func(b chosenBundle) *big.Int {
		return new(big.Int).Set(b.grossLoan)
	})
	if !ok {
		return chosenBundle{}, skipNoLegs
	}
	return best.bundle, ""
}

// selectNetBundle maximizes bounded after-cost net while preserving deterministic tie-breaks and the shared
// collateral budget. A lower-gross subset can beat a gross-best subset once gas and the bid are priced in.
func (s *Solver) selectNetBundle(scored []scoredLeg, rate *big.Int, gasState *gasPredictorState, gasPrice *big.Int, gasLimit uint64, feedCount int) (chosenBundle, string) {
	if len(scored) == 0 {
		return chosenBundle{}, skipNoLegs
	}
	if rate == nil || rate.Sign() <= 0 {
		return s.selectBundleWithGas(scored, gasState, gasLimit, feedCount)
	}
	best, ok := s.searchBundle(scored, gasState, gasLimit, feedCount, func(b chosenBundle) *big.Int {
		return s.bundleNetNativeForFeeds(b, rate, gasState, gasPrice, feedCount)
	})
	if !ok {
		return chosenBundle{}, skipGasUnprofitable
	}
	bidNative := s.bundleBidNative(best.bundle, rate)
	minNative := s.minBundleProfitNative(bidNative)
	bestNet := s.bundleNetNativeForFeeds(best.bundle, rate, gasState, gasPrice, feedCount)
	if bestNet.Cmp(minNative) < 0 {
		return best.bundle, skipGasUnprofitable
	}
	return best.bundle, ""
}

type bundleSearchState struct {
	bundle   chosenBundle
	consumed map[common.Address]*big.Int
	markets  map[common.Hash]bundleMarketState
	used     map[int]bool
	score    *big.Int
}

type bundleMarketState struct {
	info      MarketInfo
	positions map[common.Address]morpho.PositionState
}

type replayedScoredLeg struct {
	leg      scoredLeg
	marketID common.Hash
	market   bundleMarketState
}

func (s *Solver) searchBundle(scored []scoredLeg, gasState *gasPredictorState, gasLimit uint64, feedCount int, scoreFn func(chosenBundle) *big.Int) (bundleSearchState, bool) {
	maxDepth := bundleSearchDepth(gasLimit, feedCount)
	if maxDepth == 0 {
		return bundleSearchState{}, false
	}
	group := sortedScoredLegs(scored)
	start := bundleSearchState{
		bundle:   chosenBundle{grossLoan: new(big.Int)},
		consumed: make(map[common.Address]*big.Int),
		markets:  make(map[common.Hash]bundleMarketState),
		used:     make(map[int]bool),
		score:    new(big.Int),
	}
	beam := []bundleSearchState{start}
	best := start
	for depth := 0; depth < maxDepth && depth < len(group); depth++ {
		nextBeam := make([]bundleSearchState, 0, min(len(group), netBundleBeamWidth))
		for _, state := range beam {
			for i, sl := range group {
				if state.used[i] {
					continue
				}
				trial, ok := s.extendBundleState(state, sl, i)
				if !ok {
					continue
				}
				if !bundleFitsGasLimit(trial.bundle, gasState, gasLimit, feedCount) {
					continue
				}
				trial.score = scoreFn(trial.bundle)
				nextBeam = append(nextBeam, trial)
			}
		}
		if len(nextBeam) == 0 {
			break
		}
		slices.SortStableFunc(nextBeam, func(a, b bundleSearchState) int {
			return b.score.Cmp(a.score)
		})
		if len(nextBeam) > netBundleBeamWidth {
			nextBeam = nextBeam[:netBundleBeamWidth]
		}
		if len(best.bundle.legs) == 0 || nextBeam[0].score.Cmp(best.score) > 0 {
			best = nextBeam[0]
		}
		beam = nextBeam
	}
	return best, len(best.bundle.legs) > 0
}

func bundleSearchDepth(gasLimit uint64, feedCount int) int {
	usable := usableBundleGasLimit(gasLimit)
	fixed := saturatingAddUint64(fixedGasUnits(feedCount), gasFirstAcquireLeg)
	if usable < fixed {
		return 0
	}
	return 1 + int((usable-fixed)/gasAdditionalAcquireLeg)
}

func (s *Solver) extendBundleState(state bundleSearchState, sl scoredLeg, idx int) (bundleSearchState, bool) {
	next, ok := s.replayScoredLeg(sl, state.markets)
	if !ok || !fitsCollateralBudget(state.consumed, next.leg) {
		return bundleSearchState{}, false
	}
	trial := bundleSearchState{
		bundle:   cloneBundleWithLeg(state.bundle, next.leg),
		consumed: cloneCollateralBudget(state.consumed),
		markets:  cloneBundleMarkets(state.markets),
		used:     cloneUsed(state.used),
	}
	trial.used[idx] = true
	if next.marketID != (common.Hash{}) {
		trial.markets[next.marketID] = next.market
	}
	commitCollateralBudget(trial.consumed, next.leg)
	return trial, true
}

func (s *Solver) replayScoredLeg(sl scoredLeg, markets map[common.Hash]bundleMarketState) (replayedScoredLeg, bool) {
	if !sl.replay {
		return replayedScoredLeg{leg: sl}, true
	}
	id := sl.source.cand.MarketID
	if id == (common.Hash{}) {
		return replayedScoredLeg{}, false
	}
	ms, ok := markets[id]
	if !ok {
		ms = bundleMarketState{info: cloneMarketInfo(sl.source.cand.Market), positions: make(map[common.Address]morpho.PositionState)}
	}
	pos, ok := ms.positions[sl.source.cand.Borrower]
	if !ok {
		pos = clonePositionState(sl.source.cand.Position)
	}
	cand := sl.source.cand
	cand.Market = ms.info
	cand.Position = pos
	leg, profit, ok := sizeLeg(cand, sl.source.price, sl.source.quote, ms.info.State.TotalBorrowAssets, s.cfg.Sizing)
	if !ok {
		return replayedScoredLeg{}, false
	}
	replay, ok := morpho.ApplySeizeLiquidation(ms.info.State, pos, leg.MaxSeizeAssets, sl.source.price)
	if !ok {
		return replayedScoredLeg{}, false
	}
	nextMarket := cloneBundleMarketState(ms)
	nextMarket.info.State = replay.Market
	nextMarket.positions[cand.Borrower] = replay.Position
	nextLeg := sl
	nextLeg.leg = leg
	nextLeg.expectedLoanOut = expectedLoanOutFor(leg.MaxSeizeAssets, sl.source.quote, s.cfg.Sizing.SwapHaircutBps)
	nextLeg.profit = profit
	nextLeg.collateral = cand.Market.Params.CollateralToken
	nextLeg.maxAssets = sl.source.quote.MaxAssets
	return replayedScoredLeg{leg: nextLeg, marketID: id, market: nextMarket}, true
}

func sortedScoredLegs(scored []scoredLeg) []scoredLeg {
	group := slices.Clone(scored)
	slices.SortFunc(group, func(a, b scoredLeg) int {
		return cmp.Or(
			b.profit.Cmp(a.profit),             // higher gross loan profit first
			a.leg.MarketId.Cmp(b.leg.MarketId), // then (marketId, borrower) — unique, deterministic
			a.leg.Borrower.Cmp(b.leg.Borrower),
		)
	})
	return group
}

func fitsCollateralBudget(consumed map[common.Address]*big.Int, sl scoredLeg) bool {
	if sl.maxAssets == nil || sl.maxAssets.Sign() <= 0 {
		return true
	}
	next := new(big.Int).Add(orZero(consumed[sl.collateral]), orZero(sl.expectedLoanOut))
	return next.Cmp(sl.maxAssets) <= 0
}

func commitCollateralBudget(consumed map[common.Address]*big.Int, sl scoredLeg) {
	if sl.maxAssets == nil || sl.maxAssets.Sign() <= 0 {
		return
	}
	consumed[sl.collateral] = new(big.Int).Add(orZero(consumed[sl.collateral]), orZero(sl.expectedLoanOut))
}

func cloneCollateralBudget(in map[common.Address]*big.Int) map[common.Address]*big.Int {
	out := make(map[common.Address]*big.Int, len(in))
	for collateral, amount := range in {
		out[collateral] = orZero(amount)
	}
	return out
}

func cloneBundleMarkets(in map[common.Hash]bundleMarketState) map[common.Hash]bundleMarketState {
	out := make(map[common.Hash]bundleMarketState, len(in))
	for id, state := range in {
		out[id] = cloneBundleMarketState(state)
	}
	return out
}

func cloneBundleMarketState(in bundleMarketState) bundleMarketState {
	out := bundleMarketState{info: cloneMarketInfo(in.info), positions: make(map[common.Address]morpho.PositionState, len(in.positions))}
	for borrower, position := range in.positions {
		out.positions[borrower] = clonePositionState(position)
	}
	return out
}

func cloneUsed(in map[int]bool) map[int]bool {
	out := make(map[int]bool, len(in))
	maps.Copy(out, in)
	return out
}

func cloneMarketInfo(in MarketInfo) MarketInfo {
	in.State = cloneMarketState(in.State)
	return in
}

func cloneMarketState(in morpho.MarketState) morpho.MarketState {
	return morpho.MarketState{
		TotalSupplyAssets: cloneBig(in.TotalSupplyAssets),
		TotalSupplyShares: cloneBig(in.TotalSupplyShares),
		TotalBorrowAssets: cloneBig(in.TotalBorrowAssets),
		TotalBorrowShares: cloneBig(in.TotalBorrowShares),
		LastUpdate:        in.LastUpdate,
		Fee:               cloneBig(in.Fee),
		Lltv:              cloneBig(in.Lltv),
		BorrowRatePerSec:  cloneBig(in.BorrowRatePerSec),
	}
}

func clonePositionState(in morpho.PositionState) morpho.PositionState {
	return morpho.PositionState{BorrowShares: cloneBig(in.BorrowShares), Collateral: cloneBig(in.Collateral)}
}

func appendScoredLeg(b *chosenBundle, sl scoredLeg) {
	b.legs = append(b.legs, sl.leg)
	b.expectedLoanOuts = append(b.expectedLoanOuts, cloneBig(sl.expectedLoanOut))
	b.collaterals = append(b.collaterals, sl.collateral)
	b.borrowers = append(b.borrowers, strings.ToLower(sl.leg.Borrower.Hex()))
	b.grossLoan.Add(b.grossLoan, sl.profit)
}

func cloneBundleWithLeg(b chosenBundle, sl scoredLeg) chosenBundle {
	out := chosenBundle{
		legs:             slices.Clone(b.legs),
		expectedLoanOuts: cloneBigSlice(b.expectedLoanOuts),
		collaterals:      slices.Clone(b.collaterals),
		borrowers:        slices.Clone(b.borrowers),
		grossLoan:        new(big.Int).Set(b.grossLoan),
	}
	appendScoredLeg(&out, sl)
	return out
}

func cloneBigSlice(in []*big.Int) []*big.Int {
	out := make([]*big.Int, len(in))
	for i, v := range in {
		out[i] = cloneBig(v)
	}
	return out
}

func (s *Solver) bundleNetNative(b chosenBundle, rate *big.Int, gasState *gasPredictorState, gasPrice *big.Int) *big.Int {
	return s.bundleNetNativeForFeeds(b, rate, gasState, gasPrice, defaultPriceUpdateFeeds)
}

func (s *Solver) bundleNetNativeForFeeds(b chosenBundle, rate *big.Int, gasState *gasPredictorState, gasPrice *big.Int, feedCount int) *big.Int {
	grossNative := loanToNative(b.grossLoan, rate)
	gasUnits := gasPredictionForBundleFeeds(b, gasState, feedCount).Units
	gasNative := gasCostNative(gasUnits, gasPrice)
	grossNative.Sub(grossNative, gasNative)
	return grossNative.Sub(grossNative, s.bundleBidNative(b, rate))
}

func (s *Solver) bundleBidNative(b chosenBundle, rate *big.Int) *big.Int {
	minimal := orZero(s.cfg.BidWei)
	if s.cfg.TotalBundleProfitBps <= 0 {
		return new(big.Int).Set(minimal)
	}
	share := ceilMulDiv(loanToNative(b.grossLoan, rate), big.NewInt(int64(s.cfg.TotalBundleProfitBps)), big.NewInt(10_000))
	if share.Cmp(minimal) < 0 {
		return new(big.Int).Set(minimal)
	}
	return share
}

func (s *Solver) minBundleProfitNative(bidNative *big.Int) *big.Int {
	if s.cfg.MinBundleProfitBidBps <= 0 {
		return new(big.Int)
	}
	return ceilMulDiv(orZero(bidNative), big.NewInt(int64(s.cfg.MinBundleProfitBidBps)), big.NewInt(10_000))
}

func (s *Solver) minBundleProfitLoan(b chosenBundle, rate *big.Int, gas gasPrediction, gasPrice *big.Int) *big.Int {
	bidNative := s.bundleBidNative(b, rate)
	requiredNative := gasCostNative(gas.Units, gasPrice)
	requiredNative.Add(requiredNative, bidNative)
	requiredNative.Add(requiredNative, s.minBundleProfitNative(bidNative))
	return nativeToLoan(requiredNative, rate)
}

func (s *Solver) priceBundle(b chosenBundle, rate *big.Int, gasState *gasPredictorState, gasPrice *big.Int, feedCount int) pricedBundle {
	gas := gasPredictionForBundleFeeds(b, gasState, feedCount)
	return pricedBundle{
		gas:                 gas,
		gasNative:           gasCostNative(gas.Units, gasPrice),
		bidNative:           s.bundleBidNative(b, rate),
		minBundleProfitLoan: s.minBundleProfitLoan(b, rate, gas, gasPrice),
		callbackLegs:        legsWithProfitFloors(b.legs, gas, gasPrice, rate),
	}
}

func ceilMulDiv(x, y, denom *big.Int) *big.Int {
	if x == nil || y == nil || denom == nil || x.Sign() <= 0 || y.Sign() <= 0 || denom.Sign() <= 0 {
		return new(big.Int)
	}
	num := new(big.Int).Mul(x, y)
	q, r := new(big.Int).QuoRem(num, denom, new(big.Int))
	if r.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	return q
}
