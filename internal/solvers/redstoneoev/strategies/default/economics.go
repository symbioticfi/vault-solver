package defaultstrategy

import (
	"math/big"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

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
	return new(big.Int).Add(bigmath.OrZero(minDeposit), bigmath.OrZero(gasNative))
}

func depositCoversSettlementGas(deposit, minimum, gasCost *big.Int) bool {
	available := new(big.Int).Sub(bigmath.OrZero(deposit), bigmath.OrZero(minimum))
	return available.Cmp(bigmath.OrZero(gasCost)) >= 0
}

func clampTsAt(auctionMs int64, now time.Time) uint64 {
	timestamp, current := auctionMs/1000, now.Unix()
	if auctionMs > 0 && timestamp >= current-600 && timestamp <= current {
		return uint64(max(timestamp, 0))
	}
	return uint64(max(current, 0))
}

func (b chosenBundle) legsWithProfitFloors(prediction gasPrediction, gasPrice, rate *big.Int) []selectedLeg {
	return b.selectedLegs(func(index int) *big.Int {
		route := liquidlanegas.RouteUnknown
		if index < len(prediction.Routes) {
			route = prediction.Routes[index]
		}
		nativeCost := gasCostNative(liquidlanegas.UnitsForRouteAt(route, index == 0), gasPrice)
		return nativeToLoan(nativeCost, rate)
	})
}
