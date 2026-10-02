package uniswapx

import (
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestLateFillReceiptRecordsMetricsWithoutReplayingCompletion(t *testing.T) {
	fixture := newDirectExecutionFixture(t)
	metrics, reg := newUniswapXTestMetricsWithRegistry(t, fixture.solver)
	fixture.solver.metrics = metrics
	pending, err := fixture.solver.startFill(t.Context(), []liquidlane.Route{fixture.route},
		fixture.order, fixture.now, fixture.now)
	if err != nil || pending == nil {
		t.Fatalf("start fill: pending=%v err=%v", pending, err)
	}
	fixture.solver.completePendingFill(t.Context(), pending, txmanager.Result{
		Outcome: txmanager.OutcomeAbandoned, Err: txmanager.ErrAbandoned,
	})
	// Later work may have opened the breaker; an old receipt must not clear it or retire a retry.
	fixture.solver.failureTimes = []time.Time{fixture.now}
	blockedUntil := fixture.now.Add(time.Minute).Unix()
	fixture.solver.localBlockUntil.Store(blockedUntil)
	retryAt := fixture.solver.retryAt[fixture.order.Hash]
	exclusive := fixture.solver.exclusiveUntil[fixture.order.Hash]
	if fixture.txm.reqs[0].ObserveReceipt == nil {
		t.Fatal("abandoned fill has no receipt metrics observer")
	}
	fixture.order.AmountIn.SetInt64(1)
	fixture.order.AmountOut.SetInt64(1)
	fixture.order.TokenIn, fixture.order.TokenOut = fixture.order.TokenOut, fixture.order.TokenIn
	fixture.strategy.plan.Routes[0].ExpectedAmountOut.SetInt64(1)
	fixture.txm.reqs[0].ObserveReceipt(t.Context(), txmanager.Result{
		Outcome: txmanager.OutcomeConfirmed, Receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful},
	})
	if fixture.solver.capacity.Len() != 0 || len(fixture.solver.filled) != 0 ||
		!fixture.solver.retryAt[fixture.order.Hash].Equal(retryAt) || fixture.solver.attempts[fixture.order.Hash] != 0 ||
		fixture.solver.localBlockUntil.Load() != blockedUntil || len(fixture.solver.failureTimes) != 1 ||
		!reflect.DeepEqual(exclusive, fixture.solver.exclusiveUntil[fixture.order.Hash]) {
		t.Fatal("late receipt replayed business completion, retry, or breaker state")
	}
	metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, 1)
	metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.route.TokenIn.Hex(), liquidlane.FillAmountInput, 100)
	metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.route.TokenOut.Hex(), liquidlane.FillAmountOutput, 90)
	metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.route.TokenOut.Hex(), liquidlane.FillAmountPlannedSurplus, 10)
	metricstest.RequireWorkflowEventCount(t, reg, Name, "exclusive_obligation", exclusiveOutcomeSettledInTime, 0)
}

func TestOrdinaryFillReceiptRecordsSuccessOnce(t *testing.T) {
	for _, outcome := range []txmanager.Outcome{txmanager.OutcomeConfirmed, txmanager.OutcomeIncludedUnconfirmed} {
		t.Run(string(outcome), func(t *testing.T) {
			fixture := newDirectExecutionFixture(t)
			metrics, reg := newUniswapXTestMetricsWithRegistry(t, fixture.solver)
			fixture.solver.metrics = metrics
			pending, err := fixture.solver.startFill(t.Context(), []liquidlane.Route{fixture.route},
				fixture.order, fixture.now, fixture.now)
			if err != nil || pending == nil {
				t.Fatalf("start fill: pending=%v err=%v", pending, err)
			}
			fixture.txm.complete(txmanager.Result{
				Outcome: outcome, Receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful},
			})
			fixture.solver.completePendingFill(t.Context(), pending, <-pending.result)
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, 1)
			metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.route.TokenIn.Hex(), liquidlane.FillAmountInput, 100)
		})
	}
}
