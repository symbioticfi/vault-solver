package chain

import (
	"math/big"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

func cancellationRPCResult(method string) string {
	switch method {
	case "eth_chainId":
		return `"0x7a69"`
	case rpcMethodSendRawTransaction:
		return `"0x0000000000000000000000000000000000000000000000000000000000000001"`
	case rpcMethodGetTransactionReceipt:
		return `null`
	default:
		return `"0x1"`
	}
}

func cancellationRPCTestTx() *types.Transaction {
	return types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(31337), GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(1),
		Gas: 21_000, Value: new(big.Int),
	})
}

func TestCancellationRPCRoutesBroadcastsWithoutMovingReads(t *testing.T) {
	for _, tc := range []struct {
		name           string
		separateWrite  bool
		separateCancel bool
	}{
		{name: "dedicated cancellation endpoint", separateWrite: true, separateCancel: true},
		{name: "cancellation endpoint with shared read and write", separateCancel: true},
		{name: "unset cancellation uses write", separateWrite: true},
		{name: "unset endpoints use primary"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var readMethods, writeMethods, cancelMethods []string
			read := rpcRecorder(&readMethods, cancellationRPCResult)
			defer read.Close()
			write := rpcRecorder(&writeMethods, cancellationRPCResult)
			defer write.Close()
			cancel := rpcRecorder(&cancelMethods, cancellationRPCResult)
			defer cancel.Close()
			var writeURL, cancelURL string
			if tc.separateWrite {
				writeURL = write.URL
			}
			if tc.separateCancel {
				cancelURL = cancel.URL
			}
			metrics, err := NewRPCMetrics(prometheus.NewRegistry())
			if err != nil {
				t.Fatal(err)
			}
			client, err := DialWithMetrics(t.Context(), []string{read.URL}, writeURL, cancelURL, testMulticall, 0, metrics)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			tx := cancellationRPCTestTx()
			if err := client.SendTransaction(t.Context(), tx); err != nil {
				t.Fatal(err)
			}
			if err := client.SendCancellationTransaction(t.Context(), tx); err != nil {
				t.Fatal(err)
			}
			if _, err := client.PendingNonceAt(t.Context(), common.Address{}); err != nil {
				t.Fatal(err)
			}
			if _, err := client.NonceAt(t.Context(), common.Address{}, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := client.TransactionReceipt(t.Context(), tx.Hash()); !errors.Is(err, ethereum.NotFound) {
				t.Fatalf("receipt error = %v", err)
			}
			if _, err := client.BlockNumber(t.Context()); err != nil {
				t.Fatal(err)
			}
			wantRead := []string{"eth_chainId"}
			var wantWrite, wantCancel []string
			writes := &wantRead
			if tc.separateWrite {
				wantWrite = append(wantWrite, "eth_chainId")
				writes = &wantWrite
			}
			*writes = append(*writes, rpcMethodSendRawTransaction)
			if tc.separateCancel {
				wantCancel = []string{"eth_chainId", rpcMethodSendRawTransaction}
				metricstest.RequireValue(t, metrics.requests.WithLabelValues("cancel", rpcMethodSendRawTransaction, string(rpcOutcomeSuccess)), 1)
			} else {
				*writes = append(*writes, rpcMethodSendRawTransaction)
			}
			*writes = append(*writes, rpcMethodGetTransactionCount, rpcMethodGetTransactionCount)
			wantRead = append(wantRead, rpcMethodGetTransactionReceipt, "eth_blockNumber")
			if !slices.Equal(readMethods, wantRead) || !slices.Equal(writeMethods, wantWrite) || !slices.Equal(cancelMethods, wantCancel) {
				t.Fatalf("RPC routing: read=%v want=%v; write=%v want=%v; cancel=%v want=%v", readMethods, wantRead, writeMethods, wantWrite, cancelMethods, wantCancel)
			}
		})
	}
}

func TestCancellationRPCRejectsWrongChain(t *testing.T) {
	var readMethods, cancelMethods []string
	read := rpcRecorder(&readMethods, cancellationRPCResult)
	defer read.Close()
	cancel := rpcRecorder(&cancelMethods, func(string) string { return `"0x1"` })
	defer cancel.Close()
	client, err := Dial(t.Context(), []string{read.URL}, "", cancel.URL, testMulticall, defaultRPCAttemptTimeout)
	if client != nil {
		client.Close()
	}
	if err == nil {
		t.Fatal("expected mismatched cancellation chain to fail startup")
	}
}

func TestCancellationRPCFailureDoesNotBroadcastToOtherEndpoints(t *testing.T) {
	var readMethods, writeMethods []string
	read := rpcRecorder(&readMethods, cancellationRPCResult)
	defer read.Close()
	write := rpcRecorder(&writeMethods, cancellationRPCResult)
	defer write.Close()
	var unavailable atomic.Bool
	cancel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if unavailable.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x7a69"}`))
	}))
	defer cancel.Close()
	client, err := Dial(t.Context(), []string{read.URL}, write.URL, cancel.URL, testMulticall, defaultRPCAttemptTimeout)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	unavailable.Store(true)
	if err := client.SendCancellationTransaction(t.Context(), cancellationRPCTestTx()); err == nil {
		t.Fatal("expected cancellation endpoint error")
	}
	if slices.Contains(readMethods, rpcMethodSendRawTransaction) || slices.Contains(writeMethods, rpcMethodSendRawTransaction) {
		t.Fatalf("cancellation leaked to another endpoint: read=%v write=%v", readMethods, writeMethods)
	}
}
