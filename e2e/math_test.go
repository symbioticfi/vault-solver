package e2e

import (
	"math/big"
	"strings"
	"testing"
)

const (
	bpsScale                = int64(10_000)
	ppmScale                = int64(1_000_000)
	virtualAssets           = int64(1)
	virtualShares           = int64(1_000_000)
	liquidationCursor       = int64(300_000_000_000_000_000)
	maxLiquidationIncentive = int64(1_150_000_000_000_000_000)
)

type rangeQuoteInput struct {
	minimumInput   *big.Int
	maximumInput   *big.Int
	candidateRate  *big.Int
	inputDecimals  uint8
	outputDecimals uint8
	priceBufferBPS int64
}

type rangeQuote struct {
	rate      *big.Int
	amountOut *big.Int
}

func mulDivDown(left, right, denominator *big.Int) *big.Int {
	return new(big.Int).Div(new(big.Int).Mul(left, right), denominator)
}

func mulDivUp(left, right, denominator *big.Int) *big.Int {
	product := new(big.Int).Mul(left, right)
	product.Add(product, new(big.Int).Sub(denominator, big.NewInt(1)))
	return product.Div(product, denominator)
}

func amountOutForRate(amountIn, rate *big.Int, inputDecimals, outputDecimals uint8) *big.Int {
	numerator := new(big.Int).Mul(amountIn, rate)
	numerator.Mul(numerator, pow10(outputDecimals))
	denominator := new(big.Int).Mul(pow10(18), pow10(inputDecimals))
	return numerator.Div(numerator, denominator)
}

func rateForAmountOut(amountOut, amountIn *big.Int, inputDecimals, outputDecimals uint8) *big.Int {
	numerator := new(big.Int).Mul(amountOut, pow10(18))
	numerator.Mul(numerator, pow10(inputDecimals))
	denominator := new(big.Int).Mul(amountIn, pow10(outputDecimals))
	return numerator.Div(numerator, denominator)
}

func conservativeAdvertisedAmountOut(amountIn, advertisedRate *big.Int, inputDecimals, outputDecimals uint8) *big.Int {
	advertisedAmountOut := amountOutForRate(amountIn, advertisedRate, inputDecimals, outputDecimals)
	if advertisedAmountOut.Cmp(big.NewInt(1)) <= 0 {
		return new(big.Int)
	}
	safeAmount := new(big.Int).Sub(advertisedAmountOut, big.NewInt(1))
	safeRate := rateForAmountOut(safeAmount, amountIn, inputDecimals, outputDecimals)
	return amountOutForRate(amountIn, safeRate, inputDecimals, outputDecimals)
}

func discountedAmountOut(grossAmountOut, discountPPM *big.Int) *big.Int {
	factor := new(big.Int).Sub(big.NewInt(ppmScale), discountPPM)
	return mulDivDown(grossAmountOut, factor, big.NewInt(ppmScale))
}

func advertisedDiscountRate(grossUnitAmountOut, discountPPM *big.Int, outputDecimals uint8) *big.Int {
	return mulDivDown(discountedAmountOut(grossUnitAmountOut, discountPPM), pow10(18), pow10(outputDecimals))
}

func quoteBufferedAmount(amount *big.Int, priceBufferBPS int64) *big.Int {
	factor := big.NewInt(bpsScale - 2*priceBufferBPS)
	return mulDivDown(amount, factor, big.NewInt(bpsScale))
}

func maximumNonOverquotingRate(amountOut, amountIn *big.Int, inputDecimals, outputDecimals uint8) *big.Int {
	rate := rateForAmountOut(new(big.Int).Add(amountOut, big.NewInt(1)), amountIn, inputDecimals, outputDecimals)
	if amountOutForRate(amountIn, rate, inputDecimals, outputDecimals).Cmp(amountOut) > 0 {
		rate.Sub(rate, big.NewInt(1))
	}
	return rate
}

func singleRouteRangeQuote(input rangeQuoteInput) rangeQuote {
	lowerOutput := quoteBufferedAmount(
		amountOutForRate(input.minimumInput, input.candidateRate, input.inputDecimals, input.outputDecimals),
		input.priceBufferBPS,
	)
	upperOutput := quoteBufferedAmount(
		amountOutForRate(input.maximumInput, input.candidateRate, input.inputDecimals, input.outputDecimals),
		input.priceBufferBPS,
	)
	lowerRate := maximumNonOverquotingRate(
		lowerOutput,
		input.minimumInput,
		input.inputDecimals,
		input.outputDecimals,
	)
	upperRate := maximumNonOverquotingRate(
		upperOutput,
		input.maximumInput,
		input.inputDecimals,
		input.outputDecimals,
	)
	floorRate := mulDivDown(
		input.candidateRate,
		big.NewInt(bpsScale-2*input.priceBufferBPS),
		big.NewInt(bpsScale),
	)
	floorRate.Sub(floorRate, rateForAmountOut(big.NewInt(1), input.minimumInput, input.inputDecimals, input.outputDecimals))
	floorRate.Sub(floorRate, big.NewInt(1))
	if floorRate.Sign() < 0 {
		floorRate.SetInt64(0)
	}
	rate := minBigInt(lowerRate, minBigInt(upperRate, floorRate))
	return rangeQuote{
		rate:      rate,
		amountOut: amountOutForRate(input.maximumInput, rate, input.inputDecimals, input.outputDecimals),
	}
}

