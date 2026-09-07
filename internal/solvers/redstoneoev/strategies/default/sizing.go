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
	leg             selectedLeg
	expectedLoanOut *big.Int
	profit          *big.Int
}

// expectedLoanOutFor estimates the loan-token output for selling `collIn` of seized collateral through
// quote q at the adapter's discounted rate minus the extra safety haircut:
// collIn × maxRate × 10^loanDec / (1e18 × 10^collDec), then × (1 − haircut).
// These scales are precomputed from the verified adapter snapshot.
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

// collForBudget is the inverse of expectedLoanOutFor: the most collateral whose expected loan output stays
// within `budget` (loan-token / getMaxAssets units), so the selected leg does not rely on more redemption
// liquidity than the cached adapter state says exists. The callback still reads the live cap at settlement.
// Returns 0 when the quote can't price an exit. SwapHaircutBps is config-validated to [0, 10000), so
// 10000−haircut > 0.
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

// sizeLeg caps a liquidation by collateral, full debt and known adapter capacity.
// Candidate.Market.State is already accrued or replayed to the decision point.
// The repayment and exit are then priced at that exact seizure; gas belongs to the bundle.
func sizeLeg(c Candidate, price *big.Int, q AdapterQuote, sp SizingParams) (sizedLeg, bool) {
	market, position := c.Market.State, c.Position
	accrued := market.TotalBorrowAssets
	for _, amount := range []*big.Int{price, q.MaxRate, q.LoanScale, q.CollScale, position.Collateral, position.BorrowShares} {
		if amount == nil || amount.Sign() <= 0 {
			return sizedLeg{}, false
		}
	}
	if accrued == nil || accrued.Sign() < 0 || market.TotalBorrowShares == nil || market.TotalBorrowShares.Sign() < 0 ||
		market.Lltv == nil || market.Lltv.Sign() < 0 || market.Lltv.Cmp(morpho.Wad) > 0 ||
		sp.SwapHaircutBps < 0 || sp.SwapHaircutBps >= 10_000 {
		return sizedLeg{}, false
	}
	if !morpho.IsLiquidatableAt(position, price, market.Lltv, accrued, market.TotalBorrowShares) {
		return sizedLeg{}, false
	}
	incentive := morpho.LiquidationIncentiveFactor(market.Lltv)
	seize := targetSeize(position.Collateral, sp.AllowFullLiquidation)
	debtCap := morpho.MaxSeizeForFullDebt(position.BorrowShares, price, incentive, accrued, market.TotalBorrowShares)
	// The full-debt inverse rounds down. A zero dust cap is a rejection, never an absent limit.
	if debtCap.Cmp(seize) < 0 {
		seize = debtCap
	}
	// nil/zero remains the explicitly unbounded quote form used by local sizing.
	// The production candidate join admits only positive physical capacity.
	if q.MaxAssets != nil && q.MaxAssets.Sign() > 0 {
		capacityCap := collForBudget(q.MaxAssets, q, sp.SwapHaircutBps)
		if capacityCap.Cmp(seize) < 0 {
			seize = capacityCap
		}
	}
	if seize.Sign() <= 0 {
		return sizedLeg{}, false
	}
	proceeds := expectedLoanOutFor(seize, q, sp.SwapHaircutBps)
	repayment := morpho.RepaidAssetsForSeizeAt(seize, price, incentive, accrued, market.TotalBorrowShares)
	profit := new(big.Int).Sub(proceeds, repayment)
	if profit.Sign() <= 0 {
		return sizedLeg{}, false
	}
	return sizedLeg{
		leg:             selectedLeg{MarketId: c.MarketID, Borrower: c.Borrower, MaxSeizeAssets: seize},
		expectedLoanOut: proceeds, profit: profit,
	}, true
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
