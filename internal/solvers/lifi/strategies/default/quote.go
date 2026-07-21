package defaultstrategy

import (
	"context"
	"math/big"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

type quoteCandidate struct {
	liquidlane.Inventory

	maxInput *big.Int
}

func (c quoteCandidate) id() liquidlane.CandidateID {
	return liquidlane.NewCandidateID(c.Route, c.DiscountID)
}

type quoteRoute struct {
	id           liquidlane.RouteID
	alternatives []quoteCandidate
	maxInput     *big.Int
	bestRate     *big.Int
}

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
	inventory := s.allocateQuoteCapacity(
		filterQuoteInventory(input.Inventory, input.ChainTime.Add(s.executionBuffer)),
		input.Reservations,
	)
	groups := make(map[strategyPairKey][]quoteCandidate)
	for _, item := range inventory {
		candidate := s.newQuoteCandidate(item)
		if candidate == nil {
			continue
		}
		key := strategyPairKey{
			tokenIn: item.TokenIn, tokenOut: item.TokenOut,
			inputDecimals: item.TokenInDecimals, outputDecimals: item.TokenOutDecimals,
		}
		groups[key] = append(groups[key], *candidate)
	}

	keys := make([]strategyPairKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return pairLess(keys[i], keys[j]) })
	out := types.QuoteOutput{Quotes: make([]types.Quote, 0, len(keys))}
	for _, key := range keys {
		pricing, err := newGasPricing(
			input.MaxFeePerGas, key.tokenOut, input.GasPrices, input.GasSnapshot, s.cfg.InventoryReserveBps,
		)
		if err != nil {
			return types.QuoteOutput{}, err
		}
		ladder := buildQuoteLadder(groups[key])
		routeLimit := types.MaxRoutes
		if input.SingleRouteTokens[key.tokenIn] {
			routeLimit = 1
		}
		if len(ladder) > routeLimit {
			ladder = ladder[:routeLimit]
		}
		ranges, used := s.buildQuoteRanges(ladder, pricing)
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

func filterQuoteInventory(inventory []liquidlane.Inventory, validAfter time.Time) []liquidlane.Inventory {
	seen := make(map[liquidlane.CandidateID]bool, len(inventory))
	out := make([]liquidlane.Inventory, 0, len(inventory))
	for _, candidate := range inventory {
		id := liquidlane.NewCandidateID(candidate.Route, candidate.DiscountID)
		if (!candidate.ValidUntil.IsZero() && !candidate.ValidUntil.After(validAfter)) || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, candidate)
	}
	return out
}

func (s *Strategy) newQuoteCandidate(item liquidlane.Inventory) *quoteCandidate {
	if item.MaxAssets == nil || item.MaxAssets.Sign() <= 0 || item.MaxRate == nil || item.MaxRate.Sign() <= 0 {
		return nil
	}
	maxInput := liquidlane.MaxAmountInForRate(
		s.quoteCapacity(item), item.MaxRate, item.TokenInDecimals, item.TokenOutDecimals,
	)
	if maxInput.Sign() <= 0 ||
		liquidlane.AmountOutForRate(maxInput, item.MaxRate, item.TokenInDecimals, item.TokenOutDecimals).Sign() <= 0 {
		return nil
	}
	return &quoteCandidate{Inventory: item, maxInput: maxInput}
}

func buildQuoteLadder(candidates []quoteCandidate) []quoteRoute {
	byRoute := make(map[liquidlane.RouteID][]quoteCandidate)
	for _, candidate := range candidates {
		byRoute[candidate.ID] = append(byRoute[candidate.ID], candidate)
	}
	ladder := make([]quoteRoute, 0, len(byRoute))
	for routeID, candidates := range byRoute {
		alternatives := bestQuoteAlternatives(candidates)
		route := quoteRoute{
			id: routeID, alternatives: alternatives, maxInput: new(big.Int), bestRate: new(big.Int),
		}
		for _, candidate := range alternatives {
			if candidate.maxInput.Cmp(route.maxInput) > 0 {
				route.maxInput.Set(candidate.maxInput)
			}
			if candidate.MaxRate.Cmp(route.bestRate) > 0 {
				route.bestRate.Set(candidate.MaxRate)
			}
		}
		ladder = append(ladder, route)
	}
	sort.Slice(ladder, func(i, j int) bool {
		if cmp := ladder[i].bestRate.Cmp(ladder[j].bestRate); cmp != 0 {
			return cmp > 0
		}
		if cmp := ladder[i].maxInput.Cmp(ladder[j].maxInput); cmp != 0 {
			return cmp > 0
		}
		return ladder[i].id < ladder[j].id
	})
	return ladder
}

// bestQuoteAlternatives keeps one direct fallback and one private alternative per physical route.
func bestQuoteAlternatives(candidates []quoteCandidate) []quoteCandidate {
	var direct, private quoteCandidate
	var hasDirect, hasPrivate bool
	for i := range candidates {
		candidate := candidates[i]
		if candidate.DiscountID == nil {
			if !hasDirect || quoteCandidateBetter(candidate, direct) {
				direct, hasDirect = candidate, true
			}
		} else if !hasPrivate || quoteCandidateBetter(candidate, private) {
			private, hasPrivate = candidate, true
		}
	}
	alternatives := make([]quoteCandidate, 0, 2)
	if hasDirect {
		alternatives = append(alternatives, direct)
	}
	if hasPrivate {
		alternatives = append(alternatives, private)
	}
	return alternatives
}

func quoteCandidateBetter(left, right quoteCandidate) bool {
	if cmp := left.MaxRate.Cmp(right.MaxRate); cmp != 0 {
		return cmp > 0
	}
	if cmp := left.maxInput.Cmp(right.maxInput); cmp != 0 {
		return cmp > 0
	}
	return preferQuoteCandidate(left, right)
}

func preferQuoteCandidate(left, right quoteCandidate) bool {
	leftDirect := left.DiscountID == nil
	rightDirect := right.DiscountID == nil
	if leftDirect != rightDirect {
		return leftDirect
	}
	if left.ValidUntil.IsZero() != right.ValidUntil.IsZero() {
		return left.ValidUntil.IsZero()
	}
	if !left.ValidUntil.Equal(right.ValidUntil) {
		return left.ValidUntil.After(right.ValidUntil)
	}
	return left.id() < right.id()
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
