package uniswapx

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	liquidplanning "github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
)

// MaxRoutes bounds the physical LiquidLane routes used by one quote or fill.
const MaxRoutes = 3

type Planner interface {
	DecideQuote(ctx context.Context, input QuoteInput) (*Quote, error)
	DecideFill(ctx context.Context, input FillInput) (*liquidlane.Plan, error)
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
	Trace          liquidplanning.DecisionTrace    `json:"-"`
}

func (input QuoteInput) RouteLimit() int {
	if input.RequireSingleRoute {
		return 1
	}
	return MaxRoutes
}

type Quote struct {
	AmountIn  *big.Int `json:"amountIn"`
	AmountOut *big.Int `json:"amountOut"`
}

type FillInput struct {
	OrderID string `json:"orderId"`
	QuoteID string `json:"quoteId"`

	TokenIn            common.Address `json:"tokenIn"`
	TokenOut           common.Address `json:"tokenOut"`
	AmountIn           *big.Int       `json:"amountIn"`
	OutputAmount       *big.Int       `json:"outputAmount"`
	Deadline           uint32         `json:"deadline"`
	RequireSingleRoute bool           `json:"requireSingleRoute"`

	Quotes       []liquidlane.FillQuote          `json:"quotes"`
	Reservations liquidlane.CapacityReservations `json:"reservations"`
	GasSnapshot  *liquidlanegas.Snapshot         `json:"gasSnapshot"`
	GasPrices    *liquidlanegas.PriceSnapshot    `json:"gasPrices"`
	MaxFeePerGas *big.Int                        `json:"maxFeePerGas"`
	ChainTime    time.Time                       `json:"chainTime"`
	Trace        liquidplanning.DecisionTrace    `json:"-"`
}

func (input FillInput) RouteLimit() int {
	if input.RequireSingleRoute {
		return 1
	}
	return MaxRoutes
}
