package uniswapx

import (
	"context"
	"math/big"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	uxexecutor "github.com/symbioticfi/vault-solver/api/bindings/uniswapx/executor"
	"github.com/symbioticfi/vault-solver/internal/capacity"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquiddiscounts "github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	liquidplanning "github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

var (
	errOrderNotFillable = errors.New("order is not fillable at current chain state")
	errFillPreflight    = errors.New("fill preflight failed")
)

type fillFacts struct {
	input             FillInput
	decisionRoutes    []liquidlane.Route
	pricingMaxFee     *big.Int
	transactionMaxFee *big.Int
}

func (s *Solver) startFill(
	ctx context.Context,
	routes []liquidlane.Route,
	order *resolvedOrder,
	now time.Time,
	chainObservedAt time.Time,
) error {
	if order.TokenOut == (common.Address{}) {
		return errOrderNotFillable
	}
	if order.Deadline == 0 || int64(order.Deadline) <= now.Unix() {
		return errOrderNotFillable
	}
	facts, err := s.loadFillFacts(ctx, routes, order, now)
	if err != nil {
		return err
	}
	plan, reservations, err := s.chooseFillPlan(ctx, order, facts.input)
	if err != nil {
		return err
	}
	return s.submitFillPlan(ctx, order, facts, plan, reservations, now, chainObservedAt)
}

func (s *Solver) loadFillFacts(
	ctx context.Context,
	routes []liquidlane.Route,
	order *resolvedOrder,
	now time.Time,
) (*fillFacts, error) {
	decisionRoutes, listed, discountErr := s.fillRoutesWithDiscounts(
		ctx,
		routes,
		order.TokenIn,
		order.TokenOut,
		now,
	)
	if discountErr != nil {
		s.log.Error(discountErr, "refresh fill discount routes", "orderHash", order.Hash.Hex())
	}
	snapshot, err := s.reader.Fill(
		ctx,
		decisionRoutes,
		order.Executor,
		order.TokenIn,
		order.AmountIn,
		now,
	)
	if err != nil {
		return nil, err
	}
	if s.cfg.usesDiscounts() {
		snapshot.Direct = directFillQuotesForAdapters(snapshot.Direct, s.cfg.Adapters)
		if listed != nil {
			snapshot.Direct = append(snapshot.Direct, s.discountFillQuotes(listed, snapshot.Physical, now)...)
		}
	}
	s.log.V(1).Info(
		"order fill snapshot loaded",
		"source", order.Source,
		"orderHash", order.Hash.Hex(),
		"quoteId", order.QuoteID,
		"routes", len(decisionRoutes),
		"fillQuotes", len(snapshot.Direct),
		"physicalQuotes", len(snapshot.Physical),
	)
	pricingMaxFee := new(big.Int)
	var transactionMaxFee *big.Int
	if s.cfg.Gas != nil {
		maxFee, err := s.txm.MaxFeePerGas(ctx)
		if err != nil {
			return nil, err
		}
		pricingMaxFee = maxFee
		transactionMaxFee = new(big.Int).Set(maxFee)
	}
	input := FillInput{
		OrderID: order.Hash.Hex(), QuoteID: order.QuoteID,
		TokenIn: order.TokenIn, TokenOut: order.TokenOut, AmountIn: order.AmountIn, OutputAmount: order.AmountOut,
		Deadline:           order.Deadline,
		RequireSingleRoute: s.cfg.TokenPolicy.RequiresSingleRoute(order.TokenIn), Quotes: snapshot.Direct,
		Reservations: s.capacity.Snapshot(),
		GasSnapshot:  snapshot.GasSnapshot, GasPrices: snapshot.GasPrices, MaxFeePerGas: pricingMaxFee, ChainTime: now,
		Trace: liquidplanning.NewDecisionTrace(s.log,
			"source", order.Source,
			"orderHash", order.Hash.Hex(),
			"quoteId", order.QuoteID,
		),
	}
	return &fillFacts{
		input: input, decisionRoutes: decisionRoutes,
		pricingMaxFee: pricingMaxFee, transactionMaxFee: transactionMaxFee,
	}, nil
}

func (s *Solver) chooseFillPlan(
	ctx context.Context,
	order *resolvedOrder,
	input FillInput,
) (*liquidlane.Plan, liquidlane.CapacityReservations, error) {
	plan, err := s.planner.DecideFill(ctx, input)
	if err != nil {
		return nil, nil, err
	}
	if plan == nil || len(plan.Routes) == 0 {
		s.log.V(1).Info(
			"order fill strategy declined",
			"source", order.Source,
			"orderHash", order.Hash.Hex(),
			"quoteId", order.QuoteID,
			"fillQuotes", len(input.Quotes),
			"amountIn", order.AmountIn.String(),
			"requiredAmountOut", order.AmountOut.String(),
		)
		return nil, nil, errOrderNotFillable
	}
	validatedRoutes, err := liquidplanning.ValidateFillRoutes(liquidplanning.FillValidation{
		TokenIn: input.TokenIn, TokenOut: input.TokenOut, AmountIn: input.AmountIn,
		RequiredAmountOut: input.OutputAmount, RequireSingleRoute: input.RequireSingleRoute,
		MaxRoutes: MaxRoutes, Quotes: input.Quotes, Reservations: input.Reservations,
		GasSnapshot: input.GasSnapshot, GasPrices: input.GasPrices, MaxFeePerGas: input.MaxFeePerGas,
		GasEnvelope: liquidplanning.ExecutorGasEnvelope(),
	}, plan.Routes)
	if err != nil {
		return nil, nil, errors.Errorf("strategy returned invalid fill plan: %w", err)
	}
	plan.Routes = validatedRoutes
	s.logFillPlan(order, plan)
	reservations, ok := liquidplanning.FillRouteReservations(plan.Routes)
	if !ok {
		return nil, nil, errors.New("strategy returned invalid capacity reservations")
	}
	return plan, reservations, nil
}

func (s *Solver) submitFillPlan(
	ctx context.Context,
	order *resolvedOrder,
	facts *fillFacts,
	plan *liquidlane.Plan,
	reservations liquidlane.CapacityReservations,
	now time.Time,
	chainObservedAt time.Time,
) error {
	data, discountValidUntil, err := s.buildExecutorCalldata(ctx, order, plan, facts.decisionRoutes, now)
	if err != nil {
		return err
	}
	if _, err := s.chain.CallContract(ctx, ethereum.CallMsg{From: s.solverAddress, To: &order.Executor, Data: data}, nil); err != nil {
		return errors.Errorf("%w: %v", errFillPreflight, err)
	}
	deadline := fillDeadline(order, discountValidUntil)
	cancelAt, ok := liquidlane.CancellationDeadline(deadline, now, chainObservedAt, time.Now())
	if !ok {
		return errOrderNotFillable
	}
	reservationKey := capacity.NewOwner(Name, order.Hash.Hex())
	lease, err := s.capacity.Acquire(
		reservationKey,
		reservations,
		capacity.Limits(facts.input.Reservations, reservations),
	)
	if err != nil {
		return errors.Errorf("reserve fill capacity: %w", err)
	}
	defer s.clearPendingReservations(order.Hash, lease)
	s.log.V(1).Info(
		"order fill preflight succeeded",
		"source", order.Source,
		"orderHash", order.Hash.Hex(),
		"quoteId", order.QuoteID,
		"executor", order.Executor.Hex(),
		"caller", s.solverAddress.Hex(),
		"calldataBytes", len(data),
		"gasAccounting", s.cfg.Gas != nil,
		"pricingMaxFeePerGas", facts.pricingMaxFee.String(),
		"deadline", deadline.Unix(),
		"deadlineRemaining", deadline.Sub(now),
		"cancelAt", cancelAt.Unix(),
	)
	s.onPendingReservationsAcquired(order.Hash, reservations)
	result := s.txm.Send(ctx, txmanager.Request{
		To: order.Executor, Data: data, MaxFeePerGas: facts.transactionMaxFee, CancelAt: cancelAt,
		Label: "uniswapx-fill",
	})
	s.completeFill(order, result)
	return nil
}

func (s *Solver) buildExecutorCalldata(
	ctx context.Context,
	order *resolvedOrder,
	plan *liquidlane.Plan,
	routes []liquidlane.Route,
	now time.Time,
) ([]byte, time.Time, error) {
	fillRoutes := make([]uxexecutor.ILiquidLaneUniswapXExecutorFillRoute, 0, len(plan.Routes))
	discountRoutes := make([]uxexecutor.ILiquidLaneUniswapXExecutorDiscountRoute, 0, len(plan.Routes))
	var discountValidUntil time.Time
	for _, route := range plan.Routes {
		if route.DiscountID == nil {
			fillRoutes = append(fillRoutes, uxexecutor.ILiquidLaneUniswapXExecutorFillRoute{
				Adapter: route.Adapter, AmountIn: route.AmountIn, AmountOut: route.MinAmountOut,
			})
			continue
		}
		selectedRoute, ok := findRoute(routes, route.RouteID)
		if !ok {
			return nil, time.Time{}, errors.Errorf("selected discount route %s is unavailable", route.RouteID)
		}
		s.log.V(1).Info(
			"selected discount route repricing",
			"orderHash", order.Hash.Hex(),
			"quoteId", order.QuoteID,
			"discountId", route.DiscountID.Hex(),
			"routeId", route.RouteID,
			"adapter", route.Adapter.Hex(),
			"amountIn", route.AmountIn.String(),
		)
		physicalQuotes, err := s.reader.ReadFillQuotes(
			ctx,
			[]liquidlane.Route{selectedRoute},
			order.TokenIn,
			route.AmountIn,
		)
		if err != nil {
			return nil, time.Time{}, errors.Errorf("reprice selected discount %s: %w", route.DiscountID.Hex(), err)
		}
		signed, err := s.resolveDiscount(ctx, liquiddiscounts.Selection{
			DiscountID:   *route.DiscountID,
			Adapter:      route.Adapter,
			TokenIn:      order.TokenIn,
			TokenOut:     order.TokenOut,
			AmountIn:     route.AmountIn,
			MinAmountOut: route.MinAmountOut,
		}, physicalQuotes, now)
		if err != nil {
			return nil, time.Time{}, errors.Errorf("resolve selected discount %s: %w", route.DiscountID.Hex(), err)
		}
		discountValidUntil = earlierTime(discountValidUntil, liquiddiscounts.ValidUntil(signed))
		s.log.V(1).Info(
			"selected discount resolved",
			"orderHash", order.Hash.Hex(),
			"quoteId", order.QuoteID,
			"discountId", route.DiscountID.Hex(),
			"routeId", route.RouteID,
			"adapter", route.Adapter.Hex(),
			"amountIn", route.AmountIn.String(),
			"discountDeadline", signed.Terms.Deadline,
			"protocolDeadline", signed.ProtocolDeadline,
		)
		discountRoutes = append(discountRoutes, uxexecutor.ILiquidLaneUniswapXExecutorDiscountRoute{
			Adapter: route.Adapter, AmountIn: route.AmountIn,
			DiscountSwap: uxexecutor.ILiquidLaneAdapterDiscountSwap{
				Discount: uxexecutor.ILiquidLaneAdapterDiscount{
					TokenToRedeem: signed.Terms.TokenToRedeem,
					Discount:      signed.Terms.Discount,
					Signer:        signed.Terms.Signer,
					Protocol:      signed.Terms.Protocol,
					Nonce:         signed.Terms.Nonce,
					Deadline:      signed.Terms.Deadline,
				},
				SignerSignature: signed.SignerSignature, ProtocolDeadline: signed.ProtocolDeadline,
			},
			ProtocolSignature: signed.ProtocolSignature,
		})
	}
	data, err := uniswapXExecutor.TryPackExecute(
		uxexecutor.UniswapXSignedOrder{Order: order.Encoded, Sig: order.Signature},
		uxexecutor.ILiquidLaneUniswapXExecutorFillCall{Routes: fillRoutes, DiscountRoutes: discountRoutes},
	)
	return data, discountValidUntil, err
}

func fillDeadline(order *resolvedOrder, discountValidUntil time.Time) time.Time {
	return earlierTime(time.Unix(int64(order.Deadline), 0), discountValidUntil)
}

func earlierTime(left, right time.Time) time.Time {
	if left.IsZero() || !right.IsZero() && right.Before(left) {
		return right
	}
	return left
}

func findRoute(routes []liquidlane.Route, id liquidlane.RouteID) (liquidlane.Route, bool) {
	for _, route := range routes {
		if route.ID == id {
			return route, true
		}
	}
	return liquidlane.Route{}, false
}

func (s *Solver) logFillPlan(order *resolvedOrder, plan *liquidlane.Plan) {
	discountRoutes := 0
	for index, route := range plan.Routes {
		if route.DiscountID != nil {
			discountRoutes++
		}
		fields := []any{
			"source", order.Source,
			"orderHash", order.Hash.Hex(),
			"quoteId", order.QuoteID,
			"route", index,
			"routeId", route.RouteID,
			"adapter", route.Adapter.Hex(),
			"amountIn", route.AmountIn.String(),
			"expectedAmountOut", route.ExpectedAmountOut.String(),
			"minAmountOut", route.MinAmountOut.String(),
			"reservedAmountOut", route.ReservedAmountOut.String(),
			"capacityId", route.CapacityID,
			"private", route.DiscountID != nil,
		}
		if route.DiscountID != nil {
			fields = append(fields, "discountId", route.DiscountID.Hex())
		}
		s.log.V(1).Info("order fill route selected", fields...)
	}
	s.log.V(1).Info(
		"order fill plan selected",
		"source", order.Source,
		"orderHash", order.Hash.Hex(),
		"quoteId", order.QuoteID,
		"routes", len(plan.Routes),
		"discountRoutes", discountRoutes,
	)
}

func (s *Solver) completeFill(order *resolvedOrder, result txmanager.Result) {
	now := time.Now()
	if result.Err != nil {
		if result.NotAdmitted {
			s.retry(order.Hash, now, false)
			s.metrics.fillFailed(true)
			s.log.V(1).Info(
				"order fill was not admitted",
				"source", order.Source,
				"orderHash", order.Hash.Hex(),
				"quoteId", order.QuoteID,
				"error", result.Err,
			)
			return
		}
		s.retry(order.Hash, now, true)
		s.recordOrderFillFailure(order, now)
		s.metrics.fillFailed(false)
		s.log.Error(result.Err, "order fill failed", "source", order.Source,
			"orderHash", order.Hash.Hex(), "quoteId", order.QuoteID, "tx", result.Hash.Hex())
		return
	}
	s.recordFillSuccess()
	s.ledger.complete(order.Hash, now)
	s.metrics.successfulFill(result.Receipt, order.TokenIn, order.AmountIn, order.TokenOut, order.AmountOut)
	s.log.Info("order filled", "source", order.Source, "executor", order.Executor.Hex(),
		"orderHash", order.Hash.Hex(), "quoteId", order.QuoteID, "tx", result.Hash.Hex())
}

func (s *Solver) recordOrderFillFailure(order *resolvedOrder, now time.Time) {
	// An exclusive attempt can legitimately lose to a timely soft override. Its tracked
	// obligation is classified from terminal API and canonical receipt state after the deadline.
	if order.Source != orderSourceExclusiveV2 {
		s.recordFillFailure(now)
	}
}
