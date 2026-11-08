package bridgefacilitator

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"

	txmanager "github.com/symbioticfi/merkle-executor/sdk/transaction"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

type transactionSenderFunc func(context.Context, txmanager.Request) txmanager.Result

func (f transactionSenderFunc) Send(ctx context.Context, req txmanager.Request) txmanager.Result {
	result := f(ctx, req)
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
		return txmanager.Result{
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
		return txmanager.Result{
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
	for _, outcome := range []error{txmanager.ErrNonceConsumed, context.DeadlineExceeded} {
		t.Run(outcome.Error(), func(t *testing.T) { redeemConsumedNonceRechecksRequestsWithoutReportingSuccess(t, outcome) })
	}
}

func redeemConsumedNonceRechecksRequestsWithoutReportingSuccess(t *testing.T, outcome error) {
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
		return txmanager.Result{
			Hash: common.HexToHash("0xfeed"), Err: outcome}
	})
	s.redeemAll(t.Context())
	s.redeemAll(t.Context())
	if sent != 1 {
		t.Fatalf("sent %d batches; consumed-nonce retry must exclude finalized requests", sent)
	}
	metricstest.RequireWorkflowEventCount(t, reg, Name, threeFEventRedeem, "success", 0)
	submit := tracetest.Ended(t, rec, "3f.redeem.submit")
	tracetest.RequireAttr(t, submit, "tx.outcome", "uncertain")
	if !tracetest.HasEvent(submit, "declined") {
		t.Fatal("consumed nonce did not record an expected decline")
	}
	tracetest.RequireNoErrorSpans(t, rec)
}

func TestRedeemExpiredBatchRebuildsFromFreshReadyRequests(t *testing.T) {
	adapter := common.HexToAddress("0xa0")
	finalized := common.HexToAddress("0xb0")
	pending := common.HexToAddress("0xb1")
	c, stop := newMulticallFakeClient(t,
		abiEncodeAggregate3Results(t, abiEncodeUint256(t, 2)),
		abiEncodeAggregate3Results(t, abiEncodeAddress(t, finalized), abiEncodeAddress(t, pending)),
		abiEncodeAggregate3Results(t, abiEncodeBool(t, true), abiEncodeBool(t, true)),
		abiEncodeAggregate3Results(t, abiEncodeUint256(t, 2)),
		abiEncodeAggregate3Results(t, abiEncodeAddress(t, finalized), abiEncodeAddress(t, pending)),
		abiEncodeAggregate3Results(t, abiEncodeBool(t, false), abiEncodeBool(t, true)),
	)
	defer stop()
	metrics, reg := newThreeFTestMetrics(t)
	const poll = 20 * time.Millisecond
	s := &Solver{
		cfg:    &Config{RedeemBatchSize: 10, Intervals: Intervals{RedeemPoll: poll}},
		reader: newReader(c, common.Address{}), log: logr.Discard(), metrics: metrics,
		targets: []Target{{Adapter: adapter}}, targetsAuthoritative: true,
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	sends := 0
	s.txManager = transactionSenderFunc(func(ctx context.Context, req txmanager.Request) txmanager.Result {
		sends++
		if req.Deadline.IsZero() || req.Deadline.After(time.Now().Add(poll)) {
			t.Fatalf("redemption batch has no next-scan deadline: %s", req.Deadline)
		}
		if sends == 1 {
			want := bfAdapter.PackMulticall([][]byte{bfAdapter.PackFinalizeRequest(finalized), bfAdapter.PackFinalizeRequest(pending)})
			if !bytes.Equal(req.Data, want) {
				t.Fatal("initial batch did not contain both ready requests")
			}
			batchCtx, stop := context.WithDeadline(ctx, req.Deadline)
			defer stop()
			<-batchCtx.Done()
			if ctx.Err() != nil {
				t.Fatal("stale redemption batch outlived its next scan")
			}
			// The first request was finalized elsewhere while this signed batch remained unresolved.
			return txmanager.Result{Hash: common.HexToHash("0xfeed"), Err: batchCtx.Err()}
		}
		want := bfAdapter.PackMulticall([][]byte{bfAdapter.PackFinalizeRequest(pending)})
		if !bytes.Equal(req.Data, want) {
			t.Fatal("fresh batch retained an externally finalized request")
		}
		return txmanager.Result{Receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful}}
	})
	s.redeemAll(ctx)
	metricstest.RequireWorkflowEventCount(t, reg, Name, threeFEventRedeem, "success", 0)
	s.redeemAll(ctx)
	if sends != 2 {
		t.Fatalf("sends=%d, want stale batch then freshly rebuilt batch", sends)
	}
	metricstest.RequireWorkflowEventCount(t, reg, Name, threeFEventRedeem, "success", 1)
}
