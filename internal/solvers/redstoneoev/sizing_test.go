package redstoneoev

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/morpho"
)

// newQuote builds a USDC(6)/RWA(18) adapter quote with the hot-path scales precomputed, mirroring the
// production buildQuote invariant that LoanScale/CollScale are always non-nil. maxAssets nil ⇒ uncapped.
func newQuote(maxRate string, maxAssets *big.Int) AdapterQuote {
	return AdapterQuote{
		MaxRate: mustBig(maxRate), MaxAssets: maxAssets,
		LoanScale: chain.Exp10(6), CollScale: chain.Exp10(18),
	}
}

func TestTargetSeizeModes(t *testing.T) {
	collateral := mustBig("1000000000000000000")
	if got := targetSeize(collateral, true); got.Cmp(collateral) != 0 {
		t.Fatalf("full target = %s, want %s", got, collateral)
	}
	wantPartial := mustBig("900000000000000000")
	if got := targetSeize(collateral, false); got.Cmp(wantPartial) != 0 {
		t.Fatalf("partial target = %s, want %s", got, wantPartial)
	}
}

// evalLeg sizes a position against the single configured adapter's quote — the only repay→swap→profit
// core (sizeLeg). The adapter argument is ignored (the leg no longer carries an adapter; the contract
// pins the LiquidLane adapter); it is kept so the assertions below read unchanged.
func evalLeg(c Candidate, price *big.Int, _ common.Address, q AdapterQuote, nowTs uint64, sp SizingParams) (LiquidationLeg, *big.Int, bool) {
	accrued := morpho.AccruedTotalBorrowAssets(c.Market.State, nowTs)
	return sizeLeg(c, price, q, accrued, sp)
}

// TestEvaluateLegTargetsFullCollateral proves the default sizing path captures full-collateral opportunities,
// including bad-debt-style cases, instead of leaving a configurable bps slice behind.
func TestEvaluateLegTargetsFullCollateral(t *testing.T) {
	m := goldenMarket()
	cand := Candidate{
		MarketID: common.HexToHash("0x6209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5"),
		Borrower: common.HexToAddress("0x629d764eC8563AFA701709B52c1a215e865632dE"),
		Market:   MarketInfo{State: m},
		Position: goldenBorrower(),
	}
	price := mustBig("1550000000000000000000000000") // $1550 market price
	// Adapter sells the RWA at $1550 minus the curator's 1% minDiscount -> getMaxRate 1534.5e18.
	q := newQuote("1534500000000000000000", nil)
	sp := SizingParams{AllowFullLiquidation: true, SwapHaircutBps: 0}

	leg, profit, ok := evalLeg(cand, price, seedAdapter, q, m.LastUpdate, sp)
	if !ok {
		t.Fatal("expected a profitable leg at $1550")
	}
	if leg.MaxSeizeAssets.String() != "1000000000000000000" { // 1 TCOL
		t.Fatalf("seized = %s, want 1 TCOL", leg.MaxSeizeAssets)
	}
	// expectedLoanOut at the adapter rate: 1 TCOL × 1534.5 = 1534.5 TLOAN; profit ≈ 49.6 TLOAN.
	expectedLoanOut := expectedLoanOutFor(leg.MaxSeizeAssets, q, sp.SwapHaircutBps)
	if expectedLoanOut.Cmp(big.NewInt(1_533_000_000)) < 0 || expectedLoanOut.Cmp(big.NewInt(1_536_000_000)) > 0 {
		t.Fatalf("expectedLoanOut = %s, want ~1534.5e6", expectedLoanOut)
	}
	if profit.Cmp(big.NewInt(45_000_000)) < 0 || profit.Cmp(big.NewInt(55_000_000)) > 0 {
		t.Fatalf("profit = %s, want ~50e6", profit)
	}
}

