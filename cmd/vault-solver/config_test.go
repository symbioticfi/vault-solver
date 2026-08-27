package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigValidateCommandAcceptsOfflineOEVConfig(t *testing.T) {
	setExampleEnvironment(t)
	path := filepath.Join("..", "..", "config", "redstone-oev.example.yaml")
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

func writeConfigFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
