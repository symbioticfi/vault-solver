package defaultstrategy

// bundle.go holds the leg-selection engine that turns scored legs into one priced solve.

import (
	"cmp"
	"maps"
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/morpho"
)

const netBundleBeamWidth = 64

type bundleEngine struct {
	cfg Config
	log logr.Logger
}

func newBundleEngine(cfg Config, log logr.Logger) bundleEngine {
	return bundleEngine{cfg: cfg, log: log}
}

// bundleLeg is one selected liquidation plus estimates used for bundle pricing and gas prediction.
type bundleLeg struct {
	selectedLeg

	expectedLoanOut *big.Int       // loan-token output estimate used for route/gas prediction
	collateral      common.Address // seized collateral; legs sharing it share the adapter's getMaxAssets pool
}

// scoredLeg is a liquidatable, sized leg paired with replay source and the cached adapter budget.
type scoredLeg struct {
	bundleLeg

	profit    *big.Int // loan-token base units
	maxAssets *big.Int // cached adapter getMaxAssets budget (loan units; nil ⇒ uncapped)
	source    evalItem
	replay    bool
}

// chosenBundle is the set of legs selected for one solve. Single-token by design: the on-chain callback
// runs every leg against its one immutable LiquidLaneAdapter and a single loan token.
type chosenBundle struct {
	legs      []bundleLeg
	grossLoan *big.Int // Σ leg profit in the loan token's units
}

type pricedBundle struct {
	gas                 gasPrediction
	gasNative           *big.Int
	bidNative           *big.Int
	minBundleProfitLoan *big.Int
	selectedLegs        []selectedLeg
}

// selectBundle is the gross-profit fallback for dry-run/no-rate paths. Live bidding uses selectNetBundle.
//
// Legs sharing collateral also share the adapter's getMaxAssets pool, so selection caps cumulative expected
// loan output per collateral against the cached adapter liquidity.
func (e bundleEngine) selectBundle(scored []scoredLeg) (chosenBundle, string) {
	return e.selectBundleWithGas(scored, nil, 0, defaultPriceUpdateFeeds)
}

func (e bundleEngine) selectBundleWithGas(scored []scoredLeg, laneState *liquidLaneState, gasLimit uint64, feedCount int) (chosenBundle, string) {
	if len(scored) == 0 {
		return chosenBundle{}, skipNoLegs
	}
	best, ok := e.searchBundle(scored, laneState, gasLimit, feedCount, func(b chosenBundle) *big.Int {
		return new(big.Int).Set(b.grossLoan)
	})
	if !ok {
		return chosenBundle{}, skipNoLegs
	}
	return best.bundle, ""
}

// selectNetBundle maximizes bounded after-cost net while preserving deterministic tie-breaks and the shared
// collateral budget. A lower-gross subset can beat a gross-best subset once gas and the bid are priced in.
func (e bundleEngine) selectNetBundle(scored []scoredLeg, rate *big.Int, laneState *liquidLaneState, gasPrice *big.Int, gasLimit uint64, feedCount int) (chosenBundle, string) {
	if len(scored) == 0 {
		return chosenBundle{}, skipNoLegs
	}
	if rate == nil || rate.Sign() <= 0 {
		return e.selectBundleWithGas(scored, laneState, gasLimit, feedCount)
	}
	best, ok := e.searchBundle(scored, laneState, gasLimit, feedCount, func(b chosenBundle) *big.Int {
		return e.bundleNetNativeForFeeds(b, rate, laneState, gasPrice, feedCount)
	})
	if !ok {
		return chosenBundle{}, skipGasUnprofitable
	}
	bidNative := e.bundleBidNative(best.bundle, rate)
	minNative := e.minBundleProfitNative(bidNative)
	bestNet := e.bundleNetNativeForFeeds(best.bundle, rate, laneState, gasPrice, feedCount)
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
	scored   scoredLeg
	marketID common.Hash
	market   bundleMarketState
}

func (e bundleEngine) searchBundle(scored []scoredLeg, laneState *liquidLaneState, gasLimit uint64, feedCount int, scoreFn func(chosenBundle) *big.Int) (bundleSearchState, bool) {
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
				trial, ok := e.extendBundleState(state, sl, i)
				if !ok {
					continue
				}
				if !fitsGasLimit(legHints(trial.bundle.legs), laneState, gasLimit, feedCount) {
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
	usable := usableGasLimit(gasLimit)
	fixed := fixedSettlementGasUnits(feedCount) + liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteAcquire, true)
	if usable < fixed {
		return 0
	}
	return 1 + int((usable-fixed)/liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteAcquire, false))
}

