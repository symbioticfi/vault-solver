package redstoneoev

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/morpho"
)

// Candidate is a position to evaluate. The Morpho worker (our independently-tracked at-risk set) is the
// SOLE position source — the auction frame supplies prices only (docs/OEV-PLAN.md §3.1). sizeLeg (below)
// is the single shared decision/sizing path.
type Candidate struct {
	MarketID common.Hash
	Borrower common.Address
	Market   MarketInfo
	Position morpho.PositionState
}

// AdapterQuote is the LiquidLane adapter's redemption terms for a market's collateral: the discounted
// sell rate (getMaxRate = the adapter's oracle price × (1 − curator minDiscount), 1e18-scaled) and
// the token decimals needed to convert it to a loan-token amount. This is the price at which we
// actually offload the seized RWA into the vault — distinct from the Morpho market price that drives
// the liquidation itself.
type AdapterQuote struct {
	MaxRate   *big.Int // 1e18-scaled, minDiscount already applied
	MaxAssets *big.Int // getMaxAssets: cap on swap output (loan units) before the adapter reverts

	// LoanScale/CollScale are 10^loanDec / 10^collDec (the vault asset / collateral token decimals),
	// precomputed once when the quote is built (always — see buildQuote and the tests' newQuote) so the
	// per-leg hot path reads them directly instead of recomputing big.Int.Exp. Invariant: both are
	// non-nil on every AdapterQuote.
	LoanScale *big.Int
	CollScale *big.Int
}

// partialSeizeFractionBps is the fixed fallback when full-collateral liquidations are disabled.
const partialSeizeFractionBps = 9000

// SizingParams controls liquidation sizing (from config). When full liquidation is allowed, a leg targets
// all borrower collateral; otherwise it targets a fixed partial seize. The target is still clamped down by
// the borrower's debt and the adapter's getMaxAssets redemption liquidity. Profit is linear in seize, so
// the full mode captures bad-debt opportunities instead of deliberately leaving the last collateral slice.
type SizingParams struct {
	AllowFullLiquidation bool // true => target all collateral; false => fixed partialSeizeFractionBps
	SwapHaircutBps       int  // EXTRA safety margin on the adapter's already-discounted output (slippage/staleness)
}

// swapOutFor is the loan-token we receive for selling `collIn` of seized collateral through quote q, at the
// adapter's discounted rate minus the extra safety haircut — mirrors the adapter's getAmountOut(getMaxRate):
// collIn × maxRate × 10^loanDec / (1e18 × 10^collDec), then × (1 − haircut). The RFQ solver replicates this
// same adapter formula in rfq/strategy.go amountOutForRate — keep both in sync (not unified: a shared helper
// would take several same-type big.Int args, a swap-footgun for fund pricing).
func swapOutFor(collIn *big.Int, q AdapterQuote, haircutBps int) *big.Int {
	adapterOut := morpho.MulDivDown(new(big.Int).Mul(collIn, q.MaxRate), q.LoanScale, new(big.Int).Mul(morpho.Wad, q.CollScale))
	out := morpho.MulDivDown(adapterOut, big.NewInt(int64(10_000-haircutBps)), big.NewInt(10_000))
	// swapAmountOut is a MIN-out. The adapter recomputes its InvalidSwapRate ceiling with a different nested
	// rounding (floor getAmountOut, THEN apply the curator discount) than our getMaxRate-derived value (the
	// discount is pre-floored onto the oracle price), so ours can land 1 base unit above the ceiling when the
	// haircut is ~0 → InvalidSwapRate revert. Shave one unit so the requested min-out never exceeds the
	// on-chain ceiling (review K5); negligible vs leg profit, and only binds at a near-zero haircut.
	if out.Sign() > 0 {
		out.Sub(out, big.NewInt(1))
	}
	return out
}

// collForBudget is the inverse of swapOutFor: the most collateral whose swapOutFor(...) stays within
// `budget` (loan-token / getMaxAssets units), so an exit sized to it never asks the vault for more than its
// remaining redemption liquidity (which would revert InsufficientAllocate). The single flooring division
// guarantees swapOutFor(result) ≤ budget. Returns 0 when the quote can't price an exit. SwapHaircutBps is
// config-validated to [0, 10000), so 10000−haircut > 0.
func collForBudget(budget *big.Int, q AdapterQuote, haircutBps int) *big.Int {
	h := int64(10_000 - haircutBps)
	if h <= 0 || q.MaxRate == nil || q.MaxRate.Sign() <= 0 {
		return new(big.Int)
	}
	num := new(big.Int).Mul(morpho.Wad, q.CollScale)
	num.Mul(num, big.NewInt(10_000))
	den := new(big.Int).Mul(q.MaxRate, q.LoanScale)
	den.Mul(den, big.NewInt(h))
	if den.Sign() == 0 {
		return new(big.Int)
	}
	return morpho.MulDivDown(budget, num, den)
}

