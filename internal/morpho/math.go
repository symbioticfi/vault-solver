package morpho

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

// Morpho Blue math, ported verbatim from morpho-org/morpho-blue (see docs/OEV-PLAN.md §6.4). This is
// the SINGLE source of truth for health/sizing over a worker-derived candidate set. All arithmetic is
// big.Int with the exact rounding directions Morpho uses on-chain; an off-by-one here means reverted or
// unprofitable fills. Lives in internal/morpho so any solver can reuse it.

// WAD-scaled constants (1e18 fixed point) and Morpho library constants.
var (
	one               = big.NewInt(1) // reused divisor adjustment in MulDivUp (avoids a per-call alloc)
	Wad               = big.NewInt(1e18)
	twoWad            = big.NewInt(2e18)    // 2·WAD — Taylor-series denominators, hoisted out of the hot path
	threeWad          = big.NewInt(3e18)    // 3·WAD
	oraclePriceScale  = chain.Exp10(36)     // ORACLE_PRICE_SCALE = 1e36
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

// AccruedTotalBorrowAssets returns totalBorrowAssets grown to `nowTs` using the Taylor-compounded
// borrow rate — the off-chain replica of Morpho `_accrueInterest` (borrow shares are unchanged by
// accrual; only the assets side grows). Returns the original value when elapsed is 0 or the rate is 0.
func AccruedTotalBorrowAssets(m MarketState, nowTs uint64) *big.Int {
	tba := new(big.Int).Set(m.TotalBorrowAssets)
	if m.BorrowRatePerSec == nil || m.BorrowRatePerSec.Sign() == 0 || nowTs <= m.LastUpdate {
		return tba
	}
	elapsed := new(big.Int).SetUint64(nowTs - m.LastUpdate)
	growth := WTaylorCompounded(m.BorrowRatePerSec, elapsed)
	interest := WMulDown(tba, growth)
	return tba.Add(tba, interest)
}

// AccruedMarketState returns Morpho's market accounting after `_accrueInterest`, including the supply side
// needed for bad-debt replay. Borrow shares never change on accrual.
func AccruedMarketState(m MarketState, nowTs uint64) MarketState {
	out := cloneMarketState(m)
	if out.BorrowRatePerSec == nil || out.BorrowRatePerSec.Sign() == 0 || nowTs <= out.LastUpdate {
		return out
	}
	elapsed := new(big.Int).SetUint64(nowTs - out.LastUpdate)
	growth := WTaylorCompounded(out.BorrowRatePerSec, elapsed)
	interest := WMulDown(out.TotalBorrowAssets, growth)
	out.TotalBorrowAssets.Add(out.TotalBorrowAssets, interest)
	out.TotalSupplyAssets.Add(out.TotalSupplyAssets, interest)
	if out.Fee != nil && out.Fee.Sign() != 0 {
		feeAmount := WMulDown(interest, out.Fee)
		supplyExFee := new(big.Int).Sub(out.TotalSupplyAssets, feeAmount)
		feeShares := ToSharesDown(feeAmount, supplyExFee, out.TotalSupplyShares)
		out.TotalSupplyShares.Add(out.TotalSupplyShares, feeShares)
	}
	out.LastUpdate = nowTs
	return out
}

// BorrowedAssetsAt is BorrowedAssets given a pre-accrued total — so the hot path can accrue once per
// candidate and reuse it across the health check and sizing instead of recomputing the Taylor series.
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

// IsLiquidatableAt is IsLiquidatable given a pre-accrued total (hot-path variant).
func IsLiquidatableAt(p PositionState, collateralPrice, lltv, accruedTotal, totalShares *big.Int) bool {
	borrowed := BorrowedAssetsAt(p, accruedTotal, totalShares)
	if borrowed.Sign() == 0 {
		return false
	}
	return MaxBorrow(p.Collateral, collateralPrice, lltv).Cmp(borrowed) < 0
}

// LiquidationProximity returns the two quantities whose ratio is the position's distance to liquidation:
// borrowed = BorrowedAssetsAt(p, …) and maxBorrow = MaxBorrow(p.Collateral, …). Higher borrowed/maxBorrow
// ⇒ closer to (or past) liquidation; borrowed >= maxBorrow is exactly the IsLiquidatableAt boundary. A
// caller ranks without dividing by cross-multiplying the two pairs (no float, no division).
func LiquidationProximity(p PositionState, collateralPrice, lltv, accruedTotal, totalShares *big.Int) (borrowed, maxBorrow *big.Int) {
	return BorrowedAssetsAt(p, accruedTotal, totalShares), MaxBorrow(p.Collateral, collateralPrice, lltv)
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
	seizedQuoted := MulDivUp(seizedAssets, collateralPrice, oraclePriceScale)
	repaidShares := ToSharesUp(WDivUp(seizedQuoted, lif), accruedTotal, totalShares)
	return ToAssetsUp(repaidShares, accruedTotal, totalShares)
}

// ApplySeizeLiquidation replays Morpho Blue liquidate(market, borrower, seizedAssets, 0, data) on local
// state. It assumes m is already accrued to the settlement timestamp and returns ok=false for any state
// transition that would underflow or cannot be priced.
func ApplySeizeLiquidation(m MarketState, p PositionState, seizedAssets, collateralPrice *big.Int) (LiquidationReplay, bool) {
	if seizedAssets == nil || seizedAssets.Sign() <= 0 || collateralPrice == nil || collateralPrice.Sign() <= 0 ||
		m.TotalBorrowAssets == nil || m.TotalBorrowShares == nil || m.TotalSupplyAssets == nil ||
		p.BorrowShares == nil || p.Collateral == nil || m.Lltv == nil {
		return LiquidationReplay{}, false
	}
	lif := LiquidationIncentiveFactor(m.Lltv)
	seizedQuoted := MulDivUp(seizedAssets, collateralPrice, oraclePriceScale)
	repaidShares := ToSharesUp(WDivUp(seizedQuoted, lif), m.TotalBorrowAssets, m.TotalBorrowShares)
	repaidAssets := ToAssetsUp(repaidShares, m.TotalBorrowAssets, m.TotalBorrowShares)
	if p.BorrowShares.Cmp(repaidShares) < 0 || m.TotalBorrowShares.Cmp(repaidShares) < 0 || p.Collateral.Cmp(seizedAssets) < 0 {
		return LiquidationReplay{}, false
	}
	out := LiquidationReplay{
		Market:        cloneMarketState(m),
		Position:      clonePositionState(p),
		RepaidAssets:  repaidAssets,
		RepaidShares:  repaidShares,
		BadDebtAssets: new(big.Int),
		BadDebtShares: new(big.Int),
	}
	out.Position.BorrowShares.Sub(out.Position.BorrowShares, repaidShares)
	out.Market.TotalBorrowShares.Sub(out.Market.TotalBorrowShares, repaidShares)
	out.Market.TotalBorrowAssets = zeroFloorSub(out.Market.TotalBorrowAssets, repaidAssets)
	out.Position.Collateral.Sub(out.Position.Collateral, seizedAssets)
	if out.Position.Collateral.Sign() == 0 {
		out.BadDebtShares = new(big.Int).Set(out.Position.BorrowShares)
		out.BadDebtAssets = minBig(out.Market.TotalBorrowAssets, ToAssetsUp(out.BadDebtShares, out.Market.TotalBorrowAssets, out.Market.TotalBorrowShares))
		if out.Market.TotalSupplyAssets.Cmp(out.BadDebtAssets) < 0 || out.Market.TotalBorrowShares.Cmp(out.BadDebtShares) < 0 {
			return LiquidationReplay{}, false
		}
		out.Market.TotalBorrowAssets.Sub(out.Market.TotalBorrowAssets, out.BadDebtAssets)
		out.Market.TotalSupplyAssets.Sub(out.Market.TotalSupplyAssets, out.BadDebtAssets)
		out.Market.TotalBorrowShares.Sub(out.Market.TotalBorrowShares, out.BadDebtShares)
		out.Position.BorrowShares = new(big.Int)
	}
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

/* ───────── whole-market convenience forms (accrue once, then forward) ───────── */

// BorrowedAssets accrues the market to nowTs, then forwards to BorrowedAssetsAt. The hot path accrues once
// and calls the *At forms directly; these whole-market forms are for callers (and tests) holding a raw state.
func BorrowedAssets(m MarketState, p PositionState, nowTs uint64) *big.Int {
	return BorrowedAssetsAt(p, AccruedTotalBorrowAssets(m, nowTs), m.TotalBorrowShares)
}

// IsLiquidatable accrues the market to nowTs, then forwards to IsLiquidatableAt.
func IsLiquidatable(m MarketState, p PositionState, collateralPrice *big.Int, nowTs uint64) bool {
	return IsLiquidatableAt(p, collateralPrice, m.Lltv, AccruedTotalBorrowAssets(m, nowTs), m.TotalBorrowShares)
}

// RepaidAssetsForSeize accrues the market to nowTs, then forwards to RepaidAssetsForSeizeAt.
func RepaidAssetsForSeize(m MarketState, seizedAssets, collateralPrice, lltv *big.Int, nowTs uint64) *big.Int {
	return RepaidAssetsForSeizeAt(seizedAssets, collateralPrice, LiquidationIncentiveFactor(lltv), AccruedTotalBorrowAssets(m, nowTs), m.TotalBorrowShares)
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

func cloneMarketState(m MarketState) MarketState {
	return MarketState{
		TotalSupplyAssets: cloneBig(m.TotalSupplyAssets),
		TotalSupplyShares: cloneBig(m.TotalSupplyShares),
		TotalBorrowAssets: cloneBig(m.TotalBorrowAssets),
		TotalBorrowShares: cloneBig(m.TotalBorrowShares),
		LastUpdate:        m.LastUpdate,
		Fee:               cloneBig(m.Fee),
		Lltv:              cloneBig(m.Lltv),
		BorrowRatePerSec:  cloneBig(m.BorrowRatePerSec),
	}
}

func clonePositionState(p PositionState) PositionState {
	return PositionState{BorrowShares: cloneBig(p.BorrowShares), Collateral: cloneBig(p.Collateral)}
}

func cloneBig(n *big.Int) *big.Int {
	if n == nil {
		return nil
	}
	return new(big.Int).Set(n)
}

func zeroFloorSub(x, y *big.Int) *big.Int {
	if x.Cmp(y) <= 0 {
		return new(big.Int)
	}
	return new(big.Int).Sub(x, y)
}

func minBig(a, b *big.Int) *big.Int {
	if a.Cmp(b) <= 0 {
		return new(big.Int).Set(a)
	}
	return new(big.Int).Set(b)
}