func TestSizeLegAllowsFullBadDebtSeize(t *testing.T) {
	lltv := mustBig("500000000000000000")                    // 0.5
	price := mustBig("1000000000000000000000000000")         // 1000 loan per 1e18 collateral
	totalBorrowAssets := mustBig("10000000000")              // 10,000 loan tokens at 6 decimals
	totalBorrowShares := new(big.Int).Set(totalBorrowAssets) // 1:1 shares↔assets
	collateral := mustBig("1000000000000000000")             // 1 collateral
	debtShares := mustBig("1200000000")                      // 1,200 loan tokens: debt > full collateral value
	state := morpho.MarketState{TotalBorrowAssets: totalBorrowAssets, TotalBorrowShares: totalBorrowShares,
		Lltv: lltv, BorrowRatePerSec: big.NewInt(0), Fee: big.NewInt(0), LastUpdate: assignNowTs}
	c := Candidate{
		MarketID: assignMarketID, Borrower: common.Address{19: 9},
		Market:   MarketInfo{Params: abiMarketParams{LoanToken: tokenA}, State: state},
		Position: morpho.PositionState{BorrowShares: debtShares, Collateral: collateral},
	}
	accrued := morpho.AccruedTotalBorrowAssets(state, assignNowTs)
	lif := morpho.LiquidationIncentiveFactor(lltv)
	if maxSeize := morpho.MaxSeizeForFullDebt(debtShares, price, lif, accrued, totalBorrowShares); maxSeize.Cmp(collateral) <= 0 {
		t.Fatalf("fixture must be bad-debt-like: maxSeizeForFullDebt=%s <= collateral=%s", maxSeize, collateral)
	}
	q := newQuote("1200000000000000000000", nil) // exit at 1200 loan per collateral, above the full-collateral repayment.
	full, fullProfit, ok := sizeLeg(c, price, q, accrued, SizingParams{AllowFullLiquidation: true, SwapHaircutBps: 0})
	if !ok {
		t.Fatal("full bad-debt seize should be profitable")
	}
	if full.MaxSeizeAssets.Cmp(collateral) != 0 {
		t.Fatalf("full bad-debt seize = %s, want all collateral %s", full.MaxSeizeAssets, collateral)
	}
	partial, partialProfit, ok := sizeLeg(c, price, q, accrued, SizingParams{AllowFullLiquidation: false, SwapHaircutBps: 0})
	if !ok {
		t.Fatal("partial fallback should also size")
	}
	if partial.MaxSeizeAssets.Cmp(mustBig("900000000000000000")) != 0 {
		t.Fatalf("partial seize = %s, want fixed 90%%", partial.MaxSeizeAssets)
	}
	if fullProfit.Cmp(partialProfit) <= 0 {
		t.Fatalf("full bad-debt seize should capture more total profit: full=%s partial=%s", fullProfit, partialProfit)
	}
}

// TestEvaluateLegRejectsBadPrice covers the fail-closed guard against a zero/negative settlement price
// (a malformed auction frame) — without it, maxBorrow=0 flags every position liquidatable with phantom
// profit and the bot bids into a guaranteed revert.
func TestEvaluateLegRejectsBadPrice(t *testing.T) {
	m := goldenMarket()
	cand := Candidate{Market: MarketInfo{State: m}, Position: goldenBorrower()}
	q := newQuote("1534500000000000000000", mustBig("100000000000"))
	sp := SizingParams{AllowFullLiquidation: true}
	for _, bad := range []*big.Int{big.NewInt(0), big.NewInt(-1), nil} {
		if _, _, ok := evalLeg(cand, bad, seedAdapter, q, m.LastUpdate, sp); ok {
			t.Fatalf("price %v must be rejected", bad)
		}
	}
}

func TestEvaluateLegSkipsHealthy(t *testing.T) {
	m := goldenMarket()
	cand := Candidate{Market: MarketInfo{State: m}, Position: goldenBorrower()}
	q := newQuote("1534500000000000000000", nil)
	// Healthy at the live $2000 price.
	if _, _, ok := evalLeg(cand, mustBig("2000000000000000000000000000"), seedAdapter, q, m.LastUpdate,
		SizingParams{AllowFullLiquidation: true, SwapHaircutBps: 200}); ok {
		t.Fatal("must not liquidate a healthy position")
	}
}

