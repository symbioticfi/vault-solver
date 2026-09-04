package lifi

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/capacity"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidplanning "github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func (s *Solver) submitFill(
	ctx context.Context,
	order *submittedOrder,
	plan *liquidlane.Plan,
	calldata *fillCalldata,
	maxFeePerGas *big.Int,
	chainTime time.Time,
	chainObservedAt time.Time,
	plannedAgainst liquidlane.CapacityReservations,
) error {
	reservations, ok := liquidplanning.FillRouteReservations(plan.Routes)
	if !ok {
		s.log.Error(errors.New("strategy returned invalid capacity reservations"),
			"order fill: reject strategy plan", "orderId", order.OrderID, "quoteId", order.QuoteID)
		return nil
	}
	status, err := s.reader.orderStatus(ctx, s.cfg.InputSettler, calldata.OrderID)
	if err != nil {
		return errors.Errorf("read order status for %s: %w", calldata.OrderID.Hex(), err)
	}
	if status == lifiOrderStatusNone {
		s.log.Info("on-chain order deposit is not visible at submission", "orderId", order.OrderID,
			"onChainOrderId", calldata.OrderID.Hex(), "quoteId", order.QuoteID, "status", status)
		return errOrderDepositNotVisible
	}
	if status != lifiOrderStatusDeposited {
		s.log.Info("order skipped: on-chain order is no longer fillable at submission", "orderId", order.OrderID,
			"onChainOrderId", calldata.OrderID.Hex(), "quoteId", order.QuoteID, "status", status)
		return errOrderNotFillable
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
			return nil
		}
	}
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
	lease, err := s.capacity.Acquire(
		capacity.NewOwner(Name, calldata.OrderID.Hex()),
		reservations,
		capacity.Limits(plannedAgainst, reservations),
	)
	if err != nil {
		return errors.Errorf("reserve fill capacity: %w", err)
	}
	defer func() {
		lease.Release()
		s.requestQuoteRefresh()
	}()
	s.requestQuoteRefresh()
	result := s.txm.Send(ctx, txmanager.Request{
		To: s.cfg.Executor, Data: calldata.Finalise, MaxFeePerGas: liquidlane.CloneBig(maxFeePerGas),
		CancelAt: cancelAt,
		Obsolete: func(checkCtx context.Context) (bool, error) {
			return s.fillRequestObsolete(checkCtx, order, calldata.OrderID)
		},
		Label: "lifi-fill",
	})
	if result.Err != nil {
		if s.metrics != nil {
			s.metrics.fill.ObserveFailure(result.NotAdmitted)
			s.metrics.order("error")
		}
		s.log.Error(result.Err, "order fill failed",
			"orderId", order.OrderID, "onChainOrderId", calldata.OrderID.Hex(),
			"quoteId", order.QuoteID, "tx", result.Hash.Hex(), "notAdmitted", result.NotAdmitted)
		return nil
	}
	if s.metrics != nil {
		planned := new(big.Int)
		for _, route := range plan.Routes {
			planned.Add(planned, route.ExpectedAmountOut)
		}
		s.metrics.fill.Observe(
			result.Receipt, order.TokenIn, order.AmountIn, order.TokenOut, order.OutputAmount,
			liquidlane.PlannedSurplus(planned, order.OutputAmount),
		)
		s.metrics.order("submitted")
	}
	s.log.Info("order filled", "orderId", order.OrderID, "onChainOrderId", calldata.OrderID.Hex(),
		"quoteId", order.QuoteID, "tx", result.Hash.Hex(), "routes", len(plan.Routes))
	return nil
}

func (s *Solver) fillRequestObsolete(
	ctx context.Context,
	order *submittedOrder,
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
		s.log.Info("order fill invalidated by on-chain status",
			"orderId", order.OrderID,
			"onChainOrderId", orderID.Hex(),
			"quoteId", order.QuoteID,
			"status", status,
		)
		return true, nil
	default:
		return false, errors.Errorf("unsupported order status %d for %s", status, orderID.Hex())
	}
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
