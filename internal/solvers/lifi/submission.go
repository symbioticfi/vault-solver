package lifi

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"go.opentelemetry.io/otel/attribute"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidstrategies "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func (s *Solver) submitFill(
	ctx context.Context,
	order *submittedOrder,
	plan *types.FillPlan,
	calldata *fillCalldata,
	maxFeePerGas *big.Int,
	chainTime time.Time,
	chainObservedAt time.Time,
) (*pendingFill, error) {
	log := observability.TraceLogger(ctx, s.log)
	reservations, ok := fillPlanReservations(plan)
	if !ok {
		log.Error(errors.New("strategy returned invalid capacity reservations"),
			"order fill: reject strategy plan", "orderId", order.OrderID, "quoteId", order.QuoteID)
		return nil, nil
	}
	status, err := s.reader.orderStatus(ctx, s.cfg.InputSettler, calldata.OrderID)
	if err != nil {
		return nil, errors.Errorf("read order status for %s: %w", calldata.OrderID.Hex(), err)
	}
	if status == lifiOrderStatusNone {
		observability.Decline(ctx, "order_deferred", "on-chain deposit is not visible at submission")
		log.Info("on-chain order deposit is not visible at submission", "orderId", order.OrderID,
			"onChainOrderId", calldata.OrderID.Hex(), "quoteId", order.QuoteID, "status", status)
		return nil, errOrderDepositNotVisible
	}
	if status != lifiOrderStatusDeposited {
		observability.Decline(ctx, "order_skipped", "on-chain order is no longer fillable at submission")
		log.Info("order skipped: on-chain order is no longer fillable at submission", "orderId", order.OrderID,
			"onChainOrderId", calldata.OrderID.Hex(), "quoteId", order.QuoteID, "status", status)
		return nil, errOrderNotFillable
	}
	var cancelAt time.Time
	if !calldata.Deadline.IsZero() {
		var deadlineValid bool
		cancelAt, deadlineValid = liquidlane.CancellationDeadline(
			calldata.Deadline,
			chainTime,
			chainObservedAt,
			s.wallNow(),
		)
		if !deadlineValid {
			observability.Decline(ctx, "fill_skipped", "execution deadline elapsed before submission")
			log.Info("order skipped: execution deadline elapsed before submission",
				"orderId", order.OrderID, "onChainOrderId", calldata.OrderID.Hex(),
				"quoteId", order.QuoteID, "deadline", calldata.Deadline.Unix())
			return nil, nil
		}
	}
	reservationKey := calldata.OrderID.Hex()
	deadline := int64(0)
	deadlineRemaining := time.Duration(0)
	cancelAtUnix := int64(0)
	if !calldata.Deadline.IsZero() {
		deadline = calldata.Deadline.Unix()
		deadlineRemaining = calldata.Deadline.Sub(chainTime)
		cancelAtUnix = cancelAt.Unix()
	}
	log.V(1).Info(
		"order fill ready for submission",
		"orderId", order.OrderID,
		"onChainOrderId", calldata.OrderID.Hex(),
		"quoteId", order.QuoteID,
		"executor", s.cfg.Executor.Hex(),
		"caller", s.caller.Hex(),
		"calldataBytes", len(calldata.Finalise),
		"gasAccounting", s.cfg.Gas != nil,
		"requestMaxFeePerGas", bigString(maxFeePerGas),
		"deadline", deadline,
		"deadlineRemaining", deadlineRemaining,
		"cancelAt", cancelAtUnix,
	)
	result, accepted := s.sendFill(ctx, txmanager.Request{
		Solver: Name,
		To:     s.cfg.Executor, Data: calldata.Finalise, MaxFeePerGas: liquidlane.CloneBig(maxFeePerGas),
		CancelAt: cancelAt,
		Obsolete: func(checkCtx context.Context) (bool, error) {
			return s.fillRequestObsolete(checkCtx, calldata.OrderID)
		},
		Label: "lifi-fill",
	})
	if !accepted {
		log.Info("order skipped: transaction submission canceled", "orderId", order.OrderID,
			"onChainOrderId", calldata.OrderID.Hex(), "quoteId", order.QuoteID)
		return nil, nil
	}
	if s.reserveWithoutRefresh(reservationKey, reservations) {
		log.V(1).Info(
			"fill capacity reserved",
			"orderId", order.OrderID,
			"onChainOrderId", calldata.OrderID.Hex(),
			"quoteId", order.QuoteID,
			"capacityGroups", len(reservations),
			"pendingFills", s.capacity.Len(),
		)
	}
	log.V(1).Info(
		"order fill submitted",
		"orderId", order.OrderID,
		"onChainOrderId", calldata.OrderID.Hex(),
		"quoteId", order.QuoteID,
		"routes", len(plan.Routes),
		"reservationDomains", len(reservations),
		"pendingFills", s.capacity.Len(),
		"gasAccounting", s.cfg.Gas != nil,
		"requestMaxFeePerGas", bigString(maxFeePerGas),
	)
	return &pendingFill{
		order:          order,
		orderID:        calldata.OrderID,
		reservationKey: reservationKey,
		plannedSurplus: liquidstrategies.PlannedSurplus(plan.Routes, order.OutputAmount),
		result:         result,
	}, nil
}

