package morpho

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/bigmath"
)

// Morpho Blue accounting, matching morpho-org/morpho-blue (see docs/OEV-PLAN.md §6.4). This is
// the SINGLE source of truth for health/sizing over a worker-derived candidate set. All arithmetic is
// big.Int with the exact rounding directions Morpho uses on-chain; an off-by-one here means reverted or
// unprofitable fills. Lives in internal/morpho so any solver can reuse it.

// WAD-scaled constants (1e18 fixed point) and Morpho library constants.
var (
	one               = big.NewInt(1) // reused divisor adjustment in MulDivUp (avoids a per-call alloc)
	Wad               = big.NewInt(1e18)
	twoWad            = big.NewInt(2e18)    // 2·WAD — Taylor-series denominators, hoisted out of the hot path
	threeWad          = big.NewInt(3e18)    // 3·WAD
	oraclePriceScale  = bigmath.Exp10(36)   // ORACLE_PRICE_SCALE = 1e36
	virtualShares     = big.NewInt(1e6)     // SharesMathLib.VIRTUAL_SHARES
	virtualAssets     = big.NewInt(1)       // SharesMathLib.VIRTUAL_ASSETS
	liquidationCursor = big.NewInt(0.3e18)  // ConstantsLib.LIQUIDATION_CURSOR (β)
	maxLiqIncentive   = big.NewInt(1.15e18) // ConstantsLib.MAX_LIQUIDATION_INCENTIVE_FACTOR (M)
)

// MarketState is the on-chain market accounting (Morpho `market(id)`), plus the IRM rate needed to
// accrue interest locally. Amounts are big.Int (uint128 on-chain).
type MarketState struct {
	TotalSupplyAssets *big.Int
	TotalSupplyShares *big.Int
	TotalBorrowAssets *big.Int
	TotalBorrowShares *big.Int
	LastUpdate        uint64
	Fee               *big.Int
	Lltv              *big.Int // from idToMarketParams; the market's liquidation LTV (wad)
	BorrowRatePerSec  *big.Int // IRM borrowRateView (wad/sec); zero ⇒ no accrual (irm == 0)
}

// PositionState is a borrower's position (Morpho `position(id, borrower)`).
type PositionState struct {
	BorrowShares *big.Int
	Collateral   *big.Int
}

// LiquidationReplay is the local post-state of Morpho's seize-driven liquidate branch.
type LiquidationReplay struct {
	Market        MarketState
	Position      PositionState
	RepaidAssets  *big.Int
	RepaidShares  *big.Int
	BadDebtAssets *big.Int
	BadDebtShares *big.Int
}

// AccruedMarketState applies the same interest to both sides of the market and
// mints fee shares at the supply balance excluding that fee, as Morpho does.
func AccruedMarketState(m MarketState, nowTs uint64) MarketState {
	out := CloneMarketState(m)
	if nowTs <= m.LastUpdate || m.BorrowRatePerSec == nil || m.BorrowRatePerSec.Sign() == 0 {
		return out
	}
	elapsed := new(big.Int).SetUint64(nowTs - m.LastUpdate)
	interest := WMulDown(m.TotalBorrowAssets, WTaylorCompounded(m.BorrowRatePerSec, elapsed))
	out.TotalBorrowAssets.Add(out.TotalBorrowAssets, interest)
	out.TotalSupplyAssets.Add(out.TotalSupplyAssets, interest)
	if m.Fee != nil && m.Fee.Sign() != 0 {
		feeAssets := WMulDown(interest, m.Fee)
		base := new(big.Int).Sub(out.TotalSupplyAssets, feeAssets)
		out.TotalSupplyShares.Add(out.TotalSupplyShares, ToSharesDown(feeAssets, base, m.TotalSupplyShares))
	}
	out.LastUpdate = nowTs
	return out
}

// BorrowedAssetsAt values borrower shares against already accrued market totals.
// Accrue once per market, then reuse those totals for health checks and sizing.
func BorrowedAssetsAt(p PositionState, accruedTotal, totalShares *big.Int) *big.Int {
	if p.BorrowShares == nil || p.BorrowShares.Sign() == 0 {
		return big.NewInt(0)
	}
	return ToAssetsUp(p.BorrowShares, accruedTotal, totalShares)
}

// MaxBorrow returns the largest debt the position may carry at `collateralPrice` (1e36-scaled),
// rounding down in the protocol's favor: collateral.mulDivDown(price, 1e36).wMulDown(lltv).
func MaxBorrow(collateral, collateralPrice, lltv *big.Int) *big.Int {
	return WMulDown(MulDivDown(collateral, collateralPrice, oraclePriceScale), lltv)
}

// IsLiquidatableAt checks whether debt strictly exceeds the collateral borrowing
// limit using accrued market totals. Equality is healthy.
func IsLiquidatableAt(p PositionState, collateralPrice, lltv, accruedTotal, totalShares *big.Int) bool {
	borrowed := BorrowedAssetsAt(p, accruedTotal, totalShares)
	if borrowed.Sign() == 0 {
		return false
	}
	return MaxBorrow(p.Collateral, collateralPrice, lltv).Cmp(borrowed) < 0
}

