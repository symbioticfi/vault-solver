package rfq

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

type Planner interface {
	DecideQuote(ctx context.Context, input QuoteInput) (QuoteOutput, error)
	BuildFillPlan(ctx context.Context, input FillInput) (*liquidlane.Plan, error)
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

func (input QuoteInput) RouteLimit() int {
	if input.RequireSingleRoute {
		return 1
	}
	return len(input.Candidates)
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

// liquidlane.Plan is canonical LiquidLane execution output. RFQ retains its quote webhook wire
// contract, then canonicalizes selected candidates into this shared plan before admission.
