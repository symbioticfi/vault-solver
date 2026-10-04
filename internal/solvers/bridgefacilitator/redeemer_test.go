package bridgefacilitator

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type transactionSenderFunc func(context.Context, txmanager.Request) txmanager.Result

func (f transactionSenderFunc) Send(ctx context.Context, req txmanager.Request) txmanager.Result {
	result := f(ctx, req)
	if req.ObserveReceipt != nil && result.Receipt != nil {
		req.ObserveReceipt(ctx, result)
	}
	return result
}

func TestRedeemAllScansOncePerAdapterBeforeItsSend(t *testing.T) {
	t.Parallel()

	adapter0 := common.HexToAddress("0x00000000000000000000000000000000000000a0")
	adapter1 := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	request0 := common.HexToAddress("0x00000000000000000000000000000000000000b0")
	request1 := common.HexToAddress("0x00000000000000000000000000000000000000b1")
	c, stop := newMulticallFakeClient(t,
		abiEncodeAggregate3Results(t, abiEncodeUint256(t, 1)),
		abiEncodeAggregate3Results(t, abiEncodeAddress(t, request0)),
		abiEncodeAggregate3Results(t, abiEncodeBool(t, true)),
		abiEncodeAggregate3Results(t, abiEncodeUint256(t, 1)),
		abiEncodeAggregate3Results(t, abiEncodeAddress(t, request1)),
		abiEncodeAggregate3Results(t, abiEncodeBool(t, true)),
	)
	defer stop()

	metrics, reg := newThreeFTestMetrics(t)
	sent := 0
	s := &Solver{
		cfg:                  &Config{RedeemBatchSize: 10},
		reader:               newReader(c, common.Address{}),
		log:                  logr.Discard(),
		metrics:              metrics,
		targets:              []Target{{Adapter: adapter0}, {Adapter: adapter1}},
		targetsAuthoritative: true,
	}
	wantAdapters := []common.Address{adapter0, adapter1}
	s.txManager = transactionSenderFunc(func(_ context.Context, req txmanager.Request) txmanager.Result {
		wantAdapter := wantAdapters[sent]
		sent++
		metricstest.RequireFamilyValue(t, reg, "solver_bot_workflow_observed_items", map[string]string{
			"solver": Name, "view": threeFStateRedeemable,
		}, 0)
		if req.To != wantAdapter || req.Label != "redeem" || len(req.Data) == 0 {
			t.Fatalf("unexpected redeem request: to=%s label=%q dataLen=%d", req.To.Hex(), req.Label, len(req.Data))
		}
		return txmanager.Result{Outcome: txmanager.OutcomeConfirmed,
			Receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful}}
	})

	s.redeemAll(t.Context())

	if sent != 2 {
		t.Fatalf("sent transactions = %d, want one per ready adapter", sent)
	}
	metricstest.RequireFamilyValue(t, reg, "solver_bot_workflow_observed_items", map[string]string{
		"solver": Name, "view": threeFStateRedeemable,
	}, 2)
	if got := threeFObservationTimestamp(t, reg, threeFStateRedeemable); got <= 0 {
		t.Fatalf("redeemable freshness = %v, want completed-pass timestamp", got)
	}
	metricstest.RequireWorkflowEventCount(t, reg, Name, threeFEventRedeem, "success", 2)
}

