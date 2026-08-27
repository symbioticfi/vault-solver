package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigValidateCommandAcceptsExamples(t *testing.T) {
	setExampleEnvironment(t)
	paths, err := filepath.Glob(filepath.Join("..", "..", "config", "*.example.yaml"))
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no example configs found")
	}
	for _, path := range paths {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".example.yaml"), func(t *testing.T) {
			cmd := newConfigValidateCmd()
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetArgs([]string{"--config", path})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute config validate: %v", err)
			}
			if !strings.Contains(output.String(), "is valid") {
				t.Fatalf("output = %q", output.String())
			}
		})
	}
}

func TestValidateConfigFileRequiresFeesForTransactionSolver(t *testing.T) {
	path := writeConfigFile(t, `
chain:
  rpcUrl: https://rpc.example
  chainId: 1
signer:
  keyEnv: TEST_PRIVATE_KEY
solvers:
  - name: 3f-bridge-facilitator
    config:
      apiBaseUrl: https://3f.example
      adapterFactory: "0x1111111111111111111111111111111111111111"
      strategy: {name: default, config: {}}
`)
	err := validateConfigFile(path)
	if err == nil || !strings.Contains(err.Error(), "maxFeeGwei") {
		t.Fatalf("validateConfigFile() error = %v, want maxFeeGwei error", err)
	}
}

func TestValidateConfigFileDoesNotResolveWebhookSecretEnv(t *testing.T) {
	t.Setenv("MISSING_WEBHOOK_SECRET", "")
	path := writeConfigFile(t, `
chain:
  rpcUrl: https://rpc.example
  chainId: 1
signer:
  keyEnv: TEST_PRIVATE_KEY
txManager:
  maxFeeGwei: 50
solvers:
  - name: 3f-bridge-facilitator
    config:
      apiBaseUrl: https://3f.example
      adapterFactory: "0x1111111111111111111111111111111111111111"
      strategy:
        name: webhook
        config:
          url: https://strategy.example
          headers:
            Authorization:
              env: MISSING_WEBHOOK_SECRET
`)
	if err := validateConfigFile(path); err != nil {
		t.Fatalf("validateConfigFile() resolved a secret or rejected its env reference: %v", err)
	}
}

func TestValidateConfigFileRejectsUnknownStrategy(t *testing.T) {
	path := writeConfigFile(t, `
chain:
  rpcUrl: https://rpc.example
  chainId: 1
signer:
  keyEnv: TEST_PRIVATE_KEY
txManager:
  maxFeeGwei: 50
solvers:
  - name: 3f-bridge-facilitator
    config:
      apiBaseUrl: https://3f.example
      adapterFactory: "0x1111111111111111111111111111111111111111"
      strategy: {name: missing, config: {}}
`)
	err := validateConfigFile(path)
	if err == nil || !strings.Contains(err.Error(), "unknown 3F strategy") {
		t.Fatalf("validateConfigFile() error = %v, want unknown 3F strategy error", err)
	}
}

func setExampleEnvironment(t *testing.T) {
	t.Helper()
	values := map[string]string{
		"ETH_RPC_URL_MAINNET":        "https://rpc.example",
		"ETH_RPC_URL_SEPOLIA":        "https://rpc.example",
		"ETH_USD_FEED":               "0x0000000000000000000000000000000000000011",
		"LIQUIDLANE_ADAPTER":         "0x0000000000000000000000000000000000000012",
		"LIFI_EXECUTOR":              "0x0000000000000000000000000000000000000018",
		"OEV_MORPHO_API_URL":         "https://morpho.example/graphql",
		"RFQ_BACKEND_URL":            "https://rfq.example",
		"TLOAN_USD_FEED":             "0x0000000000000000000000000000000000000013",
		"TOKEN_TO_REDEEM":            "0x0000000000000000000000000000000000000014",
		"UNISWAPX_EXECUTOR":          "0x0000000000000000000000000000000000000015",
		"VAULT_ASSET":                "0x0000000000000000000000000000000000000016",
		"VAULT_ASSET_USD_FEED":       "0x0000000000000000000000000000000000000017",
		"WRITE_RPC_URL":              "https://write-rpc.example",
		"ETH_RPC_URL_MAINNET_BACKUP": "https://backup-rpc.example",
		"ETH_RPC_URL_SEPOLIA_BACKUP": "https://backup-rpc.example",
	}
	for name, value := range values {
		t.Setenv(name, value)
	}
}

func writeConfigFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