func (e bundleEngine) extendBundleState(state bundleSearchState, sl scoredLeg, idx int) (bundleSearchState, bool) {
	next, ok := e.replayScoredLeg(sl, state.markets)
	if !ok || !fitsCollateralBudget(state.consumed, next.scored) {
		return bundleSearchState{}, false
	}
	trial := bundleSearchState{
		bundle:   cloneBundleWithLeg(state.bundle, next.scored),
		consumed: cloneCollateralBudget(state.consumed),
		markets:  cloneBundleMarkets(state.markets),
		used:     cloneUsed(state.used),
	}
	trial.used[idx] = true
	if next.marketID != (common.Hash{}) {
		trial.markets[next.marketID] = next.market
	}
	commitCollateralBudget(trial.consumed, next.scored)
	return trial, true
}

func (e bundleEngine) replayScoredLeg(sl scoredLeg, markets map[common.Hash]bundleMarketState) (replayedScoredLeg, bool) {
	if !sl.replay {
		return replayedScoredLeg{scored: sl}, true
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
		pos = morpho.ClonePositionState(sl.source.cand.Position)
	}
	cand := sl.source.cand
	cand.Market = ms.info
	cand.Position = pos
	sized, ok := sizeLeg(cand, sl.source.price, sl.source.quote, ms.info.State.TotalBorrowAssets, e.cfg.Sizing)
	if !ok {
		return replayedScoredLeg{}, false
	}
	replay, ok := morpho.ApplySeizeLiquidation(ms.info.State, pos, sized.leg.MaxSeizeAssets, sl.source.price)
	if !ok {
		return replayedScoredLeg{}, false
	}
	nextMarket := cloneBundleMarketState(ms)
	nextMarket.info.State = replay.Market
	nextMarket.positions[cand.Borrower] = replay.Position
	nextLeg := sl
	nextLeg.selectedLeg = sized.leg
	nextLeg.expectedLoanOut = sized.expectedLoanOut
	nextLeg.profit = sized.profit
	nextLeg.collateral = cand.Market.Params.CollateralToken
	nextLeg.maxAssets = sl.source.quote.MaxAssets
	return replayedScoredLeg{scored: nextLeg, marketID: id, market: nextMarket}, true
}

