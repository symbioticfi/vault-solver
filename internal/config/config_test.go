package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

const validConfig = `
chain:
  rpcUrl: https://sepolia.example.org
  chainId: 11155111
signer:
  keyEnv: SOLVER_PRIVATE_KEY
solver:
  name: 3f-bridge-facilitator
  config:
    apiBaseUrl: https://bf.dev.gcp.3f.xyz
    answer: 42
`

func TestLoad_ValidAppliesDefaults(t *testing.T) {
	cfg, err := Load(writeTemp(t, validConfig))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TxManager.Confirmations != DefaultConfirmations {
		t.Fatalf("expected default confirmations %d, got %d", DefaultConfirmations, cfg.TxManager.Confirmations)
	}
	if cfg.Observability.Addr != DefaultObservabilityAddr {
		t.Fatalf("expected default addr %q, got %q", DefaultObservabilityAddr, cfg.Observability.Addr)
	}
}

func TestLoad_TwoStageSolverDecode(t *testing.T) {
	cfg, err := Load(writeTemp(t, validConfig))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// The framework keeps solver.config opaque; a solver decodes it into its own type.
	var sub struct {
		APIBaseURL string `yaml:"apiBaseUrl"`
		Answer     int    `yaml:"answer"`
	}
	if err := cfg.Solver.Config.Decode(&sub); err != nil {
		t.Fatalf("decode solver.config: %v", err)
	}
	if sub.APIBaseURL != "https://bf.dev.gcp.3f.xyz" || sub.Answer != 42 {
		t.Fatalf("unexpected decoded solver config: %+v", sub)
	}
}

func TestLoad_ExpandsEnvButNotSecretNames(t *testing.T) {
	t.Setenv("TEST_RPC_URL", "https://rpc.from.env")
	body := `
chain:
  rpcUrl: ${TEST_RPC_URL}
  chainId: 11155111
signer:
  keyEnv: SOLVER_PRIVATE_KEY
solver:
  name: x
  config: {}
`
	cfg, err := Load(writeTemp(t, body))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Chain.RPCURL != "https://rpc.from.env" {
		t.Fatalf("rpcUrl not expanded from env: %q", cfg.Chain.RPCURL)
	}
	// The secret env-var NAME must pass through unchanged (it holds a name, not ${...}).
	if cfg.Signer.KeyEnv != "SOLVER_PRIVATE_KEY" {
		t.Fatalf("keyEnv should be the literal name, got %q", cfg.Signer.KeyEnv)
	}
}

func TestLoad_ExpandsEnvInSolverConfigBlock(t *testing.T) {
	// Expansion runs on the raw bytes before decode, so it reaches the opaque solver.config block
	// (the deferred two-stage decode) too — not just the framework-level fields.
	t.Setenv("TEST_API_URL", "https://api.from.env")
	body := `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
solver:
  name: x
  config:
    apiBaseUrl: ${TEST_API_URL}
`
	cfg, err := Load(writeTemp(t, body))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var sub struct {
		APIBaseURL string `yaml:"apiBaseUrl"`
	}
	if err := cfg.Solver.Config.Decode(&sub); err != nil {
		t.Fatalf("decode solver.config: %v", err)
	}
	if sub.APIBaseURL != "https://api.from.env" {
		t.Fatalf("solver.config not env-expanded: %q", sub.APIBaseURL)
	}
}

func TestLoad_RejectsInvalid(t *testing.T) {
	cases := map[string]string{ //nolint:gosec // G101 false positive: YAML test fixtures, not credentials.
		"missing rpcUrl": `
chain: {chainId: 1}
signer: {keyEnv: K}
solver: {name: x}
`,
		"missing chainId": `
chain: {rpcUrl: http://x}
signer: {keyEnv: K}
solver: {name: x}
`,
		"no signer source": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {}
solver: {name: x}
`,
		"both signer sources": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K, keystorePath: ./k.json, passphraseEnv: P}
solver: {name: x}
`,
		"keystore without passphrase": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keystorePath: ./k.json}
solver: {name: x}
`,
		"missing solver name": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
solver: {}
`,
		"unknown field": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
solver: {name: x}
bogus: true
`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(writeTemp(t, body)); err == nil {
				t.Fatalf("expected error for %q", name)
			}
		})
	}
}
