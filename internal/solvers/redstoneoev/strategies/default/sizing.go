package defaultstrategy

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

// partialSeizeFractionBps is the fixed fallback when full-collateral liquidations are disabled.
const partialSeizeFractionBps = 9000

type sizedLeg struct {
	leg               selectedLeg
	settlementLoanOut *big.Int
	profit            *big.Int
}

// expectedLoanOutFor estimates the loan-token output for selling `collIn` of seized collateral through
// quote q at the adapter's discounted rate minus the extra safety haircut:
// collIn × maxRate × 10^loanDec / (1e18 × 10^collDec), then × (1 − haircut). The RFQ solver replicates this
// same adapter formula in rfq/strategy.go amountOutForRate — keep both in sync (not unified: a shared helper
// would take several same-type big.Int args, a swap-footgun for fund pricing).
func expectedLoanOutFor(collIn *big.Int, q AdapterQuote, haircutBps int) *big.Int {
	adapterOut := morpho.MulDivDown(new(big.Int).Mul(collIn, q.MaxRate), q.LoanScale, new(big.Int).Mul(morpho.Wad, q.CollScale))
	out := morpho.MulDivDown(adapterOut, big.NewInt(int64(10_000-haircutBps)), big.NewInt(10_000))
	// The adapter recomputes its rate ceiling with a different nested rounding (floor getAmountOut, THEN
	// apply the curator discount) than our getMaxRate-derived value. Shave one unit so our estimate stays
	// below the on-chain ceiling; negligible vs leg profit, and only binds at a near-zero haircut.
	if out.Sign() > 0 {
		out.Sub(out, big.NewInt(1))
	}
	return out
}

// settlementLoanOutFor rounds up the cached rate so capacity/gas never use the profit haircut.
// getMaxRate itself is rounded down; one rate unit covers that rounding before amount conversion.
func settlementLoanOutFor(collIn *big.Int, q AdapterQuote) *big.Int {
	rate := new(big.Int).Add(q.MaxRate, big.NewInt(1))
	return morpho.MulDivUp(new(big.Int).Mul(collIn, rate), q.LoanScale, new(big.Int).Mul(morpho.Wad, q.CollScale))
}

// collForBudget bounds uncapped callback output, independently of the profit safety haircut.
func collForBudget(budget *big.Int, q AdapterQuote) *big.Int {
	if q.MaxRate == nil || q.MaxRate.Sign() <= 0 {
		return new(big.Int)
	}
	rate := new(big.Int).Add(q.MaxRate, big.NewInt(1))
	return morpho.MulDivDown(budget, new(big.Int).Mul(morpho.Wad, q.CollScale), new(big.Int).Mul(rate, q.LoanScale))
}

// sizeLeg sizes ONE liquidation leg for candidate c, selling its WHOLE seizure through the single
// configured adapter (quote q) in one swap. It targets either all collateral or the fixed partial seize,
// CLAMPED by the borrower's full debt (so a small-debt / large-collateral position can't over-seize and
// revert the Morpho borrowShares underflow) AND by the adapter's getMaxAssets redemption liquidity (so the
// swap can't ask for more than the vault can allocate and revert InsufficientAllocate).
//
// Returns the callback leg, expected loan output, and gross loan profit (expectedLoanOut - repaid). ok=false
// when the position cannot liquidate profitably here. Bundle and gas economics are applied later by bundle
// selection and operationData.
func sizeLeg(c Candidate, price *big.Int, q AdapterQuote, accrued *big.Int, sp SizingParams) (sizedLeg, bool) {
	m, p := c.Market.State, c.Position
	if price == nil || price.Sign() <= 0 {
		return sizedLeg{}, false
	}
	if q.MaxRate == nil || q.MaxRate.Sign() <= 0 {
		return sizedLeg{}, false // can't price the exit
	}
	if !morpho.IsLiquidatableAt(p, price, m.Lltv, accrued, m.TotalBorrowShares) {
		return sizedLeg{}, false
	}
	target := targetSeize(p.Collateral, sp.AllowFullLiquidation)
	if target.Sign() <= 0 {
		return sizedLeg{}, false
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
	// Clamp the seize by cached adapter redemption liquidity. This is a bidding-time safety check; the
	// callback reads the current getMaxAssets again before swapping. nil/0 ⇒ uncapped (unknown liquidity).
	if q.MaxAssets != nil && q.MaxAssets.Sign() > 0 {
		if fit := collForBudget(q.MaxAssets, q); fit.Cmp(target) < 0 {
			target = fit
		}
	}
	if target.Sign() <= 0 {
		return sizedLeg{}, false
	}
	expectedLoanOut := expectedLoanOutFor(target, q, sp.SwapHaircutBps)
	if expectedLoanOut.Sign() <= 0 {
		return sizedLeg{}, false
	}
	repaid := morpho.RepaidAssetsForSeizeAt(target, price, lif, accrued, m.TotalBorrowShares)
	if expectedLoanOut.Cmp(repaid) <= 0 {
		return sizedLeg{}, false // proceeds can't cover repayment after discount + haircut
	}
	profit := new(big.Int).Sub(expectedLoanOut, repaid) // > 0 here
	leg := selectedLeg{
		MarketId:       c.MarketID,
		Borrower:       c.Borrower,
		MaxSeizeAssets: target,
	}
	return sizedLeg{leg: leg, settlementLoanOut: settlementLoanOutFor(target, q), profit: profit}, true
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
