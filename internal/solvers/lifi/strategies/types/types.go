// Package types defines the LI.FI same-chain solver strategy contract.
package types

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

const (
	// MaxRoutes bounds the physical LiquidLane routes used by one quote or fill.
	MaxRoutes = 3
	// MaxQuoteRanges bounds the amount ranges published for one token pair.
	MaxQuoteRanges = 16
)

type Strategy interface {
	DecideQuotes(ctx context.Context, input QuoteInput) (QuoteOutput, error)
	DecideFill(ctx context.Context, input FillInput) (*FillPlan, error)
}

type QuoteInput struct {
	Solver            common.Address                     `json:"solver"`
	Inventory         []liquidlane.Inventory             `json:"inventory"`
	Reservations      map[liquidlane.CapacityID]*big.Int `json:"reservations"`
	SingleRouteTokens map[common.Address]bool            `json:"singleRouteTokens"`
	GasSnapshot       *liquidlanegas.Snapshot            `json:"gasSnapshot"`
	GasPrices         *liquidlanegas.PriceSnapshot       `json:"gasPrices"`
	MaxFeePerGas      *big.Int                           `json:"maxFeePerGas"`
	ChainTime         time.Time                          `json:"chainTime"`
	ServerTime        time.Time                          `json:"serverTime"`
	QuoteExpiresAt    time.Time                          `json:"quoteExpiresAt"`
}

type QuoteOutput struct {
	Quotes []Quote `json:"quotes"`
}

type Quote struct {
	FromAsset common.Address `json:"fromAsset"`
	ToAsset   common.Address `json:"toAsset"`

	FromDecimals int `json:"fromDecimals"`
	ToDecimals   int `json:"toDecimals"`

	Ranges       []QuoteRange   `json:"ranges"`
	Expiry       int64          `json:"expiry"`
	ExclusiveFor common.Address `json:"exclusiveFor"`
}

type QuoteRange struct {
	MinAmount *big.Int `json:"minAmount"`
	MaxAmount *big.Int `json:"maxAmount"`
	Quote     string   `json:"quote"`
}

type FillInput struct {
	OrderID string         `json:"orderId"`
	QuoteID string         `json:"quoteId"`
	Solver  common.Address `json:"solver"`

	TokenIn            common.Address `json:"tokenIn"`
	TokenOut           common.Address `json:"tokenOut"`
	AmountIn           *big.Int       `json:"amountIn"`
	OutputAmount       *big.Int       `json:"outputAmount"`
	OutputContext      []byte         `json:"outputContext"`
	Expires            uint32         `json:"expires"`
	FillDeadline       uint32         `json:"fillDeadline"`
	RequireSingleRoute bool           `json:"requireSingleRoute"`

	Quotes       []liquidlane.FillQuote             `json:"quotes"`
	Reservations map[liquidlane.CapacityID]*big.Int `json:"reservations"`
	GasSnapshot  *liquidlanegas.Snapshot            `json:"gasSnapshot"`
	GasPrices    *liquidlanegas.PriceSnapshot       `json:"gasPrices"`
	MaxFeePerGas *big.Int                           `json:"maxFeePerGas"`
	ChainTime    time.Time                          `json:"chainTime"`
}

type FillPlan struct {
	Routes []FillRoute `json:"routes"`
}

type FillRoute struct {
	RouteID           liquidlane.RouteID    `json:"routeId"`
	CapacityID        liquidlane.CapacityID `json:"capacityId"`
	Adapter           common.Address        `json:"adapter"`
	AmountIn          *big.Int              `json:"amountIn"`
	ExpectedAmountOut *big.Int              `json:"expectedAmountOut"`
	MinAmountOut      *big.Int              `json:"minAmountOut"`
	ReservedAmountOut *big.Int              `json:"reservedAmountOut"`
	DiscountID        *common.Hash          `json:"discountId"`
}
