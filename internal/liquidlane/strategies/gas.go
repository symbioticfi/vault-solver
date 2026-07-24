// Package strategies contains neutral inputs and economics shared by
// LiquidLane decision strategies.
package strategies

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

const (
	nativeUnit = 1_000_000_000_000_000_000
)

// GasEnvelope is the protocol-specific fixed gas around LiquidLane route execution.
type GasEnvelope struct {
	SettlementUnits   uint64
	PrivateRouteUnits uint64
}

// GasLeg describes one LiquidLane swap included in a settlement.
type GasLeg struct {
	Route     liquidlane.Route
	AmountOut *big.Int
	Private   bool
}

// GasPricing converts predicted settlement gas into tokenOut.
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
		feePerGas: new(big.Int).Set(maxFeePerGas), tokenOutPerNative: rate,
		snapshot: liquidlanegas.WithReserveBps(snapshot, reserveBps), envelope: envelope,
	}, nil
}

func (p GasPricing) Cost(legs []GasLeg) *big.Int {
	return fillGasCostAtRate(p.feePerGas, p.tokenOutPerNative, p.snapshot, p.envelope, legs)
}

// MaxCost bounds settlement gas without depending on adapter liquidity or route ordering.
func (p GasPricing) MaxCost(routeCount, privateRouteCount int) *big.Int {
	if routeCount <= 0 || p.feePerGas == nil || p.feePerGas.Sign() <= 0 ||
		p.tokenOutPerNative == nil || p.tokenOutPerNative.Sign() <= 0 {
		return new(big.Int)
	}
	privateRouteCount = min(max(privateRouteCount, 0), routeCount)
	units := p.envelope.SettlementUnits
	for range routeCount {
		units = saturatingAdd(
			units,
			liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteUnknown, true),
		)
	}
	for range privateRouteCount {
		units = saturatingAdd(units, p.envelope.PrivateRouteUnits)
	}
	nativeCost := new(big.Int).Mul(p.feePerGas, new(big.Int).SetUint64(units))
	return liquidlane.MulDivUp(nativeCost, p.tokenOutPerNative, big.NewInt(nativeUnit))
}

// FillGasCost predicts a LiquidLane settlement and converts its native gas cost into tokenOut.
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
	return fillGasCostAtRate(maxFeePerGas, rate, snapshot, envelope, legs), nil
}

func fillGasCostAtRate(
	maxFeePerGas, tokenOutPerNative *big.Int,
	snapshot *liquidlanegas.Snapshot,
	envelope GasEnvelope,
	legs []GasLeg,
) *big.Int {
	if maxFeePerGas == nil || maxFeePerGas.Sign() <= 0 || tokenOutPerNative == nil ||
		tokenOutPerNative.Sign() <= 0 || len(legs) == 0 {
		return new(big.Int)
	}
	demands := make([]liquidlanegas.AdapterDemand, 0, len(legs))
	units := envelope.SettlementUnits
	for _, leg := range legs {
		demands = append(demands, liquidlanegas.AdapterDemand{
			Adapter: leg.Route.Adapter,
			Vault:   leg.Route.Vault,
			Demand: liquidlanegas.Demand{
				Collateral: leg.Route.TokenIn,
				AmountOut:  liquidlane.CloneBig(leg.AmountOut),
			},
		})
		if leg.Private {
			units = saturatingAdd(units, envelope.PrivateRouteUnits)
		}
	}
	units = saturatingAdd(units, liquidlanegas.PredictAdapters(demands, snapshot).Units)
	nativeCost := new(big.Int).Mul(maxFeePerGas, new(big.Int).SetUint64(units))
	return liquidlane.MulDivUp(nativeCost, tokenOutPerNative, big.NewInt(nativeUnit))
}

func saturatingAdd(left, right uint64) uint64 {
	if right > ^uint64(0)-left {
		return ^uint64(0)
	}
	return left + right
}
