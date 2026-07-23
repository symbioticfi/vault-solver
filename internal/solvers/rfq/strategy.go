package rfq

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
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

// solverInventory is one LiquidLane candidate leg; RFQ maps backend adapter snapshots and fill-time
// recovery reads into the shared LiquidLane inventory shape.
type solverInventory = liquidlane.Inventory

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
	for _, v := range inv {
		id := string(liquidlane.NewCandidateID(v.Route, v.DiscountID))
		candidates = append(candidates, types.QuoteCandidate{
			ID:            id,
			Adapter:       v.Adapter,
			Asset:         v.TokenOut,
			AssetDecimals: v.TokenOutDecimals,
			MaxAssets:     liquidlane.CloneBig(v.MaxAssets),
			MaxRate:       liquidlane.CloneBig(v.MaxRate),
			DiscountID:    liquidlane.CloneHash(v.DiscountID),
		})
	}
	return types.QuoteInput{
		RequestID:          req.RequestID,
		QuoteID:            req.QuoteID,
		ChainID:            chainID,
		Executor:           executor,
		TokenIn:            req.TokenIn,
		TokenOut:           req.TokenOut,
		AmountIn:           liquidlane.CloneBig(req.Amount),
		RequiredAmountOut:  liquidlane.CloneBig(required),
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