// LiquidationIncentiveFactor = min(M, 1 / (1 - cursor*(1 - lltv))) in wad, matching liquidate().
func LiquidationIncentiveFactor(lltv *big.Int) *big.Int {
	// WAD.wDivDown(WAD - LIQUIDATION_CURSOR.wMulDown(WAD - lltv))
	oneMinusLltv := new(big.Int).Sub(Wad, lltv)
	denom := new(big.Int).Sub(Wad, WMulDown(liquidationCursor, oneMinusLltv))
	lif := WDivDown(Wad, denom)
	if lif.Cmp(maxLiqIncentive) > 0 {
		return new(big.Int).Set(maxLiqIncentive)
	}
	return lif
}

// RepaidAssetsForSeizeAt replicates liquidate()'s seize→shares→assets path with Morpho's rounding (quote
// up, divide by LIF up, shares up, assets up) given a pre-accrued total and the precomputed
// LiquidationIncentiveFactor — the hot-path variant (sizeLeg computes the LIF once and passes it here and
// to MaxSeizeForFullDebt).
func RepaidAssetsForSeizeAt(seizedAssets, collateralPrice, lif, accruedTotal, totalShares *big.Int) *big.Int {
	shares := repaidSharesForSeize(seizedAssets, collateralPrice, lif, accruedTotal, totalShares)
	return ToAssetsUp(shares, accruedTotal, totalShares)
}

func repaidSharesForSeize(seize, price, incentive, assets, shares *big.Int) *big.Int {
	quoted := MulDivUp(seize, price, oraclePriceScale)
	return ToSharesUp(WDivUp(quoted, incentive), assets, shares)
}

// ApplySeizeLiquidation replays Morpho Blue liquidate(market, borrower, seizedAssets, 0, data) on local
// state. It assumes m is already accrued to the settlement timestamp and returns ok=false for any state
// transition that would underflow or cannot be priced.
func ApplySeizeLiquidation(m MarketState, p PositionState, seizedAssets, collateralPrice *big.Int) (LiquidationReplay, bool) {
	if seizedAssets == nil || seizedAssets.Sign() <= 0 || collateralPrice == nil || collateralPrice.Sign() <= 0 {
		return LiquidationReplay{}, false
	}
	for _, value := range []*big.Int{m.TotalBorrowAssets, m.TotalBorrowShares, m.TotalSupplyAssets, m.Lltv, p.BorrowShares, p.Collateral} {
		if value == nil || value.Sign() < 0 {
			return LiquidationReplay{}, false
		}
	}
	if m.Lltv.Cmp(Wad) > 0 {
		return LiquidationReplay{}, false
	}
	shares := repaidSharesForSeize(seizedAssets, collateralPrice, LiquidationIncentiveFactor(m.Lltv), m.TotalBorrowAssets, m.TotalBorrowShares)
	if shares.Cmp(p.BorrowShares) > 0 || shares.Cmp(m.TotalBorrowShares) > 0 || seizedAssets.Cmp(p.Collateral) > 0 {
		return LiquidationReplay{}, false
	}
	out := LiquidationReplay{
		Market: CloneMarketState(m), Position: ClonePositionState(p), RepaidShares: shares,
		RepaidAssets:  ToAssetsUp(shares, m.TotalBorrowAssets, m.TotalBorrowShares),
		BadDebtAssets: new(big.Int), BadDebtShares: new(big.Int),
	}
	out.Position.Collateral.Sub(out.Position.Collateral, seizedAssets)
	out.Position.BorrowShares.Sub(out.Position.BorrowShares, shares)
	out.Market.TotalBorrowShares.Sub(out.Market.TotalBorrowShares, shares)
	out.Market.TotalBorrowAssets = zeroFloorSub(m.TotalBorrowAssets, out.RepaidAssets)
	if out.Position.Collateral.Sign() != 0 {
		return out, true
	}
	// Once collateral is exhausted the remaining borrower shares become bad debt.
	// Round using the post-repayment market, then socialize the loss to suppliers.
	out.BadDebtShares.Set(out.Position.BorrowShares)
	out.BadDebtAssets = bigmath.Min(out.Market.TotalBorrowAssets,
		ToAssetsUp(out.BadDebtShares, out.Market.TotalBorrowAssets, out.Market.TotalBorrowShares))
	if out.BadDebtAssets.Cmp(out.Market.TotalSupplyAssets) > 0 || out.BadDebtShares.Cmp(out.Market.TotalBorrowShares) > 0 {
		return LiquidationReplay{}, false
	}
	out.Market.TotalBorrowAssets.Sub(out.Market.TotalBorrowAssets, out.BadDebtAssets)
	out.Market.TotalSupplyAssets.Sub(out.Market.TotalSupplyAssets, out.BadDebtAssets)
	out.Market.TotalBorrowShares.Sub(out.Market.TotalBorrowShares, out.BadDebtShares)
	out.Position.BorrowShares.SetInt64(0)
	return out, true
}

