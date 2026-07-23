package lifi

import (
	"context"
	"math/big"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func (s *Solver) submitFill(
	ctx context.Context,
	order *submittedOrder,
	plan *types.FillPlan,
	calldata *fillCalldata,
	maxFeePerGas *big.Int,
) *pendingFill {
	reservations, ok := fillPlanReservations(plan)
	if !ok {
		s.log.Error(errors.New("strategy returned invalid capacity reservations"),
			"order fill: reject strategy plan", "orderId", order.OrderID, "quoteId", order.QuoteID)
		return nil
	}
	status, err := s.reader.orderStatus(ctx, s.cfg.InputSettler, calldata.OrderID)
	if err != nil {
		s.log.Error(err, "order fill: read order status", "orderId", order.OrderID,
			"onChainOrderId", calldata.OrderID.Hex(), "quoteId", order.QuoteID)
		return nil
	}
	if status != lifiOrderStatusDeposited {
		s.log.Info("order skipped: on-chain order is not deposited", "orderId", order.OrderID,
			"onChainOrderId", calldata.OrderID.Hex(), "quoteId", order.QuoteID, "status", status)
		return nil
	}
	reservationKey := calldata.OrderID.Hex()
	confirmations := uint64(0)
	result, accepted := s.txm.SendAsync(ctx, txmanager.Request{
		To: s.cfg.Executor, Data: calldata.Finalise, MaxFeePerGas: new(big.Int).Set(maxFeePerGas),
		Confirmations: &confirmations, Label: "lifi-fill",
	})
	if !accepted {
		s.log.Info("order skipped: transaction submission canceled", "orderId", order.OrderID,
			"onChainOrderId", calldata.OrderID.Hex(), "quoteId", order.QuoteID)
		return nil
	}
	return &pendingFill{
		order: order, orderID: calldata.OrderID, reservationKey: reservationKey,
		reservations: reservations, result: result,
	}
}

func (s *Solver) completeFill(ctx context.Context, pending *pendingFillState, completion fillCompletion) {
	fill := completion.fill
	pending.remove(fill.reservationKey)
	s.releaseReservation(ctx, fill.reservationKey)
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

func fillPlanReservations(plan *types.FillPlan) ([]quoteReservation, bool) {
	if plan == nil || len(plan.Routes) == 0 {
		return nil, false
	}
	reservations := make([]quoteReservation, 0, len(plan.Routes))
	for _, route := range plan.Routes {
		if route.CapacityID == "" || route.ReservedAmountOut == nil || route.ReservedAmountOut.Sign() <= 0 {
			return nil, false
		}
		reservations = append(reservations, quoteReservation{
			capacityID: route.CapacityID, amountOut: liquidlane.CloneBig(route.ReservedAmountOut),
		})
	}
	return reservations, true
}

func (s *Solver) reserve(ctx context.Context, orderKey string, reservations []quoteReservation) {
	if s.quoteEvents == nil || len(reservations) == 0 {
		return
	}
	select {
	case s.quoteEvents <- quoteEvent{orderKey: orderKey, reservations: reservations}:
	case <-ctx.Done():
	}
}

func (s *Solver) releaseReservation(ctx context.Context, orderKey string) {
	if s.quoteEvents == nil {
		return
	}
	select {
	case s.quoteEvents <- quoteEvent{orderKey: orderKey, release: true}:
	case <-ctx.Done():
	}
}