func TestEvaluateLegSkipsUnprofitableExit(t *testing.T) {
	m := goldenMarket()
	cand := Candidate{Market: MarketInfo{State: m}, Position: goldenBorrower()}
	q := newQuote("1400000000000000000000", nil)
	// The position is liquidatable at $1550, but the adapter exit proceeds do not cover repayment.
	if _, _, ok := evalLeg(cand, mustBig("1550000000000000000000000000"), seedAdapter, q, m.LastUpdate,
		SizingParams{AllowFullLiquidation: true, SwapHaircutBps: 0}); ok {
		t.Fatal("must skip when adapter output cannot cover repayment")
	}
}

const assignNowTs = 1781243340

var assignMarketID = common.HexToHash("0x6209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5")

// sizeFixture builds the sizing params + a candidate factory over the seeded golden market (loan token
// tokenA), so the sizing tests can size real legs at a given adapter quote.
func sizeFixture() (SizingParams, func(b byte) Candidate, *big.Int) {
	sp := SizingParams{AllowFullLiquidation: true, SwapHaircutBps: 0}
	info := MarketInfo{Params: abiMarketParams{LoanToken: tokenA}, State: goldenMarket()}
	cand := func(b byte) Candidate {
		var addr common.Address
		addr[19] = b
		return Candidate{MarketID: assignMarketID, Borrower: addr, Market: info, Position: goldenBorrower()}
	}
	return sp, cand, mustBig("1550000000000000000000000000") // price
}

// TestSizeLegClampsToGetMaxAssets proves the single adapter's getMaxAssets liquidity clamp: when the
// adapter can't absorb the full target seize, the leg is re-sized down so its expected loan output stays
// within the cached getMaxAssets budget.
func TestSizeLegClampsToGetMaxAssets(t *testing.T) {
	_, cand, price := sizeFixture()
	sp := SizingParams{AllowFullLiquidation: false, SwapHaircutBps: 0}
	c := cand(1)
	accrued := morpho.AccruedTotalBorrowAssets(c.Market.State, assignNowTs)

	// Uncapped first, to learn the full expectedLoanOut.
	full, fullProfit, ok := sizeLeg(c, price, newQuote("1780000000000000000000", nil), accrued, sp)
	if !ok {
		t.Fatal("uncapped leg should size")
	}

	// Cap the adapter below the full expectedLoanOut so the clamp binds.
	uncapped := expectedLoanOutFor(full.MaxSeizeAssets, newQuote("1780000000000000000000", nil), sp.SwapHaircutBps)
	budget := new(big.Int).Div(uncapped, big.NewInt(2))
	capped, cappedProfit, ok := sizeLeg(c, price, newQuote("1780000000000000000000", budget), accrued, sp)
	if !ok {
		t.Fatal("capped leg should still size (smaller)")
	}
	cappedOut := expectedLoanOutFor(capped.MaxSeizeAssets, newQuote("1780000000000000000000", budget), sp.SwapHaircutBps)
	if cappedOut.Cmp(budget) > 0 {
		t.Fatalf("leg over-draws the adapter: expectedLoanOut=%s > getMaxAssets=%s", cappedOut, budget)
	}
	if cappedOut.Cmp(uncapped) >= 0 {
		t.Fatalf("a tight budget must trim below the uncapped expectedLoanOut: capped=%s uncapped=%s", cappedOut, uncapped)
	}
	if cappedProfit.Cmp(fullProfit) >= 0 {
		t.Fatalf("clamped leg should net less profit: capped=%s full=%s", cappedProfit, fullProfit)
	}
}

