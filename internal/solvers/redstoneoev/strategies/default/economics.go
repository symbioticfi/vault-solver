package defaultstrategy

import (
	"math/big"
	"time"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

const (
	skipNoLegs          = types.SkipReasonNoLegs
	skipGasUnprofitable = types.SkipReasonGasUnprofitable
	skipStaleEpoch      = types.SkipReasonStaleEpoch
	skipStaleState      = types.SkipReasonStaleState
)

func validRate(rate *big.Int) *big.Int {
	if rate != nil && rate.Sign() > 0 {
		return rate
	}
	return nil
}

func composeLoanPerEth(ethUsd, loanUsd *big.Int, ethFeedDec, loanFeedDec, loanDec int) *big.Int {
	if ethUsd == nil || loanUsd == nil || ethUsd.Sign() <= 0 || loanUsd.Sign() <= 0 {
		return nil
	}
	num := new(big.Int).Mul(ethUsd, exp10(loanDec+loanFeedDec))
	den := new(big.Int).Mul(loanUsd, exp10(ethFeedDec))
	rate := new(big.Int).Quo(num, den)
	if rate.Sign() <= 0 {
		return nil
	}
	return rate
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
	const skew = 600
	if ts < nowSec-skew || ts > nowSec {
		return uint64(nowSec)
	}
	return uint64(ts)
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
