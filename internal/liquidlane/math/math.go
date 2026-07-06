// Package math contains LiquidLane fixed-point rate calculations.
package math

import "math/big"

var rateScale = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

func pow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

func AmountOutForRate(amountIn, rate *big.Int, tokenInDec, assetDec int) *big.Int {
	num := new(big.Int).Mul(amountIn, rate)
	num.Mul(num, pow10(assetDec))
	den := new(big.Int).Mul(rateScale, pow10(tokenInDec))
	if den.Sign() == 0 {
		return new(big.Int)
	}
	return num.Div(num, den)
}

func MaxAmountInForRate(maxAssets, rate *big.Int, tokenInDec, assetDec int) *big.Int {
	den := new(big.Int).Mul(rate, pow10(assetDec))
	if den.Sign() == 0 {
		return new(big.Int)
	}
	num := new(big.Int).Mul(maxAssets, rateScale)
	num.Mul(num, pow10(tokenInDec))
	return num.Div(num, den)
}

func MinAmountInForAmountOut(amountOut, rate *big.Int, tokenInDec, assetDec int) *big.Int {
	den := new(big.Int).Mul(rate, pow10(assetDec))
	if den.Sign() == 0 {
		return new(big.Int)
	}
	num := new(big.Int).Mul(amountOut, rateScale)
	num.Mul(num, pow10(tokenInDec))
	num.Add(num, new(big.Int).Sub(den, big.NewInt(1)))
	return num.Div(num, den)
}

func RateForAmountOut(amountOut, amountIn *big.Int, tokenInDec, assetDec int) *big.Int {
	if amountIn.Sign() == 0 {
		return new(big.Int)
	}
	num := new(big.Int).Mul(amountOut, rateScale)
	num.Mul(num, pow10(tokenInDec))
	den := new(big.Int).Mul(amountIn, pow10(assetDec))
	return num.Div(num, den)
}
