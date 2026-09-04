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

	declineReason     string
	quotedPairBounded bool
}

type orderPage struct {
	Orders []orderEntry
	Cursor string
}

func (page orderPage) done() bool {
	return page.Cursor == ""
}

type orderEntry struct {
	Type         string
	EncodedOrder string
	Signature    string
	OrderHash    string
	OrderStatus  string
	ChainID      int64
	QuoteID      string
	Input        orderToken
	Outputs      []orderOutput
}

type orderToken struct {
	Token       string
	StartAmount string
	EndAmount   string
}

type orderOutput struct {
	Token       string
	StartAmount string
	EndAmount   string
	Recipient   string
}
