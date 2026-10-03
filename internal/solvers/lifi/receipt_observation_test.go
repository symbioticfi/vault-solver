package lifi

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestLateFillReceiptRecordsMetricsAfterWorkerReleasedOrder(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatal(err)
	}
	txm := &fakeLifiTxSender{result: txmanager.Result{Outcome: txmanager.OutcomeAbandoned, Err: txmanager.ErrAbandoned}}
	solver := newProcessTestSolver(fixture.cfg, fixture.caller, txm, strategy,
		fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited)
	reg := prometheus.NewRegistry()
	solver.metrics, err = newLIFIMetrics(reg, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	order := testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	wantInput, _ := new(big.Float).SetInt(order.AmountIn).Float64()
	wantOutput, _ := new(big.Float).SetInt(order.OutputAmount).Float64()
	orders := make(chan *submittedOrder, 1)
	orders <- order
	close(orders)
	if err = solver.runOrderWorker(t.Context(), testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter), orders, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(txm.reqs) != 1 || solver.capacity.Len() != 0 {
		t.Fatal("worker did not release its abandoned fill")
	}
	if txm.reqs[0].ObserveReceipt == nil {
		t.Fatal("abandoned fill has no receipt metrics observer")
	}
	order.AmountIn.SetInt64(1)
	order.OutputAmount.SetInt64(1)
	txm.reqs[0].ObserveReceipt(t.Context(), txmanager.Result{
		Outcome: txmanager.OutcomeConfirmed, Receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful},
	})
	if solver.capacity.Len() != 0 || len(txm.reqs) != 1 {
		t.Fatal("late receipt changed capacity or submitted another fill")
	}
	metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, 1)
	metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.tokenIn.Hex(), liquidlane.FillAmountInput, wantInput)
	metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.tokenOut.Hex(), liquidlane.FillAmountOutput, wantOutput)
	metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.tokenOut.Hex(), liquidlane.FillAmountPlannedSurplus, 1_000_000-wantOutput)
}

func TestOrdinaryFillReceiptRecordsSuccessOnce(t *testing.T) {
	for _, outcome := range []txmanager.Outcome{txmanager.OutcomeConfirmed, txmanager.OutcomeIncludedUnconfirmed} {
		t.Run(string(outcome), func(t *testing.T) {
			fixture := immediateTestSetup(t)
			strategy, err := defaultstrategy.New(defaultstrategy.Config{})
			if err != nil {
				t.Fatal(err)
			}
			txm := &fakeLifiTxSender{result: txmanager.Result{
				Outcome: outcome, Receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful},
			}}
			solver := newProcessTestSolver(fixture.cfg, fixture.caller, txm, strategy,
				fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited)
			reg := prometheus.NewRegistry()
			solver.metrics, err = newLIFIMetrics(reg, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			solver.processOrder(t.Context(), testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter),
				testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut))
			metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, 1)
			metricstest.RequireWorkflowAmount(t, reg, Name, "fill", fixture.tokenIn.Hex(), liquidlane.FillAmountInput, 1_000_000)
		})
	}
}
