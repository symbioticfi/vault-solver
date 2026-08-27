package rfq

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
)

func newStrategy(spec StrategyConfig) (types.Strategy, error) {
	return strategies.New(spec.Name, spec.Config)
}

func validateStrategyConfig(spec StrategyConfig) error {
	return strategies.Validate(spec.Name, spec.Config)
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
	candidates []liquidlane.QuoteCandidate,
	required *big.Int,
	requireSingleRoute bool,
	now time.Time,
) types.QuoteInput {
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

func validateSingleRoute(requireSingleRoute bool, legCount int) error {
	if requireSingleRoute && legCount != 1 {
		return errors.Errorf("single-route input requires exactly one leg, got %d", legCount)
	}
	return nil
}
