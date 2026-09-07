// Package snapshot assembles the same direct/physical/gas decision view for
// inventory publication and amount-specific execution.
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

// View owns one read result. Direct is an authorized subset of Physical; both
// share immutable numeric values. All gas observations belong to this decision.
type View[T any] struct {
	Direct, Physical []T
	GasSnapshot      *liquidlanegas.Snapshot
	GasPrices        *liquidlanegas.PriceSnapshot
}

type Quote = View[liquidlane.Inventory]
type Fill = View[liquidlane.FillQuote]

type liquidReader interface {
	ResolveRoutes(ctx context.Context, adapters []common.Address) ([]liquidlane.Route, error)
	ReadInventory(ctx context.Context, routes []liquidlane.Route) ([]liquidlane.Inventory, error)
	ReadFillQuotes(ctx context.Context, routes []liquidlane.Route, tokenIn common.Address, amountIn *big.Int) ([]liquidlane.FillQuote, error)
	FilterAuthorizedRoutes(ctx context.Context, routes []liquidlane.Route, executor common.Address) ([]liquidlane.Route, error)
	ReadAdapterState(ctx context.Context, addresses []common.Address, executor common.Address, gasRoutes []liquidlane.Route) ([]liquidlane.Auth, *liquidlanegas.Snapshot, error)
}

type gasReader interface {
	ValidateTokens([]liquidlanegas.Token) error
	Read(ctx context.Context, tokens []liquidlanegas.Token, now time.Time) (*liquidlanegas.PriceSnapshot, error)
}

// Reader composes protocol-neutral chain reads and optional price feeds. It owns
// no cache or goroutine; every returned view is assembled for the caller's context.
type Reader struct {
	liquidReader

	gas gasReader
}

func New(c *chain.Client, log logr.Logger, gasCfg *liquidlanegas.OracleConfig, lens common.Address) (*Reader, error) {
	r := &Reader{liquidReader: liquidlane.NewReader(c, log, lens)}
	if gasCfg != nil {
		gas, err := liquidlanegas.NewOracleReader(c, *gasCfg)
		if err != nil {
			return nil, err
		}
		r.gas = gas
	}
	return r, nil
}

func (r *Reader) ValidateGasTokens(routes []liquidlane.Route) error {
	if r.gas != nil {
		return r.gas.ValidateTokens(routeTokens(routes))
	}
	return nil
}

func (r *Reader) Quote(ctx context.Context, routes []liquidlane.Route, executor common.Address, now time.Time) (Quote, error) {
	physical, err := r.ReadInventory(ctx, routes)
	if err != nil {
		return Quote{}, err
	}
	return assemble(ctx, r, routes, executor, now, physical, func(item liquidlane.Inventory) liquidlane.Route { return item.Route })
}

func (r *Reader) Fill(ctx context.Context, routes []liquidlane.Route, executor, tokenIn common.Address, amountIn *big.Int, now time.Time) (Fill, error) {
	physical, err := r.ReadFillQuotes(ctx, routes, tokenIn, amountIn)
	if err != nil {
		return Fill{}, err
	}
	return assemble(ctx, r, routes, executor, now, physical, func(item liquidlane.FillQuote) liquidlane.Route { return item.Route })
}

// Authorization is evaluated only for returned physical routes. Private routes
// keep the same physical observation even when direct access is unavailable.
func assemble[T any](ctx context.Context, r *Reader, routes []liquidlane.Route, executor common.Address,
	now time.Time, physical []T, routeOf func(T) liquidlane.Route,
) (View[T], error) {
	view := View[T]{Physical: physical}
	addresses := make([]common.Address, len(physical))
	for i, item := range physical {
		addresses[i] = routeOf(item).Adapter
	}
	gasRoutes := routes
	if r.gas == nil {
		gasRoutes = nil
	}
	auth, gas, err := r.ReadAdapterState(ctx, addresses, executor, gasRoutes)
	if err != nil {
		return View[T]{}, err
	}
	allowed := make(map[common.Address]bool, len(auth))
	for _, item := range auth {
		allowed[item.Adapter] = item.Authorized
	}
	for _, item := range physical {
		if allowed[routeOf(item).Adapter] {
			view.Direct = append(view.Direct, item)
		}
	}
	if r.gas == nil {
		return view, nil
	}
	prices, err := r.gas.Read(ctx, routeTokens(routes), now)
	if err != nil {
		return View[T]{}, err
	}
	view.GasSnapshot, view.GasPrices = gas, prices
	return view, nil
}

func routeTokens(routes []liquidlane.Route) []liquidlanegas.Token {
	result := make([]liquidlanegas.Token, len(routes))
	for i := range routes {
		result[i].Address, result[i].Decimals = routes[i].TokenOut, routes[i].TokenOutDecimals
	}
	return result
}
