package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadTxManagerStateFile(t *testing.T) {
	for _, tc := range []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "memory only"},
		{name: "relative durable path", path: "./state/transactions.json"},
		{name: "absolute durable path", path: "/var/lib/vault-solver/transactions.json"},
		{name: "blank path", path: " \t ", wantErr: true},
		{name: "NUL path", path: "state/tx\x00.json", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encodedPath, err := yaml.Marshal(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			body := strings.Replace(validConfig, "  maxFeeGwei: 100", "  maxFeeGwei: 100\n  stateFile: "+string(encodedPath), 1)
			cfg, err := Load(writeTemp(t, body))
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "txManager.stateFile") {
					t.Fatalf("Load error = %v, want stateFile validation failure", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.TxManager.StateFile != tc.path {
				t.Fatalf("stateFile = %q, want %q", cfg.TxManager.StateFile, tc.path)
			}
			if err := cfg.ValidateTxManager(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLoadTxManagerStateFileEnv(t *testing.T) {
	t.Setenv("VAULT_SOLVER_STATE_FILE", "/var/lib/vault-solver/transactions.json")
	body := strings.Replace(validConfig, "  maxFeeGwei: 100", "  maxFeeGwei: 100\n  stateFile: ${VAULT_SOLVER_STATE_FILE}", 1)
	cfg, err := Load(writeTemp(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TxManager.StateFile != "/var/lib/vault-solver/transactions.json" {
		t.Fatalf("stateFile = %q", cfg.TxManager.StateFile)
	}
}
