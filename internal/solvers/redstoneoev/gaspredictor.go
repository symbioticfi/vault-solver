package redstoneoev

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

type gasRoute uint8

const (
	gasRouteUnknown gasRoute = iota
	gasRouteAcquire
	gasRouteAllocate
	gasRouteDeallocate
)

const (
	gasRouteUnknownLabel = "unknown"

	// Calibrated on the Sepolia OEV no-preview callback fork. fixedGasUnits adds RedStone overhead; first
	// route units include the cold Morpho/callback/adapter path, later route units are marginal.
	gasBaseUnits               uint64 = 100_000
	gasFirstAcquireLeg         uint64 = 300_000
	gasAdditionalAcquireLeg    uint64 = 140_000
	gasFirstAllocateLeg        uint64 = 530_000
	gasAdditionalAllocateLeg   uint64 = 350_000
	gasFirstDeallocateLeg      uint64 = 650_000
	gasAdditionalDeallocateLeg uint64 = 450_000
	gasFirstUnknownLeg         uint64 = 850_000
	gasAdditionalUnknownLeg    uint64 = 650_000

	// RedStone debits (gasUsed + 35k) * tx.gasprice from the Executor deposit after settlement. Their
	// price update path adds roughly 40k per updated feed before our callback runs. Both are fixed bundle
	// costs for economics and gas-limit sizing; per-leg route units are converted to loan floors by buildBid.
	gasExecutorDebitSurcharge uint64 = 35_000
	gasPriceUpdatePerFeed     uint64 = 40_000

	redstoneExecutorMaxGasUnits uint64 = 2_000_000
	bundleGasLimitSafetyBps     uint64 = 8_500
	defaultPriceUpdateFeeds            = 1
)

type gasPredictorState struct {
	FreeAssets   *big.Int
	Withdrawable *big.Int
	Acquire      map[common.Address]*big.Int
}

type gasPrediction struct {
	Units  uint64
	Routes []gasRoute
}

func cloneBig(v *big.Int) *big.Int {
	if v == nil {
		return nil
	}
	return new(big.Int).Set(v)
}

func gasPredictionForBundle(b chosenBundle, st *gasPredictorState) gasPrediction {
	return gasPredictionForBundleFeeds(b, st, defaultPriceUpdateFeeds)
}

func gasPredictionForBundleFeeds(b chosenBundle, st *gasPredictorState, feedCount int) gasPrediction {
	legUnits, routes := gasLegPredictionForBundle(b, st)
	return gasPrediction{Units: saturatingAddUint64(fixedGasUnits(feedCount), legUnits), Routes: routes}
}

func gasLegPredictionForBundle(b chosenBundle, st *gasPredictorState) (uint64, []gasRoute) {
	if len(b.legs) == 0 {
		return 0, nil
	}
	routes := make([]gasRoute, 0, len(b.legs))
	if st == nil || st.FreeAssets == nil || st.Withdrawable == nil {
		var total uint64
		for i := range b.legs {
			routes = append(routes, gasRouteUnknown)
			total = saturatingAddUint64(total, gasUnitsForRouteAt(gasRouteUnknown, i == 0))
		}
		return total, routes
	}
	acquire := make(map[common.Address]*big.Int, len(st.Acquire))
	for k, v := range st.Acquire {
		acquire[k] = cloneBig(v)
	}
	free := cloneBig(st.FreeAssets)
	withdrawable := cloneBig(st.Withdrawable)
	var total uint64
	for i := range b.legs {
		coll := common.Address{}
		if i < len(b.collaterals) {
			coll = b.collaterals[i]
		}
		expectedLoanOut := (*big.Int)(nil)
		if i < len(b.expectedLoanOuts) {
			expectedLoanOut = b.expectedLoanOuts[i]
		}
		route := predictGasRoute(expectedLoanOut, coll, acquire, free, withdrawable)
		routes = append(routes, route)
		total = saturatingAddUint64(total, gasUnitsForRouteAt(route, i == 0))
	}
	return total, routes
}

