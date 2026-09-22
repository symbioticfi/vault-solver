package config

import "testing"

func TestLoadCancellationRPC(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field string
		value string
	}{
		{name: "configured", field: "  cancelRpcUrl: ${CANCEL_RPC_URL}\n", value: "https://cancel.example"},
		{name: "unset variable", field: "  cancelRpcUrl: ${CANCEL_RPC_URL}\n"},
		{name: "omitted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CANCEL_RPC_URL", tc.value)
			body := `
chain:
  rpcUrl: https://read.example
  writeRpcUrl: https://write.example
  chainId: 1
` + tc.field + `
signer:
  keyEnv: SOLVER_PRIVATE_KEY
txManager:
  maxFeeGwei: 100
solvers:
  - name: x
    config: {}
`
			cfg, err := Load(writeTemp(t, body))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Chain.CancelRPCURL != tc.value || cfg.Chain.WriteRPCURL != "https://write.example" || cfg.Chain.RPCURL != "https://read.example" {
				t.Fatalf("unexpected endpoint config: %+v", cfg.Chain)
			}
		})
	}
}
