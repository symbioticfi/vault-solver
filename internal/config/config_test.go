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
txManager:
  maxFeeGwei: 100
solvers:
  - name: 3f-bridge-facilitator
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
	if cfg.TxManager.ReplacementIntervalMs != DefaultReplacementIntervalMs ||
		cfg.TxManager.PendingTimeoutMs != DefaultPendingTimeoutMs ||
		cfg.TxManager.ShutdownTimeoutMs != DefaultShutdownTimeoutMs {
		t.Fatalf("unexpected tx replacement defaults: %+v", cfg.TxManager)
	}
	if cfg.Observability.Addr != DefaultObservabilityAddr {
		t.Fatalf("expected default addr %q, got %q", DefaultObservabilityAddr, cfg.Observability.Addr)
	}
}

const multiSolverConfig = `
chain:
  rpcUrl: https://sepolia.example.org
  chainId: 11155111
signer:
  keyEnv: SOLVER_PRIVATE_KEY
txManager:
  maxFeeGwei: 100
solvers:
  - name: 3f-bridge-facilitator
    config: {apiBaseUrl: https://bf.dev.gcp.3f.xyz}
  - name: rfq-filler
    config: {backendUrl: https://rfq.example}
`

func TestLoad_MultipleSolvers(t *testing.T) {
	cfg, err := Load(writeTemp(t, multiSolverConfig))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Solvers) != 2 ||
		cfg.Solvers[0].Name != "3f-bridge-facilitator" || cfg.Solvers[1].Name != "rfq-filler" {
		t.Fatalf("expected two distinct solvers, got %+v", cfg.Solvers)
	}
}

func TestLoad_RejectsDuplicateSolverType(t *testing.T) {
	dup := `
chain:
  rpcUrl: https://sepolia.example.org
  chainId: 11155111
signer:
  keyEnv: SOLVER_PRIVATE_KEY
txManager:
  maxFeeGwei: 100
solvers:
  - name: rfq-filler
    config: {}
  - name: rfq-filler
    config: {}
`
	if _, err := Load(writeTemp(t, dup)); err == nil {
		t.Fatal("expected an error for duplicate solver type")
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
	if err := cfg.Solvers[0].Config.Decode(&sub); err != nil {
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
txManager:
  maxFeeGwei: 100
solvers:
  - name: x
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

func TestLoad_ExpandsWriteRpcUrl(t *testing.T) {
	t.Setenv("WRITE_RPC_URL", "https://write.from.env")
	body := `
chain:
  rpcUrl: https://read.example
  writeRpcUrl: ${WRITE_RPC_URL}
  chainId: 1
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
		t.Fatalf("Load: %v", err)
	}
	if cfg.Chain.WriteRPCURL != "https://write.from.env" {
		t.Fatalf("writeRpcUrl not expanded from env: %q", cfg.Chain.WriteRPCURL)
	}
	// The general read RPC is untouched — writeRpcUrl affects broadcasts and account nonce reads.
	if cfg.Chain.RPCURL != "https://read.example" {
		t.Fatalf("rpcUrl changed unexpectedly: %q", cfg.Chain.RPCURL)
	}
}

func TestLoad_WriteRpcUrlOptional(t *testing.T) {
	body := `
chain:
  rpcUrl: https://read.example
  chainId: 1
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
		t.Fatalf("Load: %v", err)
	}
	if cfg.Chain.WriteRPCURL != "" {
		t.Fatalf("writeRpcUrl should default to empty, got %q", cfg.Chain.WriteRPCURL)
	}
}

func TestLoad_ExpandsEnvInSolverConfigBlock(t *testing.T) {
	// Expansion runs on the raw bytes before decode, so it reaches the opaque solver.config block
	// (the deferred two-stage decode) too — not just the framework-level fields.
	t.Setenv("TEST_API_URL", "https://api.from.env")
	body := `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
txManager: {maxFeeGwei: 100}
solvers:
  - name: x
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
	if err := cfg.Solvers[0].Config.Decode(&sub); err != nil {
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
solvers: [{name: x}]
`,
		"missing chainId": `
chain: {rpcUrl: http://x}
signer: {keyEnv: K}
solvers: [{name: x}]
`,
		"no signer source": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {}
solvers: [{name: x}]
`,
		"both signer sources": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K, keystorePath: ./k.json, passphraseEnv: P}
solvers: [{name: x}]
`,
		"keystore without passphrase": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keystorePath: ./k.json}
solvers: [{name: x}]
`,
		"missing solver name": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
txManager: {maxFeeGwei: 100}
solvers: [{}]
`,
		"negative max fee cap": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
txManager: {maxFeeGwei: -1}
solvers: [{name: x}]
`,
		"non-finite max fee cap": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
txManager: {maxFeeGwei: .nan}
solvers: [{name: x}]
`,
		"negative tip": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
txManager: {maxFeeGwei: 100, tipGwei: -1}
solvers: [{name: x}]
`,
		"unknown field": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
solvers: [{name: x}]
bogus: true
`,
		"negative replacement interval": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
txManager: {maxFeeGwei: 100, replacementIntervalMs: -1}
solvers: [{name: x}]
`,
		"timeout below replacement interval": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
txManager: {maxFeeGwei: 100, replacementIntervalMs: 30000, pendingTimeoutMs: 10000}
solvers: [{name: x}]
`,
		"negative shutdown timeout": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
txManager: {maxFeeGwei: 100, shutdownTimeoutMs: -1}
solvers: [{name: x}]
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

func TestValidateTxManagerRequiresMaxFeeOnlyWhenUsed(t *testing.T) {
	cfg, err := Load(writeTemp(t, `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
solvers: [{name: x}]
`))
	if err != nil {
		t.Fatalf("Load without txManager: %v", err)
	}
	if err := cfg.ValidateTxManager(); err == nil {
		t.Fatal("expected maxFeeGwei to be required for a transaction-sending solver")
	}

	cfg.TxManager.MaxFeeGwei = 100
	if err := cfg.ValidateTxManager(); err != nil {
		t.Fatalf("valid txManager: %v", err)
	}
}
