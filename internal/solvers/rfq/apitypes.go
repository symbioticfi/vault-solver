package rfq

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/parse"
)

// quoteRequest is the backend → filler RFQ quote request (POST /quote). The validation tags drive
// both Huma's request validation and the generated OpenAPI schema; toStrategy then parses the
// already-validated payload and applies the quote-logic checks that decide 204.
type quoteRequest struct {
	RequestID       string         `json:"requestId" format:"uuid"`
	TokenInChainID  int64          `json:"tokenInChainId" minimum:"1"`
	TokenOutChainID int64          `json:"tokenOutChainId" minimum:"1"`
	Swapper         string         `json:"swapper" pattern:"^0x[a-fA-F0-9]{40}$"`
	TokenIn         string         `json:"tokenIn" pattern:"^0x[a-fA-F0-9]{40}$"`
	TokenOut        string         `json:"tokenOut" pattern:"^0x[a-fA-F0-9]{40}$"`
	Amount          string         `json:"amount" pattern:"^[0-9]+$"`
	Type            string         `json:"type" enum:"EXACT_INPUT"`
	Protocol        string         `json:"protocol" enum:"v1"`
	NumOutputs      int            `json:"numOutputs" minimum:"1"`
	QuoteID         string         `json:"quoteId" format:"uuid"`
	Adapters        []quoteAdapter `json:"adapters" maxItems:"256"`
}

// quoteAdapter is one adapter inventory entry the backend offers for the request.
type quoteAdapter struct {
	Adapter       string  `json:"adapter" pattern:"^0x[a-fA-F0-9]{40}$"`
	Asset         string  `json:"asset" pattern:"^0x[a-fA-F0-9]{40}$"`
	AssetDecimals int     `json:"assetDecimals" minimum:"0" maximum:"255"`
	MaxAssets     string  `json:"maxAssets" pattern:"^[0-9]+$"`
	MaxRate       string  `json:"maxRate" pattern:"^[0-9]+$"`
	DiscountID    *string `json:"discountId,omitempty" pattern:"^0x[a-fA-F0-9]{64}$"`
}

// quoteResponse is the filler → backend quote (POST /quote 200).
type quoteResponse struct {
	ChainID   int64  `json:"chainId"`
	AmountIn  string `json:"amountIn"`
	AmountOut string `json:"amountOut"`
	Filler    string `json:"filler"`
	RequestID string `json:"requestId"`
	Swapper   string `json:"swapper"`
	TokenIn   string `json:"tokenIn"`
	TokenOut  string `json:"tokenOut"`
	QuoteID   string `json:"quoteId"`
}

const (
	quoteTypeExactInput = "EXACT_INPUT"
	quoteProtocolV1     = "v1"
)

// parsedQuote is the validated, typed form of a quotable request.
type parsedQuote struct {
	req strategyRequest
	inv []solverInventory
}

// toStrategy validates and parses the request. A non-nil error is a malformed payload (→ 400). A nil
// result with nil error means the request is well-formed but not quotable here — wrong chain or no
// adapters — and the caller replies 204.
func (q *quoteRequest) toStrategy(chainID int64) (*parsedQuote, error) {
	switch {
	case q.Type != quoteTypeExactInput:
		return nil, errors.Errorf("type must be %q", quoteTypeExactInput)
	case q.Protocol != quoteProtocolV1:
		return nil, errors.Errorf("protocol must be %q", quoteProtocolV1)
	case q.NumOutputs < 1:
		return nil, errors.New("numOutputs must be positive")
	case q.RequestID == "" || q.QuoteID == "":
		return nil, errors.New("quoteId and requestId are required")
	}
	_, swapperErr := parse.Address(q.Swapper, "swapper")
	tokenIn, inputErr := parse.Address(q.TokenIn, "tokenIn")
	tokenOut, outputErr := parse.Address(q.TokenOut, "tokenOut")
	amount, amountErr := parseUint256(q.Amount, "amount")
	if err := errors.Join(swapperErr, inputErr, outputErr, amountErr); err != nil {
		return nil, err
	}
	parsed := &parsedQuote{req: strategyRequest{RequestID: q.RequestID, QuoteID: q.QuoteID,
		TokenIn: tokenIn, TokenOut: tokenOut, Amount: amount}, inv: make([]solverInventory, len(q.Adapters))}
	for index := range q.Adapters {
		inventory, err := q.Adapters[index].parse(index, q.TokenInChainID, tokenIn)
		if err != nil {
			return nil, err
		}
		parsed.inv[index] = inventory
	}
	// Validate the entire payload before classifying a different chain as a decline.
	if q.TokenInChainID != chainID || q.TokenOutChainID != chainID || len(parsed.inv) == 0 {
		return nil, nil
	}
	return parsed, nil
}

func (v *quoteAdapter) parse(index int, chainID int64, tokenIn common.Address) (inventory solverInventory, err error) {
	defer func() {
		if err != nil {
			err = errors.Errorf("adapters[%d]: %w", index, err)
		}
	}()
	adapter, adapterErr := parse.Address(v.Adapter, "adapter")
	asset, assetErr := parse.Address(v.Asset, "asset")
	assets, assetsErr := parseUint256(v.MaxAssets, "maxAssets")
	rate, rateErr := parseUint256(v.MaxRate, "maxRate")
	if err := errors.Join(adapterErr, assetErr, assetsErr, rateErr); err != nil {
		return solverInventory{}, err
	}
	if v.AssetDecimals < 0 || v.AssetDecimals > 255 {
		return solverInventory{}, errors.Errorf("assetDecimals out of range: %d", v.AssetDecimals)
	}
	route := liquidlane.NewRoute(chainID, adapter, common.Address{}, tokenIn, asset, 0, v.AssetDecimals)
	if v.DiscountID == nil || *v.DiscountID == "" {
		return liquidlane.DirectInventory(route, assets, rate), nil
	}
	id, err := parse.Hash(*v.DiscountID, "discountId")
	if err != nil {
		return solverInventory{}, err
	}
	return liquidlane.DiscountInventory(route, assets, rate, id, time.Time{}), nil
}

func parseUint256(raw, field string) (*big.Int, error) { return parse.Uint(raw, field, 256) }
