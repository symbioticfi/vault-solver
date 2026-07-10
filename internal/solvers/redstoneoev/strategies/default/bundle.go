package defaultstrategy

// bundle.go holds the leg-selection engine that turns scored legs into one priced solve.

import (
	"cmp"
	"container/heap"
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
	scored      scoredLeg
	marketID    common.Hash
	marketInfo  MarketInfo
	marketState morpho.MarketState
	borrower    common.Address
	position    morpho.PositionState
}

type bundleTrial struct {
	parent    bundleSearchState
	next      replayedScoredLeg
	idx       int
	grossLoan *big.Int
	score     *big.Int
	seq       uint64
}

type bundleTrialHeap []bundleTrial

func (h bundleTrialHeap) Len() int { return len(h) }

func (h bundleTrialHeap) Less(i, j int) bool {
	if scoreCmp := h[i].score.Cmp(h[j].score); scoreCmp != 0 {
		return scoreCmp < 0
	}
	return h[i].seq > h[j].seq
}

func (h bundleTrialHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *bundleTrialHeap) Push(v any) { *h = append(*h, v.(bundleTrial)) }

func (h *bundleTrialHeap) Pop() any {
	old := *h
	n := len(old)
	v := old[n-1]
	*h = old[:n-1]
	return v
}

func trialBetter(a, b bundleTrial) bool {
	if scoreCmp := a.score.Cmp(b.score); scoreCmp != 0 {
		return scoreCmp > 0
	}
	return a.seq < b.seq
}

func freezeBundleTrial(trial bundleTrial) bundleTrial {
	trial.grossLoan = new(big.Int).Set(trial.grossLoan)
	trial.score = new(big.Int).Set(trial.score)
	return trial
}

func keepBundleTrial(h *bundleTrialHeap, trial bundleTrial) {
	if h.Len() < netBundleBeamWidth {
		heap.Push(h, freezeBundleTrial(trial))
		return
	}
	if trialBetter(trial, (*h)[0]) {
		heap.Pop(h)
		heap.Push(h, freezeBundleTrial(trial))
	}
}

type bundleSearchStats struct {
	materialized    int
	probeLegBuffers int
}

func (e bundleEngine) searchBundle(
	scored []scoredLeg,
	laneState *liquidLaneState,
	gasLimit uint64,
	feedCount int,
	scoreFn func(chosenBundle) *big.Int,
) (bundleSearchState, bool) {
	return e.searchBundleWithStats(scored, laneState, gasLimit, feedCount, scoreFn, nil)
}

func (e bundleEngine) searchBundleWithStats(
	scored []scoredLeg,
	laneState *liquidLaneState,
	gasLimit uint64,
	feedCount int,
	scoreFn func(chosenBundle) *big.Int,
	stats *bundleSearchStats,
) (bundleSearchState, bool) {
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
	seq := uint64(0)
	for depth := 0; depth < maxDepth && depth < len(group); depth++ {
		frontier := &bundleTrialHeap{}
		for _, state := range beam {
			probeLegs := make([]bundleLeg, len(state.bundle.legs)+1)
			copy(probeLegs, state.bundle.legs)
			probeGross := new(big.Int)
			if stats != nil {
				stats.probeLegBuffers++
			}
			for i, scored := range group {
				if state.used[i] {
					continue
				}
				next, ok := e.replayScoredLeg(scored, state.markets)
				if !ok || !fitsCollateralBudget(state.consumed, next.scored) {
					continue
				}
				candidate := probeBundle(state.bundle, next.scored, probeLegs, probeGross)
				if !fitsGasLimit(legHints(candidate.legs), laneState, gasLimit, feedCount) {
					continue
				}
				keepBundleTrial(frontier, bundleTrial{
					parent:    state,
					next:      next,
					idx:       i,
					grossLoan: candidate.grossLoan,
					score:     scoreFn(candidate),
					seq:       seq,
				})
				seq++
			}
		}
		if frontier.Len() == 0 {
			break
		}
		trials := slices.Clone(*frontier)
		slices.SortFunc(trials, func(a, b bundleTrial) int {
			return cmp.Or(b.score.Cmp(a.score), cmp.Compare(a.seq, b.seq))
		})
		nextBeam := make([]bundleSearchState, len(trials))
		for i, trial := range trials {
			nextBeam[i] = materializeBundleTrial(trial)
			if stats != nil {
				stats.materialized++
			}
		}
		if len(best.bundle.legs) == 0 || nextBeam[0].score.Cmp(best.score) > 0 {
			best = nextBeam[0]
		}
		beam = nextBeam
	}
	return best, len(best.bundle.legs) > 0
}

