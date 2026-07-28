package defaultstrategy

import (
	"math/big"
	"strings"
)

func minBig(left, right *big.Int) *big.Int {
	if left.Cmp(right) <= 0 {
		return new(big.Int).Set(left)
	}
	return new(big.Int).Set(right)
}

func fixedPointDecimal(n *big.Int, scale int) string {
	if n.Sign() == 0 {
		return "0"
	}
	unit := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	intPart := new(big.Int).Div(new(big.Int).Set(n), unit)
	fracPart := new(big.Int).Mod(new(big.Int).Set(n), unit)
	if fracPart.Sign() == 0 {
		return intPart.String()
	}
	frac := fracPart.String()
	if len(frac) < scale {
		frac = strings.Repeat("0", scale-len(frac)) + frac
	}
	frac = strings.TrimRight(frac, "0")
	return intPart.String() + "." + frac
}
