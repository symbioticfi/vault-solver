package chain

import (
	"testing"

	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
)

const testMulticall = "0xcA11bde05977b3631167028862bE2a173976CA11"

// emptyAggregate3Result is the ABI encoding of an empty aggregate3 Result[] return.
const emptyAggregate3Result = `"0x0000000000000000000000000000000000000000000000000000000000000020` +
	`0000000000000000000000000000000000000000000000000000000000000000"`

// chainRPCResult answers the chain id, a head block number, an empty multicall return, and a null
// receipt — what a node replies while a transaction is still unmined.
func chainRPCResult(method string) string {
	switch method {
	case "eth_blockNumber":
		return `"0x1"`
	case rpcMethodCall:
		return emptyAggregate3Result
	case rpcMethodGetTransactionReceipt:
		return `null`
	default:
		return `"0x7a69"` // 31337
	}
}

// Each RPC call is traced once by the HTTP transport.
func TestDialHTTP_TracesEachRequestOnce(t *testing.T) {
	rec := tracetest.Install(t)
	srv := rpcRecorder(new([]string), chainRPCResult)
	defer srv.Close()

	c, err := Dial(t.Context(), []string{srv.URL}, "", testMulticall, defaultRPCAttemptTimeout)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if _, err = c.Multicall(t.Context(), nil); err != nil {
		t.Fatalf("Multicall: %v", err)
	}
	if _, err = c.BlockNumber(t.Context()); err != nil {
		t.Fatalf("BlockNumber: %v", err)
	}

	if n := len(tracetest.AllEnded(rec, "chain.rpc.connect")); n != 0 {
		t.Fatalf("connect spans = %d, want none on the HTTP path", n)
	}
	for _, method := range []string{rpcMethodCall, "eth_blockNumber", rpcMethodChainID} {
		if n := len(tracetest.AllEnded(rec, method)); n != 1 {
			t.Fatalf("%s spans = %d, want the single transport span", method, n)
		}
	}
	for _, s := range rec.Ended() {
		if tracetest.Attr(s, "chain.rpc.transport") != "" {
			t.Fatalf("span %q carries chain.rpc.transport = %q on the HTTP path",
				s.Name(), tracetest.Attr(s, "chain.rpc.transport"))
		}
	}
}
