package defaultstrategy

import (
	"math/big"
	"strings"
)

func applyBpsDown(amount *big.Int, bps int) *big.Int {
	if amount == nil || amount.Sign() <= 0 || bps <= 0 {
		return new(big.Int)
	}
	out := new(big.Int).Mul(amount, big.NewInt(int64(bps)))
	return out.Div(out, big.NewInt(bpsDenominator))
}

func applyBpsUp(amount *big.Int, bps int) *big.Int {
	if amount == nil || amount.Sign() <= 0 || bps <= 0 {
		return new(big.Int)
	}
	return mulDivUp(amount, big.NewInt(int64(bps)), big.NewInt(bpsDenominator))
}

func mulDivUp(a, b, denominator *big.Int) *big.Int {
	if a == nil || b == nil || denominator == nil || a.Sign() <= 0 || b.Sign() <= 0 || denominator.Sign() <= 0 {
		return new(big.Int)
	}
	numerator := new(big.Int).Mul(a, b)
	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	if remainder.Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}

func minBig(left, right *big.Int) *big.Int {
	if left.Cmp(right) < 0 {
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