// TestSizeLegReturnsExpectedLoanOut proves the strategy computes one adapter output per liquidatable
// candidate while keeping that output out of the signed callback leg.
func TestSizeLegReturnsExpectedLoanOut(t *testing.T) {
	sp, cand, price := sizeFixture()
	c := cand(1)
	accrued := morpho.AccruedTotalBorrowAssets(c.Market.State, assignNowTs)
	q := newQuote("1780000000000000000000", mustBig("1000000000000"))
	leg, _, ok := sizeLeg(c, price, q, accrued, sp)
	if !ok {
		t.Fatal("position should liquidate")
	}
	expectedLoanOut := expectedLoanOutFor(leg.MaxSeizeAssets, q, sp.SwapHaircutBps)
	if expectedLoanOut == nil || expectedLoanOut.Sign() <= 0 {
		t.Fatalf("sizing must return a positive expectedLoanOut, got %v", expectedLoanOut)
	}
	if leg.MaxSeizeAssets.Sign() <= 0 {
		t.Fatalf("leg should seize collateral, got maxSeizeAssets=%s", leg.MaxSeizeAssets)
	}
}

// TestSizeLegClampsSeizeToDebt is the regression for review F2: a barely-liquidatable position with a
// small debt but large collateral. The leg sets MaxSeizeAssets with RepaidShares=0, so Morpho derives the
// repaid shares from the seize and reverts (borrowShares underflow) if the implied repayment exceeds the
// borrower's outstanding debt. Seizing the unclamped 90% target would over-repay; the fix clamps the seize so
// morpho.RepaidAssetsForSeizeAt(MaxSeizeAssets) ≤ the borrower's debt — a full liquidation that never reverts.
func TestSizeLegClampsSeizeToDebt(t *testing.T) {
	lltv := mustBig("500000000000000000")                     // 0.5 — widens the over-repay window vs the golden 0.86
	price := mustBig("1000000000000000000000000000000000000") // 1e36 (collateral≈loan units)
	totalBorrowAssets := mustBig("1000000000000000000000000") // 1:1 shares↔assets, no accrual
	totalBorrowShares := new(big.Int).Set(totalBorrowAssets)
	coll := mustBig("1000000000000000000") // 1e18 collateral
	// Debt just above maxBorrow → liquidatable, but worth far less than 90% of the collateral's value.
	debtShares := new(big.Int).Add(morpho.MaxBorrow(coll, price, lltv), big.NewInt(1_000_000))

	state := morpho.MarketState{
		TotalBorrowAssets: totalBorrowAssets, TotalBorrowShares: totalBorrowShares,
		Lltv: lltv, BorrowRatePerSec: big.NewInt(0), Fee: big.NewInt(0), LastUpdate: assignNowTs,
	}
	coll18 := common.HexToAddress("0x00000000000000000000000000000000000000c0")
	c := Candidate{
		MarketID: assignMarketID, Borrower: common.Address{19: 1},
		Market:   MarketInfo{Params: abiMarketParams{LoanToken: tokenA, CollateralToken: coll18}, State: state},
		Position: morpho.PositionState{BorrowShares: debtShares, Collateral: coll},
	}
	accrued := morpho.AccruedTotalBorrowAssets(state, assignNowTs)
	debt := morpho.BorrowedAssetsAt(c.Position, accrued, totalBorrowShares)

	// Sanity: the UNCLAMPED 90% target really would over-repay (so the clamp is exercised, not vacuous).
	unclampedTarget := morpho.MulDivDown(coll, big.NewInt(9000), big.NewInt(10_000))
	if morpho.RepaidAssetsForSeizeAt(unclampedTarget, price, morpho.LiquidationIncentiveFactor(lltv), accrued, totalBorrowShares).Cmp(debt) <= 0 {
		t.Fatal("test fixture is vacuous: the unclamped 90% seize does not over-repay")
	}

	sp := SizingParams{AllowFullLiquidation: false, SwapHaircutBps: 0}
	// MaxRate sized so the swap proceeds clear the repayment (profitable): expectedLoanOut = collIn·rate·1e6/(1e18·1e18).
	q := newQuote("2000000000000000000000000000000", mustBig("100000000000000000000000000000000"))
	leg, _, ok := sizeLeg(c, price, q, accrued, sp)
	if !ok {
		t.Fatal("a liquidatable position should size a leg")
	}
	repaid := morpho.RepaidAssetsForSeizeAt(leg.MaxSeizeAssets, price, morpho.LiquidationIncentiveFactor(lltv), accrued, totalBorrowShares)
	if repaid.Cmp(debt) > 0 {
		t.Fatalf("seize over-repays: morpho.RepaidAssetsForSeizeAt(%s)=%s > borrowerDebt=%s → Morpho borrowShares underflow",
			leg.MaxSeizeAssets, repaid, debt)
	}
	if leg.MaxSeizeAssets.Cmp(unclampedTarget) >= 0 {
		t.Fatalf("seize was not clamped below the 90%% target: seized=%s target=%s", leg.MaxSeizeAssets, unclampedTarget)
	}
}

