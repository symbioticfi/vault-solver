// Package types defines the UniswapX solver strategy contract.
package types

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
)

const (
	DefaultName = "default"
	SingleName  = "single"
	WebhookName = "webhook"

	// MaxRoutes bounds the physical LiquidLane routes used by one quote or fill.
	MaxRoutes            = 3
	settlementGasUnits   = 250_000
	privateRouteGasUnits = 75_000
)

// LiquidLaneGasEnvelope returns the fixed UniswapX executor overhead around route execution.
func LiquidLaneGasEnvelope() strategies.GasEnvelope {
	return strategies.GasEnvelope{
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
	Trace          strategies.DecisionTrace        `json:"-"`
}

type Quote struct {
	CandidateID liquidlane.CandidateID `json:"-"`
	AmountIn    *big.Int               `json:"amountIn"`
	AmountOut   *big.Int               `json:"amountOut"`
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
	// Full physical budgets before reservations, retained when selecting source subsets.
	CapacityLimits map[liquidlane.CapacityID]*big.Int `json:"capacityLimits,omitempty"`
	GasSnapshot    *liquidlanegas.Snapshot            `json:"gasSnapshot"`
	GasPrices      *liquidlanegas.PriceSnapshot       `json:"gasPrices"`
	MaxFeePerGas   *big.Int                           `json:"maxFeePerGas"`
	ChainTime      time.Time                          `json:"chainTime"`
	Trace          strategies.DecisionTrace           `json:"-"`
}

type FillPlan struct {
	Routes []FillRoute `json:"routes"`
}

type FillRoute = strategies.FillRoute
