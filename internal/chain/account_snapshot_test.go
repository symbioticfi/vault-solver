package chain

import (
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/prometheus/client_golang/prometheus"
)

// Telemetry reads stay on the read endpoint even when a distinct write endpoint is configured; the
// write endpoint only ever sees the dial-time chain-id check.
func TestAccountSnapshotUsesOnlyReadEndpoint(t *testing.T) {
	var readMethods, writeMethods []string
	read := rpcRecorder(&readMethods, cancellationRPCResult)
	defer read.Close()
	write := rpcRecorder(&writeMethods, cancellationRPCResult)
	defer write.Close()
	metrics, err := NewRPCMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	client, err := DialWithMetrics(t.Context(), []string{read.URL}, write.URL, "", testMulticall, 0, metrics)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	account := common.HexToAddress("0x1")
	balance, err := client.ReadBalanceAt(t.Context(), account)
	if err != nil {
		t.Fatal(err)
	}
	latest, pending, err := client.ReadNonces(t.Context(), account)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Int64() != 1 || latest != 1 || pending != 1 {
		t.Fatalf("snapshot = (%s, %d, %d), want the read endpoint's 0x1 values", balance, latest, pending)
	}
	for _, method := range []string{rpcMethodGetBalance, rpcMethodGetTransactionCount} {
		if !slices.Contains(readMethods, method) {
			t.Fatalf("read endpoint never saw %s: %v", method, readMethods)
		}
		if slices.Contains(writeMethods, method) {
			t.Fatalf("write endpoint served %s: %v", method, writeMethods)
		}
	}
}