// sizeLeg sizes ONE liquidation leg for candidate c, selling its WHOLE seizure through the single
// configured adapter (quote q) in one swap. It targets either all collateral or the fixed partial seize,
// CLAMPED by the borrower's full debt (so a small-debt / large-collateral position can't over-seize and
// revert the Morpho borrowShares underflow) AND by the adapter's getMaxAssets redemption liquidity (so the
// swap can't ask for more than the vault can allocate and revert InsufficientAllocate). swapAmountOut is
// the min loan-token out for the whole seize at the adapter's discounted getMaxRate minus the safety
// haircut.
//
// Returns the leg (one swap, SwapAmountOut set) and its gross loan profit (swapOut − repaid). ok=false when
// the position can't liquidate profitably here — including the contract's own guards: swapAmountOut must
// EXCEED the repayment. Bundle and gas economics are applied later by bundle selection and operationData.
func sizeLeg(c Candidate, price *big.Int, q AdapterQuote, accrued *big.Int, sp SizingParams) (LiquidationLeg, *big.Int, bool) {
	m, p := c.Market.State, c.Position
	if price == nil || price.Sign() <= 0 {
		return LiquidationLeg{}, nil, false
	}
	if q.MaxRate == nil || q.MaxRate.Sign() <= 0 {
		return LiquidationLeg{}, nil, false // can't price the exit
	}
	if !morpho.IsLiquidatableAt(p, price, m.Lltv, accrued, m.TotalBorrowShares) {
		return LiquidationLeg{}, nil, false
	}
	target := targetSeize(p.Collateral, sp.AllowFullLiquidation)
	if target.Sign() <= 0 {
		return LiquidationLeg{}, nil, false
	}
	// LiquidationIncentiveFactor depends only on the market's lltv, so compute it ONCE here and feed it to
	// both the full-debt clamp and the repayment quote (each recomputed it per leg before) — provably the
	// same value.
	lif := morpho.LiquidationIncentiveFactor(m.Lltv)
	// Clamp the seize so the implied repayment never exceeds the borrower's debt. The leg sets MaxSeizeAssets
	// with RepaidShares=0, so Morpho derives repaidShares from the seize and reverts (borrowShares underflow)
	// once the implied repayment would exceed the outstanding debt — which happens whenever the target
	// collateral is worth more debt than the borrower carries (small debt vs large collateral). maxSeize is
	// the inverse forward-map at the full-debt point (rounded down), so a full liquidation clamps here and
	// can't round up past the debt. maxSeize can floor to 0 for a dust position (debt worth < ~1 collateral
	// unit); clamping target to 0 then returns ok=false below, so we skip it rather than submit a
	// guaranteed-revert over-seize (do NOT guard on maxSeize > 0).
	if maxSeize := morpho.MaxSeizeForFullDebt(p.BorrowShares, price, lif, accrued, m.TotalBorrowShares); target.Cmp(maxSeize) > 0 {
		target = maxSeize
	}
	// Clamp the seize by the adapter's getMaxAssets redemption liquidity: collForBudget is the most
	// collateral whose swapOut stays within MaxAssets, so the requested min-out never exceeds what the vault
	// can allocate (InsufficientAllocate). nil/0 ⇒ uncapped (unknown liquidity).
	if q.MaxAssets != nil && q.MaxAssets.Sign() > 0 {
		if fit := collForBudget(q.MaxAssets, q, sp.SwapHaircutBps); fit.Cmp(target) < 0 {
			target = fit
		}
	}
	if target.Sign() <= 0 {
		return LiquidationLeg{}, nil, false
	}
	swapOut := swapOutFor(target, q, sp.SwapHaircutBps)
	if swapOut.Sign() <= 0 {
		return LiquidationLeg{}, nil, false
	}
	repaid := morpho.RepaidAssetsForSeizeAt(target, price, lif, accrued, m.TotalBorrowShares)
	if swapOut.Cmp(repaid) <= 0 {
		return LiquidationLeg{}, nil, false // proceeds can't cover repayment after discount + haircut
	}
	profit := new(big.Int).Sub(swapOut, repaid) // > 0 here
	leg := LiquidationLeg{
		MarketId:       c.MarketID,
		Borrower:       c.Borrower,
		MaxSeizeAssets: target,
		SwapAmountOut:  swapOut,
	}
	return leg, profit, true
}

func targetSeize(collateral *big.Int, allowFull bool) *big.Int {
	if collateral == nil {
		return new(big.Int)
	}
	if allowFull {
		return new(big.Int).Set(collateral)
	}
	return morpho.MulDivDown(collateral, big.NewInt(partialSeizeFractionBps), big.NewInt(10_000))
}

func orZero(n *big.Int) *big.Int {
	if n == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(n)
}
