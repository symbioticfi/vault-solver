package bridgefacilitator

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type transactionSenderFunc func(context.Context, txmanager.Request) txmanager.Result

func (f transactionSenderFunc) Send(ctx context.Context, req txmanager.Request) txmanager.Result {
	return f(ctx, req)
}

func TestRedeemAllPublishesCompleteSnapshotBeforeSubmission(t *testing.T) {
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

	metrics, err := newThreeFMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	sent := 0
	s := &Solver{
		cfg:     &Config{RedeemBatchSize: 10},
		reader:  newReader(c, common.Address{}),
		log:     logr.Discard(),
		metrics: metrics,
		targets: []Target{{Adapter: adapter0}, {Adapter: adapter1}},
	}
	s.txManager = transactionSenderFunc(func(_ context.Context, req txmanager.Request) txmanager.Result {
		sent++
		if got := testutil.ToFloat64(metrics.observedItems.WithLabelValues(threeFStateRedeemable)); got != 2 {
			t.Fatalf("redeemable gauge at Send = %v, want complete snapshot 2", got)
		}
		if got := testutil.ToFloat64(metrics.lastObservation.WithLabelValues(threeFStateRedeemable)); got <= 0 {
			t.Fatalf("freshness at Send = %v, want published timestamp", got)
		}
		if req.Label != "redeem" || len(req.Data) == 0 {
			t.Fatalf("request label=%q dataLen=%d, want redeem calldata", req.Label, len(req.Data))
		}
		return txmanager.Result{Outcome: txmanager.OutcomeConfirmed}
	})

	s.redeemAll(t.Context())

	if sent != 2 {
		t.Fatalf("sent transactions = %d, want 2", sent)
	}
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

	metrics, err := newThreeFMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	metrics.observedItems.WithLabelValues(threeFStateRedeemable).Set(7)
	metrics.lastObservation.WithLabelValues(threeFStateRedeemable).Set(123)
	sent := 0
	s := &Solver{
		cfg:     &Config{RedeemBatchSize: 10},
		reader:  newReader(c, common.Address{}),
		log:     logr.Discard(),
		metrics: metrics,
		targets: []Target{{Adapter: adapterAddr}},
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
	if got := testutil.ToFloat64(metrics.observedItems.WithLabelValues(threeFStateRedeemable)); got != 7 {
		t.Fatalf("redeemable gauge = %v, want retained 7", got)
	}
	if got := testutil.ToFloat64(metrics.lastObservation.WithLabelValues(threeFStateRedeemable)); got != 123 {
		t.Fatalf("freshness = %v, want retained 123", got)
	}
}
