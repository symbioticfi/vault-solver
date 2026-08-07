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
	if status != lifiOrderStatusDeposited {
		s.log.Info("order skipped: on-chain order is not deposited", "orderId", order.OrderID,
			"onChainOrderId", calldata.OrderID.Hex(), "quoteId", order.QuoteID, "status", status)
		return nil, nil
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
	result, accepted := s.txm.SendAsync(ctx, txmanager.Request{
		To: s.cfg.Executor, Data: calldata.Finalise, MaxFeePerGas: new(big.Int).Set(maxFeePerGas),
		CancelAt: cancelAt, Label: "lifi-fill",
	})
	if !accepted {
		s.log.Info("order skipped: transaction submission canceled", "orderId", order.OrderID,
			"onChainOrderId", calldata.OrderID.Hex(), "quoteId", order.QuoteID)
		return nil, nil
	}
	s.reserveWithoutRefresh(reservationKey, reservations)
	return &pendingFill{
		order: order, orderID: calldata.OrderID, reservationKey: reservationKey,
		result: result,
	}, nil
}

func (s *Solver) completeFill(pending *pendingFillState, completion fillCompletion) {
	fill := completion.fill
	pending.remove(fill.reservationKey)
	if completion.result.Err == nil {
		s.log.Info("order filled", "orderId", fill.order.OrderID, "onChainOrderId", fill.orderID.Hex(),
			"quoteId", fill.order.QuoteID, "tx", completion.result.Hash.Hex())
		return
	}
	s.log.Error(completion.result.Err, "order fill failed",
		"orderId", fill.order.OrderID,
		"onChainOrderId", fill.orderID.Hex(),
		"quoteId", fill.order.QuoteID,
		"tx", completion.result.Hash.Hex(),
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
