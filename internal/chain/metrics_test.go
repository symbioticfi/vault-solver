package chain

import (
	"context"
	"net/http"
	"testing"
)

func TestRPCMethodLabelsAreBounded(t *testing.T) {
	tests := map[string]string{
		rpcMethodCall:     rpcMethodCall,
		"eth_getBalance":  "eth_getBalance",
		"wallet_secretOp": "other",
		"":                "other",
	}
	for method, want := range tests {
		if got := boundedRPCMethodName(method); got != want {
			t.Errorf("boundedRPCMethodName(%q) = %q, want %q", method, got, want)
		}
	}
}

func TestClassifyRPCResponse(t *testing.T) {
	tests := []struct {
		name, method, body string
		status             int
		truncated          bool
		err                error
		want               rpcOutcome
	}{
		{"success", rpcMethodCall, `{"jsonrpc":"2.0","id":1,"result":"0x1"}`, http.StatusOK, false, nil, rpcOutcomeSuccess},
		{"rpc error", rpcMethodCall, `{"jsonrpc":"2.0","id":1,"error":{"code":-1}}`, http.StatusOK, false, nil, rpcOutcomeRPCError},
		{"batch error", "batch", `[{"jsonrpc":"2.0","id":1,"error":{"code":-1}}]`, http.StatusOK, false, nil, rpcOutcomeRPCError},
		{"decode error", rpcMethodCall, `{`, http.StatusOK, false, nil, rpcOutcomeDecodeError},
		{"large result", rpcMethodCall, `{`, http.StatusOK, true, nil, rpcOutcomeSuccess},
		{"rate limited", rpcMethodCall, ``, http.StatusTooManyRequests, false, nil, rpcOutcomeRateLimited},
		{"read canceled", rpcMethodCall, ``, http.StatusOK, false, context.Canceled, rpcOutcomeContextCanceled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyRPCResponse(test.method, test.status, []byte(test.body), test.truncated, test.err); got != test.want {
				t.Fatalf("outcome = %q, want %q", got, test.want)
			}
		})
	}
}
