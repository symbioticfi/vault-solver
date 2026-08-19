package lifi

import (
	"context"
	"math/big"
	"time"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidstrategies "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
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
	reservations, ok := fillPlanReservations(plan)
	if !ok {
		s.log.Error(errors.New("strategy returned invalid capacity reservations"),
			"order fill: reject strategy plan", "orderId", order.OrderID, "quoteId", order.QuoteID)
		return nil, nil
	}
	status, err := s.reader.orderStatus(ctx, s.cfg.InputSettler, calldata.OrderID)
	if err != nil {
		return nil, errors.Errorf("read order status for %s: %w", calldata.OrderID.Hex(), err)
	}
	if status == lifiOrderStatusNone {
		s.log.Info("on-chain order deposit is not visible at submission", "orderId", order.OrderID,
			"onChainOrderId", calldata.OrderID.Hex(), "quoteId", order.QuoteID, "status", status)
		return nil, errOrderDepositNotVisible
	}
	if status != lifiOrderStatusDeposited {
		s.log.Info("order skipped: on-chain order is no longer fillable at submission", "orderId", order.OrderID,
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
			s.log.Info("order skipped: execution deadline elapsed before submission",
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
	s.log.V(1).Info(
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
	result, accepted := s.txm.SendAsync(ctx, txmanager.Request{
		To: s.cfg.Executor, Data: calldata.Finalise, MaxFeePerGas: liquidlane.CloneBig(maxFeePerGas),
		CancelAt: cancelAt, Label: "lifi-fill",
	})
	if !accepted {
		s.log.Info("order skipped: transaction submission canceled", "orderId", order.OrderID,
			"onChainOrderId", calldata.OrderID.Hex(), "quoteId", order.QuoteID)
		return nil, nil
	}
	if s.reserveWithoutRefresh(reservationKey, reservations) {
		s.log.V(1).Info(
			"fill capacity reserved",
			"orderId", order.OrderID,
			"onChainOrderId", calldata.OrderID.Hex(),
			"quoteId", order.QuoteID,
			"capacityGroups", len(reservations),
			"pendingFills", s.capacity.Len(),
		)
	}
	s.log.V(1).Info(
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

func (s *Solver) completeFill(pending *pendingFillState, completion fillCompletion) {
	fill := completion.fill
	pending.remove(fill.reservationKey)
	outcome := completion.result.Outcome
	if outcome == txmanager.OutcomeConfirmed {
		s.observeFillAmounts(completion.result, fill)
		s.log.Info("order filled", "orderId", fill.order.OrderID, "onChainOrderId", fill.orderID.Hex(),
			"quoteId", fill.order.QuoteID, "tx", completion.result.Hash.Hex())
		return
	}
	if outcome == txmanager.OutcomeIncludedUnconfirmed {
		s.observeFillAmounts(completion.result, fill)
		s.log.Error(completion.result.Err, "order fill included but confirmation wait failed",
			"orderId", fill.order.OrderID,
			"onChainOrderId", fill.orderID.Hex(),
			"quoteId", fill.order.QuoteID,
			"tx", completion.result.Hash.Hex(),
		)
		return
	}
	err := completion.result.Err
	if err == nil {
		err = errors.Errorf("unknown transaction outcome %q", outcome)
	}
	s.log.Error(err, "order fill failed",
		"orderId", fill.order.OrderID,
		"onChainOrderId", fill.orderID.Hex(),
		"quoteId", fill.order.QuoteID,
		"tx", completion.result.Hash.Hex(),
		"notAdmitted", completion.result.NotAdmitted,
	)
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