func TestRedeemAllMalformedCanWithdrawRetainsFreshnessAndRedeemsValidSubset(t *testing.T) {
	t.Parallel()

	adapterAddr := common.HexToAddress("0x00000000000000000000000000000000000000a0")
	request0 := common.HexToAddress("0x00000000000000000000000000000000000000b0")
	request1 := common.HexToAddress("0x00000000000000000000000000000000000000b1")
	c, stop := newMulticallFakeClient(t,
		abiEncodeAggregate3Results(t, abiEncodeUint256(t, 2)),
		abiEncodeAggregate3Results(t, abiEncodeAddress(t, request0), abiEncodeAddress(t, request1)),
		abiEncodeAggregate3CallResults(t, []chain.CallResult{
			{Success: true, ReturnData: []byte{0x01}},
			{Success: true, ReturnData: abiEncodeBool(t, true)},
		}),
	)
	defer stop()

	metrics, reg := newThreeFTestMetrics(t)
	seedThreeFObservation(metrics, threeFStateRedeemable)
	sent := 0
	s := &Solver{
		cfg:                  &Config{RedeemBatchSize: 10},
		reader:               newReader(c, common.Address{}),
		log:                  logr.Discard(),
		metrics:              metrics,
		targets:              []Target{{Adapter: adapterAddr}},
		targetsAuthoritative: true,
	}
	s.txManager = transactionSenderFunc(func(_ context.Context, req txmanager.Request) txmanager.Result {
		sent++
		if req.To != adapterAddr {
			t.Fatalf("tx target = %s, want %s", req.To.Hex(), adapterAddr.Hex())
		}
		return txmanager.Result{Outcome: txmanager.OutcomeIncludedUnconfirmed,
			Receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful}}
	})

	s.redeemAll(t.Context())

	if sent != 1 {
		t.Fatalf("sent transactions = %d, want one valid-subset batch", sent)
	}
	requireThreeFObservation(t, reg, threeFStateRedeemable, 7, 123)
	metricstest.RequireWorkflowEventCount(t, reg, Name, threeFEventRedeem, "success", 1)
}

func TestRedeemConsumedNonceRechecksRequestsWithoutReportingSuccess(t *testing.T) {
	for _, outcome := range []txmanager.Outcome{txmanager.OutcomeNonceConsumed, txmanager.OutcomeNonceConflict, txmanager.OutcomeAbandoned} {
		t.Run(string(outcome), func(t *testing.T) { redeemConsumedNonceRechecksRequestsWithoutReportingSuccess(t, outcome) })
	}
}

func redeemConsumedNonceRechecksRequestsWithoutReportingSuccess(t *testing.T, outcome txmanager.Outcome) {
	t.Helper()
	rec := tracetest.Install(t)
	adapter := common.HexToAddress("0xa0")
	request := common.HexToAddress("0xb0")
	c, stop := newMulticallFakeClient(t,
		abiEncodeAggregate3Results(t, abiEncodeUint256(t, 1)),
		abiEncodeAggregate3Results(t, abiEncodeAddress(t, request)),
		abiEncodeAggregate3Results(t, abiEncodeBool(t, true)),
		abiEncodeAggregate3Results(t, abiEncodeUint256(t, 1)),
		abiEncodeAggregate3Results(t, abiEncodeAddress(t, request)),
		abiEncodeAggregate3Results(t, abiEncodeBool(t, false)),
	)
	defer stop()
	metrics, reg := newThreeFTestMetrics(t)
	sent := 0
	s := &Solver{
		cfg: &Config{RedeemBatchSize: 10}, reader: newReader(c, common.Address{}),
		log: logr.Discard(), metrics: metrics, targets: []Target{{Adapter: adapter}},
		targetsAuthoritative: true,
	}
	s.txManager = transactionSenderFunc(func(context.Context, txmanager.Request) txmanager.Result {
		sent++
		return txmanager.Result{Outcome: outcome,
			Hash: common.HexToHash("0xfeed"), Err: nonceOutcomeError(outcome)}
	})
	s.redeemAll(t.Context())
	s.redeemAll(t.Context())
	if sent != 1 {
		t.Fatalf("sent %d batches; consumed-nonce retry must exclude finalized requests", sent)
	}
	metricstest.RequireWorkflowEventCount(t, reg, Name, threeFEventRedeem, "success", 0)
	submit := tracetest.Ended(t, rec, "3f.redeem.submit")
	tracetest.RequireAttr(t, submit, "tx.outcome", string(outcome))
	if !tracetest.HasEvent(submit, "declined") {
		t.Fatal("consumed nonce did not record an expected decline")
	}
	tracetest.RequireNoErrorSpans(t, rec)
}

func nonceOutcomeError(outcome txmanager.Outcome) error {
	if outcome == txmanager.OutcomeAbandoned {
		return txmanager.ErrAbandoned
	}
	if outcome == txmanager.OutcomeNonceConflict {
		return txmanager.ErrNonceConflict
	}
	return txmanager.ErrNonceConsumed
}
