package defaultstrategy

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

const (
	// RedStone settlement overhead around LiquidLane swaps. The shared liquidlane/gas package only
	// predicts adapter swap route gas; executor/callback/feed overhead and auction gas limits live here.
	executorBaseGasUnits    uint64 = 100_000
	callbackDebitGasUnits   uint64 = 35_000
	priceUpdateGasPerFeed   uint64 = 40_000
	maxSettlementGasUnits   uint64 = 2_000_000
	gasLimitSafetyBps       uint64 = 8_500
	defaultPriceUpdateFeeds        = 1
)

type gasPrediction = liquidlanegas.Prediction

func predictGasForFeeds(demands []liquidlanegas.Demand, state *liquidLaneState, feedCount int) gasPrediction {
	prediction := liquidlanegas.Predict(demands, state)
	prediction.Units = bigmath.SaturatingAdd(prediction.Units, fixedSettlementGasUnits(feedCount))
	return prediction
}

func gasCostNative(units uint64, gasPrice *big.Int) *big.Int {
	return new(big.Int).Mul(new(big.Int).SetUint64(units), bigmath.OrZero(gasPrice))
}

func fixedSettlementGasUnits(feedCount int) uint64 {
	feeds := uint64(max(feedCount, defaultPriceUpdateFeeds))
	return bigmath.SaturatingAdd(executorBaseGasUnits+callbackDebitGasUnits, bigmath.SaturatingMul(feeds, priceUpdateGasPerFeed))
}

func usableGasLimit(headerGasLimit uint64) uint64 {
	limit := maxSettlementGasUnits
	if headerGasLimit != 0 {
		limit = min(limit, headerGasLimit)
	}
	return limit * gasLimitSafetyBps / 10_000 // limit is bounded to 2M, so multiplication cannot overflow.
}
