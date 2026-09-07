package rfq

import (
	"math/big"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	local "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
	remote "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/webhook"
)

func newStrategy(spec StrategyConfig) (types.Strategy, error) {
	switch spec.Name {
	case "", local.Name:
		return local.NewFromConfig(spec.Config)
	case remote.Name:
		return remote.NewFromConfig(spec.Config)
	default:
		return nil, errors.Errorf("unknown RFQ strategy %q (available: default, webhook)", spec.Name)
	}
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
		AmountIn:           bigmath.Clone(req.Amount),
		RequiredAmountOut:  bigmath.Clone(required),
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
