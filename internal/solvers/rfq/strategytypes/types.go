// Package strategytypes defines the RFQ-local strategy contract.
package strategytypes

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

// QuoteInput is the RFQ strategy decision snapshot. It is intentionally solver-local.
type QuoteInput struct {
	RequestID string
	QuoteID   string
	ChainID   int64
	Executor  common.Address

	TokenIn  common.Address
	TokenOut common.Address
	AmountIn *big.Int

	RequiredAmountOut *big.Int
	Candidates        []QuoteCandidate
	Now               time.Time
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
