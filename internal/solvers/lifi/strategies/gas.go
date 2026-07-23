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
	// Foundry measured the full mocked LI.FI direct settlement at <=478,838 gas. The shared
	// LiquidLane predictor already charges at least 300,000 for the first swap.
	settlementGasUnits uint64 = 250_000
	// Private routes additionally decode and verify signed discount payloads.
	privateRouteGasUnits uint64 = 75_000
)

// GasLeg describes one LiquidLane swap included in a LI.FI settlement.
type GasLeg struct {
	Route     liquidlane.Route
	AmountOut *big.Int
	Private   bool
}

// FillGasCost predicts one LI.FI settlement and converts its native gas cost into tokenOut.
func FillGasCost(
	maxFeePerGas *big.Int,
	tokenOut common.Address,
	prices *liquidlanegas.PriceSnapshot,
	snapshot *liquidlanegas.Snapshot,
	legs []GasLeg,
) (*big.Int, error) {
	if maxFeePerGas == nil || maxFeePerGas.Sign() == 0 || len(legs) == 0 {
		return new(big.Int), nil
	}
	if maxFeePerGas.Sign() < 0 {
		return nil, errors.New("max fee per gas must be non-negative")
	}
	tokenOutPerNative := prices.TokenOutPerNative(tokenOut)
	if tokenOutPerNative == nil || tokenOutPerNative.Sign() <= 0 {
		return nil, errors.Errorf("gas oracle: missing tokenOut rate for %s", tokenOut.Hex())
	}
	return FillGasCostAtRate(maxFeePerGas, tokenOutPerNative, snapshot, legs), nil
}

// FillGasCostAtRate predicts one LI.FI settlement using an already-validated token/native rate.
func FillGasCostAtRate(
	maxFeePerGas, tokenOutPerNative *big.Int,
	snapshot *liquidlanegas.Snapshot,
	legs []GasLeg,
) *big.Int {
	if maxFeePerGas == nil || maxFeePerGas.Sign() <= 0 || tokenOutPerNative == nil ||
		tokenOutPerNative.Sign() <= 0 || len(legs) == 0 {
		return new(big.Int)
	}
	demands := make([]liquidlanegas.AdapterDemand, 0, len(legs))
	units := settlementGasUnits
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
			units = saturatingAdd(units, privateRouteGasUnits)
		}
	}
	units = saturatingAdd(units, liquidlanegas.PredictAdapters(demands, snapshot).Units)
	nativeCost := new(big.Int).Mul(maxFeePerGas, new(big.Int).SetUint64(units))
	return mulDivUp(nativeCost, tokenOutPerNative, big.NewInt(nativeUnit))
}

func mulDivUp(x, y, denominator *big.Int) *big.Int {
	if x == nil || y == nil || denominator == nil || denominator.Sign() <= 0 {
		return new(big.Int)
	}
	numerator := new(big.Int).Mul(x, y)
	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	if remainder.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}

func saturatingAdd(a, b uint64) uint64 {
	if b > ^uint64(0)-a {
		return ^uint64(0)
	}
	return a + b
}
