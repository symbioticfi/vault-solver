package uniswapx

import (
	"context"
	"math/big"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	uxexecutor "github.com/symbioticfi/vault-solver/api/bindings/uniswapx/executor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquiddiscounts "github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	liquidstrategies "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	"github.com/symbioticfi/vault-solver/internal/observability"
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
	// span and end keep the uniswapx.fill span open from submission until the transaction result
	// arrives: span carries the outcome attributes, end closes it. Both are nil when the fill was
	// built outside startFill; end is idempotent, so the span is never ended twice.
	span trace.Span
	end  observability.EndFunc
}

// traceContext returns ctx carrying the fill span, or ctx unchanged when there is none.
func (f *pendingUniswapFill) traceContext(ctx context.Context) context.Context {
	if f.span == nil {
		return ctx
	}
	return trace.ContextWithSpan(ctx, f.span)
}

func (f *pendingUniswapFill) endFill(err error) {
	if f.end != nil {
		f.end(err)
	}
}

type uniswapFillCompletion struct {
	fill   *pendingUniswapFill
	result txmanager.Result
}

func (s *Solver) fillLoop(
	ctx context.Context,
	routes []liquidlane.Route,
	orders <-chan *resolvedOrder,
) error {
	completions := make(chan uniswapFillCompletion, orderQueueCapacity)
	pending := make(map[common.Hash]*pendingUniswapFill)
	ctxDone := ctx.Done()
	var shutdownErr error
	for orders != nil || len(pending) > 0 {
		select {
		case <-ctxDone:
			shutdownErr = ctx.Err()
			ctxDone = nil
		case completion := <-completions:
			delete(pending, completion.fill.order.Hash)
			s.completePendingFill(ctx, completion)
		case order, ok := <-orders:
			if !ok {
				orders = nil
				continue
			}
			// The order's track span and quote-linked logger, so every admission line below and
			// every line the fill logs carries this order's trace ids.
			orderCtx := observability.WithLogger(
				trace.ContextWithSpanContext(ctx, order.span), s.orderLogger(order),
			)
			if shutdownErr != nil || ctx.Err() != nil {
				if shutdownErr == nil {
					shutdownErr = ctx.Err()
				}
				ctxDone = nil
				s.endFillPlanning()
				s.retry(order.Hash, time.Now(), false)
				continue
			}
			if !s.txm.Available() {
				s.endFillPlanning()
				s.retry(order.Hash, time.Now(), false)
				observability.Log(orderCtx).V(1).Info(
					"order fill deferred while transaction nonce lane is paused",
					"source", order.Source,
					"orderHash", order.Hash.Hex(),
					"quoteId", order.QuoteID,
				)
				continue
			}
			observability.Log(orderCtx).V(1).Info(
				"order fill planning started",
				"source", order.Source,
				"orderHash", order.Hash.Hex(),
				"quoteId", order.QuoteID,
			)
			chainObservedAt := time.Now()
			now, err := s.reader.latestBlockTime(ctx)
			if err != nil {
				s.endFillPlanning()
				s.retry(order.Hash, time.Now(), false)
				observability.Log(orderCtx).Error(
					err, "order fill: read current chain time", "orderHash", order.Hash.Hex(),
				)
				continue
			}
			fill, err := s.startFill(orderCtx, routes, order, now, chainObservedAt)
			s.endFillPlanning()
			if err != nil {
				s.retry(order.Hash, now, errors.Is(err, errFillPreflight))
				if errors.Is(err, errFillPreflight) {
					s.recordOrderFillFailure(order, now)
				}
				if errors.Is(err, errOrderNotFillable) {
					observability.Log(orderCtx).V(1).Info("order not fillable yet", "source", order.Source,
						"orderHash", order.Hash.Hex(), "quoteId", order.QuoteID)
					continue
				}
				observability.Log(orderCtx).Error(
					err, "order fill preparation failed", "orderHash", order.Hash.Hex(), "quoteId", order.QuoteID,
				)
				continue
			}
			pending[order.Hash] = fill
			go awaitUniswapFill(fill, completions)
		}
	}
	return shutdownErr
}

