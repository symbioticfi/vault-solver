package uniswapx

import (
	"context"
	"math/big"
	"slices"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

type preparedFill struct {
	plan       *types.FillPlan
	data       []byte
	validUntil time.Time
}

// Keep physical budgets separate from advertised discount limits and source subsets.
func fillCapacityLimits(snapshot fillSnapshot) map[liquidlane.CapacityID]*big.Int {
	limits := make(map[liquidlane.CapacityID]*big.Int)
	for _, quotes := range [][]liquidlane.FillQuote{snapshot.Physical, snapshot.Direct} {
		for _, quote := range quotes {
			if quote.MaxAssets == nil || quote.MaxAssets.Sign() <= 0 {
				continue
			}
			id := liquidlane.RouteCapacityID(quote.Route)
			if limits[id] == nil || quote.MaxAssets.Cmp(limits[id]) > 0 {
				limits[id] = liquidlane.CloneBig(quote.MaxAssets)
			}
		}
	}
	return limits
}

func (s *Solver) prepareFill(
	ctx context.Context, order *resolvedOrder, input types.FillInput,
	routes []liquidlane.Route, now, observedAt time.Time, revision uint64,
) (preparedFill, error) {
	tryPlan := func(plan *types.FillPlan) (preparedFill, error) {
		reservations, ok := strategies.FillRouteReservations(plan.Routes)
		if !ok {
			return preparedFill{}, errors.New("strategy returned invalid capacity reservations")
		}
		if !s.setPendingReservations(ctx, order.Hash, reservations, revision) {
			return preparedFill{}, declineFill(ctx, "fill_declined", "capacity changed during planning")
		}
		revision++ // Only our own replacement may advance the version accepted by this attempt.
		return s.preflightPlan(ctx, order, plan, routes, now)
	}
	if !s.cfg.singleSource() {
		plan, err := s.decideFill(ctx, input)
		if err != nil {
			return preparedFill{}, err
		}
		if plan == nil || len(plan.Routes) == 0 {
			return preparedFill{}, declineFill(ctx, "fill_declined", "strategy returned no fill plan")
		}
		if err := validatePreparedFill(input, plan); err != nil {
			return preparedFill{}, err
		}
		return tryPlan(plan)
	}

	// Gas is paid by the sender; single-source fills retain the configured price
	// buffer without requiring additional gas repayment. Transaction fee caps remain.
	input.MaxFeePerGas = new(big.Int)
	deadline := observedAt.Add(time.Unix(int64(order.Deadline), 0).Sub(now))
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	remaining := slices.Clone(input.Quotes)
	var preferred liquidlane.CandidateID
	// Exclusivity is known only once the signed order arrives. The cache TTL
	// bounds retention; chain time decides whether its source preference still applies.
	if order.ExclusiveUntil == 0 || uint64(now.Unix()) <= order.ExclusiveUntil {
		preferred = s.preferredSource(order, time.Now())
	}
	var sourceErr error
	for len(remaining) > 0 {
		if err := ctx.Err(); err != nil {
			return preparedFill{}, err
		}
		input.Quotes = remaining
		tryingPreferred := false
		if preferred != "" {
			for _, quote := range remaining {
				if liquidlane.NewCandidateID(quote.Route, quote.DiscountID) == preferred {
					input.Quotes = []liquidlane.FillQuote{quote}
					tryingPreferred = true
					break
				}
			}
			preferred = ""
		}
		plan, err := s.decideFill(ctx, input)
		if err != nil {
			return preparedFill{}, err
		}
		if plan == nil || len(plan.Routes) == 0 {
			if tryingPreferred {
				continue
			}
			break
		}
		if err := validatePreparedFill(input, plan); err != nil {
			return preparedFill{}, err
		}
		prepared, err := tryPlan(plan)
		if err == nil || errors.Is(err, errOrderNotFillable) {
			return prepared, err
		}
		// Try alternatives within this attempt, retaining preflight failures for
		// the existing retry backoff and breaker if none succeeds.
		if sourceErr == nil || errors.Is(err, errFillPreflight) {
			sourceErr = err
		}
		selected := plan.Routes[0].CandidateID
		observability.Log(ctx).V(1).Info("try another single source", "candidateId", selected, "error", err.Error())
		remaining = slices.DeleteFunc(remaining, func(quote liquidlane.FillQuote) bool {
			return liquidlane.NewCandidateID(quote.Route, quote.DiscountID) == selected
		})
	}
	if err := ctx.Err(); err != nil {
		return preparedFill{}, err
	}
	if sourceErr != nil {
		return preparedFill{}, sourceErr
	}
	return preparedFill{}, declineFill(ctx, "fill_declined", "no source covers the awarded output")
}

func validatePreparedFill(input types.FillInput, plan *types.FillPlan) error {
	routes, err := strategies.ValidateFillRoutes(strategies.FillValidation{
		TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: input.AmountIn,
		RequiredAmountOut: input.OutputAmount, RequireSingleRoute: input.RequireSingleRoute,
		MaxRoutes: types.MaxRoutes, Quotes: input.Quotes, Reservations: input.Reservations,
		CapacityLimits: input.CapacityLimits,
		GasSnapshot:    input.GasSnapshot, GasPrices: input.GasPrices, MaxFeePerGas: input.MaxFeePerGas,
		GasEnvelope: types.LiquidLaneGasEnvelope(),
	}, plan.Routes)
	if err != nil {
		return errors.Errorf("strategy returned invalid fill plan: %w", err)
	}
	plan.Routes = routes
	return nil
}

func (s *Solver) preflightPlan(
	ctx context.Context, order *resolvedOrder, plan *types.FillPlan,
	routes []liquidlane.Route, now time.Time,
) (preparedFill, error) {
	data, until, err := s.buildExecutorCalldata(ctx, order, plan, routes, now)
	if err != nil {
		return preparedFill{}, err
	}
	if _, err := s.chain.CallContract(ctx, ethereum.CallMsg{From: s.solverAddress, To: &order.Executor, Data: data}, nil); err != nil {
		return preparedFill{}, errors.Errorf("%w: %w", errFillPreflight, err)
	}
	return preparedFill{plan: plan, data: data, validUntil: until}, nil
}
