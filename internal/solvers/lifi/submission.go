package lifi

import (
	"context"
	"math/big"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// submitFill rechecks admission facts after planning, then transfers the exact
// calldata to txmanager. Capacity becomes owned only after that transfer succeeds.
func (s *Solver) submitFill(ctx context.Context, order *submittedOrder, plan *types.FillPlan,
	calldata *fillCalldata, maxFeePerGas *big.Int, chainTime, chainObservedAt time.Time,
) (*pendingFill, error) {
	log := s.orderLogger(order, calldata.OrderID)
	if plan == nil {
		return nil, errors.New("fill plan is missing")
	}
	reservations, valid := planning.FillRouteReservations(plan.Routes)
	if !valid {
		log.Error(errors.New("strategy returned invalid capacity reservations"), "order fill: reject strategy plan")
		return nil, nil
	}
	if err := s.requireDeposited(ctx, order, calldata.OrderID, "submission"); err != nil {
		return nil, err
	}
	var cancelAt time.Time
	if !calldata.Deadline.IsZero() {
		var live bool
		cancelAt, live = liquidlane.CancellationDeadline(calldata.Deadline, chainTime, chainObservedAt, s.wallNow())
		if !live {
			log.Info("order skipped: execution deadline elapsed before submission", "deadline", calldata.Deadline.Unix())
			return nil, nil
		}
	}
	request := txmanager.Request{
		Solver: Name, Label: "lifi-fill", To: s.cfg.Executor, Data: calldata.Finalise,
		MaxFeePerGas: bigmath.Clone(maxFeePerGas), CancelAt: cancelAt,
		Obsolete: func(check context.Context) (bool, error) {
			return s.fillRequestObsolete(check, order, calldata.OrderID)
		},
	}
	log.V(1).Info("order fill ready for submission", "executor", request.To.Hex(), "caller", s.caller.Hex(),
		"calldataBytes", len(request.Data), "gasAccounting", s.cfg.Gas != nil,
		"requestMaxFeePerGas", bigString(request.MaxFeePerGas), "deadline", calldata.Deadline, "cancelAt", cancelAt)
	result, accepted := s.txm.SendAsync(ctx, request)
	if !accepted {
		log.Info("order skipped: transaction submission canceled")
		return nil, nil
	}
	fill := &pendingFill{
		order: order, orderID: calldata.OrderID, reservationKey: calldata.OrderID.Hex(), result: result,
		plannedSurplus: planning.PlannedSurplus(plan.Routes, order.OutputAmount),
	}
	if s.capacity.Set(fill.reservationKey, reservations) {
		log.V(1).Info("fill capacity reserved", "capacityGroups", len(reservations), "pendingFills", s.capacity.Len())
	}
	log.V(1).Info("order fill submitted", "routes", len(plan.Routes), "reservationDomains", len(reservations),
		"pendingFills", s.capacity.Len(), "gasAccounting", s.cfg.Gas != nil, "requestMaxFeePerGas", bigString(maxFeePerGas))
	return fill, nil
}

func (s *Solver) fillRequestObsolete(ctx context.Context, order *submittedOrder, id common.Hash) (bool, error) {
	status, err := s.reader.orderStatus(ctx, s.cfg.InputSettler, id)
	if err != nil {
		return false, errors.Errorf("read order status for %s: %w", id.Hex(), err)
	}
	if status == lifiOrderStatusNone || status == lifiOrderStatusDeposited {
		return false, nil
	}
	if status != lifiOrderStatusClaimed && status != lifiOrderStatusRefunded {
		return false, errors.Errorf("unsupported order status %d for %s", status, id.Hex())
	}
	s.orderLogger(order, id).Info("order fill invalidated by on-chain status", "status", status)
	return true, nil
}

func (s *Solver) completeFill(pending map[string]bool, completion fillCompletion) {
	fill, result := completion.fill, completion.result
	delete(pending, fill.reservationKey)
	log := s.orderLogger(fill.order, fill.orderID).WithValues("tx", result.Hash.Hex())
	if result.Outcome.Included() {
		s.observeFillAmounts(result, fill)
		if result.Outcome == txmanager.OutcomeConfirmed {
			log.Info("order filled")
		} else {
			log.Error(result.Err, "order fill included but confirmation wait failed")
		}
		return
	}
	err := result.Err
	if err == nil {
		err = errors.Errorf("unknown transaction outcome %q", result.Outcome)
	}
	log.Error(err, "order fill failed", "notAdmitted", result.NotAdmitted)
}

func (s *Solver) observeFillAmounts(result txmanager.Result, fill *pendingFill) {
	if s.metrics != nil {
		order := fill.order
		s.metrics.fillAmounts.Observe(result.Receipt, order.TokenIn, order.AmountIn, order.TokenOut, order.OutputAmount, fill.plannedSurplus)
	}
}

func (s *Solver) requestQuoteRefresh() {
	select {
	case s.quoteRefresh <- struct{}{}:
	default:
	}
}