// Once txmanager accepts a fill, shutdown may stop new admission but must not drop its terminal result.
func awaitUniswapFill(
	fill *pendingUniswapFill,
	out chan<- uniswapFillCompletion,
) {
	result, ok := <-fill.result
	if !ok {
		result.Err = errors.New("transaction result channel closed without a result")
	}
	out <- uniswapFillCompletion{fill: fill, result: result}
}

// startFill plans and submits the fill for one accepted order. The uniswapx.fill span continues the
// order's track trace and outlives this call: it is handed to the returned pending fill and ended
// when the transaction result arrives. Only a fill that never reaches submission ends it here.
func (s *Solver) startFill(
	ctx context.Context,
	routes []liquidlane.Route,
	order *resolvedOrder,
	now time.Time,
	chainObservedAt time.Time,
) (fill *pendingUniswapFill, err error) {
	ctx, end := tracer.Start(trace.ContextWithSpanContext(ctx, order.span), "uniswapx.fill",
		observability.AttrOrderHash.String(order.Hash.Hex()),
		observability.AttrQuoteID.String(order.QuoteID),
	)
	defer func() {
		switch {
		case fill != nil: // the span lives until the transaction result arrives
		case errors.Is(err, errOrderNotFillable):
			// An order we decline to fill is the poll's ordinary outcome, already recorded as a
			// declined event: the loop retries it later rather than treating it as a failure.
			end(nil)
		default:
			end(err)
		}
	}()

	if order.TokenOut == (common.Address{}) {
		return nil, declineFill(ctx, "fill_skipped", "order has no output token")
	}
	if order.Deadline == 0 || int64(order.Deadline) <= now.Unix() {
		return nil, declineFill(ctx, "fill_skipped", "order deadline has passed")
	}
	decisionRoutes, listed, discountErr := s.fillRoutesWithDiscounts(
		ctx,
		routes,
		order.TokenIn,
		order.TokenOut,
		now,
	)
	if discountErr != nil {
		observability.Log(ctx).Error(discountErr, "refresh fill discount routes", "orderHash", order.Hash.Hex())
	}
	snapshot, err := s.reader.fillSnapshot(
		ctx,
		decisionRoutes,
		order.Executor,
		order.TokenIn,
		order.AmountIn,
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
	observability.Log(ctx).V(1).Info(
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
	fillInput := strategytypes.FillInput{
		OrderID: order.Hash.Hex(), QuoteID: order.QuoteID,
		TokenIn: order.TokenIn, TokenOut: order.TokenOut, AmountIn: order.AmountIn, OutputAmount: order.AmountOut,
		Deadline:           order.Deadline,
		RequireSingleRoute: s.cfg.TokenPolicy.RequiresSingleRoute(order.TokenIn), Quotes: snapshot.Direct,
		Reservations: s.capacity.Snapshot(),
		GasSnapshot:  snapshot.GasSnapshot, GasPrices: snapshot.GasPrices, MaxFeePerGas: pricingMaxFee, ChainTime: now,
		Trace: s.decisionTrace(
			"source", order.Source,
			"orderHash", order.Hash.Hex(),
			"quoteId", order.QuoteID,
		),
	}
	plan, err := s.decideFill(ctx, fillInput)
	if err != nil {
		return nil, err
	}
	if plan == nil || len(plan.Routes) == 0 {
		observability.Log(ctx).V(1).Info(
			"order fill strategy declined",
			"source", order.Source,
			"orderHash", order.Hash.Hex(),
			"quoteId", order.QuoteID,
			"fillQuotes", len(fillInput.Quotes),
			"amountIn", order.AmountIn.String(),
			"requiredAmountOut", order.AmountOut.String(),
		)
		return nil, declineFill(ctx, "fill_declined", "strategy returned no fill plan")
	}
	validatedRoutes, err := liquidstrategies.ValidateFillRoutes(liquidstrategies.FillValidation{
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
	s.logFillPlan(observability.Log(ctx), order, plan)
	reservations, ok := liquidstrategies.FillRouteReservations(plan.Routes)
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
		return nil, declineFill(ctx, "fill_skipped", "fill execution deadline elapsed before submission")
	}
	observability.Log(ctx).V(1).Info(
		"order fill preflight succeeded",
		"source", order.Source,
		"orderHash", order.Hash.Hex(),
		"quoteId", order.QuoteID,
		"executor", order.Executor.Hex(),
		"caller", s.solverAddress.Hex(),
		"calldataBytes", len(data),
		"gasAccounting", s.cfg.Gas != nil,
		"pricingMaxFeePerGas", pricingMaxFee.String(),
		"deadline", deadline.Unix(),
		"deadlineRemaining", deadline.Sub(now),
		"cancelAt", cancelAt.Unix(),
	)
	result, err := s.submitFill(ctx, txmanager.Request{
		Solver: Name,
		To:     order.Executor, Data: data, MaxFeePerGas: transactionMaxFee, CancelAt: cancelAt,
		Label: "uniswapx-fill",
	})
	if err != nil {
		return nil, err
	}
	s.setPendingReservations(order.Hash, reservations)
	observability.Log(ctx).V(1).Info(
		"order fill submitted",
		"source", order.Source,
		"orderHash", order.Hash.Hex(),
		"quoteId", order.QuoteID,
		"routes", len(plan.Routes),
		"reservationDomains", len(reservations),
		"gasAccounting", s.cfg.Gas != nil,
		"pricingMaxFeePerGas", pricingMaxFee.String(),
	)
	return &pendingUniswapFill{
		order: order, plannedSurplus: liquidstrategies.PlannedSurplus(plan.Routes, order.AmountOut), result: result,
		span: trace.SpanFromContext(ctx), end: end,
	}, nil
}

// declineFill records an order this solver will not fill as an expected skip on the fill span and
// returns the sentinel the fill loop already treats as "retry later, not a failure". Pairing the two
// here keeps every unfillable path named on the trace and out of the error statistics.
func declineFill(ctx context.Context, decision, reason string) error {
	observability.Decline(ctx, decision, reason)
	return errOrderNotFillable
}

// decideFill runs the strategy as the uniswapx.fill.plan stage.
func (s *Solver) decideFill(
	ctx context.Context, input strategytypes.FillInput,
) (plan *strategytypes.FillPlan, err error) {
	ctx, end := tracer.Start(ctx, "uniswapx.fill.plan", observability.AttrStrategy.String(s.cfg.Strategy.Name))
	defer func() { end(err) }()
	return s.strategy.DecideFill(ctx, input)
}

// submitFill hands the fill to the shared txmanager as the uniswapx.fill.submit stage. The result is
// asynchronous, so the transaction attributes are recorded on completion, not here.
func (s *Solver) submitFill(
	ctx context.Context, request txmanager.Request,
) (result <-chan txmanager.Result, err error) {
	ctx, end := tracer.Start(ctx, "uniswapx.fill.submit")
	defer func() { end(err) }()

	result, accepted := s.txm.SendAsync(ctx, request)
	if !accepted {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("transaction submission was not accepted")
	}
	return result, nil
}

// buildExecutorCalldata encodes the executor call as the uniswapx.fill.build stage, resolving a
// fresh signed discount for every discount leg the plan selected.
func (s *Solver) buildExecutorCalldata(
	ctx context.Context,
	order *resolvedOrder,
	plan *strategytypes.FillPlan,
	routes []liquidlane.Route,
	now time.Time,
) (data []byte, discountValidUntil time.Time, err error) {
	ctx, end := tracer.Start(ctx, "uniswapx.fill.build")
	defer func() { end(err) }()

	fillRoutes := make([]uxexecutor.ILiquidLaneUniswapXExecutorFillRoute, 0, len(plan.Routes))
	discountRoutes := make([]uxexecutor.ILiquidLaneUniswapXExecutorDiscountRoute, 0, len(plan.Routes))
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
		observability.Log(ctx).V(1).Info(
			"selected discount route repricing",
			"orderHash", order.Hash.Hex(),
			"quoteId", order.QuoteID,
			"discountId", route.DiscountID.Hex(),
			"routeId", route.RouteID,
			"adapter", route.Adapter.Hex(),
			"amountIn", route.AmountIn.String(),
		)
		physicalQuotes, err := s.reader.physicalFillQuotes(
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
		observability.Log(ctx).V(1).Info(
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
	data, err = uniswapXExecutor.TryPackExecute(
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

func (s *Solver) logFillPlan(log logr.Logger, order *resolvedOrder, plan *strategytypes.FillPlan) {
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
		log.V(1).Info("order fill route selected", fields...)
	}
	log.V(1).Info(
		"order fill plan selected",
		"source", order.Source,
		"orderHash", order.Hash.Hex(),
		"quoteId", order.QuoteID,
		"routes", len(plan.Routes),
		"discountRoutes", discountRoutes,
	)
}

// completePendingFill reports the transaction outcome as the uniswapx.fill.complete stage and closes
// the fill span the submission opened.
func (s *Solver) completePendingFill(ctx context.Context, completion uniswapFillCompletion) {
	order := completion.fill.order
	txAttrs := []attribute.KeyValue{observability.AttrTxOutcome.String(string(completion.result.Outcome))}
	if completion.result.Hash != (common.Hash{}) { // a request that never reached the wire has no hash
		txAttrs = append(txAttrs, observability.AttrTxHash.String(completion.result.Hash.Hex()))
	}
	// The fill span this result belongs to, plus the order's quote-linked logger.
	fillCtx := observability.WithLogger(completion.fill.traceContext(ctx), s.orderLogger(order))
	observability.SetAttributes(fillCtx, txAttrs...) // the fill span this result belongs to
	ctx, end := tracer.Start(fillCtx, "uniswapx.fill.complete", txAttrs...)
	var err error
	// Deferred so both spans end on every path, including a panic; ending twice is a no-op.
	defer func() {
		end(err)
		completion.fill.endFill(err)
	}()

	now := time.Now()
	s.clearPendingReservations(order.Hash)
	if completion.result.NotAdmitted {
		// The lane refusing a fill is an expected outcome, not a failure of this fill.
		observability.Decline(ctx, "fill_not_admitted", errorReason(completion.result.Err))
		s.observeFillOutcome(liquidlane.FillOutcomeNotAdmitted)
		s.retry(order.Hash, now, false)
		observability.Log(ctx).V(1).Info(
			"order fill was not admitted",
			"source", order.Source,
			"orderHash", order.Hash.Hex(),
			"quoteId", order.QuoteID,
			"error", completion.result.Err,
		)
		return
	}
	outcome := completion.result.Outcome
	if !outcome.Included() {
		err = completion.result.Err
		if err == nil {
			err = errors.Errorf("unknown transaction outcome %q", outcome)
		}
		s.observeFillOutcome(liquidlane.FillOutcomeFailure)
		s.retry(order.Hash, now, true)
		s.recordOrderFillFailure(order, now)
		observability.Log(ctx).Error(
			err,
			"order fill failed",
			"source", order.Source, "orderHash", order.Hash.Hex(), "quoteId", order.QuoteID,
			"tx", completion.result.Hash.Hex(),
		)
		return
	}
	if outcome == txmanager.OutcomeConfirmed {
		observability.Log(ctx).Info("order filled", "source", order.Source, "executor", order.Executor.Hex(),
			"orderHash", order.Hash.Hex(), "quoteId", order.QuoteID, "tx", completion.result.Hash.Hex())
	} else {
		// Included, but the confirmation wait failed: the fill stands, the wait error is the span's.
		err = completion.result.Err
		observability.Log(ctx).Error(err, "order fill included but confirmation wait failed",
			"source", order.Source, "executor", order.Executor.Hex(),
			"orderHash", order.Hash.Hex(), "quoteId", order.QuoteID, "tx", completion.result.Hash.Hex())
	}
	s.recordFillSuccess()
	s.complete(order.Hash, now)
	if s.metrics != nil {
		s.metrics.fillAmounts.Observe(
			completion.result.Receipt,
			order.TokenIn,
			order.AmountIn,
			order.TokenOut,
			order.AmountOut,
			completion.fill.plannedSurplus,
		)
	}
}

// errorReason renders an error for a span event attribute, naming its absence rather than "".
func errorReason(err error) string {
	if err == nil {
		return "not reported"
	}
	return err.Error()
}

func (s *Solver) recordOrderFillFailure(order *resolvedOrder, now time.Time) {
	// An exclusive attempt can legitimately lose to a timely soft override. Its tracked
	// obligation is classified from terminal API and canonical receipt state after the deadline.
	if order.Source != orderSourceExclusiveV2 {
		s.recordFillFailure(now)
	}
}
