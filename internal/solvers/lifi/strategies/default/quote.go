package defaultstrategy

import (
	"context"
	"fmt"
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
	liveInventory := liquidgreedy.FilterLiveInventory(input.Inventory, validAfter.Add(s.executionBuffer))
	allocatedInventory := liquidgreedy.AllocateInventoryCapacity(
		liveInventory, input.Reservations, s.cfg.InventoryReserveBps,
	)
	inputCounts, liveCounts := countByPair(input.Inventory), countByPair(liveInventory)
	groups := make(map[strategyPairKey][]liquidlane.Inventory)
	for _, item := range allocatedInventory {
		key := pairKey(item)
		groups[key] = append(groups[key], item)
	}

	keys := make([]strategyPairKey, 0, len(inputCounts))
	for key := range inputCounts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return pairLess(keys[i], keys[j]) })
	out := types.QuoteOutput{Quotes: make([]types.Quote, 0, len(keys))}
	decline := func(key strategyPairKey, format string, args ...any) {
		out.Declines = append(out.Declines, types.QuoteDecline{
			FromAsset: key.tokenIn, ToAsset: key.tokenOut, Reason: fmt.Sprintf(format, args...),
		})
	}
	for _, key := range keys {
		inventory := groups[key]
		if len(inventory) == 0 {
			decline(key, "no inventory with capacity: %d items, %d live past execution buffer, 0 with capacity left after reservations and reserve",
				inputCounts[key], liveCounts[key])
			continue
		}
		candidates := make([]liquidlane.QuoteCandidate, 0, len(inventory))
		trace, lastDecline := liquidstrategies.DecisionTrace(nil).RecordDeclines()
		for _, item := range inventory {
			candidate := liquidgreedy.NewQuoteCandidateWithTrace(item, liquidgreedy.QuoteCapacity(item, s.cfg.PriceBufferBps), trace)
			if candidate != nil {
				candidates = append(candidates, *candidate)
			}
		}
		if len(candidates) == 0 {
			decline(key, "no quote candidate from %d allocated routes; last decline: %s", len(inventory), lastDecline())
			continue
		}
		pricing, err := liquidstrategies.NewGasPricing(
			input.MaxFeePerGas, key.tokenOut, input.GasPrices, input.GasSnapshot, s.cfg.InventoryReserveBps,
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
		if errors.Is(err, errNoQuote) {
			decline(key, "%v (%d items, %d live, %d with capacity, %d candidates, route limit %d)",
				err, inputCounts[key], liveCounts[key], len(inventory), len(candidates), routeLimit)
			continue
		}
		if err != nil {
			return types.QuoteOutput{}, err
		}
		expiry := quoteExpiry(input.QuoteExpiresAt, s.executionBuffer, used)
		if expiry <= input.ServerTime.Unix() {
			decline(key, "quote expiry %d is not after server time %d (candidate validity less %s execution buffer)",
				expiry, input.ServerTime.Unix(), s.executionBuffer)
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

func pairKey(item liquidlane.Inventory) strategyPairKey {
	return strategyPairKey{
		tokenIn: item.TokenIn, tokenOut: item.TokenOut,
		inputDecimals: item.TokenInDecimals, outputDecimals: item.TokenOutDecimals,
	}
}

func countByPair(inventory []liquidlane.Inventory) map[strategyPairKey]int {
	counts := make(map[strategyPairKey]int)
	for _, item := range inventory {
		counts[pairKey(item)]++
	}
	return counts
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
