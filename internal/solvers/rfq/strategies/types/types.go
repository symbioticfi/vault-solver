// Package types defines the RFQ-local strategy contract.
package types

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type Decision string

const (
	DecisionQuote   Decision = "quote"
	DecisionDecline Decision = "decline"
)

type Strategy interface {
	DecideQuote(ctx context.Context, input QuoteInput) (QuoteOutput, error)
	BuildFillPlan(ctx context.Context, input FillInput) (*FillPlan, error)
}

type Pricing interface {
	TokenDecimals(ctx context.Context, token common.Address) (int, error)
	AmountsOut(
		ctx context.Context,
		tokenIn common.Address,
		candidates []QuoteCandidate,
		amount *big.Int,
	) (map[common.Address]*big.Int, error)
}

// QuoteInput is the RFQ strategy decision snapshot. RequireSingleRoute is a solver-owned structural
// constraint; pricing and candidate selection remain strategy-owned.
type QuoteInput struct {
	RequestID string
	QuoteID   string
	ChainID   int64
	Executor  common.Address

	TokenIn  common.Address
	TokenOut common.Address
	AmountIn *big.Int

	RequiredAmountOut  *big.Int
	RequireSingleRoute bool
	Candidates         []QuoteCandidate
	Now                time.Time
}

type QuoteCandidate struct {
	ID string

	Adapter       common.Address
	Asset         common.Address
	AssetDecimals int
	MaxAssets     *big.Int
	MaxRate       *big.Int
	DiscountID    *common.Hash
}

type QuoteOutput struct {
	Decision        Decision
	Reason          string
	QuotedAmountOut *big.Int
	Legs            []QuoteLeg
}

type QuoteLeg struct {
	CandidateID string
	AmountIn    *big.Int
	AmountOut   *big.Int
}

// FillInput is the fill-time snapshot the solver hands back to the strategy. The strategy may return
// a cached quote-time plan or rebuild one from the provided candidates.
type FillInput struct {
	RequestID string
	QuoteID   string
	ChainID   int64
	Executor  common.Address

	TokenIn            common.Address
	TokenOut           common.Address
	AmountIn           *big.Int
	RequiredAmountOut  *big.Int
	RequireSingleRoute bool

	Candidates []QuoteCandidate
	Now        time.Time
}

// FillPlan is the execution output trusted strategies hand to the solver. The solver enforces its
// structural constraints, then translates the plan into Executor calldata.
type FillPlan struct {
	QuoteID         string
	RequestID       string
	TokenIn         common.Address
	TokenOut        common.Address
	AmountIn        *big.Int
	QuotedAmountOut *big.Int
	Legs            []FillLeg
}

type FillLeg struct {
	Adapter    common.Address
	AmountIn   *big.Int
	AmountOut  *big.Int
	MaxRate    *big.Int
	DiscountID *common.Hash
}
