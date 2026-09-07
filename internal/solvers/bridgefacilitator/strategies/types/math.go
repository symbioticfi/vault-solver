package types

import (
	"math"
	"math/big"

	"github.com/go-errors/errors"
)

// PpmPerBps scales a basis-point rate to parts per million (1 bps = 100 ppm).
const PpmPerBps = 100.0

// yieldPpmScale is YIELD_PRECISION on-chain: yield is expectedReturn * 1e6 / principal (ppm).
const yieldPpmScale = 1_000_000

// Read-only big.Int constants for the yield math, hoisted out of the per-offer hot path.
var (
	bigYieldPpmScale = big.NewInt(yieldPpmScale)
)

// ValidateYield checks that expectedReturn on principal is acceptable to BOTH the adapter's on-chain
// minYieldPerRequest floor and the 3F auction's max rate, in exact integer ppm — so the offer can't
// revert on-chain (below floor) nor be rejected by the auction (above maxRate). Yield is compared the way
// the contract computes it, floor(expectedReturn*1e6/principal), which for integer bounds is equivalent
// to the exact integer comparisons below. A zero/absent floor or maxRate skips that bound; nil or
// non-positive amounts are rejected.
func ValidateYield(expectedReturn, principal, minYieldPpm *big.Int, maxRateBps float64) error {
	if expectedReturn == nil || principal == nil || expectedReturn.Sign() <= 0 || principal.Sign() <= 0 {
		return errors.Errorf("invalid offer amounts (must be positive): principal=%v expectedReturn=%v", principal, expectedReturn)
	}
	if !MeetsMinYield(expectedReturn, principal, minYieldPpm) {
		return errors.Errorf("yield below minYieldPerRequest floor %s ppm", minYieldPpm)
	}
	// maxRate carries tenths of a basis point: rounding removes API float noise before integer math.
	if math.Round(maxRateBps*PpmPerBps) > 0 && expectedReturn.Cmp(ExpectedReturn(principal, maxRateBps)) > 0 {
		return errors.Errorf("yield above auction maxRate %g bps", maxRateBps)
	}
	return nil
}

// ExpectedReturn is the return for principal at rateBps, truncated down. maxRate has tenths-of-a-bps
// precision so rateBps*100 is whole ppm; the math is exact integer floor(principal*ppm/1e6) — a big.Float
// path drifts by 1 wei for principals above ~2^64.
func ExpectedReturn(principal *big.Int, rateBps float64) *big.Int {
	return yieldAmount(principal, big.NewInt(int64(math.Round(rateBps*PpmPerBps))), false)
}

// MinYieldReturn is the smallest expectedReturn on principal that clears the adapter's
// minYieldPerRequest floor: ceil(principal * minYieldPpm / 1e6). Pricing an offer here quotes the most
// competitive rate the adapter allows, rounded up so the realised yield is never a hair below the floor
// (which would revert the fill). Returns 0 when there is no floor (minYieldPpm <= 0).
func MinYieldReturn(principal, minYieldPpm *big.Int) *big.Int {
	return yieldAmount(principal, minYieldPpm, true)
}

func yieldAmount(principal, ppm *big.Int, ceil bool) *big.Int {
	result := new(big.Int)
	if principal == nil || ppm == nil || principal.Sign() <= 0 || ppm.Sign() <= 0 {
		return result
	}
	remainder := new(big.Int)
	result.QuoRem(result.Mul(principal, ppm), bigYieldPpmScale, remainder)
	if ceil && remainder.Sign() != 0 {
		result.Add(result, big.NewInt(1))
	}
	return result
}

// MeetsMinYield reports whether expectedReturn on principal clears the adapter's on-chain
// minYieldPerRequest floor (minYieldPpm, parts per million): expectedReturn/principal >= minYieldPpm/1e6.
// The on-chain fill enforces this exactly, so an offer under it settles as FAILED — the check is integer
// to avoid the float/bps rounding that lets a truncated maxRate offer land a hair below the floor.
func MeetsMinYield(expectedReturn, principal, minYieldPpm *big.Int) bool {
	if minYieldPpm == nil || minYieldPpm.Sign() <= 0 {
		return true
	}
	return expectedReturn != nil && principal != nil && expectedReturn.Cmp(MinYieldReturn(principal, minYieldPpm)) >= 0
}

// bigTwo is the minimum partial-consumption pricing margin (see PartialSafeMinYieldReturn).
var bigTwo = big.NewInt(2)

// PartialSafeMinYieldReturn prices an offer above MinYieldReturn by a margin that keeps PARTIAL
// consumptions clear of the floor. The Request contract pro-rates a partially consumed offer's return
// with floor division — yt = expectedReturn*pt/principal — and requires ceil(pt*minYieldPpm/1e6), so an
// offer priced exactly at MinYieldReturn carries under one base unit of slack and most partial amounts
// truncate one unit below the floor, reverting TooLowYield. (Mainnet tx 0xc637…8386: 30,000.035000 USDC
// offered at the 190 ppm floor was consumed at 29,946.365238 and delivered 5,689,809 against the
// required 5,689,810.)
//
// The margin is max(2, ceil(principal/1e6)) — one ppm of principal, floored at two base units. Since
// margin*pt >= principal implies the pro-rated return exceeds the requirement by at least a full unit,
// this guarantees floor(return*pt/principal) >= ceil(pt*minYieldPpm/1e6) for every pt >=
// principal/margin: every consumption of at least half the offer and, for principals above 2e6 base
// units, everything down to the ppm quantum (1e6 base units) — for any floor ppm and any token scale.
// The rate cost is ~1 ppm (two base units on dust principals). Returns 0 when there is no floor, like
// MinYieldReturn.
func PartialSafeMinYieldReturn(principal, minYieldPpm *big.Int) *big.Int {
	result := MinYieldReturn(principal, minYieldPpm)
	if result.Sign() == 0 {
		return result
	}
	margin := yieldAmount(principal, big.NewInt(1), true)
	if margin.Cmp(bigTwo) < 0 {
		margin.Set(bigTwo)
	}
	return result.Add(result, margin)
}
