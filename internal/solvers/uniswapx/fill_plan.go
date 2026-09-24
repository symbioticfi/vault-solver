package uniswapx

import (
	"context"
	"math/big"
	"slices"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquiddiscounts "github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

var errNoFillPlan = errors.New("no fill plan covers the awarded order")

type preparedFill struct {
	plan       *types.FillPlan
	data       []byte
	validUntil time.Time
}

func (s *Solver) prepareFill(
	ctx context.Context, order *resolvedOrder, input types.FillInput,
	routes []liquidlane.Route, now, observedAt time.Time,
) (preparedFill, error) {
	if !s.cfg.singleSource() {
		plan, err := s.decideFill(ctx, input)
		if err != nil {
			return preparedFill{}, err
		}
		if plan == nil || len(plan.Routes) == 0 {
			return preparedFill{}, errNoFillPlan
		}
		if err := validatePreparedFill(input, plan); err != nil {
			return preparedFill{}, err
		}
		prepared, err := s.preflightPlan(ctx, order, plan, routes, now)
		if unavailableFillSource(err) {
			return preparedFill{}, errNoFillPlan
		}
		return prepared, err
	}

	// Gas is paid by the sender: an awarded single-source order needs its promised
	// output, not a new margin for gas or price buffer. Transaction fee caps remain.
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
	var uncertain error
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
		prepared, err := s.preflightPlan(ctx, order, plan, routes, now)
		if err == nil {
			return prepared, nil
		}
		// Unclassified failures may be transient. Try alternatives now, but keep
		// the order retryable if no source can be verified.
		if !unavailableFillSource(err) {
			uncertain = err
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
	if uncertain != nil {
		return preparedFill{}, uncertain
	}
	return preparedFill{}, errNoFillPlan
}

func validatePreparedFill(input types.FillInput, plan *types.FillPlan) error {
	routes, err := strategies.ValidateFillRoutes(strategies.FillValidation{
		TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: input.AmountIn,
		RequiredAmountOut: input.OutputAmount, RequireSingleRoute: input.RequireSingleRoute,
		MaxRoutes: types.MaxRoutes, Quotes: input.Quotes, Reservations: input.Reservations,
		GasSnapshot: input.GasSnapshot, GasPrices: input.GasPrices, MaxFeePerGas: input.MaxFeePerGas,
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

func unavailableFillSource(err error) bool {
	if errors.Is(err, liquiddiscounts.ErrUnavailable) {
		return true
	}
	var rpcErr rpc.Error
	return errors.Is(err, errFillPreflight) && errors.As(err, &rpcErr) && rpcErr.ErrorCode() == 3
}
