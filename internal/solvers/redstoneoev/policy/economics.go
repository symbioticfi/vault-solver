package policy

import (
	"math/big"
	"time"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/decision"
)

const (
	skipNoLegs          = decision.SkipReasonNoLegs
	skipGasUnprofitable = decision.SkipReasonGasUnprofitable
	skipStaleEpoch      = decision.SkipReasonStaleEpoch
	skipStaleState      = decision.SkipReasonStaleState
)

func validRate(rate *big.Int) *big.Int {
	if rate != nil && rate.Sign() > 0 {
		return rate
	}
	return nil
}

func loanToNative(loan, rate *big.Int) *big.Int {
	if rate == nil || rate.Sign() <= 0 || loan == nil {
		return new(big.Int)
	}
	return morpho.MulDivDown(loan, morpho.Wad, rate)
}

func nativeToLoan(native, rate *big.Int) *big.Int {
	if rate == nil || rate.Sign() <= 0 || native == nil {
		return new(big.Int)
	}
	return morpho.MulDivUp(native, rate, morpho.Wad)
}

func executorDepositRequired(minDeposit, gasNative *big.Int) *big.Int {
	return new(big.Int).Add(orZero(minDeposit), orZero(gasNative))
}

func depositCoversSettlementGas(deposit, minDeposit, gasNative *big.Int) bool {
	return orZero(deposit).Cmp(executorDepositRequired(minDeposit, gasNative)) >= 0
}

func clampTsAt(auctionMs int64, now time.Time) uint64 {
	nowSec := now.Unix()
	if auctionMs <= 0 {
		return uint64(nowSec)
	}
	ts := auctionMs / 1000
	if !timestampWithinAuctionWindow(ts, nowSec) {
		return uint64(nowSec)
	}
	return uint64(ts)
}

func timestampWithinAuctionWindow(timestamp, now int64) bool {
	const maximumPastSkew = 600
	return timestamp >= now-maximumPastSkew && timestamp <= now
}

func legsWithProfitFloors(legs []selectedLeg, gas gasPrediction, gasPrice, rate *big.Int) []selectedLeg {
	out := make([]selectedLeg, len(legs))
	copy(out, legs)
	for i := range out {
		route := liquidlanegas.RouteUnknown
		if i < len(gas.Routes) {
			route = gas.Routes[i]
		}
		units := liquidlanegas.UnitsForRouteAt(route, i == 0)
		out[i].MinProfit = nativeToLoan(gasCostNative(units, gasPrice), rate)
	}
	return out
}

func legsWithMinimumProfit(legs []selectedLeg, floor *big.Int) []selectedLeg {
	out := make([]selectedLeg, len(legs))
	copy(out, legs)
	for i := range out {
		out[i].MinProfit = cloneBig(floor)
	}
	return out
}