func fillBufferedAmount(amount *big.Int, priceBufferBPS int64) *big.Int {
	buffer := mulDivUp(amount, big.NewInt(priceBufferBPS), big.NewInt(bpsScale))
	return new(big.Int).Sub(amount, buffer)
}

func minYieldReturn(principal, minimumYieldPPM *big.Int) *big.Int {
	return mulDivUp(principal, minimumYieldPPM, big.NewInt(ppmScale))
}

func partialSafeMinYieldReturn(principal, minimumYieldPPM *big.Int) *big.Int {
	minimum := minYieldReturn(principal, minimumYieldPPM)
	if minimum.Sign() == 0 {
		return new(big.Int)
	}
	margin := maxBigInt(big.NewInt(2), mulDivUp(principal, big.NewInt(1), big.NewInt(ppmScale)))
	return new(big.Int).Add(minimum, margin)
}

func proratedYield(expectedReturn, consumedPrincipal, signedPrincipal *big.Int) *big.Int {
	return mulDivDown(expectedReturn, consumedPrincipal, signedPrincipal)
}

func requiredYield(consumedPrincipal, minimumYieldPPM *big.Int) *big.Int {
	return mulDivUp(consumedPrincipal, minimumYieldPPM, big.NewInt(ppmScale))
}

func parseFixed(value string, decimals uint8) (*big.Int, bool) {
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" || (len(parts) == 2 && len(parts[1]) > int(decimals)) {
		return nil, false
	}
	whole, ok := new(big.Int).SetString(parts[0], 10)
	if !ok || whole.Sign() < 0 {
		return nil, false
	}
	whole.Mul(whole, pow10(decimals))
	if len(parts) == 1 {
		return whole, true
	}
	fractionText := parts[1] + strings.Repeat("0", int(decimals)-len(parts[1]))
	if fractionText == "" {
		return whole, true
	}
	fraction, ok := new(big.Int).SetString(fractionText, 10)
	if !ok {
		return nil, false
	}
	return whole.Add(whole, fraction), true
}

func liquidationIncentiveFactor(lltv *big.Int) *big.Int {
	rateScale := pow10(18)
	difference := new(big.Int).Sub(rateScale, lltv)
	cursor := big.NewInt(liquidationCursor)
	denominator := new(big.Int).Sub(rateScale, mulDivDown(cursor, difference, rateScale))
	incentive := mulDivDown(rateScale, rateScale, denominator)
	return minBigInt(incentive, big.NewInt(maxLiquidationIncentive))
}

func maxSeizeForFullDebt(borrowShares, collateralPrice, lltv, totalBorrowAssets, totalBorrowShares *big.Int) *big.Int {
	debtAssets := mulDivDown(
		borrowShares,
		new(big.Int).Add(totalBorrowAssets, big.NewInt(virtualAssets)),
		new(big.Int).Add(totalBorrowShares, big.NewInt(virtualShares)),
	)
	incentivizedDebt := mulDivDown(debtAssets, liquidationIncentiveFactor(lltv), pow10(18))
	return mulDivDown(incentivizedDebt, pow10(36), collateralPrice)
}

func repaidAssetsForSeize(seizedAssets, collateralPrice, lltv, totalBorrowAssets, totalBorrowShares *big.Int) *big.Int {
	seizedQuoted := mulDivUp(seizedAssets, collateralPrice, pow10(36))
	debtAssets := mulDivUp(seizedQuoted, pow10(18), liquidationIncentiveFactor(lltv))
	repaidShares := mulDivUp(
		debtAssets,
		new(big.Int).Add(totalBorrowShares, big.NewInt(virtualShares)),
		new(big.Int).Add(totalBorrowAssets, big.NewInt(virtualAssets)),
	)
	return mulDivUp(
		repaidShares,
		new(big.Int).Add(totalBorrowAssets, big.NewInt(virtualAssets)),
		new(big.Int).Add(totalBorrowShares, big.NewInt(virtualShares)),
	)
}

