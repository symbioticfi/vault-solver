package liquidlane

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

// Snapshot contains one coherent physical/authorized read and optional gas facts.
type Snapshot[T any] struct {
	Direct      []T
	Physical    []T
	GasSnapshot *liquidlanegas.Snapshot
	GasPrices   *liquidlanegas.PriceSnapshot
}

type QuoteSnapshot = Snapshot[Inventory]
type FillSnapshot = Snapshot[FillQuote]

type priceReader interface {
	ValidateTokens([]liquidlanegas.Token) error
	Read(ctx context.Context, tokens []liquidlanegas.Token, now time.Time) (*liquidlanegas.PriceSnapshot, error)
}

func (r *Reader) ValidateGasTokens(routes []Route) error {
	if r.gas == nil {
		return nil
	}
	return r.gas.ValidateTokens(gasTokens(routes))
}

func (r *Reader) Quote(
	ctx context.Context,
	routes []Route,
	executor common.Address,
	now time.Time,
) (QuoteSnapshot, error) {
	physical, err := r.ReadInventory(ctx, routes)
	if err != nil {
		return QuoteSnapshot{}, err
	}
	direct, err := r.FilterAuthorized(ctx, physical, executor)
	if err != nil {
		return QuoteSnapshot{}, err
	}
	gasSnapshot, prices, err := r.gasFacts(ctx, routes, now)
	if err != nil {
		return QuoteSnapshot{}, err
	}
	return QuoteSnapshot{Direct: direct, Physical: physical, GasSnapshot: gasSnapshot, GasPrices: prices}, nil
}

func (r *Reader) Fill(
	ctx context.Context,
	routes []Route,
	executor, token common.Address,
	amount *big.Int,
	now time.Time,
) (FillSnapshot, error) {
	physical, err := r.ReadFillQuotes(ctx, routes, token, amount)
	if err != nil {
		return FillSnapshot{}, err
	}
	authorized, err := r.FilterAuthorizedRoutes(ctx, routes, executor)
	if err != nil {
		return FillSnapshot{}, err
	}
	allowed := make(map[RouteID]bool, len(authorized))
	for _, route := range authorized {
		allowed[route.ID] = true
	}
	direct := make([]FillQuote, 0, len(physical))
	for _, quote := range physical {
		if allowed[quote.ID] {
			direct = append(direct, quote)
		}
	}
	gasSnapshot, prices, err := r.gasFacts(ctx, routes, now)
	if err != nil {
		return FillSnapshot{}, err
	}
	return FillSnapshot{Direct: direct, Physical: physical, GasSnapshot: gasSnapshot, GasPrices: prices}, nil
}

func (r *Reader) ReadGasPrices(
	ctx context.Context,
	tokens []liquidlanegas.Token,
	now time.Time,
) (*liquidlanegas.PriceSnapshot, error) {
	if r.gas == nil {
		return nil, nil
	}
	return r.gas.Read(ctx, tokens, now)
}

func (r *Reader) gasFacts(
	ctx context.Context,
	routes []Route,
	now time.Time,
) (*liquidlanegas.Snapshot, *liquidlanegas.PriceSnapshot, error) {
	if r.gas == nil {
		return nil, nil, nil
	}
	snapshot, err := r.ReadGasSnapshot(ctx, routes)
	if err != nil {
		return nil, nil, err
	}
	prices, err := r.gas.Read(ctx, gasTokens(routes), now)
	return snapshot, prices, err
}

func gasTokens(routes []Route) []liquidlanegas.Token {
	tokens := make([]liquidlanegas.Token, len(routes))
	for i, route := range routes {
		tokens[i] = liquidlanegas.Token{Address: route.TokenOut, Decimals: route.TokenOutDecimals}
	}
	return tokens
}
