package rfq

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	sdkparse "github.com/symbioticfi/merkle-executor/sdk/parse"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/vault-solver/api/bindings/erc20"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

type advertisedConnector struct {
	selectedRoute

	Account common.Address
	Offer   discounts.ListItem
}

// Use the same exact-input allocator as quoting, at fresh connector prices.
// Snapshot reservations are charged once per funding account and output asset.
func (s *Solver) splitLegs(ctx context.Context, request executor.IReactorRequest, asset common.Address, required, block *big.Int, now time.Time, routes []advertisedConnector) ([]connectorLeg, error) {
	distinct := make(map[common.Address]bool)
	for _, route := range routes {
		distinct[route.Venue] = true
	}
	if len(distinct) < 2 {
		return nil, nil
	}
	inventory, err := s.splitInventory(ctx, request.TokenIn, asset, block, now, routes)
	if err != nil {
		return nil, err
	}
	available := greedy.AllocateInventoryCapacity(inventory, s.Reservations(inventory), 0)
	candidates := make([]liquidlane.QuoteCandidate, 0, len(available))
	executors := make(map[liquidlane.RouteID]common.Address, len(available))
	for _, item := range available {
		if candidate := greedy.NewQuoteCandidate(item, item.MaxAssets); candidate != nil {
			candidates = append(candidates, *candidate)
			executors[item.ID] = item.RecipeExecutor
		}
	}
	solution, err := greedy.SolveQuote(strategies.QuoteTask{
		ExactInput: request.AmountIn, Candidates: candidates, MaxRoutes: len(distinct),
		InputPolicy: strategies.RejectUncoveredInput,
	})
	if err != nil || solution == nil || solution.AmountOut.Cmp(required) < 0 {
		return nil, err
	}
	legs := make([]connectorLeg, 0, len(solution.Allocations))
	for _, allocation := range solution.Allocations {
		c := allocation.Candidate
		legs = append(legs, connectorLeg{
			Venue: c.Route.Adapter, Executor: executors[c.Route.ID], DiscountID: *c.DiscountID, MaxAssets: c.MaxAmountOut,
			AmountIn: allocation.AmountIn,
		})
	}
	return legs, nil
}

func (s *Solver) splitInventory(ctx context.Context, token, asset common.Address, block *big.Int, now time.Time, routes []advertisedConnector) ([]liquidlane.Inventory, error) {
	tokenABI := erc20.NewERC20()
	decimals, err := protocolio.ReadGroup(ctx, s.workflow.chain,
		protocolio.ContractRead(token, tokenABI.PackDecimals(), block, true),
		protocolio.ContractRead(asset, tokenABI.PackDecimals(), block, true))
	if err != nil {
		return nil, errors.Errorf("read connector token decimals: %w", err)
	}
	readDecimals := func(index int) (int, error) {
		result := decimals.At(index)
		if result.Err != nil {
			return 0, result.Err
		}
		decimals, err := tokenABI.UnpackDecimals(result.Data)
		return int(decimals), err
	}
	inDec, err := readDecimals(0)
	if err != nil {
		return nil, errors.Errorf("connector input decimals: %w", err)
	}
	outDec, err := readDecimals(1)
	if err != nil {
		return nil, errors.Errorf("connector output decimals: %w", err)
	}
	probe, ok := liquidlane.OraclePriceProbe(inDec, outDec)
	if !ok || probe.BitLen() > 256 {
		return nil, errors.New("unsupported connector oracle price probe")
	}
	index := make(map[common.Address]int, len(routes))
	requests := make([]protocolio.ReadRequest, 0, len(routes)*3)
	for _, route := range routes {
		if _, seen := index[route.Venue]; seen {
			continue
		}
		index[route.Venue] = len(requests)
		requests = append(requests,
			protocolio.ContractRead(route.Venue, connectorBinding.PackGetAmountOut(token, probe), block, true),
			protocolio.ContractRead(route.Venue, connectorBinding.PackRemainingAssets(token), block, true),
			protocolio.ContractRead(route.Venue, connectorBinding.PackMinDiscount(token), block, true))
	}
	reads, err := protocolio.ReadGroup(ctx, s.workflow.chain, requests...)
	if err != nil {
		return nil, errors.Errorf("read split connector inventory: %w", err)
	}
	inventory := make([]liquidlane.Inventory, 0, len(routes))
	for _, route := range routes {
		deadline := time.Unix(route.Offer.Deadline, 0)
		discount, err := sdkparse.Uint256(route.Offer.Discount)
		if !deadline.After(now) || route.Offer.CollateralDecimals != outDec || err != nil || discount.Cmp(big.NewInt(connectorDiscountPrecision)) >= 0 {
			continue
		}
		i := index[route.Venue]
		priceRead, limitRead, minRead := reads.At(i), reads.At(i+1), reads.At(i+2)
		if priceRead.Err != nil || limitRead.Err != nil || minRead.Err != nil {
			observability.Log(ctx).V(1).Info("connector pricing read failed", "connector", route.Venue.Hex())
			continue
		}
		price, priceErr := connectorBinding.UnpackGetAmountOut(priceRead.Data)
		limit, limitErr := connectorBinding.UnpackRemainingAssets(limitRead.Data)
		minDiscount, minErr := connectorBinding.UnpackMinDiscount(minRead.Data)
		if priceErr != nil || limitErr != nil || minErr != nil || discount.Cmp(minDiscount) < 0 {
			continue
		}
		capacity := limit
		if route.MaxAssets.Cmp(capacity) < 0 {
			capacity = route.MaxAssets
		}
		path := liquidlane.NewRoute(s.workflow.chainID.Int64(), route.Venue, route.Account, token, asset, inDec, outDec)
		item := liquidlane.DiscountInventory(path, capacity, nil, route.DiscountID, deadline)
		item.DiscountPrecision = connectorDiscountPrecision
		item.RecipeExecutor = route.Executor
		item.Price, item.Discount = price, discount
		item.BlockNumber = route.Offer.BlockNumber
		inventory = append(inventory, item)
	}
	return greedy.FilterLiveInventory(inventory, now), nil
}
