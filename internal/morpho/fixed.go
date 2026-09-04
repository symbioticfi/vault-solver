package morpho

import "math/big"

var (
	oraclePriceScale = pow10(36)
	Wad              = big.NewInt(1e18)
	twoWad           = big.NewInt(2e18)
	threeWad         = big.NewInt(3e18)
	virtualShares    = big.NewInt(1e6)
	virtualAssets    = big.NewInt(1)
)

func pow10(exponent int64) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(exponent), nil)
}

func ToSharesUp(assets, totalAssets, totalShares *big.Int) *big.Int {
	return MulDivUp(
		assets,
		new(big.Int).Add(totalShares, virtualShares),
		new(big.Int).Add(totalAssets, virtualAssets),
	)
}

func ToAssetsUp(shares, totalAssets, totalShares *big.Int) *big.Int {
	return MulDivUp(
		shares,
		new(big.Int).Add(totalAssets, virtualAssets),
		new(big.Int).Add(totalShares, virtualShares),
	)
}

func ToSharesDown(assets, totalAssets, totalShares *big.Int) *big.Int {
	return MulDivDown(
		assets,
		new(big.Int).Add(totalShares, virtualShares),
		new(big.Int).Add(totalAssets, virtualAssets),
	)
}

func ToAssetsDown(shares, totalAssets, totalShares *big.Int) *big.Int {
	return MulDivDown(
		shares,
		new(big.Int).Add(totalAssets, virtualAssets),
		new(big.Int).Add(totalShares, virtualShares),
	)
}

func WTaylorCompounded(ratePerSec, elapsed *big.Int) *big.Int {
	first := new(big.Int).Mul(ratePerSec, elapsed)
	second := MulDivDown(first, first, twoWad)
	third := MulDivDown(second, first, threeWad)
	return new(big.Int).Add(new(big.Int).Add(first, second), third)
}

func WMulDown(left, right *big.Int) *big.Int {
	return MulDivDown(left, right, Wad)
}

func WDivDown(left, right *big.Int) *big.Int {
	return MulDivDown(left, Wad, right)
}

func WDivUp(left, right *big.Int) *big.Int {
	return MulDivUp(left, Wad, right)
}

func MulDivDown(left, right, denominator *big.Int) *big.Int {
	return new(big.Int).Div(new(big.Int).Mul(left, right), denominator)
}

func MulDivUp(left, right, denominator *big.Int) *big.Int {
	product := new(big.Int).Mul(left, right)
	product.Add(product, new(big.Int).Sub(denominator, big.NewInt(1)))
	return product.Div(product, denominator)
}

func minBig(left, right *big.Int) *big.Int {
	if left.Cmp(right) <= 0 {
		return new(big.Int).Set(left)
	}
	return new(big.Int).Set(right)
}

func zeroFloorSub(left, right *big.Int) *big.Int {
	if left.Cmp(right) <= 0 {
		return new(big.Int)
	}
	return new(big.Int).Sub(left, right)
}