// TestSizeLegSkipsDustPosition closes the F2 verify gap: maxSeizeForFullDebt = floor(debtAssets·lif·1e36/price)
// floors to 0 for a dust position (tiny debt under a high collateral price). The clamp must then drive target to 0
// so the leg is SKIPPED (ok=false); the earlier guard that ignored a zero maxSeize left target unclamped and
// over-seized into a borrowShares underflow. Fixture: 2-wei collateral (target=1, past the early guard) at a 2e36
// price with lltv 0.2 (keeps it liquidatable: MaxBorrow floors to 0) and a 1-share debt → maxSeize floors to 0.
func TestSizeLegSkipsDustPosition(t *testing.T) {
	lltv := mustBig("200000000000000000")                     // 0.2 — keeps the high-priced dust position liquidatable
	price := mustBig("2000000000000000000000000000000000000") // 2e36
	totalBorrowAssets := mustBig("1000000000000000000000000") // 1:1, no accrual
	totalBorrowShares := new(big.Int).Set(totalBorrowAssets)
	coll := big.NewInt(2)       // target = morpho.MulDivDown(2, 9000, 10000) = 1 (>0, clears the early target guard)
	debtShares := big.NewInt(1) // 1 share → 1 asset; > morpho.MaxBorrow(2, 2e36, 0.2) = 0 ⇒ liquidatable

	state := morpho.MarketState{
		TotalBorrowAssets: totalBorrowAssets, TotalBorrowShares: totalBorrowShares,
		Lltv: lltv, BorrowRatePerSec: big.NewInt(0), Fee: big.NewInt(0), LastUpdate: assignNowTs,
	}
	coll18 := common.HexToAddress("0x00000000000000000000000000000000000000c0")
	c := Candidate{
		MarketID: assignMarketID, Borrower: common.Address{19: 3},
		Market:   MarketInfo{Params: abiMarketParams{LoanToken: tokenA, CollateralToken: coll18}, State: state},
		Position: morpho.PositionState{BorrowShares: debtShares, Collateral: coll},
	}
	accrued := morpho.AccruedTotalBorrowAssets(state, assignNowTs)
	if !morpho.IsLiquidatableAt(c.Position, price, lltv, accrued, totalBorrowShares) {
		t.Fatal("fixture must be liquidatable so the clamp path is reached")
	}
	if morpho.MaxSeizeForFullDebt(debtShares, price, morpho.LiquidationIncentiveFactor(lltv), accrued, totalBorrowShares).Sign() != 0 {
		t.Fatal("fixture is not a dust case: maxSeizeForFullDebt must floor to 0")
	}
	sp := SizingParams{AllowFullLiquidation: false, SwapHaircutBps: 0}
	q := newQuote("2000000000000000000000000000000", mustBig("100000000000000000000000000000000"))
	if _, _, ok := sizeLeg(c, price, q, accrued, sp); ok {
		t.Fatal("a dust position whose full-debt seize floors to 0 must be skipped (ok=false), not over-seized")
	}
}
