package morpho

import (
	"math/big"
	"testing"
)

// goldenMarket is the live Sepolia test market state read on-chain (docs/OEV-PLAN.md §6.5/§6.7):
// TLOAN(6dp)/TCOL(18dp), lltv 0.86, IRM borrowRateView = 182418302 wad/sec, lastUpdate 1780059204.
func goldenMarket() MarketState {
	return MarketState{
		TotalSupplyAssets: big.NewInt(100000000068),
		TotalSupplyShares: mustBig("100000000000000000"),
		TotalBorrowAssets: big.NewInt(4730000068),
		TotalBorrowShares: mustBig("4729999932892591"),
		LastUpdate:        1780059204,
		Fee:               big.NewInt(0),
		Lltv:              mustBig("860000000000000000"),
		BorrowRatePerSec:  big.NewInt(182418302),
	}
}

// goldenBorrower is 0x629d… — 1.0 TCOL collateral, borrowShares 1685600000000000.
func goldenBorrower() PositionState {
	return PositionState{BorrowShares: mustBig("1685600000000000"), Collateral: mustBig("1000000000000000000")}
}

func TestAccrualMatchesOnChain(t *testing.T) {
	m := goldenMarket()
	// elapsed = 1781246580 - 1780059204 = 1187376s -> interest 1024624 (verified §6.7).
	got := AccruedTotalBorrowAssets(m, 1781246580)
	if want := big.NewInt(4731024692); got.Cmp(want) != 0 {
		t.Fatalf("AccruedTotalBorrowAssets = %s, want %s", got, want)
	}
	// No accrual at lastUpdate.
	if atLU := AccruedTotalBorrowAssets(m, m.LastUpdate); atLU.Cmp(m.TotalBorrowAssets) != 0 {
		t.Fatalf("accrual at lastUpdate = %s, want %s", atLU, m.TotalBorrowAssets)
	}
	full := AccruedMarketState(m, 1781246580)
	if want := big.NewInt(4731024692); full.TotalBorrowAssets.Cmp(want) != 0 {
		t.Fatalf("AccruedMarketState borrow = %s, want %s", full.TotalBorrowAssets, want)
	}
	if want := big.NewInt(100001024692); full.TotalSupplyAssets.Cmp(want) != 0 {
		t.Fatalf("AccruedMarketState supply = %s, want %s", full.TotalSupplyAssets, want)
	}
	if full.LastUpdate != 1781246580 {
		t.Fatalf("AccruedMarketState lastUpdate = %d, want 1781246580", full.LastUpdate)
	}
}

func TestBorrowedAssetsUnaccrued(t *testing.T) {
	// toAssetsUp at lastUpdate equals RedStone's pushed borrow_assets (1685600048) within 1-wei
	// rounding (§6.7): our ToAssetsUp rounds up -> 1685600049.
	got := BorrowedAssets(goldenMarket(), goldenBorrower(), goldenMarket().LastUpdate)
	if want := big.NewInt(1685600049); got.Cmp(want) != 0 {
		t.Fatalf("unaccrued borrowed = %s, want %s", got, want)
	}
}

func TestLiquidationIncentiveFactor(t *testing.T) {
	// lltv 0.86 -> 1e36 / 0.958e18 = 1043841336116910229 (floor).
	got := LiquidationIncentiveFactor(mustBig("860000000000000000"))
	if want := mustBig("1043841336116910229"); got.Cmp(want) != 0 {
		t.Fatalf("LIF = %s, want %s", got, want)
	}
}

// TestMaxSeizeForFullDebt pins the F2 clamp helper. The on-chain revert is a borrowShares underflow
// (position.borrowShares -= repaidShares), so the binding invariant is repaidShares(maxSeize) ≤
// borrowShares — every inverse step rounds down so a full liquidation clamped to this can't underflow. It
// also stays ≤ the up-rounded debt (BorrowedAssetsAt), the proxy the strategy clamps against.
func TestMaxSeizeForFullDebt(t *testing.T) {
	lltv := mustBig("500000000000000000")
	price := mustBig("1000000000000000000000000000000000000") // 1e36
	totalAssets := mustBig("1000000000000000000000000")
	totalShares := new(big.Int).Set(totalAssets) // 1:1
	lif := LiquidationIncentiveFactor(lltv)
	for _, shares := range []*big.Int{
		mustBig("1"), mustBig("500000000000000001"), mustBig("123456789012345678"), mustBig("999999999999999999"),
	} {
		maxSeize := MaxSeizeForFullDebt(shares, price, lif, totalAssets, totalShares)
		// The exact on-chain underflow condition: repaidShares must not exceed the borrower's borrowShares.
		seizedQuoted := MulDivUp(maxSeize, price, oraclePriceScale)
		repaidShares := ToSharesUp(WDivUp(seizedQuoted, lif), totalAssets, totalShares)
		if repaidShares.Cmp(shares) > 0 {
			t.Fatalf("MaxSeizeForFullDebt over-repays: shares=%s seize=%s repaidShares=%s > borrowShares=%s (underflow)",
				shares, maxSeize, repaidShares, shares)
		}
		// And ≤ the up-rounded debt the strategy uses as its assets-level proxy.
		debtUp := BorrowedAssetsAt(PositionState{BorrowShares: shares}, totalAssets, totalShares)
		if repaid := RepaidAssetsForSeizeAt(maxSeize, price, lif, totalAssets, totalShares); repaid.Cmp(debtUp) > 0 {
			t.Fatalf("repaidAssets %s > borrowerDebt(up) %s for shares=%s seize=%s", repaid, debtUp, shares, maxSeize)
		}
	}
	// Degenerate inputs fail closed to 0 (no seize), not a panic.
	if got := MaxSeizeForFullDebt(big.NewInt(0), price, lif, totalAssets, totalShares); got.Sign() != 0 {
		t.Fatalf("zero borrowShares must give zero maxSeize, got %s", got)
	}
	if got := MaxSeizeForFullDebt(mustBig("1"), big.NewInt(0), lif, totalAssets, totalShares); got.Sign() != 0 {
		t.Fatalf("zero price must give zero maxSeize, got %s", got)
	}
}

