// Package snapshot reads the common LiquidLane inventory, fill quote, and gas state
// consumed by protocol solvers.
package snapshot

import (
	"context"
	"math/big"
	"time"

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
}

type liquidReader interface {
	ResolveRoutes(ctx context.Context, adapters []common.Address) ([]liquidlane.Route, error)
	ReadInventory(ctx context.Context, routes []liquidlane.Route) ([]liquidlane.Inventory, error)
	FilterAuthorized(
		ctx context.Context,
		inventory []liquidlane.Inventory,
		filler common.Address,
	) ([]liquidlane.Inventory, error)
	ReadFillQuotes(
		ctx context.Context,
		routes []liquidlane.Route,
		tokenIn common.Address,
		amountIn *big.Int,
	) ([]liquidlane.FillQuote, error)
	FilterAuthorizedRoutes(
		ctx context.Context,
		routes []liquidlane.Route,
		filler common.Address,
	) ([]liquidlane.Route, error)
	ReadGasSnapshot(ctx context.Context, routes []liquidlane.Route) (*liquidlanegas.Snapshot, error)
}

type gasReader interface {
	ValidateTokens(tokens []liquidlanegas.Token) error
	Read(ctx context.Context, tokens []liquidlanegas.Token, now time.Time) (*liquidlanegas.PriceSnapshot, error)
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
) ([]liquidlane.Route, error) {
	return r.liquid.FilterAuthorizedRoutes(ctx, routes, executor)
}

// ReadFillQuotes reads amount-specific physical quotes without direct-route authorization or gas state.
func (r *Reader) ReadFillQuotes(
	ctx context.Context,
	routes []liquidlane.Route,
	tokenIn common.Address,
	amountIn *big.Int,
) ([]liquidlane.FillQuote, error) {
	return r.liquid.ReadFillQuotes(ctx, routes, tokenIn, amountIn)
}

func (r *Reader) Quote(
	ctx context.Context,
	routes []liquidlane.Route,
	executor common.Address,
	now time.Time,
) (Quote, error) {
	physical, err := r.liquid.ReadInventory(ctx, routes)
	if err != nil {
		return Quote{}, err
	}
	direct, err := r.liquid.FilterAuthorized(ctx, physical, executor)
	if err != nil {
		return Quote{}, err
	}
	gasSnapshot, prices, err := r.readGas(ctx, routes, now)
	if err != nil {
		return Quote{}, err
	}
	return Quote{Direct: direct, Physical: physical, GasSnapshot: gasSnapshot, GasPrices: prices}, nil
}

func (r *Reader) Fill(
	ctx context.Context,
	routes []liquidlane.Route,
	executor, tokenIn common.Address,
	amountIn *big.Int,
	now time.Time,
) (Fill, error) {
	physical, err := r.liquid.ReadFillQuotes(ctx, routes, tokenIn, amountIn)
	if err != nil {
		return Fill{}, err
	}
	authorized, err := r.liquid.FilterAuthorizedRoutes(ctx, routes, executor)
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
	gasSnapshot, prices, err := r.readGas(ctx, routes, now)
	if err != nil {
		return Fill{}, err
	}
	return Fill{Direct: direct, Physical: physical, GasSnapshot: gasSnapshot, GasPrices: prices}, nil
}

func (r *Reader) readGas(
	ctx context.Context,
	routes []liquidlane.Route,
	now time.Time,
) (*liquidlanegas.Snapshot, *liquidlanegas.PriceSnapshot, error) {
	if r.gas == nil {
		return nil, nil, nil
	}
	snapshot, err := r.liquid.ReadGasSnapshot(ctx, routes)
	if err != nil {
		return nil, nil, err
	}
	prices, err := r.gas.Read(ctx, routeTokens(routes), now)
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
