// Package snapshot reads the common LiquidLane inventory, fill quote, and gas state
// consumed by protocol solvers.
package snapshot

import (
	"context"
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

// Quote contains direct and physical inventory plus optional gas state from the same decision boundary.
type Quote struct {
	Direct      []liquidlane.Inventory
	Physical    []liquidlane.Inventory
	GasSnapshot *liquidlanegas.Snapshot
	GasPrices   *liquidlanegas.PriceSnapshot
}

// Fill contains amount-specific direct and physical quotes plus optional current gas state.
type Fill struct {
	Direct      []liquidlane.FillQuote
	Physical    []liquidlane.FillQuote
	GasSnapshot *liquidlanegas.Snapshot
	GasPrices   *liquidlanegas.PriceSnapshot
	// Drops are the routes the reads left out, with why, for a fill that cannot be planned.
	Drops []liquidlane.Drop
}

type liquidReader interface {
	ResolveRoutes(ctx context.Context, adapters []common.Address) ([]liquidlane.Route, error)
	ReadInventory(ctx context.Context, routes []liquidlane.Route, trace ...liquidlane.DropTrace) ([]liquidlane.Inventory, error)
	FilterAuthorized(
		ctx context.Context,
		inventory []liquidlane.Inventory,
		filler common.Address,
		trace ...liquidlane.DropTrace,
	) ([]liquidlane.Inventory, error)
	ReadFillQuotes(
		ctx context.Context,
		routes []liquidlane.Route,
		tokenIn common.Address,
		amountIn *big.Int,
		trace ...liquidlane.DropTrace,
	) ([]liquidlane.FillQuote, error)
	FilterAuthorizedRoutes(
		ctx context.Context,
		routes []liquidlane.Route,
		filler common.Address,
		trace ...liquidlane.DropTrace,
	) ([]liquidlane.Route, error)
	ReadGasSnapshot(
		ctx context.Context, routes []liquidlane.Route, trace ...liquidlane.DropTrace,
	) (*liquidlanegas.Snapshot, error)
}

type gasReader interface {
	Validate(ctx context.Context, tokens []liquidlanegas.Token) error
	ValidateTokens(tokens []liquidlanegas.Token) error
	Read(ctx context.Context, tokens []liquidlanegas.Token) (*liquidlanegas.PriceSnapshot, error)
}

// Reader owns the protocol-neutral LiquidLane read path shared by solver integrations.
type Reader struct {
	liquid liquidReader
	gas    gasReader
}

func New(
	c *chain.Client, log logr.Logger, gasCfg *liquidlanegas.OracleConfig, liquidityLens common.Address,
) (*Reader, error) {
	var gas gasReader
	if gasCfg != nil {
		reader, err := liquidlanegas.NewOracleReader(c, *gasCfg)
		if err != nil {
			return nil, err
		}
		gas = reader
	}
	return newReader(liquidlane.NewReader(c, log, liquidityLens), gas), nil
}

func newReader(liquid liquidReader, gas gasReader) *Reader {
	return &Reader{liquid: liquid, gas: gas}
}

func (r *Reader) ResolveRoutes(ctx context.Context, adapters []common.Address) ([]liquidlane.Route, error) {
	return r.liquid.ResolveRoutes(ctx, adapters)
}

// TokenMetadata is optional observability data; custom/test readers need not provide it.
func (r *Reader) TokenMetadata() *chain.Decimals {
	if reader, ok := r.liquid.(*liquidlane.Reader); ok {
		return reader.TokenMetadata()
	}
	return nil
}

// ValidateGasOracles checks token coverage and the timestamp selector during startup.
func (r *Reader) ValidateGasOracles(ctx context.Context, routes []liquidlane.Route) error {
	if r.gas == nil {
		return nil
	}
	return r.gas.Validate(ctx, routeTokens(routes))
}

func (r *Reader) ValidateGasTokens(routes []liquidlane.Route) error {
	if r.gas == nil {
		return nil
	}
	return r.gas.ValidateTokens(routeTokens(routes))
}

func (r *Reader) FilterAuthorizedRoutes(
	ctx context.Context,
	routes []liquidlane.Route,
	executor common.Address,
	trace ...liquidlane.DropTrace,
) ([]liquidlane.Route, error) {
	return r.liquid.FilterAuthorizedRoutes(ctx, routes, executor, trace...)
}

// ReadFillQuotes reads amount-specific physical quotes without direct-route authorization or gas state.
func (r *Reader) ReadFillQuotes(
	ctx context.Context,
	routes []liquidlane.Route,
	tokenIn common.Address,
	amountIn *big.Int,
	trace ...liquidlane.DropTrace,
) ([]liquidlane.FillQuote, error) {
	return r.liquid.ReadFillQuotes(ctx, routes, tokenIn, amountIn, trace...)
}

// Quote reads inventory, its authorized direct subset and gas state. Every route a read leaves out,
// including physical routes not authorized for direct use, goes to trace.
func (r *Reader) Quote(
	ctx context.Context,
	routes []liquidlane.Route,
	executor common.Address,
	trace ...liquidlane.DropTrace,
) (Quote, error) {
	physical, err := r.liquid.ReadInventory(ctx, routes, trace...)
	if err != nil {
		return Quote{}, err
	}
	direct, err := r.liquid.FilterAuthorized(ctx, physical, executor, trace...)
	if err != nil {
		return Quote{}, err
	}
	gasSnapshot, prices, err := r.readGas(ctx, routes, trace)
	if err != nil {
		return Quote{}, err
	}
	return Quote{Direct: direct, Physical: physical, GasSnapshot: gasSnapshot, GasPrices: prices}, nil
}

// Fill reads amount-specific quotes, their authorized direct subset and gas state. Every route a read
// leaves out, including routes not authorized for direct use, goes to trace; Fill.Drops holds only the
// routes the quote read dropped, since unauthorized routes still price through discount quotes.
func (r *Reader) Fill(
	ctx context.Context,
	routes []liquidlane.Route,
	executor, tokenIn common.Address,
	amountIn *big.Int,
	trace ...liquidlane.DropTrace,
) (Fill, error) {
	var drops []liquidlane.Drop
	if len(trace) == 0 {
		trace = []liquidlane.DropTrace{liquidlane.LogDrops(ctx)}
	}
	quoteTrace := append(slices.Clone(trace), func(d liquidlane.Drop) { drops = append(drops, d) })
	physical, err := r.liquid.ReadFillQuotes(ctx, routes, tokenIn, amountIn, quoteTrace...)
	if err != nil {
		return Fill{}, err
	}
	authorized, err := r.liquid.FilterAuthorizedRoutes(ctx, routes, executor, trace...)
	if err != nil {
		return Fill{}, err
	}
	directRoute := make(map[liquidlane.RouteID]bool, len(authorized))
	for _, route := range authorized {
		directRoute[route.ID] = true
	}
	direct := make([]liquidlane.FillQuote, 0, len(physical))
	for _, quote := range physical {
		if directRoute[quote.ID] {
			direct = append(direct, quote)
		}
	}
	gasSnapshot, prices, err := r.readGas(ctx, routes, trace)
	if err != nil {
		return Fill{}, err
	}
	return Fill{Direct: direct, Physical: physical, GasSnapshot: gasSnapshot, GasPrices: prices, Drops: drops}, nil
}

func (r *Reader) readGas(
	ctx context.Context,
	routes []liquidlane.Route,
	trace []liquidlane.DropTrace,
) (*liquidlanegas.Snapshot, *liquidlanegas.PriceSnapshot, error) {
	if r.gas == nil {
		return nil, nil, nil
	}
	snapshot, err := r.liquid.ReadGasSnapshot(ctx, routes, trace...)
	if err != nil {
		return nil, nil, err
	}
	prices, err := r.gas.Read(ctx, routeTokens(routes))
	if err != nil {
		return nil, nil, err
	}
	return snapshot, prices, nil
}

func routeTokens(routes []liquidlane.Route) []liquidlanegas.Token {
	tokens := make([]liquidlanegas.Token, 0, len(routes))
	for _, route := range routes {
		tokens = append(tokens, liquidlanegas.Token{Address: route.TokenOut, Decimals: route.TokenOutDecimals})
	}
	return tokens
}
