package defaultstrategy

// bundle.go holds the leg-selection engine that turns scored legs into one priced solve.

import (
	"cmp"
	"context"
	"maps"
	"math/big"
	"slices"
	"sort"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

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

// bundleLeg is one selected liquidation plus estimates used for bundle pricing and gas prediction.
type bundleLeg struct {
	selectedLeg

	expectedLoanOut *big.Int       // loan-token output estimate used for route/gas prediction
	collateral      common.Address // seized collateral; legs sharing it share the adapter's getMaxAssets pool
}

// scoredLeg is a liquidatable, sized leg paired with replay source and the cached adapter budget.
type scoredLeg struct {
	bundleLeg

	profit    *big.Int  // loan-token base units
	maxAssets *big.Int  // cached adapter getMaxAssets budget (loan units; nil ⇒ uncapped)
	source    *evalItem // immutable replay source; nil for an already-sized static leg
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

// Legs sharing collateral share the adapter's getMaxAssets pool, so gross selection caps cumulative
// expected loan output per collateral against cached adapter liquidity and the settlement gas limit.
func (e bundleEngine) selectBundleWithGas(ctx context.Context, scored []scoredLeg, laneState *liquidLaneState, gasLimit uint64, feedCount int) (chosenBundle, string) {
	if len(scored) == 0 {
		return chosenBundle{}, skipNoLegs
	}
	best, ok := e.searchBundle(ctx, scored, laneState, gasLimit, feedCount, func(b chosenBundle, _ uint64) *big.Int {
		return b.grossLoan
	})
	if !ok {
		return chosenBundle{}, skipNoLegs
	}
	return best.bundle, ""
}

// selectNetBundle maximizes bounded after-cost net while preserving deterministic tie-breaks and the shared
// collateral budget. A lower-gross subset can beat a gross-best subset once gas and the bid are priced in.
func (e bundleEngine) selectNetBundle(ctx context.Context, scored []scoredLeg, rate *big.Int, laneState *liquidLaneState, gasPrice *big.Int, gasLimit uint64, feedCount int) (chosenBundle, string) {
	if len(scored) == 0 {
		return chosenBundle{}, skipNoLegs
	}
	if rate == nil || rate.Sign() <= 0 {
		return chosenBundle{}, skipGasUnprofitable
	}
	best, ok := e.searchBundle(ctx, scored, laneState, gasLimit, feedCount, func(b chosenBundle, gasUnits uint64) *big.Int {
		return e.bundleNetNative(b, rate, gasPrice, gasUnits)
	})
	if !ok {
		return chosenBundle{}, skipGasUnprofitable
	}
	bidNative := e.bundleBidNative(best.bundle, rate)
	minNative := e.minBundleProfitNative(bidNative)
	bestNet := best.score
	if bestNet.Cmp(minNative) < 0 {
		return best.bundle, skipGasUnprofitable
	}
	return best.bundle, ""
}

type bundleSearchState struct {
	bundle   chosenBundle
	consumed map[common.Address]*big.Int
	markets  map[common.Hash]bundleMarketState
	used     []int
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

// Search states are immutable. Expanding a branch replaces only the affected market,
// borrower and collateral totals; untouched snapshots are shared between branches.
// The frontier stays bounded during expansion, including when thousands of positions qualify.
func (e bundleEngine) searchBundle(ctx context.Context, scored []scoredLeg, laneState *liquidLaneState, gasLimit uint64, feedCount int, scoreFn func(chosenBundle, uint64) *big.Int) (bundleSearchState, bool) {
	group := sortedScoredLegs(scored)
	start := bundleSearchState{
		bundle:   chosenBundle{grossLoan: new(big.Int)},
		consumed: make(map[common.Address]*big.Int), markets: make(map[common.Hash]bundleMarketState),
		score: new(big.Int),
	}
	frontier := []bundleSearchState{start}
	best := start
	for range min(bundleSearchDepth(gasLimit, feedCount), len(group)) {
		next := make([]bundleSearchState, 0, netBundleBeamWidth+1)
		for _, parent := range frontier {
			for index, leg := range group {
				if ctx.Err() != nil {
					return bundleSearchState{}, false
				}
				if slices.Contains(parent.used, index) {
					continue
				}
				trial, ok := e.extendBundleState(parent, leg, index)
				if !ok {
					continue
				}
				gas := predictGasForFeeds(gasDemands(trial.bundle.legs), laneState, feedCount)
				if gas.Units > usableGasLimit(gasLimit) {
					continue
				}
				trial.score = scoreFn(trial.bundle, gas.Units)
				// Insert after ties to retain deterministic generation order.
				at := sort.Search(len(next), func(i int) bool { return next[i].score.Cmp(trial.score) < 0 })
				if at == netBundleBeamWidth {
					continue
				}
				next = slices.Insert(next, at, trial)
				if len(next) > netBundleBeamWidth {
					next = next[:netBundleBeamWidth]
				}
			}
		}
		if len(next) == 0 {
			break
		}
		if len(best.bundle.legs) == 0 || next[0].score.Cmp(best.score) > 0 {
			best = next[0]
		}
		frontier = next
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
	if !ok {
		return bundleSearchState{}, false
	}
	consumed, fits := nextCollateralUsage(state.consumed, next.scored)
	if !fits {
		return bundleSearchState{}, false
	}
	trial := bundleSearchState{
		bundle:   cloneBundleWithLeg(state.bundle, next.scored),
		consumed: state.consumed,
		markets:  state.markets,
		used:     append(slices.Clone(state.used), idx),
	}
	if next.marketID != (common.Hash{}) {
		trial.markets = maps.Clone(state.markets)
		trial.markets[next.marketID] = next.market
	}
	if consumed != nil {
		trial.consumed = maps.Clone(state.consumed)
		trial.consumed[next.scored.collateral] = consumed
	}
	return trial, true
}

func (e bundleEngine) replayScoredLeg(sl scoredLeg, markets map[common.Hash]bundleMarketState) (replayedScoredLeg, bool) {
	if sl.source == nil {
		return replayedScoredLeg{scored: sl}, true
	}
	id := sl.source.cand.MarketID
	if id == (common.Hash{}) {
		return replayedScoredLeg{}, false
	}
	// Inputs remain immutable; ApplySeizeLiquidation owns the changed market
	// and borrower state, so seeding a branch needs no preparatory deep copy.
	ms, ok := markets[id]
	if !ok {
		ms = bundleMarketState{info: sl.source.cand.Market, positions: make(map[common.Address]morpho.PositionState)}
	}
	pos, ok := ms.positions[sl.source.cand.Borrower]
	if !ok {
		pos = sl.source.cand.Position
	}
	cand := sl.source.cand
	cand.Market = ms.info
	cand.Position = pos
	sized, ok := sizeLeg(cand, sl.source.price, sl.source.quote, e.cfg.Sizing)
	if !ok {
		return replayedScoredLeg{}, false
	}
	replay, ok := morpho.ApplySeizeLiquidation(ms.info.State, pos, sized.leg.MaxSeizeAssets, sl.source.price)
	if !ok {
		return replayedScoredLeg{}, false
	}
	nextMarket := bundleMarketState{info: ms.info, positions: maps.Clone(ms.positions)}
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

// A nil total means this leg has no capacity ceiling and consumes no tracked
// capped budget. Otherwise the returned amount is owned by the new branch.
func nextCollateralUsage(consumed map[common.Address]*big.Int, sl scoredLeg) (*big.Int, bool) {
	if sl.maxAssets == nil || sl.maxAssets.Sign() <= 0 {
		return nil, true
	}
	next := bigmath.OrZero(consumed[sl.collateral])
	if sl.expectedLoanOut != nil {
		next.Add(next, sl.expectedLoanOut)
	}
	return next, next.Cmp(sl.maxAssets) <= 0
}

func cloneBundleWithLeg(b chosenBundle, sl scoredLeg) chosenBundle {
	leg := sl.bundleLeg
	leg.MaxSeizeAssets, leg.MinProfit = bigmath.Clone(leg.MaxSeizeAssets), bigmath.Clone(leg.MinProfit)
	leg.expectedLoanOut = bigmath.Clone(leg.expectedLoanOut)
	out := chosenBundle{
		legs: make([]bundleLeg, len(b.legs)+1), grossLoan: new(big.Int).Add(b.grossLoan, sl.profit),
	}
	copy(out.legs, b.legs)
	out.legs[len(b.legs)] = leg
	return out
}

func (b chosenBundle) selectedLegs(profit func(int) *big.Int) []selectedLeg {
	out := make([]selectedLeg, len(b.legs))
	for i, leg := range b.legs {
		out[i] = selectedLeg{
			MarketId:       leg.MarketId,
			Borrower:       leg.Borrower,
			MaxSeizeAssets: bigmath.Clone(leg.MaxSeizeAssets),
			MinProfit:      profit(i),
		}
	}
	return out
}

func (e bundleEngine) bundleNetNative(b chosenBundle, rate, gasPrice *big.Int, gasUnits uint64) *big.Int {
	grossNative := loanToNative(b.grossLoan, rate)
	gasNative := gasCostNative(gasUnits, gasPrice)
	grossNative.Sub(grossNative, gasNative)
	return grossNative.Sub(grossNative, e.bundleBidNative(b, rate))
}

func (e bundleEngine) bundleBidNative(b chosenBundle, rate *big.Int) *big.Int {
	minimal := bigmath.OrZero(e.cfg.BidWei)
	if e.cfg.TotalBundleProfitBps <= 0 {
		return minimal
	}
	share := liquidlane.MulDivUp(loanToNative(b.grossLoan, rate), big.NewInt(int64(e.cfg.TotalBundleProfitBps)), big.NewInt(10_000))
	if share.Cmp(minimal) < 0 {
		return minimal
	}
	return share
}

func (e bundleEngine) minBundleProfitNative(bidNative *big.Int) *big.Int {
	if e.cfg.MinBundleProfitBidBps <= 0 {
		return new(big.Int)
	}
	return liquidlane.MulDivUp(bigmath.OrZero(bidNative), big.NewInt(int64(e.cfg.MinBundleProfitBidBps)), big.NewInt(10_000))
}

func (e bundleEngine) priceBundle(b chosenBundle, rate *big.Int, laneState *liquidLaneState, gasPrice *big.Int, feedCount int) pricedBundle {
	gas := predictGasForFeeds(gasDemands(b.legs), laneState, feedCount)
	gasNative, bidNative := gasCostNative(gas.Units, gasPrice), e.bundleBidNative(b, rate)
	requiredNative := new(big.Int).Add(gasNative, bidNative)
	requiredNative.Add(requiredNative, e.minBundleProfitNative(bidNative))
	return pricedBundle{
		gas: gas, gasNative: gasNative, bidNative: bidNative,
		minBundleProfitLoan: nativeToLoan(requiredNative, rate),
		selectedLegs:        b.legsWithProfitFloors(gas, gasPrice, rate),
	}
}

// priceBundleWithoutGasAccounting preserves native-only funding safety while intentionally omitting
// native-cost deductions from loan-token profitability. The positive one-unit floors keep the
// callback authorization valid without pretending an unavailable conversion is known.
func (e bundleEngine) priceBundleWithoutGasAccounting(
	b chosenBundle,
	laneState *liquidLaneState,
	gasPrice *big.Int,
	feedCount int,
) pricedBundle {
	gas := predictGasForFeeds(gasDemands(b.legs), laneState, feedCount)
	return pricedBundle{
		gas:                 gas,
		gasNative:           gasCostNative(gas.Units, gasPrice),
		bidNative:           bigmath.Clone(e.cfg.BidWei),
		minBundleProfitLoan: big.NewInt(1),
		selectedLegs:        b.selectedLegs(func(int) *big.Int { return big.NewInt(1) }),
	}
}

func (e bundleEngine) logBundleEconomics(auctionID, msg string, b chosenBundle, rate *big.Int, laneState *liquidLaneState, gasPrice *big.Int, gasLimit uint64, feedCount, scoredLegs int) {
	gas := predictGasForFeeds(gasDemands(b.legs), laneState, feedCount)
	grossNative := loanToNative(b.grossLoan, rate)
	gasNative := gasCostNative(gas.Units, gasPrice)
	netNative := e.bundleNetNative(b, rate, gasPrice, gas.Units)
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
