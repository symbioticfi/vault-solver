package bridgefacilitator

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type transactionSenderFunc func(context.Context, txmanager.Request) txmanager.Result

func (f transactionSenderFunc) Send(ctx context.Context, req txmanager.Request) txmanager.Result {
	return f(ctx, req)
}

func TestRedeemAllPublishesSnapshotAndRevalidatesBeforeSubmission(t *testing.T) {
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
		// Observation results never become calldata without a fresh per-adapter scan.
		abiEncodeAggregate3Results(t, abiEncodeUint256(t, 1)),
		abiEncodeAggregate3Results(t, abiEncodeAddress(t, request0)),
		abiEncodeAggregate3Results(t, abiEncodeBool(t, true)),
		abiEncodeAggregate3Results(t, abiEncodeUint256(t, 1)),
		abiEncodeAggregate3Results(t, abiEncodeAddress(t, request1)),
		abiEncodeAggregate3Results(t, abiEncodeBool(t, false)),
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
	s.txManager = transactionSenderFunc(func(_ context.Context, req txmanager.Request) txmanager.Result {
		sent++
		metricstest.RequireFamilyValue(t, reg, "solver_bot_workflow_observed_items", map[string]string{
			"solver": Name, "view": threeFStateRedeemable,
		}, 2)
		if got := threeFObservationTimestamp(t, reg, threeFStateRedeemable); got <= 0 {
			t.Fatalf("freshness at Send = %v, want published timestamp", got)
		}
		if req.To != adapter0 || req.Label != "redeem" || len(req.Data) == 0 {
			t.Fatalf("unexpected redeem request: to=%s label=%q dataLen=%d", req.To.Hex(), req.Label, len(req.Data))
		}
		return txmanager.Result{Outcome: txmanager.OutcomeConfirmed}
	})

	s.redeemAll(t.Context())

	if sent != 1 {
		t.Fatalf("sent transactions = %d, want 1 after revalidation", sent)
	}
	metricstest.RequireWorkflowEventCount(t, reg, Name, threeFEventRedeem, "success", 1)
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
		abiEncodeAggregate3Results(t, abiEncodeUint256(t, 2)),
		abiEncodeAggregate3Results(t, abiEncodeAddress(t, request0), abiEncodeAddress(t, request1)),
		abiEncodeAggregate3Results(t, abiEncodeBool(t, false), abiEncodeBool(t, true)),
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
		return txmanager.Result{Outcome: txmanager.OutcomeIncludedUnconfirmed}
	})

	s.redeemAll(t.Context())

	if sent != 1 {
		t.Fatalf("sent transactions = %d, want one valid-subset batch", sent)
	}
	requireThreeFObservation(t, reg, threeFStateRedeemable, 7, 123)
	metricstest.RequireWorkflowEventCount(t, reg, Name, threeFEventRedeem, "success", 1)
}
