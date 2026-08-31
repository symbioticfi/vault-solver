package uniswapx

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

type orderPage struct {
	Orders []orderEntry `json:"orders"`
	Cursor string       `json:"cursor,omitempty"`
}

type orderEntry struct {
	Type         string        `json:"type"`
	EncodedOrder string        `json:"encodedOrder"`
	Signature    string        `json:"signature"`
	OrderHash    string        `json:"orderHash"`
	OrderStatus  string        `json:"orderStatus"`
	ChainID      int64         `json:"chainId"`
	QuoteID      string        `json:"quoteId"`
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