func predictGasRoute(expectedLoanOut *big.Int, collateral common.Address, acquire map[common.Address]*big.Int, free, withdrawable *big.Int) gasRoute {
	if expectedLoanOut == nil || expectedLoanOut.Sign() <= 0 || free == nil || withdrawable == nil {
		return gasRouteUnknown
	}
	remaining := new(big.Int).Set(expectedLoanOut)
	if a := acquire[collateral]; a != nil && a.Sign() > 0 {
		used := minBig(remaining, a)
		remaining.Sub(remaining, used)
		a.Sub(a, used)
	}
	if remaining.Sign() == 0 {
		return gasRouteAcquire
	}
	if free.Cmp(remaining) >= 0 {
		free.Sub(free, remaining)
		if withdrawable.Cmp(remaining) >= 0 {
			withdrawable.Sub(withdrawable, remaining)
		} else {
			withdrawable.SetInt64(0)
		}
		return gasRouteAllocate
	}
	if withdrawable.Cmp(remaining) >= 0 {
		withdrawable.Sub(withdrawable, remaining)
		free.SetInt64(0)
		return gasRouteDeallocate
	}
	return gasRouteUnknown
}

func gasUnitsForRoute(route gasRoute) uint64 {
	return gasUnitsForRouteAt(route, false)
}

func gasUnitsForRouteAt(route gasRoute, first bool) uint64 {
	switch route {
	case gasRouteAcquire:
		if first {
			return gasFirstAcquireLeg
		}
		return gasAdditionalAcquireLeg
	case gasRouteAllocate:
		if first {
			return gasFirstAllocateLeg
		}
		return gasAdditionalAllocateLeg
	case gasRouteDeallocate:
		if first {
			return gasFirstDeallocateLeg
		}
		return gasAdditionalDeallocateLeg
	case gasRouteUnknown:
		if first {
			return gasFirstUnknownLeg
		}
		return gasAdditionalUnknownLeg
	default:
		if first {
			return gasFirstUnknownLeg
		}
		return gasAdditionalUnknownLeg
	}
}

func fixedGasUnits(feedCount int) uint64 {
	feeds := uint64(defaultPriceUpdateFeeds)
	if feedCount > 0 {
		feeds = uint64(feedCount)
	}
	feedUnits := saturatingMulUint64(gasPriceUpdatePerFeed, feeds)
	return saturatingAddUint64(saturatingAddUint64(gasBaseUnits, gasExecutorDebitSurcharge), feedUnits)
}

func usableBundleGasLimit(headerGasLimit uint64) uint64 {
	if headerGasLimit == 0 {
		headerGasLimit = redstoneExecutorMaxGasUnits
	}
	limit := min(headerGasLimit, redstoneExecutorMaxGasUnits)
	return saturatingMulUint64(limit, bundleGasLimitSafetyBps) / 10_000
}

func bundleFitsGasLimit(b chosenBundle, st *gasPredictorState, headerGasLimit uint64, feedCount int) bool {
	return gasPredictionForBundleFeeds(b, st, feedCount).Units <= usableBundleGasLimit(headerGasLimit)
}

func gasCostNative(units uint64, gasPrice *big.Int) *big.Int {
	return new(big.Int).Mul(new(big.Int).SetUint64(units), orZero(gasPrice))
}

func (r gasRoute) String() string {
	switch r {
	case gasRouteAcquire:
		return "acquire"
	case gasRouteAllocate:
		return "allocate"
	case gasRouteDeallocate:
		return "deallocate"
	case gasRouteUnknown:
		return gasRouteUnknownLabel
	default:
		return gasRouteUnknownLabel
	}
}

func gasRoutesString(routes []gasRoute) string {
	if len(routes) == 0 {
		return ""
	}
	out := make([]string, len(routes))
	for i, r := range routes {
		out[i] = r.String()
	}
	return strings.Join(out, ",")
}

func minBig(a, b *big.Int) *big.Int {
	if a.Cmp(b) <= 0 {
		return new(big.Int).Set(a)
	}
	return new(big.Int).Set(b)
}

func saturatingMulUint64(a, b uint64) uint64 {
	if a != 0 && b > ^uint64(0)/a {
		return ^uint64(0)
	}
	return a * b
}

func saturatingAddUint64(a, b uint64) uint64 {
	if b > ^uint64(0)-a {
		return ^uint64(0)
	}
	return a + b
}
