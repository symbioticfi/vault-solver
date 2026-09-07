package uniswapx

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/parse"
	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

type quoteRequest struct {
	BlockUntilTimestamp *int64 `json:"blockUntilTimestamp,omitempty"`
	RequestID           string `json:"requestId"`
	QuoteID             string `json:"quoteId"`
	TokenInChainID      int64  `json:"tokenInChainId"`
	TokenOutChainID     int64  `json:"tokenOutChainId"`
	Swapper             string `json:"swapper"`
	TokenIn             string `json:"tokenIn"`
	TokenOut            string `json:"tokenOut"`
	Amount              string `json:"amount"`
	Type                string `json:"type"`
	NumOutputs          int    `json:"numOutputs"`
	Protocol            string `json:"protocol"`
}

// quoteDeclineReason is an internal, bounded decision enum. Keeping it typed prevents request data,
// strategy messages, or arbitrary errors from becoming Prometheus label values.
type quoteDeclineReason string

const (
	quoteDeclineBlocked               quoteDeclineReason = "blocked"
	quoteDeclineInvalidRequest        quoteDeclineReason = "invalid-request"
	quoteDeclinePairOutOfScope        quoteDeclineReason = "pair-out-of-scope"
	quoteDeclineInvalidAmount         quoteDeclineReason = "invalid-amount"
	quoteDeclineQuoteStateUnavailable quoteDeclineReason = "quote-state-unavailable"
	quoteDeclineStrategy              quoteDeclineReason = "strategy-declined"
	quoteDeclineStateChanged          quoteDeclineReason = "state-changed"
)

type quoteResponse struct {
	ChainID   int64  `json:"chainId"`
	RequestID string `json:"requestId"`
	Swapper   string `json:"swapper"`
	TokenIn   string `json:"tokenIn"`
	AmountIn  string `json:"amountIn"`
	TokenOut  string `json:"tokenOut"`
	AmountOut string `json:"amountOut"`
	Filler    string `json:"filler"`
	QuoteID   string `json:"quoteId"`

	declineReason     quoteDeclineReason
	quotedPairBounded bool
}

type orderEntry struct {
	Type         string        `json:"type"`
	EncodedOrder string        `json:"encodedOrder"`
	Signature    string        `json:"signature"`
	OrderHash    string        `json:"orderHash"`
	OrderStatus  string        `json:"orderStatus"`
	ChainID      int64         `json:"chainId"`
	QuoteID      string        `json:"quoteId"`
	CreatedAt    int64         `json:"createdAt"`
	Input        orderToken    `json:"input"`
	Outputs      []orderOutput `json:"outputs"`
}

type orderToken struct {
	Token       string `json:"token"`
	StartAmount string `json:"startAmount"`
	EndAmount   string `json:"endAmount"`
}

type orderOutput struct {
	Token       string `json:"token"`
	StartAmount string `json:"startAmount"`
	EndAmount   string `json:"endAmount"`
	Recipient   string `json:"recipient"`
}

// strategyInput validates the public request before it can refer to a cached quote snapshot.
func (q quoteRequest) strategyInput(chainID int64, policy tokenpolicy.Policy) (strategytypes.QuoteInput, quoteDeclineReason) {
	input := strategytypes.QuoteInput{RequestID: q.RequestID, QuoteID: q.QuoteID}
	if q.RequestID == "" || q.QuoteID == "" || q.NumOutputs < 1 || !supportedQuoteType(q.Type) || !supportedQuoteProtocol(q.Protocol) ||
		q.TokenInChainID != chainID || q.TokenOutChainID != chainID || !common.IsHexAddress(q.Swapper) {
		return input, quoteDeclineInvalidRequest
	}
	var err error
	input.TokenIn, err = parse.Address(q.TokenIn, "tokenIn")
	if err != nil {
		return input, quoteDeclineInvalidRequest
	}
	input.TokenOut, err = parse.Address(q.TokenOut, "tokenOut")
	if err != nil {
		return input, quoteDeclineInvalidRequest
	}
	if input.TokenIn == input.TokenOut || input.TokenOut == (common.Address{}) || !policy.Allows(input.TokenIn) {
		return input, quoteDeclinePairOutOfScope
	}
	amount, err := parse.Uint(q.Amount, "amount", 256)
	if err != nil || amount.Sign() <= 0 {
		return input, quoteDeclineInvalidAmount
	}
	if q.Type == quoteTypeExactInput {
		input.AmountIn = amount
	} else {
		input.AmountOut = amount
	}
	return input, ""
}