func TestIsLiquidatableAcrossPrices(t *testing.T) {
	m := goldenMarket()
	ts := uint64(1781246580)
	cases := []struct {
		name  string
		pos   PositionState
		price string
		want  bool
	}{
		{"live 2000 healthy", goldenBorrower(), "2000000000000000000000000000", false},             // 2e27
		{"auctioned 1800.94 liquidatable", goldenBorrower(), "1800943620100000000000000000", true}, // 1.8009e27
		{"crashed 1550 liquidatable", goldenBorrower(), "1550000000000000000000000000", true},      // 1.55e27
		// Zero debt (BorrowShares=0) with collateral is healthy at ANY price — the debt-free branch through
		// IsLiquidatable can never be underwater. Priced at the crash level that liquidates a debted position.
		{"zero debt healthy at crash price", PositionState{BorrowShares: big.NewInt(0), Collateral: mustBig("1000000000000000000")}, "1550000000000000000000000000", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsLiquidatable(m, c.pos, mustBig(c.price), ts); got != c.want {
				t.Fatalf("IsLiquidatable(%s) = %v, want %v", c.price, got, c.want)
			}
		})
	}
}

// TestLiquidationProximity pins the proximity pair against BorrowedAssetsAt / MaxBorrow and checks that
// the borrowed >= maxBorrow boundary tracks IsLiquidatableAt.
func TestLiquidationProximity(t *testing.T) {
	m := goldenMarket()
	accrued := AccruedTotalBorrowAssets(m, m.LastUpdate)
	cases := []struct {
		name        string
		pos         PositionState
		price       string
		wantLiqable bool
	}{
		{"healthy at 2000", goldenBorrower(), "2000000000000000000000000000", false},
		{"liquidatable at 1550", goldenBorrower(), "1550000000000000000000000000", true},
		{"zero debt", PositionState{BorrowShares: big.NewInt(0), Collateral: mustBig("1000000000000000000")}, "1550000000000000000000000000", false},
		{"underwater: zero maxBorrow with debt", goldenBorrower(), "0", true}, // price 0 ⇒ maxBorrow 0, borrowed > 0
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			price := mustBig(c.price)
			borrowed, maxBorrow := LiquidationProximity(c.pos, price, m.Lltv, accrued, m.TotalBorrowShares)
			// The pair must equal the underlying helpers exactly.
			if want := BorrowedAssetsAt(c.pos, accrued, m.TotalBorrowShares); borrowed.Cmp(want) != 0 {
				t.Fatalf("borrowed = %s, want %s", borrowed, want)
			}
			if want := MaxBorrow(c.pos.Collateral, price, m.Lltv); maxBorrow.Cmp(want) != 0 {
				t.Fatalf("maxBorrow = %s, want %s", maxBorrow, want)
			}
			// borrowed >= maxBorrow (with borrowed > 0) is the IsLiquidatableAt boundary.
			boundary := borrowed.Sign() > 0 && borrowed.Cmp(maxBorrow) >= 0
			if boundary != c.wantLiqable {
				t.Fatalf("borrowed>=maxBorrow = %v (borrowed=%s maxBorrow=%s), want %v", boundary, borrowed, maxBorrow, c.wantLiqable)
			}
			if liq := IsLiquidatableAt(c.pos, price, m.Lltv, accrued, m.TotalBorrowShares); liq != c.wantLiqable {
				t.Fatalf("IsLiquidatableAt = %v, want %v", liq, c.wantLiqable)
			}
		})
	}
}

func TestRepaidAssetsForSeizeMatchesLiveLiquidation(t *testing.T) {
	// The real successful liquidation (§6.6) seized 0.5 TCOL at $1550 and repaid ~742.45 TLOAN
	// (swapAmountOut 760 - profit 17.55). Assert RepaidAssetsForSeize lands in that band.
	m := goldenMarket()
	got := RepaidAssetsForSeize(m, mustBig("500000000000000000"), mustBig("1550000000000000000000000000"),
		m.Lltv, m.LastUpdate)
	lo, hi := big.NewInt(742_000_000), big.NewInt(743_000_000)
	if got.Cmp(lo) < 0 || got.Cmp(hi) > 0 {
		t.Fatalf("RepaidAssetsForSeize = %s, want in [%s, %s]", got, lo, hi)
	}
}