// MaxSeizeForFullDebt is the largest collateral seize whose implied repayment never exceeds the borrower's
// outstanding debt — the inverse of RepaidAssetsForSeizeAt at the full-debt point. It mirrors Morpho
// liquidate()'s shares→seize path (the branch where repaidShares is the input): seize the full borrow
// shares back through assets-down → ×LIF down → ÷price down. Every step rounds DOWN, so the resulting seize
// repays AT MOST the full debt — a full liquidation clamps to this and can never round up past the debt
// (which would underflow borrowShares and revert). The leg's seize target is min(its fraction, this). lif is
// the precomputed LiquidationIncentiveFactor (sizeLeg computes it once for both this and
// RepaidAssetsForSeizeAt).
func MaxSeizeForFullDebt(borrowShares, collateralPrice, lif, accruedTotal, totalShares *big.Int) *big.Int {
	if borrowShares == nil || borrowShares.Sign() <= 0 || collateralPrice == nil || collateralPrice.Sign() <= 0 {
		return new(big.Int)
	}
	debtAssets := ToAssetsDown(borrowShares, accruedTotal, totalShares)
	return MulDivDown(WMulDown(debtAssets, lif), oraclePriceScale, collateralPrice)
}

/* ───────── SharesMathLib (virtual shares/assets) ───────── */

func ToSharesUp(assets, totalAssets, totalShares *big.Int) *big.Int {
	return MulDivUp(assets, new(big.Int).Add(totalShares, virtualShares), new(big.Int).Add(totalAssets, virtualAssets))
}

func ToAssetsUp(shares, totalAssets, totalShares *big.Int) *big.Int {
	return MulDivUp(shares, new(big.Int).Add(totalAssets, virtualAssets), new(big.Int).Add(totalShares, virtualShares))
}

func ToSharesDown(assets, totalAssets, totalShares *big.Int) *big.Int {
	return MulDivDown(assets, new(big.Int).Add(totalShares, virtualShares), new(big.Int).Add(totalAssets, virtualAssets))
}

// ToAssetsDown is SharesMathLib.toAssetsDown — used by liquidate()'s shares→seize path (MaxSeizeForFullDebt).
func ToAssetsDown(shares, totalAssets, totalShares *big.Int) *big.Int {
	return MulDivDown(shares, new(big.Int).Add(totalAssets, virtualAssets), new(big.Int).Add(totalShares, virtualShares))
}

/* ───────── MathLib (wad + mulDiv) ───────── */

func WTaylorCompounded(ratePerSec, n *big.Int) *big.Int {
	// firstTerm = x*n; second = firstTerm²/(2·WAD); third = second·firstTerm/(3·WAD)
	first := new(big.Int).Mul(ratePerSec, n)
	second := MulDivDown(first, first, twoWad)
	third := MulDivDown(second, first, threeWad)
	return new(big.Int).Add(new(big.Int).Add(first, second), third)
}

func WMulDown(x, y *big.Int) *big.Int { return MulDivDown(x, y, Wad) }
func WDivDown(x, y *big.Int) *big.Int { return MulDivDown(x, Wad, y) }
func WDivUp(x, y *big.Int) *big.Int   { return MulDivUp(x, Wad, y) }

func MulDivDown(x, y, d *big.Int) *big.Int {
	return new(big.Int).Div(new(big.Int).Mul(x, y), d)
}

func MulDivUp(x, y, d *big.Int) *big.Int {
	// (x*y + d - 1) / d
	num := new(big.Int).Mul(x, y)
	num.Add(num, new(big.Int).Sub(d, one))
	return num.Div(num, d)
}

// CloneMarketState returns a deep copy of a Morpho market state snapshot.
func CloneMarketState(m MarketState) MarketState {
	return MarketState{
		TotalSupplyAssets: bigmath.Clone(m.TotalSupplyAssets),
		TotalSupplyShares: bigmath.Clone(m.TotalSupplyShares),
		TotalBorrowAssets: bigmath.Clone(m.TotalBorrowAssets),
		TotalBorrowShares: bigmath.Clone(m.TotalBorrowShares),
		LastUpdate:        m.LastUpdate,
		Fee:               bigmath.Clone(m.Fee),
		Lltv:              bigmath.Clone(m.Lltv),
		BorrowRatePerSec:  bigmath.Clone(m.BorrowRatePerSec),
	}
}

// ClonePositionState returns a deep copy of a Morpho position snapshot.
func ClonePositionState(p PositionState) PositionState {
	return PositionState{BorrowShares: bigmath.Clone(p.BorrowShares), Collateral: bigmath.Clone(p.Collateral)}
}

func zeroFloorSub(x, y *big.Int) *big.Int {
	if x.Cmp(y) <= 0 {
		return new(big.Int)
	}
	return new(big.Int).Sub(x, y)
}
