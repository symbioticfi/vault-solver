package bridgefacilitator

import (
	"math"
	"math/big"

	"github.com/go-errors/errors"
)

// PpmPerBps scales a basis-point rate to parts per million (1 bps = 100 ppm).
const PpmPerBps = 100.0

// yieldPpmScale is YIELD_PRECISION on-chain: yield is expectedReturn * 1e6 / principal (ppm).
const yieldPpmScale = 1_000_000

// zeroMinAssetsSafetyUnits is the protocol compatibility floor used when minAssetsPerRequest is
// disabled. It preserves the historical 3F offer margin exercised by the integration harness without
// turning a zero configured minimum into a one-base-unit request that makes ordinary auctions unquotable.
const zeroMinAssetsSafetyUnits = yieldPpmScale

// ValidateYield checks the full offer and every partial consumption the adapter permits. Request.consume
// floors expectedReturn*consumed/principal before ThreeFAdapter applies its ppm floor, so validating only
// the signed totals is insufficient.
func ValidateYield(expectedReturn, principal, minPrincipal, minYieldPpm *big.Int, maxRateBps float64) error {
	if principal == nil || principal.Sign() <= 0 || expectedReturn == nil || expectedReturn.Sign() <= 0 {
		return errors.Errorf("invalid offer amounts (must be positive): principal=%v expectedReturn=%v", principal, expectedReturn)
	}
	if minPrincipal != nil && minPrincipal.Sign() > 0 && principal.Cmp(minPrincipal) < 0 {
		return errors.Errorf("principal %s below minAssetsPerRequest %s", principal, minPrincipal)
	}
	if !MeetsMinYield(expectedReturn, principal, minYieldPpm) {
		return errors.Errorf("yield below minYieldPerRequest floor %s ppm", minYieldPpm)
	}
	// maxRate has tenths-of-a-bps precision, so maxRateBps*100 is a whole ppm value; round off float noise.
	if maxRatePpm := int64(math.Round(maxRateBps * PpmPerBps)); maxRatePpm > 0 {
		scaled := new(big.Int).Mul(expectedReturn, ppmScale())
		if scaled.Cmp(new(big.Int).Mul(principal, big.NewInt(maxRatePpm))) > 0 {
			return errors.Errorf("yield above auction maxRate %g bps", maxRateBps)
		}
	}
	if minYieldPpm == nil || minYieldPpm.Sign() <= 0 {
		return nil
	}
	margin := new(big.Int).Sub(expectedReturn, MinYieldReturn(principal, minYieldPpm))
	requiredMargin := partialSafetyMargin(principal, minPrincipal)
	if margin.Cmp(requiredMargin) < 0 {
		return errors.Errorf("yield is unsafe for partial consumption: margin %s below required %s", margin, requiredMargin)
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
	return num.Quo(num, ppmScale())
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
	num.Add(num, big.NewInt(yieldPpmScale-1))
	return num.Quo(num, ppmScale())
}

// MeetsMinYield reports whether expectedReturn on principal clears the adapter's on-chain
// minYieldPerRequest floor (minYieldPpm, parts per million): expectedReturn/principal >= minYieldPpm/1e6.
// The on-chain fill enforces this exactly, so an offer under it settles as FAILED — the check is integer
// to avoid the float/bps rounding that lets a truncated maxRate offer land a hair below the floor.
func MeetsMinYield(expectedReturn, principal, minYieldPpm *big.Int) bool {
	if minYieldPpm == nil || minYieldPpm.Sign() <= 0 {
		return true
	}
	lhs := new(big.Int).Mul(expectedReturn, ppmScale())
	rhs := new(big.Int).Mul(principal, minYieldPpm)
	return lhs.Cmp(rhs) >= 0
}

func ppmScale() *big.Int {
	return big.NewInt(yieldPpmScale)
}

// PartialSafeMinYieldReturn adds enough margin to keep every consumption accepted by
// ThreeFAdapter.minAssetsPerRequest above the yield floor. A zero configured minimum uses the protocol
// compatibility margin max(2, ceil(principal/1e6)) exercised by the 3F integration contract.
func PartialSafeMinYieldReturn(principal, minPrincipal, minYieldPpm *big.Int) *big.Int {
	ret := MinYieldReturn(principal, minYieldPpm)
	if ret.Sign() <= 0 {
		return ret
	}
	return ret.Add(ret, partialSafetyMargin(principal, minPrincipal))
}

func partialSafetyMargin(principal, minPrincipal *big.Int) *big.Int {
	if minPrincipal != nil && minPrincipal.Sign() > 0 {
		if minPrincipal.Cmp(principal) >= 0 {
			return new(big.Int)
		}
		return divCeil(principal, minPrincipal)
	}
	margin := divCeil(principal, big.NewInt(zeroMinAssetsSafetyUnits))
	if margin.Cmp(big.NewInt(2)) < 0 {
		return big.NewInt(2)
	}
	return margin
}

func divCeil(numerator, denominator *big.Int) *big.Int {
	result := new(big.Int).Sub(numerator, big.NewInt(1))
	return result.Quo(result, denominator).Add(result, big.NewInt(1))
}
