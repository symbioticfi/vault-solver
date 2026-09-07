package uniswapx

import (
	"context"
	"math/big"
	"time"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/planning"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	uxexecutor "github.com/symbioticfi/vault-solver/api/bindings/uniswapx/executor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquiddiscounts "github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"

	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

var (
	errOrderNotFillable = errors.New("order is not fillable at current chain state")
	errFillPreflight    = errors.New("fill preflight failed")
)

type pendingUniswapFill struct {
	order          *resolvedOrder
	plannedSurplus *big.Int
	result         <-chan txmanager.Result
}

type uniswapFillCompletion struct {
	fill   *pendingUniswapFill
	result txmanager.Result
}

// Claims live in the solver's order table. This loop owns only the number of
// accepted lifecycles; completion messages carry everything needed to settle one.
func (s *Solver) fillLoop(ctx context.Context, routes []liquidlane.Route, orders <-chan *resolvedOrder) error {
	results := make(chan uniswapFillCompletion, orderQueueCapacity)
	pending := 0
	done := ctx.Done()
	for orders != nil || pending != 0 {
		select {
		case <-done:
			done = nil // Continue draining claims and accepted terminal results.
		case completion := <-results:
			pending--
			s.completePendingFill(completion)
		case order, open := <-orders:
			if !open {
				orders = nil
				continue
			}
			fill := s.admitPolledOrder(ctx, routes, order)
			if fill != nil {
				pending++
				go awaitUniswapFill(fill, results)
			}
		}
	}
	return ctx.Err()
}

func (s *Solver) admitPolledOrder(ctx context.Context, routes []liquidlane.Route, order *resolvedOrder) *pendingUniswapFill {
	defer s.endFillPlanning()
	log := s.log.WithValues("source", order.Source, "orderHash", order.Hash.Hex(), "quoteId", order.QuoteID)
	observed := time.Now()
	if ctx.Err() != nil || !s.txm.Available() {
		s.retry(order.Hash, observed, false)
		log.V(1).Info("order fill deferred while admission is unavailable")
		return nil
	}
	chainTime, err := s.reader.latestBlockTime(ctx)
	if err != nil {
		s.retry(order.Hash, observed, false)
		log.Error(err, "order fill: read current chain time")
		return nil
	}
	fill, err := s.startFill(ctx, routes, order, chainTime, observed)
	if err == nil {
		return fill
	}
	failedPreflight := errors.Is(err, errFillPreflight)
	s.retry(order.Hash, chainTime, failedPreflight)
	if failedPreflight {
		s.recordOrderFillFailure(order, chainTime)
	}
	if errors.Is(err, errOrderNotFillable) {
		log.V(1).Info("order not fillable yet")
	} else {
		log.Error(err, "order fill preparation failed")
	}
	return nil
}

// Receipt delivery belongs to accepted work and is deliberately independent of
// the intake context. A closed channel without a result is a terminal failure.
func awaitUniswapFill(fill *pendingUniswapFill, out chan<- uniswapFillCompletion) {
	completion := uniswapFillCompletion{fill: fill}
	result, open := <-fill.result
	if open {
		completion.result = result
	} else {
		completion.result = txmanager.Result{Outcome: txmanager.OutcomeTrackingStopped,
			Err: errors.New("transaction result channel closed without a result")}
	}
	out <- completion
}

// preparedFill contains all decisions made before admission. No capacity is held
// until the transaction manager accepts this exact request.
type preparedFill struct {
	request      txmanager.Request
	reservations liquidlane.CapacityReservations
	surplus      *big.Int
}

func (s *Solver) startFill(ctx context.Context, routes []liquidlane.Route, order *resolvedOrder,
	now, chainObservedAt time.Time) (*pendingUniswapFill, error) {
	prepared, err := s.prepareFill(ctx, routes, order, now, chainObservedAt)
	if err != nil {
		return nil, err
	}
	result, accepted := s.txm.SendAsync(ctx, prepared.request)
	if !accepted {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("transaction submission was not accepted")
	}
	s.setPendingReservations(order.Hash, prepared.reservations)
	s.log.V(1).Info("order fill submitted", "source", order.Source,
		"orderHash", order.Hash.Hex(), "quoteId", order.QuoteID,
		"reservationDomains", len(prepared.reservations), "gasAccounting", s.cfg.Gas != nil)
	return &pendingUniswapFill{order: order, plannedSurplus: prepared.surplus, result: result}, nil
}

func (s *Solver) prepareFill(
	ctx context.Context,
	routes []liquidlane.Route,
	order *resolvedOrder,
	now time.Time,
	chainObservedAt time.Time,
) (*preparedFill, error) {
	log := s.log.WithValues("source", order.Source, "orderHash", order.Hash.Hex(), "quoteId", order.QuoteID)
	if order.TokenOut == (common.Address{}) {
		return nil, errOrderNotFillable
	}
	if order.Deadline == 0 || int64(order.Deadline) <= now.Unix() {
		return nil, errOrderNotFillable
	}
	decisionRoutes, listed, discountErr := s.fillRoutesWithDiscounts(
		ctx,
		routes,
		order.TokenIn,
		order.TokenOut,
		now,
	)
	if discountErr != nil {
		log.Error(discountErr, "refresh fill discount routes")
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
	log.V(1).Info(
		"order fill snapshot loaded",
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
	fillInput := strategytypes.FillInput{
		OrderID: order.Hash.Hex(), QuoteID: order.QuoteID,
		TokenIn: order.TokenIn, TokenOut: order.TokenOut, AmountIn: order.AmountIn, OutputAmount: order.AmountOut,
		Deadline:           order.Deadline,
		RequireSingleRoute: s.cfg.TokenPolicy.RequiresSingleRoute(order.TokenIn), Quotes: snapshot.Direct,
		Reservations: s.capacity.Snapshot(),
		GasSnapshot:  snapshot.GasSnapshot, GasPrices: snapshot.GasPrices, MaxFeePerGas: pricingMaxFee, ChainTime: now,
		Trace: planning.NewDecisionTrace(s.log, "source", order.Source, "orderHash", order.Hash.Hex(), "quoteId", order.QuoteID),
	}
	plan, err := s.strategy.DecideFill(ctx, fillInput)
	if err != nil {
		return nil, err
	}
	if plan == nil || len(plan.Routes) == 0 {
		log.V(1).Info(
			"order fill strategy declined",
			"fillQuotes", len(fillInput.Quotes),
			"amountIn", order.AmountIn.String(),
			"requiredAmountOut", order.AmountOut.String(),
		)
		return nil, errOrderNotFillable
	}
	validatedRoutes, err := planning.ValidateFillRoutes(planning.FillValidation{
		TokenIn: fillInput.TokenIn, TokenOut: fillInput.TokenOut, AmountIn: fillInput.AmountIn,
		RequiredAmountOut: fillInput.OutputAmount, RequireSingleRoute: fillInput.RequireSingleRoute,
		MaxRoutes: strategytypes.MaxRoutes, Quotes: fillInput.Quotes, Reservations: fillInput.Reservations,
		GasSnapshot: fillInput.GasSnapshot, GasPrices: fillInput.GasPrices, MaxFeePerGas: fillInput.MaxFeePerGas,
		GasEnvelope: strategytypes.LiquidLaneGasEnvelope(),
	}, plan.Routes)
	if err != nil {
		return nil, errors.Errorf("strategy returned invalid fill plan: %w", err)
	}
	plan.Routes = validatedRoutes
	s.logFillPlan(order, plan)
	reservations, ok := planning.FillRouteReservations(plan.Routes)
	if !ok {
		return nil, errors.New("strategy returned invalid capacity reservations")
	}
	data, discountValidUntil, err := s.buildExecutorCalldata(ctx, order, plan, decisionRoutes, now)
	if err != nil {
		return nil, err
	}
	if _, err := s.chain.CallContract(ctx, ethereum.CallMsg{From: s.solverAddress, To: &order.Executor, Data: data}, nil); err != nil {
		return nil, errors.Errorf("%w: %v", errFillPreflight, err)
	}
	deadline := fillDeadline(order, discountValidUntil)
	cancelAt, ok := liquidlane.CancellationDeadline(deadline, now, chainObservedAt, time.Now())
	if !ok {
		return nil, errOrderNotFillable
	}
	log.V(1).Info(
		"order fill preflight succeeded",
		"executor", order.Executor.Hex(),
		"caller", s.solverAddress.Hex(),
		"calldataBytes", len(data),
		"gasAccounting", s.cfg.Gas != nil,
		"pricingMaxFeePerGas", pricingMaxFee.String(),
		"deadline", deadline.Unix(),
		"deadlineRemaining", deadline.Sub(now),
		"cancelAt", cancelAt.Unix(),
	)
	return &preparedFill{
		request: txmanager.Request{Solver: Name, To: order.Executor, Data: data,
			MaxFeePerGas: transactionMaxFee, CancelAt: cancelAt, Label: "uniswapx-fill"},
		reservations: reservations,
		surplus:      planning.PlannedSurplus(plan.Routes, order.AmountOut),
	}, nil
}

func (s *Solver) buildExecutorCalldata(
	ctx context.Context,
	order *resolvedOrder,
	plan *strategytypes.FillPlan,
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
				Discount:        uxexecutor.ILiquidLaneAdapterDiscount(signed.Terms.Clone()),
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

func (s *Solver) logFillPlan(order *resolvedOrder, plan *strategytypes.FillPlan) {
	log := s.log.V(1).WithValues("source", order.Source, "orderHash", order.Hash.Hex(), "quoteId", order.QuoteID)
	private := 0
	for index, route := range plan.Routes {
		routeLog := log
		if route.DiscountID != nil {
			private++
			routeLog = routeLog.WithValues("discountId", route.DiscountID.Hex())
		}
		routeLog.Info("order fill route selected", "route", index, "routeId", route.RouteID,
			"capacityId", route.CapacityID, "adapter", route.Adapter.Hex(), "private", route.DiscountID != nil,
			"amountIn", route.AmountIn.String(), "expectedAmountOut", route.ExpectedAmountOut.String(),
			"minAmountOut", route.MinAmountOut.String(), "reservedAmountOut", route.ReservedAmountOut.String())
	}
	log.Info("order fill plan selected", "routes", len(plan.Routes), "discountRoutes", private)
}

func (s *Solver) completePendingFill(completion uniswapFillCompletion) {
	fill, result := completion.fill, completion.result
	order, now := fill.order, time.Now()
	s.clearPendingReservations(order.Hash)
	log := s.log.WithValues("source", order.Source, "orderHash", order.Hash.Hex(),
		"quoteId", order.QuoteID, "tx", result.Hash.Hex())
	switch {
	case result.NotAdmitted:
		s.observeFillOutcome(liquidlane.FillOutcomeNotAdmitted)
		s.retry(order.Hash, now, false)
		log.V(1).Info("order fill was not admitted", "error", result.Err)
	case result.Outcome.Included():
		// Inclusion fulfills the order even when the stronger confirmation wait fails.
		s.recordFillSuccess()
		s.complete(order.Hash, now)
		if s.metrics != nil {
			s.metrics.fillAmounts.Observe(result.Receipt, order.TokenIn, order.AmountIn,
				order.TokenOut, order.AmountOut, fill.plannedSurplus)
		}
		if result.Outcome == txmanager.OutcomeConfirmed {
			log.Info("order filled", "executor", order.Executor.Hex())
		} else {
			log.Error(result.Err, "order fill included but confirmation wait failed", "executor", order.Executor.Hex())
		}
	default:
		failure := result.Err
		if failure == nil {
			failure = errors.Errorf("unknown transaction outcome %q", result.Outcome)
		}
		s.observeFillOutcome(liquidlane.FillOutcomeFailure)
		s.retry(order.Hash, now, true)
		s.recordOrderFillFailure(order, now)
		log.Error(failure, "order fill failed")
	}
}

func (s *Solver) recordOrderFillFailure(order *resolvedOrder, now time.Time) {
	// An exclusive attempt can legitimately lose to a timely soft override. Its tracked
	// obligation is classified from terminal API and canonical receipt state after the deadline.
	if order.Source != orderSourceExclusiveV2 {
		s.recordFillFailure(now)
	}
}
