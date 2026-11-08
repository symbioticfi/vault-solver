package bridgefacilitator

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestLateRedeemReceiptRecordsOnlyCappedBatchSuccess(t *testing.T) {
	metrics, reg := newThreeFTestMetrics(t)
	seedThreeFObservation(metrics, threeFStateRedeemable)
	var submitted txmanager.Request
	s := &Solver{cfg: &Config{RedeemBatchSize: 2}, metrics: metrics, log: logr.Discard()}
	s.txManager = transactionSenderFunc(func(_ context.Context, req txmanager.Request) txmanager.Result {
		submitted = req
		return txmanager.Result{Outcome: txmanager.OutcomeAbandoned, Err: txmanager.ErrAbandoned}
	})
	ready := []common.Address{common.HexToAddress("0xb0"), common.HexToAddress("0xb1"), common.HexToAddress("0xb2")}
	s.redeemReady(t.Context(), Target{Adapter: common.HexToAddress("0xa0")}, ready)
	metricstest.RequireWorkflowEventCount(t, reg, Name, threeFEventRedeem, "success", 0)
	if submitted.ObserveReceipt == nil {
		t.Fatal("abandoned redemption has no receipt metrics observer")
	}
	s.cfg.RedeemBatchSize = 1
	ready[0] = common.Address{}
	submitted.ObserveReceipt(t.Context(), txmanager.Result{
		Outcome: txmanager.OutcomeConfirmed, Receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful},
	})
	metricstest.RequireWorkflowEventCount(t, reg, Name, threeFEventRedeem, "success", 2)
	requireThreeFObservation(t, reg, threeFStateRedeemable, 7, 123)
}

func TestLateRevertedRedeemReceiptDoesNotReportSuccess(t *testing.T) {
	metrics, reg := newThreeFTestMetrics(t)
	var submitted txmanager.Request
	s := &Solver{cfg: &Config{RedeemBatchSize: 2}, metrics: metrics, log: logr.Discard()}
	s.txManager = transactionSenderFunc(func(_ context.Context, req txmanager.Request) txmanager.Result {
		submitted = req
		return txmanager.Result{Outcome: txmanager.OutcomeAbandoned, Err: txmanager.ErrAbandoned}
	})
	s.redeemReady(t.Context(), Target{Adapter: common.HexToAddress("0xa0")}, []common.Address{common.HexToAddress("0xb0")})
	if submitted.ObserveReceipt == nil {
		t.Fatal("abandoned redemption has no receipt metrics observer")
	}
	submitted.ObserveReceipt(t.Context(), txmanager.Result{
		Outcome: txmanager.OutcomeReverted, Receipt: &types.Receipt{Status: types.ReceiptStatusFailed},
	})
	metricstest.RequireWorkflowEventCount(t, reg, Name, threeFEventRedeem, "success", 0)
}
