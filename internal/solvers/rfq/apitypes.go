package rfq

import (
	"math/big"
	"strconv"
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
	req quoteRequestFacts
	inv []liquidlane.Inventory
}

type quoteAmounts struct {
	tokenIn  common.Address
	tokenOut common.Address
	amount   *big.Int
}

// toStrategy validates and parses the request. A non-nil error is a malformed payload (→ 400). A nil
// result with nil error means the request is well-formed but not quotable here — wrong chain or no
// adapters — and the caller replies 204.
func (q *quoteRequest) toStrategy(chainID int64) (*parsedQuote, error) {
	if err := q.validateEnvelope(); err != nil {
		return nil, err
	}
	amounts, err := q.parseAmounts()
	if err != nil {
		return nil, err
	}
	inventory, err := q.parseInventory(amounts.tokenIn)
	if err != nil {
		return nil, err
	}

	// Well-formed requests outside this solver's lane are declined, not rejected.
	if q.TokenInChainID != chainID || q.TokenOutChainID != chainID || len(inventory) == 0 {
		return nil, nil
	}
	return &parsedQuote{
		req: quoteRequestFacts{
			RequestID: q.RequestID, QuoteID: q.QuoteID,
			TokenIn: amounts.tokenIn, TokenOut: amounts.tokenOut, Amount: amounts.amount,
		},
		inv: inventory,
	}, nil
}

func (q *quoteRequest) validateEnvelope() error {
	if q.Type != quoteTypeExactInput {
		return errors.Errorf("type must be %q", quoteTypeExactInput)
	}
	if q.Protocol != quoteProtocolV1 {
		return errors.Errorf("protocol must be %q", quoteProtocolV1)
	}
	if q.NumOutputs <= 0 {
		return errors.New("numOutputs must be positive")
	}
	if q.QuoteID == "" || q.RequestID == "" {
		return errors.New("quoteId and requestId are required")
	}
	if !common.IsHexAddress(q.Swapper) {
		return errors.Errorf("swapper: invalid address %q", q.Swapper)
	}
	return nil
}

func (q *quoteRequest) parseAmounts() (quoteAmounts, error) {
	tokenIn, err := parse.Address(q.TokenIn, "tokenIn")
	if err != nil {
		return quoteAmounts{}, err
	}
	tokenOut, err := parse.Address(q.TokenOut, "tokenOut")
	if err != nil {
		return quoteAmounts{}, err
	}
	amount, err := parseUint256(q.Amount, "amount")
	if err != nil {
		return quoteAmounts{}, err
	}
	return quoteAmounts{tokenIn: tokenIn, tokenOut: tokenOut, amount: amount}, nil
}

func (q *quoteRequest) parseInventory(tokenIn common.Address) ([]liquidlane.Inventory, error) {
	inventory := make([]liquidlane.Inventory, 0, len(q.Adapters))
	for i := range q.Adapters {
		entry, perr := q.Adapters[i].parse(i, q.TokenInChainID, tokenIn)
		if perr != nil {
			return nil, perr
		}
		inventory = append(inventory, entry)
	}
	return inventory, nil
}

func (v *quoteAdapter) parse(index int, chainID int64, tokenIn common.Address) (liquidlane.Inventory, error) {
	adapter, err := parse.Address(v.Adapter, idxField(index, "adapter"))
	if err != nil {
		return liquidlane.Inventory{}, err
	}
	asset, err := parse.Address(v.Asset, idxField(index, "asset"))
	if err != nil {
		return liquidlane.Inventory{}, err
	}
	if v.AssetDecimals < 0 || v.AssetDecimals > 255 {
		return liquidlane.Inventory{}, errors.Errorf("adapters[%d].assetDecimals out of range: %d", index, v.AssetDecimals)
	}
	maxAssets, err := parseUint256(v.MaxAssets, idxField(index, "maxAssets"))
	if err != nil {
		return liquidlane.Inventory{}, err
	}
	maxRate, err := parseUint256(v.MaxRate, idxField(index, "maxRate"))
	if err != nil {
		return liquidlane.Inventory{}, err
	}
	var discountID *common.Hash
	if v.DiscountID != nil && *v.DiscountID != "" {
		h := common.HexToHash(*v.DiscountID)
		discountID = &h
	}
	route := liquidlane.NewRoute(chainID, adapter, common.Address{}, tokenIn, asset, 0, v.AssetDecimals)
	if discountID != nil {
		return liquidlane.DiscountInventory(route, maxAssets, maxRate, *discountID, time.Time{}), nil
	}
	return liquidlane.DirectInventory(route, maxAssets, maxRate), nil
}

// parseUint256 parses a base-10 non-negative integer string into a big.Int.
func parseUint256(s, field string) (*big.Int, error) {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Sign() < 0 {
		return nil, errors.Errorf("%s: invalid non-negative integer %q", field, s)
	}
	return n, nil
}

func idxField(i int, field string) string {
	return "adapters[" + strconv.Itoa(i) + "]." + field
}