func sortedScoredLegs(scored []scoredLeg) []scoredLeg {
	group := slices.Clone(scored)
	slices.SortFunc(group, func(a, b scoredLeg) int {
		return cmp.Or(
			b.profit.Cmp(a.profit),     // higher gross loan profit first
			a.MarketId.Cmp(b.MarketId), // then (marketId, borrower) — unique, deterministic
			a.Borrower.Cmp(b.Borrower),
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
		out.positions[borrower] = morpho.ClonePositionState(position)
	}
	return out
}

func cloneUsed(in map[int]bool) map[int]bool {
	out := make(map[int]bool, len(in))
	maps.Copy(out, in)
	return out
}

func cloneMarketInfo(in MarketInfo) MarketInfo {
	in.State = morpho.CloneMarketState(in.State)
	return in
}

func appendScoredLeg(b *chosenBundle, sl scoredLeg) {
	b.legs = append(b.legs, cloneBundleLeg(sl.bundleLeg))
	b.grossLoan.Add(b.grossLoan, sl.profit)
}

func cloneBundleWithLeg(b chosenBundle, sl scoredLeg) chosenBundle {
	out := chosenBundle{
		legs:      cloneBundleLegs(b.legs),
		grossLoan: new(big.Int).Set(b.grossLoan),
	}
	appendScoredLeg(&out, sl)
	return out
}

func cloneBundleLeg(in bundleLeg) bundleLeg {
	in.MaxSeizeAssets = cloneBig(in.MaxSeizeAssets)
	in.MinProfit = cloneBig(in.MinProfit)
	in.expectedLoanOut = cloneBig(in.expectedLoanOut)
	return in
}

func cloneBundleLegs(in []bundleLeg) []bundleLeg {
	out := make([]bundleLeg, len(in))
	for i, leg := range in {
		out[i] = cloneBundleLeg(leg)
	}
	return out
}

func (b chosenBundle) selectedLegs() []selectedLeg {
	out := make([]selectedLeg, len(b.legs))
	for i, leg := range b.legs {
		out[i] = selectedLeg{
			MarketId:       leg.MarketId,
			Borrower:       leg.Borrower,
			MaxSeizeAssets: cloneBig(leg.MaxSeizeAssets),
			MinProfit:      cloneBig(leg.MinProfit),
		}
	}
	return out
}

func (e bundleEngine) bundleNetNative(b chosenBundle, rate *big.Int, laneState *liquidLaneState, gasPrice *big.Int) *big.Int {
	return e.bundleNetNativeForFeeds(b, rate, laneState, gasPrice, defaultPriceUpdateFeeds)
}

func (e bundleEngine) bundleNetNativeForFeeds(b chosenBundle, rate *big.Int, laneState *liquidLaneState, gasPrice *big.Int, feedCount int) *big.Int {
	grossNative := loanToNative(b.grossLoan, rate)
	gasUnits := predictGasForFeeds(legHints(b.legs), laneState, feedCount).Units
	gasNative := gasCostNative(gasUnits, gasPrice)
	grossNative.Sub(grossNative, gasNative)
	return grossNative.Sub(grossNative, e.bundleBidNative(b, rate))
}

func (e bundleEngine) bundleBidNative(b chosenBundle, rate *big.Int) *big.Int {
	minimal := orZero(e.cfg.BidWei)
	if e.cfg.TotalBundleProfitBps <= 0 {
		return new(big.Int).Set(minimal)
	}
	share := ceilMulDiv(loanToNative(b.grossLoan, rate), big.NewInt(int64(e.cfg.TotalBundleProfitBps)), big.NewInt(10_000))
	if share.Cmp(minimal) < 0 {
		return new(big.Int).Set(minimal)
	}
	return share
}

func (e bundleEngine) minBundleProfitNative(bidNative *big.Int) *big.Int {
	if e.cfg.MinBundleProfitBidBps <= 0 {
		return new(big.Int)
	}
	return ceilMulDiv(orZero(bidNative), big.NewInt(int64(e.cfg.MinBundleProfitBidBps)), big.NewInt(10_000))
}

func (e bundleEngine) minBundleProfitLoan(b chosenBundle, rate *big.Int, gas gasPrediction, gasPrice *big.Int) *big.Int {
	bidNative := e.bundleBidNative(b, rate)
	requiredNative := gasCostNative(gas.Units, gasPrice)
	requiredNative.Add(requiredNative, bidNative)
	requiredNative.Add(requiredNative, e.minBundleProfitNative(bidNative))
	return nativeToLoan(requiredNative, rate)
}

func (e bundleEngine) priceBundle(b chosenBundle, rate *big.Int, laneState *liquidLaneState, gasPrice *big.Int, feedCount int) pricedBundle {
	gas := predictGasForFeeds(legHints(b.legs), laneState, feedCount)
	return pricedBundle{
		gas:                 gas,
		gasNative:           gasCostNative(gas.Units, gasPrice),
		bidNative:           e.bundleBidNative(b, rate),
		minBundleProfitLoan: e.minBundleProfitLoan(b, rate, gas, gasPrice),
		selectedLegs:        legsWithProfitFloors(b.selectedLegs(), gas, gasPrice, rate),
	}
}

func (e bundleEngine) logBundleEconomics(auctionID, msg string, b chosenBundle, rate *big.Int, laneState *liquidLaneState, gasPrice *big.Int, gasLimit uint64, feedCount, scoredLegs int) {
	gas := predictGasForFeeds(legHints(b.legs), laneState, feedCount)
	grossNative := loanToNative(b.grossLoan, rate)
	gasNative := gasCostNative(gas.Units, gasPrice)
	netNative := e.bundleNetNativeForFeeds(b, rate, laneState, gasPrice, feedCount)
	bidNative := e.bundleBidNative(b, rate)
	e.log.Info(msg,
		"auction", auctionID,
		"scoredLegs", scoredLegs,
		"selectedLegs", len(b.legs),
		"feedCount", feedCount,
		"grossLoan", b.grossLoan,
		"grossNative", grossNative,
		"gasUnits", gas.Units,
		"gasNative", gasNative,
		"gasPriceWei", gasPrice,
		"bidNative", bidNative,
		"minBundleProfitNative", e.minBundleProfitNative(bidNative),
		"netNative", netNative,
		"gasLimit", gasLimit,
		"usableGasLimit", usableGasLimit(gasLimit),
		"routes", liquidlanegas.RoutesString(gas.Routes))
}
