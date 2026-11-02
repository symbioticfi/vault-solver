package config

import (
	"strings"
	"testing"
)

func TestLoadRejectsRemovedChainWebSocket(t *testing.T) {
	body := strings.Replace(validConfig, "chain:\n", "chain:\n  wsUrl: wss://unused.example\n", 1)
	if _, err := Load(writeTemp(t, body)); err == nil || !strings.Contains(err.Error(), "wsUrl") {
		t.Fatalf("removed chain WebSocket field should be rejected, got %v", err)
	}
}

func TestLoadRejectsRemovedCancellationRPC(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "configured", value: "https://unused.example"},
		{name: "empty"},
		{name: "unset environment variable", value: "${REMOVED_CANCEL_RPC_URL}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("REMOVED_CANCEL_RPC_URL", "")
			body := `
chain:
  rpcUrl: https://read.example
  writeRpcUrl: https://write.example
  chainId: 1
  cancelRpcUrl: ` + tc.value + `
signer:
  keyEnv: SOLVER_PRIVATE_KEY
txManager:
  maxFeeGwei: 100
solvers:
  - name: x
    config: {}
`
			if _, err := Load(writeTemp(t, body)); err == nil || !strings.Contains(err.Error(), "cancelRpcUrl") {
				t.Fatalf("removed cancellation endpoint should be rejected, got %v", err)
			}
		})
	}
}
