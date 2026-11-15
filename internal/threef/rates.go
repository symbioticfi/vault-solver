// Package threef contains rate math shared by the native and Merkle 3F solvers.
package threef

import (
	"math/big"

	"github.com/go-errors/errors"
)

const YearSeconds int64 = 31_536_000

// ReturnBounds converts the auction APR to its expectedReturn interval. The API
// requires at least a quarter of the lowest ramp APR, but exposes only the current
// maximum. A quarter of that maximum is a conservative floor, possibly higher than
// the actual minimum. Round the floor up and ceiling down to stay inside both bounds.
// An omitted (zero) basis retains the API's unscaled, per-term representation.
func ReturnBounds(principal *big.Int, maxAPR *big.Rat, basisSeconds int64) (minimum, maximum *big.Int, err error) {
	if principal == nil || principal.Sign() <= 0 || maxAPR == nil || maxAPR.Sign() <= 0 || basisSeconds < 0 {
		return nil, nil, errors.New("invalid 3F principal, auction maxRate or rateBasisSeconds")
	}
	if basisSeconds == 0 {
		basisSeconds = YearSeconds
	}
	numerator := new(big.Int).Mul(principal, maxAPR.Num())
	numerator.Mul(numerator, big.NewInt(basisSeconds))
	denominator := new(big.Int).Mul(maxAPR.Denom(), big.NewInt(10_000*YearSeconds))
	maximum = new(big.Int).Quo(numerator, denominator)
	denominator.Mul(denominator, big.NewInt(4))
	minimum, remainder := new(big.Int), new(big.Int)
	minimum.QuoRem(numerator, denominator, remainder)
	if remainder.Sign() != 0 {
		minimum.Add(minimum, big.NewInt(1))
	}
	return minimum, maximum, nil
}