func collateralForBudget(budget, maxRate *big.Int, collateralDecimals, loanDecimals uint8, haircutBPS int64) *big.Int {
	numerator := new(big.Int).Mul(budget, pow10(18))
	numerator.Mul(numerator, pow10(collateralDecimals))
	numerator.Mul(numerator, big.NewInt(bpsScale))
	denominator := new(big.Int).Mul(maxRate, pow10(loanDecimals))
	denominator.Mul(denominator, big.NewInt(bpsScale-haircutBPS))
	return numerator.Div(numerator, denominator)
}

func pow10(decimals uint8) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
}

func minBigInt(left, right *big.Int) *big.Int {
	if left.Cmp(right) < 0 {
		return new(big.Int).Set(left)
	}
	return new(big.Int).Set(right)
}

func maxBigInt(left, right *big.Int) *big.Int {
	if left.Cmp(right) > 0 {
		return new(big.Int).Set(left)
	}
	return new(big.Int).Set(right)
}

func TestIndependentSolverArithmetic(t *testing.T) {
	e18 := pow10(18)
	t.Run("liquidlane", func(t *testing.T) {
		gross := amountOutForRate(new(big.Int).Mul(big.NewInt(23), e18), bigFromDecimal(t, "1090540000000000000000"), 18, 18)
		executable := discountedAmountOut(gross, big.NewInt(50_000))
		assertBigEqual(t, gross, "25082420000000000000000")
		assertBigEqual(t, executable, "23828299000000000000000")
		assertBigEqual(t, quoteBufferedAmount(executable, 20), "23732985804000000000000")
		assertBigEqual(t, fillBufferedAmount(executable, 20), "23780642402000000000000")
		assertBigEqual(t, fillBufferedAmount(big.NewInt(1_000), 25), "997")
		advertisedRate := advertisedDiscountRate(bigFromDecimal(t, "1090540000000000000000"), big.NewInt(50_000), 18)
		assertBigEqual(
			t,
			conservativeAdvertisedAmountOut(new(big.Int).Mul(big.NewInt(23), e18), advertisedRate, 18, 18),
			"23828298999999999999977",
		)
		quote := singleRouteRangeQuote(rangeQuoteInput{
			minimumInput:   e18,
			maximumInput:   new(big.Int).Mul(big.NewInt(2), e18),
			candidateRate:  e18,
			inputDecimals:  18,
			outputDecimals: 18,
			priceBufferBPS: 20,
		})
		assertBigEqual(t, quote.rate, "995999999999999998")
		assertBigEqual(t, quote.amountOut, "1991999999999999996")
	})

	t.Run("threef", func(t *testing.T) {
		principal := new(big.Int).Mul(big.NewInt(50_000), e18)
		minimumYieldPPM := big.NewInt(5_000)
		expectedReturn := partialSafeMinYieldReturn(principal, minimumYieldPPM)
		consumed := new(big.Int).Div(principal, big.NewInt(2))
		assertBigEqual(t, minYieldReturn(principal, minimumYieldPPM), "250000000000000000000")
		assertBigEqual(t, expectedReturn, "250050000000000000000")
		assertBigEqual(t, proratedYield(expectedReturn, consumed, principal), "125025000000000000000")
		assertBigEqual(t, requiredYield(consumed, minimumYieldPPM), "125000000000000000000")
	})

	t.Run("morpho", func(t *testing.T) {
		lltv := bigFromDecimal(t, "860000000000000000")
		assertBigEqual(t, liquidationIncentiveFactor(lltv), "1043841336116910229")
		borrowShares := new(big.Int).Mul(big.NewInt(100), e18)
		total := new(big.Int).Mul(big.NewInt(1_000), e18)
		price := new(big.Int).Mul(big.NewInt(1_050), pow10(36))
		seized := maxSeizeForFullDebt(borrowShares, price, lltv, total, total)
		assertBigEqual(t, seized, "99413460582562779")
		assertBigEqual(t, repaidAssetsForSeize(seized, price, lltv, total, total), "99999999999999899459")
		budget := new(big.Int).Mul(big.NewInt(1_000), e18)
		assertBigEqual(t, collateralForBudget(budget, e18, 18, 18, 100), "1010101010101010101010")
	})

	t.Run("fixed", func(t *testing.T) {
		parsed, ok := parseFixed("0.0005", 18)
		if !ok {
			t.Fatal("parseFixed rejected valid value")
		}
		assertBigEqual(t, parsed, "500000000000000")
		if _, valid := parseFixed("0.0000001", 6); valid {
			t.Fatal("parseFixed accepted excessive precision")
		}
	})
}

func bigFromDecimal(t *testing.T, value string) *big.Int {
	t.Helper()
	parsed, ok := new(big.Int).SetString(value, 10)
	if !ok {
		t.Fatalf("invalid test integer %q", value)
	}
	return parsed
}

func assertBigEqual(t *testing.T, got *big.Int, want string) {
	t.Helper()
	if got.String() != want {
		t.Fatalf("value = %s, want %s", got, want)
	}
}
