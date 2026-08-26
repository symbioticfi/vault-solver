package defaultstrategy

import (
	"context"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidstrategies "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

type strategyPairKey struct {
	tokenIn        common.Address
	tokenOut       common.Address
	inputDecimals  int
	outputDecimals int
}

func (s *Strategy) DecideQuotes(_ context.Context, input types.QuoteInput) (types.QuoteOutput, error) {
	if !input.QuoteExpiresAt.After(input.ServerTime) {
		return types.QuoteOutput{}, errors.New("quoteExpiresAt must be after serverTime")
	}
	validAfter := input.ChainTime
	if input.ServerTime.After(validAfter) {
		validAfter = input.ServerTime
	}
	groups := make(map[strategyPairKey][]liquidlane.Inventory)
	for _, item := range liquidgreedy.FilterLiveInventory(input.Inventory, validAfter.Add(s.executionBuffer)) {
		key := strategyPairKey{
			tokenIn: item.TokenIn, tokenOut: item.TokenOut,
			inputDecimals: item.TokenInDecimals, outputDecimals: item.TokenOutDecimals,
		}
		groups[key] = append(groups[key], item)
	}

	keys := make([]strategyPairKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return pairLess(keys[i], keys[j]) })
	out := types.QuoteOutput{Quotes: make([]types.Quote, 0, len(keys))}
	for _, key := range keys {
		inventory := liquidgreedy.AllocateInventoryCapacity(
			groups[key],
			input.Reservations,
			s.inventoryReserveBps,
		)
		candidates := make([]liquidlane.QuoteCandidate, 0, len(inventory))
		for _, item := range inventory {
			candidate := liquidgreedy.NewQuoteCandidate(
				item,
				liquidgreedy.QuoteCapacity(item, s.priceBufferBps),
			)
			if candidate != nil {
				candidates = append(candidates, *candidate)
			}
		}
		pricing, err := liquidstrategies.NewGasPricing(
			input.MaxFeePerGas, key.tokenOut, input.GasPrices, input.GasSnapshot, s.inventoryReserveBps,
			types.LiquidLaneGasEnvelope(),
		)
		if err != nil {
			return types.QuoteOutput{}, err
		}
		routeLimit := types.MaxRoutes
		if input.SingleRouteTokens[key.tokenIn] {
			routeLimit = 1
		}
		ranges, used, err := s.buildQuoteRanges(candidates, routeLimit, pricing)
		if err != nil {
			return types.QuoteOutput{}, err
		}
		if len(ranges) == 0 {
			continue
		}
		expiry := quoteExpiry(input.QuoteExpiresAt, s.executionBuffer, used)
		if expiry <= input.ServerTime.Unix() {
			continue
		}
		out.Quotes = append(out.Quotes, types.Quote{
			FromAsset: key.tokenIn, ToAsset: key.tokenOut,
			FromDecimals: key.inputDecimals, ToDecimals: key.outputDecimals,
			Ranges: ranges, Expiry: expiry, ExclusiveFor: input.Solver,
		})
	}
	return out, nil
}

func pairLess(left, right strategyPairKey) bool {
	if cmp := left.tokenIn.Cmp(right.tokenIn); cmp != 0 {
		return cmp < 0
	}
	if cmp := left.tokenOut.Cmp(right.tokenOut); cmp != 0 {
		return cmp < 0
	}
	if left.inputDecimals != right.inputDecimals {
		return left.inputDecimals < right.inputDecimals
	}
	return left.outputDecimals < right.outputDecimals
}
