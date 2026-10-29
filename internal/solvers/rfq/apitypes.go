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
	DiscountPrecision string  `json:"discountPrecision,omitempty"`
	Account           string  `json:"account,omitempty"`
	Price             string  `json:"price,omitempty" pattern:"^[0-9]+$" doc:"Undiscounted oracle price scaled by 1e18; required for signed-discount quotes"`
	TokenInDecimals   *int    `json:"tokenInDecimals,omitempty" minimum:"0" maximum:"255"`
	Adapter           string  `json:"adapter" pattern:"^0x[a-fA-F0-9]{40}$"`
	Asset             string  `json:"asset" pattern:"^0x[a-fA-F0-9]{40}$"`
	AssetDecimals     int     `json:"assetDecimals" minimum:"0" maximum:"255"`
	MaxAssets         string  `json:"maxAssets" pattern:"^[0-9]+$"`
	DiscountID        *string `json:"discountId,omitempty" pattern:"^0x[a-fA-F0-9]{64}$"`
	// Discount (ppm) is what the adapter takes off this entry's payout: the signed discount of a
	// discountId entry, otherwise the adapter's minDiscount. The adapter pays at most
	// floor(getAmountOut(amountIn) * (1e6 - discount) / 1e6).
	Discount string `json:"discount" pattern:"^[0-9]+$"`
	// BlockNumber is the block maxAssets was read at.
	BlockNumber string `json:"blockNumber" pattern:"^[0-9]+$"`
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
func (q *quoteRequest) toStrategy(chainID int64, recipeExecutor common.Address) (*parsedQuote, error) {
	// Schema-level validation (malformed → 400).
	if q.Type != quoteTypeExactInput {
		return nil, errors.Errorf("type must be %q", quoteTypeExactInput)
	}
	if q.Protocol != quoteProtocolV1 {
		return nil, errors.Errorf("protocol must be %q", quoteProtocolV1)
	}
	if q.NumOutputs <= 0 {
		return nil, errors.New("numOutputs must be positive")
	}
	if q.QuoteID == "" || q.RequestID == "" {
		return nil, errors.New("quoteId and requestId are required")
	}
	if !common.IsHexAddress(q.Swapper) {
		return nil, errors.Errorf("swapper: invalid address %q", q.Swapper)
	}
	tokenIn, err := parse.Address(q.TokenIn, "tokenIn")
	if err != nil {
		return nil, err
	}
	tokenOut, err := parse.Address(q.TokenOut, "tokenOut")
	if err != nil {
		return nil, err
	}
	amount, err := parseUint256(q.Amount, "amount")
	if err != nil {
		return nil, err
	}
	inv := make([]solverInventory, 0, len(q.Adapters))
	for i := range q.Adapters {
		entry, perr := q.Adapters[i].parse(i, q.TokenInChainID, tokenIn, recipeExecutor)
		if perr != nil {
			return nil, perr
		}
		inv = append(inv, entry)
	}

	// Quote-logic checks (well-formed but not for us → 204).
	if q.TokenInChainID != chainID || q.TokenOutChainID != chainID || len(q.Adapters) == 0 {
		return nil, nil
	}
	return &parsedQuote{
		req: strategyRequest{
			RequestID: q.RequestID,
			QuoteID:   q.QuoteID,
			TokenIn:   tokenIn,
			TokenOut:  tokenOut,
			Amount:    amount,
		},
		inv: inv,
	}, nil
}

func (v *quoteAdapter) parse(index int, chainID int64, tokenIn, recipeExecutor common.Address) (solverInventory, error) {
	adapter, err := parse.Address(v.Adapter, idxField(index, "adapter"))
	if err != nil {
		return solverInventory{}, err
	}
	asset, err := parse.Address(v.Asset, idxField(index, "asset"))
	if err != nil {
		return solverInventory{}, err
	}
	if v.AssetDecimals < 0 || v.AssetDecimals > 255 {
		return solverInventory{}, errors.Errorf("adapters[%d].assetDecimals out of range: %d", index, v.AssetDecimals)
	}
	maxAssets, err := parseUint256(v.MaxAssets, idxField(index, "maxAssets"))
	if err != nil {
		return solverInventory{}, err
	}
	var discountID *common.Hash
	if v.DiscountID != nil && *v.DiscountID != "" {
		h := common.HexToHash(*v.DiscountID)
		discountID = &h
	}
	discount, err := parseUint256(v.Discount, idxField(index, "discount"))
	if err != nil {
		return solverInventory{}, err
	}
	precision := liquidlane.DiscountPrecision
	if recipeExecutor != (common.Address{}) {
		if v.DiscountPrecision != "100000000" || discountID == nil {
			return solverInventory{}, errors.Errorf("adapters[%d]: connector requires signed discount and precision 100000000", index)
		}
		precision = 100_000_000
	} else if v.DiscountPrecision != "" && v.DiscountPrecision != "1000000" {
		return solverInventory{}, errors.Errorf("adapters[%d]: invalid adapter discount precision", index)
	}
	if discount.Cmp(big.NewInt(precision)) > 0 || (recipeExecutor != (common.Address{}) && discount.Cmp(big.NewInt(precision)) == 0) {
		return solverInventory{}, errors.Errorf("%s: must be <= %d", idxField(index, "discount"), precision)
	}
	blockNumber, err := strconv.ParseUint(v.BlockNumber, 10, 64)
	if err != nil {
		return solverInventory{}, errors.Errorf("%s: invalid block number %q", idxField(index, "blockNumber"), v.BlockNumber)
	}
	account := common.Address{}
	inputDecimals := 0
	var price *big.Int
	if discountID != nil {
		account, err = parse.Address(v.Account, idxField(index, "account"))
		if err != nil || account == (common.Address{}) {
			return solverInventory{}, errors.Errorf("%s: discounted quote requires a nonzero account", idxField(index, "account"))
		}
		if v.TokenInDecimals == nil || *v.TokenInDecimals < 0 || *v.TokenInDecimals > 255 {
			return solverInventory{}, errors.Errorf("%s: discounted quote requires token decimals", idxField(index, "tokenInDecimals"))
		}
		inputDecimals = *v.TokenInDecimals
		price, err = parseUint256(v.Price, idxField(index, "price"))
		if err != nil || price.Sign() <= 0 || price.BitLen() > 256 {
			return solverInventory{}, errors.Errorf("%s: discounted quote requires a positive uint256 price", idxField(index, "price"))
		}
	}
	route := liquidlane.NewRoute(chainID, adapter, account, tokenIn, asset, inputDecimals, v.AssetDecimals)
	// Backend snapshots price discounts; direct quotes are priced from the adapter.
	inventory := liquidlane.DirectInventory(route, maxAssets, nil)
	inventory.AdapterMinDiscount = discount
	if discountID != nil {
		inventory = liquidlane.DiscountInventory(route, maxAssets, nil, *discountID, time.Time{})
		inventory.Discount = discount
	}
	if recipeExecutor != (common.Address{}) {
		inventory.DiscountPrecision = precision
		inventory.RecipeExecutor = recipeExecutor
	}
	inventory.BlockNumber = blockNumber
	inventory.Price = price
	return inventory, nil
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
