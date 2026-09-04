// Package strategies contains protocol-neutral LiquidLane decision economics.
package planning

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

const (
	nativeUnit                   = 1_000_000_000_000_000_000
	executorSettlementGasUnits   = 250_000
	executorPrivateRouteGasUnits = 75_000
)

type GasEnvelope struct {
	SettlementUnits   uint64
	PrivateRouteUnits uint64
}

// ExecutorGasEnvelope is the shared overhead of the LI.FI and UniswapX
// LiquidLane executors around adapter route execution.
func ExecutorGasEnvelope() GasEnvelope {
	return GasEnvelope{SettlementUnits: executorSettlementGasUnits, PrivateRouteUnits: executorPrivateRouteGasUnits}
}

type GasLeg struct {
	Route     liquidlane.Route
	AmountOut *big.Int
	Private   bool
}

type GasPricing struct {
	feePerGas         *big.Int
	tokenOutPerNative *big.Int
	snapshot          *liquidlanegas.Snapshot
	envelope          GasEnvelope
}

func NewGasPricing(
	maxFeePerGas *big.Int,
	tokenOut common.Address,
	prices *liquidlanegas.PriceSnapshot,
	snapshot *liquidlanegas.Snapshot,
	reserveBps int,
	envelope GasEnvelope,
) (GasPricing, error) {
	if maxFeePerGas == nil || maxFeePerGas.Sign() < 0 {
		return GasPricing{}, errors.New("max fee per gas must be non-negative")
	}
	rate := prices.TokenOutPerNative(tokenOut)
	if maxFeePerGas.Sign() > 0 && (rate == nil || rate.Sign() <= 0) {
		return GasPricing{}, errors.Errorf("gas oracle: missing tokenOut rate for %s", tokenOut.Hex())
	}
	if rate == nil {
		rate = new(big.Int)
	}
	return GasPricing{
		feePerGas: liquidlane.CloneBig(maxFeePerGas), tokenOutPerNative: rate,
		snapshot: liquidlanegas.WithReserveBps(snapshot, reserveBps), envelope: envelope,
	}, nil
}

func (pricing GasPricing) Cost(legs []GasLeg) *big.Int {
	return priceGas(pricing.feePerGas, pricing.tokenOutPerNative, pricing.snapshot, pricing.envelope, legs)
}

func (pricing GasPricing) MaxCost(routeCount, privateRouteCount int) *big.Int {
	if routeCount <= 0 || pricing.feePerGas == nil || pricing.feePerGas.Sign() <= 0 ||
		pricing.tokenOutPerNative == nil || pricing.tokenOutPerNative.Sign() <= 0 {
		return new(big.Int)
	}
	privateRouteCount = min(max(privateRouteCount, 0), routeCount)
	units := pricing.envelope.SettlementUnits
	for range routeCount {
		units = addGasUnits(units, liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteUnknown, true))
	}
	for range privateRouteCount {
		units = addGasUnits(units, pricing.envelope.PrivateRouteUnits)
	}
	return tokenCost(pricing.feePerGas, pricing.tokenOutPerNative, units)
}

func FillGasCost(
	maxFeePerGas *big.Int,
	tokenOut common.Address,
	prices *liquidlanegas.PriceSnapshot,
	snapshot *liquidlanegas.Snapshot,
	envelope GasEnvelope,
	legs []GasLeg,
) (*big.Int, error) {
	if maxFeePerGas == nil || maxFeePerGas.Sign() == 0 || len(legs) == 0 {
		return new(big.Int), nil
	}
	if maxFeePerGas.Sign() < 0 {
		return nil, errors.New("max fee per gas must be non-negative")
	}
	rate := prices.TokenOutPerNative(tokenOut)
	if rate == nil || rate.Sign() <= 0 {
		return nil, errors.Errorf("gas oracle: missing tokenOut rate for %s", tokenOut.Hex())
	}
	return priceGas(maxFeePerGas, rate, snapshot, envelope, legs), nil
}

func priceGas(
	feePerGas, tokenOutPerNative *big.Int,
	snapshot *liquidlanegas.Snapshot,
	envelope GasEnvelope,
	legs []GasLeg,
) *big.Int {
	if feePerGas == nil || feePerGas.Sign() <= 0 || tokenOutPerNative == nil ||
		tokenOutPerNative.Sign() <= 0 || len(legs) == 0 {
		return new(big.Int)
	}
	demands := make([]liquidlanegas.AdapterDemand, 0, len(legs))
	units := envelope.SettlementUnits
	for _, leg := range executionOrder(legs) {
		demands = append(demands, liquidlanegas.AdapterDemand{
			Adapter: leg.Route.Adapter, Vault: leg.Route.Vault,
			Demand: liquidlanegas.Demand{Collateral: leg.Route.TokenIn, AmountOut: liquidlane.CloneBig(leg.AmountOut)},
		})
		if leg.Private {
			units = addGasUnits(units, envelope.PrivateRouteUnits)
		}
	}
	units = addGasUnits(units, liquidlanegas.PredictAdapters(demands, snapshot).Units)
	return tokenCost(feePerGas, tokenOutPerNative, units)
}

func executionOrder(legs []GasLeg) []GasLeg {
	ordered := make([]GasLeg, 0, len(legs))
	for _, private := range []bool{false, true} {
		for _, leg := range legs {
			if leg.Private == private {
				ordered = append(ordered, leg)
			}
		}
	}
	return ordered
}

func tokenCost(feePerGas, rate *big.Int, units uint64) *big.Int {
	nativeCost := new(big.Int).Mul(feePerGas, new(big.Int).SetUint64(units))
	return liquidlane.MulDivUp(nativeCost, rate, big.NewInt(nativeUnit))
}

func addGasUnits(left, right uint64) uint64 {
	if right > ^uint64(0)-left {
		return ^uint64(0)
	}
	return left + right
}
