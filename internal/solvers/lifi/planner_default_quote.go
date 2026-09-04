package lifi

import (
	"context"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidplanning "github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
)

type strategyPairKey struct {
	tokenIn        common.Address
	tokenOut       common.Address
	inputDecimals  int
	outputDecimals int
}

func (s *defaultPlanner) DecideQuotes(_ context.Context, input QuoteInput) (QuoteOutput, error) {
	if !input.QuoteExpiresAt.After(input.ServerTime) {
		return QuoteOutput{}, errors.New("quoteExpiresAt must be after serverTime")
	}
	validAfter := input.ChainTime
	if input.ServerTime.After(validAfter) {
		validAfter = input.ServerTime
	}
	groups := make(map[strategyPairKey][]liquidlane.Inventory)
	for _, item := range liquidplanning.FilterLiveInventory(
		input.Inventory,
		validAfter.Add(s.policy.ExecutionBuffer),
	) {
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
	out := QuoteOutput{Quotes: make([]Quote, 0, len(keys))}
	for _, key := range keys {
		_, candidates := s.policy.QuoteCandidates(groups[key], input.Reservations)
		pricing, err := liquidplanning.NewGasPricing(
			input.MaxFeePerGas,
			key.tokenOut,
			input.GasPrices,
			input.GasSnapshot,
			s.policy.InventoryReserveBps,
			liquidplanning.ExecutorGasEnvelope(),
		)
		if err != nil {
			return QuoteOutput{}, err
		}
		routeLimit := input.RouteLimit(key.tokenIn)
		ranges, used, err := s.buildQuoteRanges(candidates, routeLimit, pricing)
		if err != nil {
			return QuoteOutput{}, err
		}
		if len(ranges) == 0 {
			continue
		}
		expiry := quoteExpiry(input.QuoteExpiresAt, s.policy.ExecutionBuffer, used)
		if expiry <= input.ServerTime.Unix() {
			continue
		}
		out.Quotes = append(out.Quotes, Quote{
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