func TestApplySeizeLiquidationOrdinary(t *testing.T) {
	m := MarketState{
		TotalSupplyAssets: mustBig("2000000"),
		TotalSupplyShares: mustBig("2000000"),
		TotalBorrowAssets: mustBig("1000000"),
		TotalBorrowShares: mustBig("1000000"),
		Lltv:              mustBig("500000000000000000"),
	}
	p := PositionState{BorrowShares: mustBig("500000"), Collateral: mustBig("1000000000000000000")}
	seized := mustBig("10000000000000000")
	price := mustBig("1000000000000000000000000")

	got, ok := ApplySeizeLiquidation(m, p, seized, price)
	if !ok {
		t.Fatal("ordinary liquidation should replay")
	}
	lif := LiquidationIncentiveFactor(m.Lltv)
	wantRepaid := RepaidAssetsForSeizeAt(seized, price, lif, m.TotalBorrowAssets, m.TotalBorrowShares)
	if got.RepaidAssets.Cmp(wantRepaid) != 0 {
		t.Fatalf("repaidAssets = %s, want %s", got.RepaidAssets, wantRepaid)
	}
	if got.Position.Collateral.Cmp(new(big.Int).Sub(p.Collateral, seized)) != 0 {
		t.Fatalf("collateral = %s, want %s", got.Position.Collateral, new(big.Int).Sub(p.Collateral, seized))
	}
	if got.Market.TotalBorrowShares.Cmp(new(big.Int).Sub(m.TotalBorrowShares, got.RepaidShares)) != 0 {
		t.Fatalf("totalBorrowShares = %s, want initial-repaidShares", got.Market.TotalBorrowShares)
	}
	if got.Market.TotalBorrowAssets.Cmp(new(big.Int).Sub(m.TotalBorrowAssets, got.RepaidAssets)) != 0 {
		t.Fatalf("totalBorrowAssets = %s, want initial-repaidAssets", got.Market.TotalBorrowAssets)
	}
	if got.BadDebtAssets.Sign() != 0 || got.BadDebtShares.Sign() != 0 {
		t.Fatalf("ordinary liquidation recorded bad debt assets=%s shares=%s", got.BadDebtAssets, got.BadDebtShares)
	}
}

func TestApplySeizeLiquidationBadDebt(t *testing.T) {
	m := MarketState{
		TotalSupplyAssets: mustBig("2000000"),
		TotalSupplyShares: mustBig("2000000"),
		TotalBorrowAssets: mustBig("1000000"),
		TotalBorrowShares: mustBig("1000000"),
		Lltv:              mustBig("500000000000000000"),
	}
	p := PositionState{BorrowShares: mustBig("500000"), Collateral: mustBig("100000000000000000")}
	got, ok := ApplySeizeLiquidation(m, p, p.Collateral, mustBig("1000000000000000000000000"))
	if !ok {
		t.Fatal("bad-debt liquidation should replay")
	}
	if got.Position.Collateral.Sign() != 0 || got.Position.BorrowShares.Sign() != 0 {
		t.Fatalf("borrower should be closed after bad debt, got collateral=%s borrowShares=%s", got.Position.Collateral, got.Position.BorrowShares)
	}
	if got.BadDebtShares.Sign() == 0 || got.BadDebtAssets.Sign() == 0 {
		t.Fatalf("expected bad debt, got assets=%s shares=%s", got.BadDebtAssets, got.BadDebtShares)
	}
	wantBorrowShares := new(big.Int).Sub(new(big.Int).Sub(m.TotalBorrowShares, got.RepaidShares), got.BadDebtShares)
	if got.Market.TotalBorrowShares.Cmp(wantBorrowShares) != 0 {
		t.Fatalf("totalBorrowShares = %s, want %s", got.Market.TotalBorrowShares, wantBorrowShares)
	}
	wantSupplyAssets := new(big.Int).Sub(m.TotalSupplyAssets, got.BadDebtAssets)
	if got.Market.TotalSupplyAssets.Cmp(wantSupplyAssets) != 0 {
		t.Fatalf("totalSupplyAssets = %s, want %s", got.Market.TotalSupplyAssets, wantSupplyAssets)
	}
}

func TestApplySeizeLiquidationInvalidFailsClosed(t *testing.T) {
	m := MarketState{
		TotalSupplyAssets: mustBig("2000000"),
		TotalSupplyShares: mustBig("2000000"),
		TotalBorrowAssets: mustBig("1000000"),
		TotalBorrowShares: mustBig("1000000"),
		Lltv:              mustBig("500000000000000000"),
	}
	p := PositionState{BorrowShares: big.NewInt(1), Collateral: big.NewInt(1)}
	if _, ok := ApplySeizeLiquidation(m, p, mustBig("1000000000000000000"), mustBig("10000000000000000000000000")); ok {
		t.Fatal("over-seize/over-repay must fail closed")
	}
}

func mustBig(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("bad big int: " + s)
	}
	return n
}
