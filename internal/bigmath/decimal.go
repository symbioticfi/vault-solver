package bigmath

import (
	"math/big"
	"strings"
)

// Decimal inserts a decimal point into an integer amount, without powers,
// division, exponent notation or trailing fractional zeroes.
func Decimal(amount *big.Int, scale int) string {
	digits := amount.String()
	if scale <= 0 || amount.Sign() == 0 {
		return digits
	}
	sign := ""
	if digits[0] == '-' {
		sign, digits = "-", digits[1:]
	}
	if len(digits) <= scale {
		digits = strings.Repeat("0", scale-len(digits)+1) + digits
	}
	split := len(digits) - scale
	fraction := strings.TrimRight(digits[split:], "0")
	if fraction == "" {
		return sign + digits[:split]
	}
	return sign + digits[:split] + "." + fraction
}
