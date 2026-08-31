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
