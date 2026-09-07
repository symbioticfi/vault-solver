package planning

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

const nativeUnit = 1_000_000_000_000_000_000

// GasEnvelope holds integration-owned overhead around shared adapter swaps.
type GasEnvelope struct {
	SettlementUnits   uint64
	PrivateRouteUnits uint64
}

type GasLeg struct {
	Route     liquidlane.Route
	AmountOut *big.Int
	Private   bool
}

// GasPricing owns the reserved snapshot and rates used by one planning attempt.
type GasPricing struct {
	feePerGas         *big.Int
	tokenOutPerNative *big.Int
	snapshot          *liquidlanegas.Snapshot
	envelope          GasEnvelope
}

func NewGasPricing(fee *big.Int, token common.Address, prices *liquidlanegas.PriceSnapshot,
	snapshot *liquidlanegas.Snapshot, reserveBps int, envelope GasEnvelope) (GasPricing, error) {
	if fee == nil || fee.Sign() < 0 {
		return GasPricing{}, errors.New("max fee per gas must be non-negative")
	}
	rate := prices.TokenOutPerNative(token)
	if fee.Sign() > 0 && (rate == nil || rate.Sign() <= 0) {
		return GasPricing{}, errors.Errorf("gas oracle: missing tokenOut rate for %s", token.Hex())
	}
	return GasPricing{feePerGas: bigmath.Clone(fee), tokenOutPerNative: rate,
		snapshot: liquidlanegas.WithReserveBps(snapshot, reserveBps), envelope: envelope}, nil
}

func (p GasPricing) Cost(legs []GasLeg) *big.Int {
	if len(legs) == 0 || !p.priced() {
		return new(big.Int)
	}
	demands := make([]liquidlanegas.AdapterDemand, 0, len(legs))
	units := p.envelope.SettlementUnits
	// Calldata executes every direct swap before discount swaps. Keep this order
	// when consuming the shared vault snapshot; otherwise later route gas is wrong.
	for _, private := range []bool{false, true} {
		for _, leg := range legs {
			if leg.Private != private {
				continue
			}
			demands = append(demands, liquidlanegas.AdapterDemand{Adapter: leg.Route.Adapter, Vault: leg.Route.Vault,
				Demand: liquidlanegas.Demand{Collateral: leg.Route.TokenIn, AmountOut: leg.AmountOut}})
			if private {
				units = bigmath.SaturatingAdd(units, p.envelope.PrivateRouteUnits)
			}
		}
	}
	return p.tokenCost(bigmath.SaturatingAdd(units, liquidlanegas.PredictAdapters(demands, p.snapshot).Units))
}

// MaxCost uses the worst route including first-swap overhead for every leg. It
// runs in constant time even when an invalid remote strategy supplies a huge count.
func (p GasPricing) MaxCost(routeCount, privateRouteCount int) *big.Int {
	if routeCount <= 0 || !p.priced() {
		return new(big.Int)
	}
	unknown := liquidlanegas.UnitsForRouteAt(liquidlanegas.RouteUnknown, true)
	units := bigmath.SaturatingAdd(p.envelope.SettlementUnits, bigmath.SaturatingMul(uint64(routeCount), unknown))
	units = bigmath.SaturatingAdd(units, bigmath.SaturatingMul(uint64(min(max(privateRouteCount, 0), routeCount)), p.envelope.PrivateRouteUnits))
	return p.tokenCost(units)
}

func (p GasPricing) priced() bool {
	return p.feePerGas != nil && p.feePerGas.Sign() > 0 && p.tokenOutPerNative != nil && p.tokenOutPerNative.Sign() > 0
}

func (p GasPricing) tokenCost(units uint64) *big.Int {
	native := new(big.Int).Mul(p.feePerGas, new(big.Int).SetUint64(units))
	return liquidlane.MulDivUp(native, p.tokenOutPerNative, big.NewInt(nativeUnit))
}

func FillGasCost(fee *big.Int, token common.Address, prices *liquidlanegas.PriceSnapshot,
	snapshot *liquidlanegas.Snapshot, envelope GasEnvelope, legs []GasLeg) (*big.Int, error) {
	if fee == nil || fee.Sign() == 0 || len(legs) == 0 {
		return new(big.Int), nil
	}
	pricing, err := NewGasPricing(fee, token, prices, snapshot, 0, envelope)
	if err != nil {
		return nil, err
	}
	return pricing.Cost(legs), nil
}
