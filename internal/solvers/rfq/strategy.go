package rfq

import (
	"math/big"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

func newStrategy(spec StrategyConfig, chainClient *chain.Client, log logr.Logger) (types.Strategy, error) {
	name := spec.Name
	if name == "" {
		name = defaultStrategyName
	}
	return strategies.New(name, spec.Config, strategies.Deps{Chain: chainClient, Log: log})
}

// solverInventory is one candidate adapter leg, taken from the backend quote request's snapshot
// (the filler does not re-read maxAssets/maxRate/decimals on-chain in the quote path). "adapter" is
// the address that fills (placed in the on-chain Swap's vault slot); "asset" is the output token.
type solverInventory struct {
	ID            string
	Adapter       common.Address
	Asset         common.Address
	AssetDecimals int
	MaxAssets     *big.Int
	MaxRate       *big.Int
	DiscountID    *common.Hash // nil for a direct leg; set for a discount leg
}

type fillLeg = types.FillLeg
type fillPlan = types.FillPlan

// strategyRequest is the subset of a quote request the selector needs.
type strategyRequest struct {
	RequestID string
	QuoteID   string
	TokenIn   common.Address
	TokenOut  common.Address
	Amount    *big.Int
}

func newQuoteInput(
	chainID int64,
	executor common.Address,
	req strategyRequest,
	inv []solverInventory,
	required *big.Int,
	requireSingleRoute bool,
	now time.Time,
) types.QuoteInput {
	candidates := make([]types.QuoteCandidate, 0, len(inv))
	for i, v := range inv {
		id := v.ID
		if id == "" {
			id = "candidate-" + strconv.Itoa(i)
		}
		candidates = append(candidates, types.QuoteCandidate{
			ID:            id,
			Adapter:       v.Adapter,
			Asset:         v.Asset,
			AssetDecimals: v.AssetDecimals,
			MaxAssets:     cloneBig(v.MaxAssets),
			MaxRate:       cloneBig(v.MaxRate),
			DiscountID:    cloneHash(v.DiscountID),
		})
	}
	return types.QuoteInput{
		RequestID:          req.RequestID,
		QuoteID:            req.QuoteID,
		ChainID:            chainID,
		Executor:           executor,
		TokenIn:            req.TokenIn,
		TokenOut:           req.TokenOut,
		AmountIn:           cloneBig(req.Amount),
		RequiredAmountOut:  cloneBig(required),
		RequireSingleRoute: requireSingleRoute,
		Candidates:         candidates,
		Now:                now,
	}
}

func newFillInput(
	chainID int64,
	executor common.Address,
	req strategyRequest,
	inv []solverInventory,
	required *big.Int,
	requireSingleRoute bool,
	now time.Time,
) types.FillInput {
	q := newQuoteInput(chainID, executor, req, inv, required, requireSingleRoute, now)
	return types.FillInput(q)
}

func validateSingleRoute(requireSingleRoute bool, legCount int) error {
	if requireSingleRoute && legCount != 1 {
		return errors.Errorf("single-route input requires exactly one leg, got %d", legCount)
	}
	return nil
}

func requiresSingleRoute(
	tokensToQuote string,
	permissionedTokens map[common.Address]bool,
	tokenIn common.Address,
) bool {
	return tokensToQuote == tokensToQuotePermissioned && permissionedTokens[tokenIn]
}

func cloneBig(n *big.Int) *big.Int {
	if n == nil {
		return nil
	}
	return new(big.Int).Set(n)
}

func cloneHash(h *common.Hash) *common.Hash {
	if h == nil {
		return nil
	}
	out := *h
	return &out
}
