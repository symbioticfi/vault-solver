package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	if cfg.Observability.Addr != DefaultObservabilityAddr {
		t.Fatalf("expected default addr %q, got %q", DefaultObservabilityAddr, cfg.Observability.Addr)
	}
}

func TestLoad_TxManagerReplacementDefaults(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "omitted", body: validConfig},
		{
			name: "explicit zero",
			body: strings.Replace(validConfig, "signer:", `txManager:
  pendingIntervalMs: 0
  feeBumpBps: 0
  maxReplacements: 0
signer:`, 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(writeTemp(t, tt.body))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.TxManager.PendingIntervalMs != DefaultPendingIntervalMs {
				t.Fatalf("pendingIntervalMs = %d, want %d", cfg.TxManager.PendingIntervalMs, DefaultPendingIntervalMs)
			}
			if cfg.TxManager.FeeBumpBps != DefaultFeeBumpBps {
				t.Fatalf("feeBumpBps = %d, want %d", cfg.TxManager.FeeBumpBps, DefaultFeeBumpBps)
			}
			if cfg.TxManager.MaxReplacements != DefaultMaxReplacements {
				t.Fatalf("maxReplacements = %d, want %d", cfg.TxManager.MaxReplacements, DefaultMaxReplacements)
			}
		})
	}
}

func TestLoad_RejectsInvalidTxManagerReplacementPolicy(t *testing.T) {
	tests := []struct {
		name   string
		policy string
	}{
		{name: "negative pending interval", policy: "pendingIntervalMs: -1"},
		{name: "pending interval above 24 hours", policy: "pendingIntervalMs: 86400001"},
		{name: "pending interval duration overflow", policy: "pendingIntervalMs: 9223372036854775807"},
		{name: "fee bump below client replacement floor", policy: "feeBumpBps: 999"},
		{name: "fee bump above one hundred percent", policy: "feeBumpBps: 10001"},
		{name: "too many replacements", policy: "maxReplacements: 11"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.Replace(validConfig, "signer:", "txManager:\n  "+tt.policy+"\nsigner:", 1)
			if _, err := Load(writeTemp(t, body)); err == nil {
				t.Fatalf("expected %s to be rejected", tt.policy)
			}
		})
	}
}

func TestLoad_AcceptsTxManagerReplacementPolicyBounds(t *testing.T) {
	tests := []struct {
		name              string
		policy            string
		pendingIntervalMs int
		feeBumpBps        uint64
		maxReplacements   uint64
	}{
		{
			name:              "lower bounds",
			policy:            "pendingIntervalMs: 1\n  feeBumpBps: 1000\n  maxReplacements: 1",
			pendingIntervalMs: 1,
			feeBumpBps:        1_000,
			maxReplacements:   1,
		},
		{
			name:              "upper bounds",
			policy:            "pendingIntervalMs: 86400000\n  feeBumpBps: 10000\n  maxReplacements: 10",
			pendingIntervalMs: 86_400_000,
			feeBumpBps:        10_000,
			maxReplacements:   10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.Replace(validConfig, "signer:", "txManager:\n  "+tt.policy+"\nsigner:", 1)
			cfg, err := Load(writeTemp(t, body))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.TxManager.PendingIntervalMs != tt.pendingIntervalMs ||
				cfg.TxManager.FeeBumpBps != tt.feeBumpBps ||
				cfg.TxManager.MaxReplacements != tt.maxReplacements {
				t.Fatalf("replacement policy = %+v, want interval=%d bump=%d replacements=%d",
					cfg.TxManager, tt.pendingIntervalMs, tt.feeBumpBps, tt.maxReplacements)
			}
		})
	}
}

const multiSolverConfig = `
chain:
  rpcUrl: https://sepolia.example.org
  chainId: 11155111
signer:
  keyEnv: SOLVER_PRIVATE_KEY
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
	// The read RPC is untouched — writeRpcUrl only affects broadcasts.
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

func TestLoad_RejectsGenericWSURL(t *testing.T) {
	body := `
chain:
  rpcUrl: https://read.example
  wsUrl: wss://unused.example
  chainId: 1
signer:
  keyEnv: SOLVER_PRIVATE_KEY
solvers:
  - name: x
    config: {}
`
	if _, err := Load(writeTemp(t, body)); err == nil {
		t.Fatal("expected generic chain.wsUrl to be rejected")
	}
}

func TestLoad_ExpandsEnvInSolverConfigBlock(t *testing.T) {
	// Expansion runs on the raw bytes before decode, so it reaches the opaque solver.config block
	// (the deferred two-stage decode) too — not just the framework-level fields.
	t.Setenv("TEST_API_URL", "https://api.from.env")
	body := `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
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
solvers: [{}]
`,
		"unknown field": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
solvers: [{name: x}]
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

const txManagerFeeConfig = `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
txManager:
  maxFeeGwei: %s
  tipGwei: %s
solvers: [{name: x}]
`

func TestLoad_RejectsInvalidTxManagerFees(t *testing.T) {
	tests := []struct {
		name       string
		maxFeeGwei string
		tipGwei    string
		wantField  string
	}{
		{name: "negative max fee", maxFeeGwei: "-1", tipGwei: "0", wantField: "txManager.maxFeeGwei"},
		{name: "NaN max fee", maxFeeGwei: ".nan", tipGwei: "0", wantField: "txManager.maxFeeGwei"},
		{name: "positive infinite max fee", maxFeeGwei: ".inf", tipGwei: "0", wantField: "txManager.maxFeeGwei"},
		{name: "negative infinite max fee", maxFeeGwei: "-.inf", tipGwei: "0", wantField: "txManager.maxFeeGwei"},
		{name: "negative tip", maxFeeGwei: "0", tipGwei: "-1", wantField: "txManager.tipGwei"},
		{name: "NaN tip", maxFeeGwei: "0", tipGwei: ".nan", wantField: "txManager.tipGwei"},
		{name: "positive infinite tip", maxFeeGwei: "0", tipGwei: ".inf", wantField: "txManager.tipGwei"},
		{name: "negative infinite tip", maxFeeGwei: "0", tipGwei: "-.inf", wantField: "txManager.tipGwei"},
		{name: "tip above explicit max fee", maxFeeGwei: "2", tipGwei: "3", wantField: "txManager.maxFeeGwei"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeTemp(t, fmt.Sprintf(txManagerFeeConfig, tt.maxFeeGwei, tt.tipGwei)))
			if err == nil {
				t.Fatal("expected fee validation error")
			}
			if !strings.Contains(err.Error(), tt.wantField) {
				t.Fatalf("error %q does not name %s", err, tt.wantField)
			}
		})
	}
}

func TestLoad_AcceptsValidTxManagerFees(t *testing.T) {
	tests := []struct {
		name       string
		maxFeeGwei string
		tipGwei    string
	}{
		{name: "both derived", maxFeeGwei: "0", tipGwei: "0"},
		{name: "explicit max and suggested tip", maxFeeGwei: "2", tipGwei: "0"},
		{name: "derived max and explicit tip", maxFeeGwei: "0", tipGwei: "2"},
		{name: "equal explicit values", maxFeeGwei: "2", tipGwei: "2"},
		{name: "explicit tip below max", maxFeeGwei: "2", tipGwei: "1.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Load(writeTemp(t, fmt.Sprintf(txManagerFeeConfig, tt.maxFeeGwei, tt.tipGwei))); err != nil {
				t.Fatalf("Load: %v", err)
			}
		})
	}
}
