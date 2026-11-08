package lifi

import (
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"

	txmanager "github.com/symbioticfi/merkle-executor/sdk/transaction"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/default"
)

func TestOrdinaryFillReceiptRecordsSuccessOnce(t *testing.T) {
	fixture := immediateTestSetup(t)
	strategy, err := defaultstrategy.New(defaultstrategy.Config{})
	if err != nil {
		t.Fatal(err)
	}
	txm := &fakeLifiTxSender{result: txmanager.Result{
		Receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful},
	}}
	solver := newProcessTestSolver(fixture.cfg, fixture.caller, txm, strategy,
		fixture.tokenIn, fixture.tokenOut, fixture.adapter, lifiOrderStatusDeposited)
	reg := prometheus.NewRegistry()
	solver.metrics, err = newLIFIMetrics(reg, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	orders := make(chan *submittedOrder, 1)
	orders <- testSubmittedOrder(t, fixture.cfg, fixture.tokenIn, fixture.tokenOut)
	close(orders)
	if err := solver.runOrderWorker(t.Context(), testResolvedRoutes(fixture.tokenIn, fixture.tokenOut, fixture.adapter), orders, nil, nil); err != nil {
		t.Fatal(err)
	}
	metricstest.RequireWorkflowEventCount(t, reg, Name, "fill", liquidlane.FillOutcomeSuccess, 1)
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairEventLabels(solver.chainID, fixture.tokenIn, fixture.tokenOut, "fill", "mined_success", "none"), 1)
	metricstest.RequireNoWorkflowAmounts(t, reg, Name)
}
