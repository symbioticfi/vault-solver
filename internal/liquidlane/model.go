package liquidlane

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type Adapter struct {
	Adapter common.Address `json:"adapter"`
	Vault   common.Address `json:"vault"`

	TokenOut         common.Address `json:"tokenOut"`
	TokenOutDecimals int            `json:"tokenOutDecimals"`
}

type Route struct {
	ID               RouteID        `json:"id"`
	CapacityID       CapacityID     `json:"capacityId"`
	Adapter          common.Address `json:"adapter"`
	Vault            common.Address `json:"vault"`
	TokenIn          common.Address `json:"tokenIn"`
	TokenOut         common.Address `json:"tokenOut"`
	TokenInDecimals  int            `json:"tokenInDecimals"`
	TokenOutDecimals int            `json:"tokenOutDecimals"`
}

type Inventory struct {
	Route

	MaxAssets          *big.Int     `json:"maxAssets"`
	MaxRate            *big.Int     `json:"maxRate"`
	AdapterMinDiscount *big.Int     `json:"-"`
	DiscountID         *common.Hash `json:"discountId"`
	ValidUntil         time.Time    `json:"validUntil"`
}

type QuoteCandidate struct {
	ID           CandidateID  `json:"id"`
	Route        Route        `json:"route"`
	Rate         *big.Int     `json:"rate"`
	MaxAmountIn  *big.Int     `json:"maxAmountIn"`
	MaxAmountOut *big.Int     `json:"maxAmountOut"`
	DiscountID   *common.Hash `json:"discountId"`
	ValidUntil   time.Time    `json:"validUntil"`
}

type FillQuote struct {
	Inventory

	AmountIn       *big.Int `json:"amountIn"`
	GrossAmountOut *big.Int `json:"grossAmountOut"`
	MaxAmountOut   *big.Int `json:"maxAmountOut"`
	MinDiscount    *big.Int `json:"minDiscount"`
}

type Auth struct {
	Adapter     common.Address
	MarketMaker common.Address
	Owner       common.Address
	IsFiller    bool
	Authorized  bool
}

type AdapterSnapshot struct {
	Adapter

	Paused       bool
	Authorized   bool
	FreeAssets   *big.Int
	Withdrawable *big.Int
	Routes       []RouteSnapshot
}

type RouteSnapshot struct {
	Route

	MaxAssets      *big.Int
	MaxRate        *big.Int
	AcquireBalance *big.Int
}

func NewRoute(
	chainID int64,
	adapter common.Address,
	vault common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	tokenInDecimals int,
	tokenOutDecimals int,
) Route {
	return Route{
		ID:               NewRouteID(chainID, adapter, tokenIn, tokenOut),
		CapacityID:       NewCapacityID(chainID, vault, tokenOut),
		Adapter:          adapter,
		Vault:            vault,
		TokenIn:          tokenIn,
		TokenOut:         tokenOut,
		TokenInDecimals:  tokenInDecimals,
		TokenOutDecimals: tokenOutDecimals,
	}
}

func DirectInventory(route Route, maxAssets, maxRate *big.Int) Inventory {
	return Inventory{Route: route, MaxAssets: CloneBig(maxAssets), MaxRate: CloneBig(maxRate)}
}

func DiscountInventory(
	route Route,
	maxAssets, maxRate *big.Int,
	discountID common.Hash,
	validUntil time.Time,
) Inventory {
	return Inventory{
		Route:      route,
		MaxAssets:  CloneBig(maxAssets),
		MaxRate:    CloneBig(maxRate),
		DiscountID: CloneHash(&discountID),
		ValidUntil: validUntil,
	}
}

func CloneBig(value *big.Int) *big.Int {
	if value == nil {
		return nil
	}
	return new(big.Int).Set(value)
}

func CloneHash(value *common.Hash) *common.Hash {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