func probeBundle(parent chosenBundle, next scoredLeg, legs []bundleLeg, gross *big.Int) chosenBundle {
	legs[len(parent.legs)] = next.bundleLeg
	gross.Add(parent.grossLoan, next.profit)
	return chosenBundle{legs: legs, grossLoan: gross}
}

func materializeBundleTrial(trial bundleTrial) bundleSearchState {
	bundle := cloneChosenBundle(trial.parent.bundle)
	appendScoredLeg(&bundle, trial.next.scored)
	bundle.grossLoan.Set(trial.grossLoan)
	next := bundleSearchState{
		bundle:   bundle,
		consumed: cloneCollateralBudget(trial.parent.consumed),
		markets:  maps.Clone(trial.parent.markets),
		used:     cloneUsed(trial.parent.used),
		score:    new(big.Int).Set(trial.score),
	}
	next.used[trial.idx] = true
	commitCollateralBudget(next.consumed, trial.next.scored)
	if trial.next.marketID != (common.Hash{}) {
		previous := trial.parent.markets[trial.next.marketID]
		positions := maps.Clone(previous.positions)
		if positions == nil {
			positions = make(map[common.Address]morpho.PositionState)
		}
		positions[trial.next.borrower] = morpho.ClonePositionState(trial.next.position)
		info := trial.next.marketInfo
		info.State = morpho.CloneMarketState(trial.next.marketState)
		next.markets[trial.next.marketID] = bundleMarketState{
			info:      info,
			positions: positions,
		}
	}
	return next
}

func cloneChosenBundle(bundle chosenBundle) chosenBundle {
	return chosenBundle{
		legs:      cloneBundleLegs(bundle.legs),
		grossLoan: new(big.Int).Set(bundle.grossLoan),
	}
}

func bundleSearchDepth(gasLimit uint64, feedCount int) int {
	usable := usableGasLimit(gasLimit)
	fixed := fixedSettlementGasUnits(feedCount) + liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteAcquire, true)
	if usable < fixed {
		return 0
	}
	return 1 + int((usable-fixed)/liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteAcquire, false))
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
		ms = bundleMarketState{info: sl.source.cand.Market}
	}
	pos, ok := ms.positions[sl.source.cand.Borrower]
	if !ok {
		pos = sl.source.cand.Position
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
	nextLeg := sl
	nextLeg.selectedLeg = sized.leg
	nextLeg.expectedLoanOut = sized.expectedLoanOut
	nextLeg.profit = sized.profit
	nextLeg.collateral = cand.Market.Params.CollateralToken
	nextLeg.maxAssets = sl.source.quote.MaxAssets
	return replayedScoredLeg{
		scored:      nextLeg,
		marketID:    id,
		marketInfo:  ms.info,
		marketState: replay.Market,
		borrower:    cand.Borrower,
		position:    replay.Position,
	}, true
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

func cloneUsed(in map[int]bool) map[int]bool {
	out := make(map[int]bool, len(in))
	maps.Copy(out, in)
	return out
}

func appendScoredLeg(b *chosenBundle, sl scoredLeg) {
	b.legs = append(b.legs, cloneBundleLeg(sl.bundleLeg))
	b.grossLoan.Add(b.grossLoan, sl.profit)
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
