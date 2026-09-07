// Package types defines the UniswapX solver strategy contract.
package types

import (
	"context"
	"math/big"
	"time"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/planning"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

const (
	// MaxRoutes bounds the physical LiquidLane routes used by one quote or fill.
	MaxRoutes            = 3
	settlementGasUnits   = 250_000
	privateRouteGasUnits = 75_000
)

// LiquidLaneGasEnvelope returns the fixed UniswapX executor overhead around route execution.
func LiquidLaneGasEnvelope() planning.GasEnvelope {
	return planning.GasEnvelope{
		SettlementUnits: settlementGasUnits, PrivateRouteUnits: privateRouteGasUnits,
	}
}

type Strategy interface {
	DecideQuote(ctx context.Context, input QuoteInput) (*Quote, error)
	DecideFill(ctx context.Context, input FillInput) (*FillPlan, error)
}

type QuoteInput struct {
	RequestID string `json:"requestId"`
	QuoteID   string `json:"quoteId"`

	TokenIn            common.Address `json:"tokenIn"`
	TokenOut           common.Address `json:"tokenOut"`
	AmountIn           *big.Int       `json:"amountIn,omitempty"`
	AmountOut          *big.Int       `json:"amountOut,omitempty"`
	RequireSingleRoute bool           `json:"requireSingleRoute"`

	Inventory      []liquidlane.Inventory          `json:"inventory"`
	Reservations   liquidlane.CapacityReservations `json:"reservations"`
	GasSnapshot    *liquidlanegas.Snapshot         `json:"gasSnapshot"`
	GasPrices      *liquidlanegas.PriceSnapshot    `json:"gasPrices"`
	MaxFeePerGas   *big.Int                        `json:"maxFeePerGas"`
	ChainTime      time.Time                       `json:"chainTime"`
	QuoteExpiresAt time.Time                       `json:"quoteExpiresAt"`
	Trace          planning.DecisionTrace          `json:"-"`
}

type Quote struct {
	AmountIn  *big.Int `json:"amountIn"`
	AmountOut *big.Int `json:"amountOut"`
}

type FillInput struct {
	planning.FillInput

	OrderID  string `json:"orderId"`
	QuoteID  string `json:"quoteId"`
	Deadline uint32 `json:"deadline"`
}

type FillPlan struct {
	Routes []FillRoute `json:"routes"`
}

type FillRoute = planning.FillRoute
