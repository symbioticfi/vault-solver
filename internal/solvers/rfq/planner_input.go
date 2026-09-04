package rfq

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// quoteRequestFacts is the subset of a quote request the planner needs.
type quoteRequestFacts struct {
	RequestID string
	QuoteID   string
	TokenIn   common.Address
	TokenOut  common.Address
	Amount    *big.Int
}

func newQuoteInput(
	chainID int64,
	executor common.Address,
	req quoteRequestFacts,
	candidates []liquidlane.QuoteCandidate,
	required *big.Int,
	requireSingleRoute bool,
	now time.Time,
) QuoteInput {
	return QuoteInput{
		RequestID:          req.RequestID,
		QuoteID:            req.QuoteID,
		ChainID:            chainID,
		Executor:           executor,
		TokenIn:            req.TokenIn,
		TokenOut:           req.TokenOut,
		AmountIn:           liquidlane.CloneBig(req.Amount),
		RequiredAmountOut:  liquidlane.CloneBig(required),
		RequireSingleRoute: requireSingleRoute,
		Candidates:         candidates,
		Now:                now,
	}
}

func validateSingleRoute(requireSingleRoute bool, legCount int) error {
	if !requireSingleRoute || legCount == 1 {
		return nil
	}
	return errors.Errorf("single-route input requires exactly one leg, got %d", legCount)
}
