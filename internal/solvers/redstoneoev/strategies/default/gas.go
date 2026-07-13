package defaultstrategy

import (
	"math/big"

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

type gasPrediction struct {
	Units  uint64
	Routes []liquidlanegas.Route
}

func predictGasForFeeds(legs []legHint, st *liquidLaneState, feedCount int) gasPrediction {
	routeGas := liquidlanegas.Predict(gasDemands(legs), st)
	return gasPrediction{
		Units:  saturatingAddUint64(fixedSettlementGasUnits(feedCount), routeGas.Units),
		Routes: routeGas.Routes,
	}
}

func fitsGasLimit(legs []legHint, st *liquidLaneState, headerGasLimit uint64, feedCount int) bool {
	return predictGasForFeeds(legs, st, feedCount).Units <= usableGasLimit(headerGasLimit)
}

func gasCostNative(units uint64, gasPrice *big.Int) *big.Int {
	return new(big.Int).Mul(new(big.Int).SetUint64(units), orZero(gasPrice))
}

func fixedSettlementGasUnits(feedCount int) uint64 {
	feeds := uint64(defaultPriceUpdateFeeds)
	if feedCount > 0 {
		feeds = uint64(feedCount)
	}
	feedUnits := saturatingMulUint64(priceUpdateGasPerFeed, feeds)
	return saturatingAddUint64(saturatingAddUint64(executorBaseGasUnits, callbackDebitGasUnits), feedUnits)
}

func usableGasLimit(headerGasLimit uint64) uint64 {
	if headerGasLimit == 0 {
		headerGasLimit = maxSettlementGasUnits
	}
	limit := min(headerGasLimit, maxSettlementGasUnits)
	return saturatingMulUint64(limit, gasLimitSafetyBps) / 10_000
}

func gasDemands(legs []legHint) []liquidlanegas.Demand {
	demands := make([]liquidlanegas.Demand, len(legs))
	for i, leg := range legs {
		demands[i] = liquidlanegas.Demand{Collateral: leg.Collateral, AmountOut: leg.ExpectedLoanOut}
	}
	return demands
}
