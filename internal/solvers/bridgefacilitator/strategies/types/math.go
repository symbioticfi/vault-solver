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
	bigCeilBias      = big.NewInt(yieldPpmScale - 1) // for ceil(x/1e6) = (x + 1e6-1) / 1e6
)

// ValidateYield checks that expectedReturn on principal is acceptable to BOTH the adapter's on-chain
// minYieldPerRequest floor and the 3F auction's max rate, in exact integer ppm — so the offer can't
// revert on-chain (below floor) nor be rejected by the auction (above maxRate). Yield is compared the way
// the contract computes it, floor(expectedReturn*1e6/principal), which for integer bounds is equivalent
// to the exact integer comparisons below. A zero/absent floor or maxRate skips that bound; nil or
// non-positive amounts are rejected.
func ValidateYield(expectedReturn, principal, minYieldPpm *big.Int, maxRateBps float64) error {
	// Reject a non-positive return: a 0 yield (what the pricing helpers produce when floor and maxRate are
	// both 0, or on dust principals) is never a real offer, so the pair is skipped rather than offered.
	if principal == nil || principal.Sign() <= 0 || expectedReturn == nil || expectedReturn.Sign() <= 0 {
		return errors.Errorf("invalid offer amounts (must be positive): principal=%v expectedReturn=%v", principal, expectedReturn)
	}
	if !MeetsMinYield(expectedReturn, principal, minYieldPpm) {
		return errors.Errorf("yield below minYieldPerRequest floor %s ppm", minYieldPpm)
	}
	// maxRate has tenths-of-a-bps precision, so maxRateBps*100 is a whole ppm value; round off float noise.
	if maxRatePpm := int64(math.Round(maxRateBps * PpmPerBps)); maxRatePpm > 0 {
		scaled := new(big.Int).Mul(expectedReturn, bigYieldPpmScale)
		if scaled.Cmp(new(big.Int).Mul(principal, big.NewInt(maxRatePpm))) > 0 {
			return errors.Errorf("yield above auction maxRate %g bps", maxRateBps)
		}
	}
	return nil
}

// ExpectedReturn is the return for principal at rateBps, truncated down. maxRate has tenths-of-a-bps
// precision so rateBps*100 is whole ppm; the math is exact integer floor(principal*ppm/1e6) — a big.Float
// path drifts by 1 wei for principals above ~2^64.
func ExpectedReturn(principal *big.Int, rateBps float64) *big.Int {
	ratePpm := int64(math.Round(rateBps * PpmPerBps))
	if principal == nil || principal.Sign() <= 0 || ratePpm <= 0 {
		return new(big.Int)
	}
	num := new(big.Int).Mul(principal, big.NewInt(ratePpm))
	return num.Quo(num, bigYieldPpmScale)
}

// MinYieldReturn is the smallest expectedReturn on principal that clears the adapter's
// minYieldPerRequest floor: ceil(principal * minYieldPpm / 1e6). Pricing an offer here quotes the most
// competitive rate the adapter allows, rounded up so the realised yield is never a hair below the floor
// (which would revert the fill). Returns 0 when there is no floor (minYieldPpm <= 0).
func MinYieldReturn(principal, minYieldPpm *big.Int) *big.Int {
	if principal == nil || principal.Sign() <= 0 || minYieldPpm == nil || minYieldPpm.Sign() <= 0 {
		return new(big.Int)
	}
	num := new(big.Int).Mul(principal, minYieldPpm)
	num.Add(num, bigCeilBias)
	return num.Quo(num, bigYieldPpmScale)
}

// MeetsMinYield reports whether expectedReturn on principal clears the adapter's on-chain
// minYieldPerRequest floor (minYieldPpm, parts per million): expectedReturn/principal >= minYieldPpm/1e6.
// The on-chain fill enforces this exactly, so an offer under it settles as FAILED — the check is integer
// to avoid the float/bps rounding that lets a truncated maxRate offer land a hair below the floor.
func MeetsMinYield(expectedReturn, principal, minYieldPpm *big.Int) bool {
	if minYieldPpm == nil || minYieldPpm.Sign() <= 0 {
		return true
	}
	lhs := new(big.Int).Mul(expectedReturn, bigYieldPpmScale)
	rhs := new(big.Int).Mul(principal, minYieldPpm)
	return lhs.Cmp(rhs) >= 0
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
	ret := MinYieldReturn(principal, minYieldPpm)
	if ret.Sign() <= 0 {
		return ret
	}
	margin := new(big.Int).Add(principal, bigCeilBias)
	margin.Quo(margin, bigYieldPpmScale)
	if margin.Cmp(bigTwo) < 0 {
		margin.Set(bigTwo)
	}
	return ret.Add(ret, margin)
}