// sendFill hands the fill to the shared txmanager as the lifi.order.submit stage. The transaction
// result is asynchronous, so its attributes are recorded on completion, not here.
func (s *Solver) sendFill(
	ctx context.Context, request txmanager.Request,
) (result <-chan txmanager.Result, accepted bool) {
	ctx, end := tracer.Start(ctx, "lifi.order.submit")
	defer func() { end(nil) }()

	result, accepted = s.txm.SendAsync(ctx, request)
	if !accepted {
		// The lane refusing a fill is an expected shutdown outcome, not a failure of this fill.
		observability.Decline(ctx, "fill_skipped", "transaction submission was not accepted")
	}
	return result, accepted
}

func (s *Solver) fillRequestObsolete(
	ctx context.Context,
	orderID common.Hash,
) (bool, error) {
	status, err := s.reader.orderStatus(ctx, s.cfg.InputSettler, orderID)
	if err != nil {
		return false, errors.Errorf("read order status for %s: %w", orderID.Hex(), err)
	}
	switch status {
	case lifiOrderStatusNone, lifiOrderStatusDeposited:
		return false, nil
	case lifiOrderStatusClaimed, lifiOrderStatusRefunded:
		return true, nil
	default:
		return false, errors.Errorf("unsupported order status %d for %s", status, orderID.Hex())
	}
}

// completeFill reports the transaction outcome as the lifi.order.complete stage, stamping the
// outcome on the order's processing span too, and returns the failure the processing span ends with.
func (s *Solver) completeFill(
	ctx context.Context, pending *pendingFillState, completion fillCompletion,
) (err error) {
	fill := completion.fill
	pending.remove(fill.reservationKey)
	txAttrs := []attribute.KeyValue{
		observability.AttrTxHash.String(completion.result.Hash.Hex()),
		observability.AttrTxOutcome.String(string(completion.result.Outcome)),
	}
	observability.SetAttributes(ctx, txAttrs...) // the processing span this result belongs to
	ctx, end := tracer.Start(ctx, "lifi.order.complete", txAttrs...)
	defer func() { end(err) }()

	log := observability.TraceLogger(ctx, s.log)
	outcome := completion.result.Outcome
	if outcome == txmanager.OutcomeConfirmed {
		s.observeFillAmounts(completion.result, fill)
		log.Info("order filled", "orderId", fill.order.OrderID, "onChainOrderId", fill.orderID.Hex(),
			"quoteId", fill.order.QuoteID, "tx", completion.result.Hash.Hex())
		return nil
	}
	if outcome == txmanager.OutcomeIncludedUnconfirmed {
		// The fill stands; the confirmation wait is what failed, and it is the span's error.
		s.observeFillAmounts(completion.result, fill)
		err = completion.result.Err
		log.Error(err, "order fill included but confirmation wait failed",
			"orderId", fill.order.OrderID,
			"onChainOrderId", fill.orderID.Hex(),
			"quoteId", fill.order.QuoteID,
			"tx", completion.result.Hash.Hex(),
		)
		return err
	}
	err = completion.result.Err
	if err == nil {
		err = errors.Errorf("unknown transaction outcome %q", outcome)
	}
	log.Error(err, "order fill failed",
		"orderId", fill.order.OrderID,
		"onChainOrderId", fill.orderID.Hex(),
		"quoteId", fill.order.QuoteID,
		"tx", completion.result.Hash.Hex(),
		"notAdmitted", completion.result.NotAdmitted,
	)
	return err
}

func (s *Solver) observeFillAmounts(result txmanager.Result, fill *pendingFill) {
	if s.metrics == nil {
		return
	}
	s.metrics.fillAmounts.Observe(
		result.Receipt,
		fill.order.TokenIn,
		fill.order.AmountIn,
		fill.order.TokenOut,
		fill.order.OutputAmount,
		fill.plannedSurplus,
	)
}

func fillPlanReservations(plan *types.FillPlan) (liquidlane.CapacityReservations, bool) {
	if plan == nil || len(plan.Routes) == 0 {
		return nil, false
	}
	return liquidstrategies.FillRouteReservations(plan.Routes)
}

func (s *Solver) reserveWithoutRefresh(
	orderKey string,
	reservations liquidlane.CapacityReservations,
) bool {
	return s.capacity.Set(orderKey, reservations)
}

func (s *Solver) releaseReservationWithoutRefresh(orderKey string) bool {
	return s.capacity.Delete(orderKey)
}

func (s *Solver) requestQuoteRefresh() {
	if s.quoteRefresh == nil {
		return
	}
	select {
	case s.quoteRefresh <- struct{}{}:
	default:
	}
}
