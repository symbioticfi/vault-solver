package liquidlane

import (
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type RouteID string
type CandidateID string
type CapacityID string

// DiscountPrecision is the LiquidLane parts-per-million denominator.
const DiscountPrecision int64 = 1_000_000

// Adapter is adapter-level LiquidLane metadata that is stable after startup.
type Adapter struct {
	Adapter common.Address `json:"adapter"`
	Vault   common.Address `json:"vault"`

	TokenOut         common.Address `json:"tokenOut"`
	TokenOutDecimals int            `json:"tokenOutDecimals"`
}

// Route is one LiquidLane adapter path: tokenIn -> adapter -> tokenOut.
type Route struct {
	ID         RouteID    `json:"id"`
	CapacityID CapacityID `json:"capacityId"`

	Adapter common.Address `json:"adapter"`
	Vault   common.Address `json:"vault"`

	TokenIn  common.Address `json:"tokenIn"`
	TokenOut common.Address `json:"tokenOut"`

	TokenInDecimals  int `json:"tokenInDecimals"`
	TokenOutDecimals int `json:"tokenOutDecimals"`
}

// Inventory is the current read-side liquidity/cap snapshot for one route.
type Inventory struct {
	Route

	MaxAssets *big.Int `json:"maxAssets"`
	MaxRate   *big.Int `json:"maxRate"`
	// AdapterMinDiscount is the adapter's current minimum accepted discount in parts per million.
	// It is a physical validation fact, not part of the strategy wire shape.
	AdapterMinDiscount *big.Int `json:"-"`
	// Price is the adapter oracle price (1e18) read with MaxRate, when known. With the payout
	// discount it prices amounts exactly like the adapter (see DiscountedAmountOut).
	Price *big.Int `json:"-"`
	// Discount is the signed discount (ppm) a discount-backed route pays with. Direct routes
	// leave it nil: their payout is bounded by AdapterMinDiscount instead.
	Discount *big.Int `json:"-"`
	// BlockNumber is the block the snapshot was read at when the source reports it; zero is unknown.
	// It is a freshness fact for reservation accounting, not part of the strategy wire shape.
	BlockNumber uint64 `json:"-"`

	DiscountID *common.Hash `json:"discountId"`

	ValidUntil time.Time `json:"validUntil"`
}

// QuoteCandidate is one amount-normalized route alternative ready for a
// LiquidLane quoting strategy. Candidates sharing Route.ID are mutually
// exclusive direct/private alternatives for the same physical route.
type QuoteCandidate struct {
	ID    CandidateID `json:"id"`
	Route Route       `json:"route"`

	Rate         *big.Int `json:"rate"`
	MaxAmountIn  *big.Int `json:"maxAmountIn"`
	MaxAmountOut *big.Int `json:"maxAmountOut"`

	DiscountID *common.Hash `json:"discountId"`
	ValidUntil time.Time    `json:"validUntil"`

	// Price and Discount, when both set, price amounts exactly like the adapter and Rate is
	// DiscountedRate(Price, Discount), kept for ranking. Otherwise amounts follow Rate.
	Price    *big.Int `json:"-"`
	Discount *big.Int `json:"-"`
}

// PayoutDiscount is the discount the adapter applies to this route's payout: the signed discount
// on discount-backed routes and the adapter minimum on direct ones. Nil when unknown.
func (i Inventory) PayoutDiscount() *big.Int {
	if i.DiscountID != nil {
		return i.Discount
	}
	return i.AdapterMinDiscount
}

// ExactPricing reports whether the candidate carries the adapter's price and payout discount.
func (c QuoteCandidate) ExactPricing() bool {
	return validPricing(c.Price, c.Discount)
}

// AmountOutFor returns the candidate's output for amountIn, before its MaxAmountOut cap.
func (c QuoteCandidate) AmountOutFor(amountIn *big.Int) *big.Int {
	if c.ExactPricing() {
		return DiscountedAmountOut(amountIn, c.Price, c.Discount, c.Route.TokenInDecimals, c.Route.TokenOutDecimals)
	}
	return AmountOutForRate(amountIn, c.Rate, c.Route.TokenInDecimals, c.Route.TokenOutDecimals)
}

// AmountInFor returns the smallest input whose output reaches amountOut.
func (c QuoteCandidate) AmountInFor(amountOut *big.Int) *big.Int {
	if c.ExactPricing() {
		return MinAmountInForDiscountedAmountOut(
			amountOut, c.Price, c.Discount, c.Route.TokenInDecimals, c.Route.TokenOutDecimals,
		)
	}
	return MinAmountInForAmountOut(amountOut, c.Rate, c.Route.TokenInDecimals, c.Route.TokenOutDecimals)
}

// FillQuote is a current adapter quote for one concrete amountIn.
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

// AdapterSnapshot is a current, solver-neutral view of one LiquidLane adapter and its routes.
type AdapterSnapshot struct {
	Adapter

	Paused       bool
	Authorized   bool
	FreeAssets   *big.Int
	Withdrawable *big.Int
	Routes       []RouteSnapshot
}

// RouteSnapshot combines route metadata with current inventory and adapter-local acquire liquidity.
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

func NewCapacityID(chainID int64, vault, tokenOut common.Address) CapacityID {
	return CapacityID(strings.ToLower(
		"capacity:" + strconv.FormatInt(chainID, 10) + ":" + vault.Hex() + ":" + tokenOut.Hex(),
	))
}

func RouteCapacityID(route Route) CapacityID {
	if route.CapacityID != "" {
		return route.CapacityID
	}
	return CapacityID(route.ID)
}

func NewRouteID(chainID int64, adapter, tokenIn, tokenOut common.Address) RouteID {
	return RouteID(strings.ToLower(
		"route:" + strconv.FormatInt(chainID, 10) + ":" + adapter.Hex() + ":" + tokenIn.Hex() + ":" + tokenOut.Hex(),
	))
}

func NewCandidateID(route Route, discountID *common.Hash) CandidateID {
	id := "candidate:" + string(route.ID)
	if discountID != nil {
		id += ":discount:" + discountID.Hex()
	}
	return CandidateID(strings.ToLower(id))
}

func DirectInventory(route Route, maxAssets, maxRate *big.Int) Inventory {
	return Inventory{
		Route:     route,
		MaxAssets: CloneBig(maxAssets),
		MaxRate:   CloneBig(maxRate),
	}
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

func CloneBig(n *big.Int) *big.Int {
	if n == nil {
		return nil
	}
	return new(big.Int).Set(n)
}

func CloneHash(h *common.Hash) *common.Hash {
	if h == nil {
		return nil
	}
	out := *h
	return &out
}
