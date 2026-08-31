// Package types defines the RFQ-local strategy contract.
package types

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
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

// QuoteInput is the RFQ strategy decision snapshot. The solver has already
// normalized backend inventory and current adapter reads into LiquidLane
// candidates; the strategy only decides how to allocate the request.
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
	Candidates         []liquidlane.QuoteCandidate
	Now                time.Time
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

// FillInput has the same decision shape as QuoteInput; only the solver-owned
// lifecycle stage differs. Fill-time callers populate fresh candidates and the
// awarded RequiredAmountOut.
type FillInput = QuoteInput

// FillPlan is the execution output trusted strategies hand to the solver. The solver enforces its
// structural constraints, then translates the plan into Executor calldata.
type FillPlan struct {
	Legs []FillLeg
}

type FillLeg struct {
	Adapter    common.Address
	AmountIn   *big.Int
	AmountOut  *big.Int
	DiscountID *common.Hash
}
