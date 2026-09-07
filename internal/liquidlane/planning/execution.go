package planning

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

// FillInput is the current market and capacity snapshot shared by direct executors.
// Integrations embed it in their own strategy request and retain their order terms.
type FillInput struct {
	TokenIn            common.Address `json:"tokenIn"`
	TokenOut           common.Address `json:"tokenOut"`
	AmountIn           *big.Int       `json:"amountIn"`
	OutputAmount       *big.Int       `json:"outputAmount"`
	RequireSingleRoute bool           `json:"requireSingleRoute"`

	Quotes       []liquidlane.FillQuote          `json:"quotes"`
	Reservations liquidlane.CapacityReservations `json:"reservations"`
	GasSnapshot  *liquidlanegas.Snapshot         `json:"gasSnapshot"`
	GasPrices    *liquidlanegas.PriceSnapshot    `json:"gasPrices"`
	MaxFeePerGas *big.Int                        `json:"maxFeePerGas"`
	ChainTime    time.Time                       `json:"chainTime"`
	Trace        DecisionTrace                   `json:"-"`
}

// CheckAmounts validates positive amounts and reports whether the policy minimum is met.
func (in FillInput) CheckAmounts(minimum *big.Int) (bool, error) {
	if in.AmountIn == nil || in.AmountIn.Sign() <= 0 {
		return false, errors.New("amountIn: must be positive")
	}
	if in.OutputAmount == nil || in.OutputAmount.Sign() <= 0 {
		return false, errors.New("outputAmount: must be positive")
	}
	if in.AmountIn.Cmp(minimum) < 0 {
		in.Trace.Decline("fill", "amount-below-minimum", "amountIn", in.AmountIn.String(), "minAmount", minimum.String())
		return false, nil
	}
	return true, nil
}

// Allocate applies the execution policy once to both inventory and gas reserves.
// Protocol deadlines and output conditions are checked by the owning integration.
func (in FillInput) Allocate(policy ExecutionPolicy, envelope GasEnvelope, maxRoutes int) (*FillSolution, error) {
	if in.RequireSingleRoute {
		maxRoutes = 1
	}
	pricing, err := NewGasPricing(in.MaxFeePerGas, in.TokenOut, in.GasPrices, in.GasSnapshot, policy.InventoryReserveBps, envelope)
	if err != nil {
		return nil, err
	}
	return SolveFill(FillTask{
		TokenIn: in.TokenIn, TokenOut: in.TokenOut, AmountIn: in.AmountIn,
		Quotes: in.Quotes, Reservations: in.Reservations, ValidAfter: in.ChainTime.Add(policy.ExecutionBuffer),
		MaxRoutes: maxRoutes, PriceBufferBps: policy.PriceBufferBps, InventoryReserveBps: policy.InventoryReserveBps,
		GasPricing: &pricing, Trace: in.Trace,
	})
}
