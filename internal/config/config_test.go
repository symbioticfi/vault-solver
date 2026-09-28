package config

import (
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
	if cfg.TxManager.BroadcastTimeoutMs != DefaultBroadcastTimeoutMs ||
		cfg.TxManager.AccountPollIntervalMs != DefaultAccountPollIntervalMs ||
		cfg.TxManager.ReplacementIntervalMs != DefaultReplacementIntervalMs ||
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
		"negative broadcast timeout": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
txManager: {maxFeeGwei: 100, broadcastTimeoutMs: -1}
solvers: [{name: x}]
`,
		"negative account poll interval": `
chain: {rpcUrl: http://x, chainId: 1}
signer: {keyEnv: K}
txManager: {maxFeeGwei: 100, accountPollIntervalMs: -1}
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

// withTxManager returns validConfig with extra YAML lines appended to its txManager block. Each
// extra line is written relative to the block, e.g. "fees: {policy: horizon}".
func withTxManager(extra ...string) string {
	block := "txManager:\n  maxFeeGwei: 100\n"
	var lines strings.Builder
	for _, line := range extra {
		lines.WriteString("  " + line + "\n")
	}
	return strings.Replace(validConfig, block, block+lines.String(), 1)
}

func defaultTxFees() TxFeesConfig {
	return TxFeesConfig{
		Policy:                    FeePolicyLegacy,
		BlockTimeMs:               12_000,
		MinHorizonBlocks:          2,
		MaxHorizonBlocks:          6,
		PricingHorizonBlocks:      5,
		MaxHeadLagBlocks:          2,
		TipFloorGwei:              0.02,
		SingleFullBlockTipGwei:    0.1,
		CongestedTipFloorGwei:     0.2,
		CongestedTipCapGwei:       15,
		CongestedRewardBlocks:     3,
		CongestedRewardPercentile: 50,
		EscalateAfterFullMisses:   2,
		StallAfterRoomyMisses:     3,
	}
}

// TestLoad_TxManagerNestedBlocks pins the defaults of the fees/gas/balance/shadow blocks and how an
// omitted, empty, explicit-zero, or fully set block decodes.
func TestLoad_TxManagerNestedBlocks(t *testing.T) {
	defaultGas := TxGasConfig{HeadroomBps: 500, NextBlockEstimate: true, FallbackHeadroomBps: 1000, EstimateTimeoutMs: 5000}
	defaultBalance := TxBalanceConfig{Guard: true, FundingHysteresisBps: 2000}
	defaultShadow := TxShadowConfig{Enabled: true}
	for _, tc := range []struct {
		name        string
		body        string
		wantFees    TxFeesConfig
		wantGas     TxGasConfig
		wantBalance TxBalanceConfig
		wantShadow  TxShadowConfig
	}{
		{
			name:     "omitted blocks use defaults",
			body:     validConfig,
			wantFees: defaultTxFees(), wantGas: defaultGas, wantBalance: defaultBalance, wantShadow: defaultShadow,
		},
		{
			name:     "no txManager block uses defaults",
			body:     "chain: {rpcUrl: http://x, chainId: 1}\nsigner: {keyEnv: K}\nsolvers: [{name: x}]\n",
			wantFees: defaultTxFees(), wantGas: defaultGas, wantBalance: defaultBalance, wantShadow: defaultShadow,
		},
		{
			name:     "empty and null blocks keep defaults",
			body:     withTxManager("fees: {}", "gas:", "balance: null", "shadow: {}"),
			wantFees: defaultTxFees(), wantGas: defaultGas, wantBalance: defaultBalance, wantShadow: defaultShadow,
		},
		{
			name: "explicit zero and false are honoured",
			body: withTxManager(
				"fees: {maxHeadLagBlocks: 0}",
				"gas: {headroomBps: 0, fallbackHeadroomBps: 0, nextBlockEstimate: false}",
				"balance: {guard: false, fundingHysteresisBps: 0}",
				"shadow: {enabled: false}",
			),
			wantFees: func() TxFeesConfig {
				f := defaultTxFees()
				f.MaxHeadLagBlocks = 0
				return f
			}(),
			wantGas:     TxGasConfig{EstimateTimeoutMs: 5000},
			wantBalance: TxBalanceConfig{},
			wantShadow:  TxShadowConfig{},
		},
		{
			name: "every key set",
			body: withTxManager(
				"fees:",
				"  policy: horizon",
				"  blockTimeMs: 6000",
				"  minHorizonBlocks: 3",
				"  maxHorizonBlocks: 12",
				"  pricingHorizonBlocks: 7",
				"  maxHeadLagBlocks: 4",
				"  tipFloorGwei: 0.01",
				"  singleFullBlockTipGwei: 0.05",
				"  congestedTipFloorGwei: 0.5",
				"  congestedTipCapGwei: 5",
				"  congestedRewardBlocks: 4",
				"  congestedRewardPercentile: 75",
				"  escalateAfterFullMisses: 1",
				"  stallAfterRoomyMisses: 5",
				"gas: {headroomBps: 800, nextBlockEstimate: true, fallbackHeadroomBps: 1500, estimateTimeoutMs: 2500}",
				"balance: {guard: true, referenceGasUnits: 4350000, fundingHysteresisBps: 1000, targetEth: 0.1}",
				"shadow: {enabled: true}",
			),
			wantFees: TxFeesConfig{
				Policy: FeePolicyHorizon, BlockTimeMs: 6000, MinHorizonBlocks: 3, MaxHorizonBlocks: 12,
				PricingHorizonBlocks: 7, MaxHeadLagBlocks: 4, TipFloorGwei: 0.01, SingleFullBlockTipGwei: 0.05,
				CongestedTipFloorGwei: 0.5, CongestedTipCapGwei: 5, CongestedRewardBlocks: 4,
				CongestedRewardPercentile: 75, EscalateAfterFullMisses: 1, StallAfterRoomyMisses: 5,
			},
			wantGas:     TxGasConfig{HeadroomBps: 800, NextBlockEstimate: true, FallbackHeadroomBps: 1500, EstimateTimeoutMs: 2500},
			wantBalance: TxBalanceConfig{Guard: true, ReferenceGasUnits: 4_350_000, FundingHysteresisBps: 1000, TargetEth: 0.1},
			wantShadow:  defaultShadow,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Load(writeTemp(t, tc.body))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			got := cfg.TxManager
			if got.Fees != tc.wantFees || got.Gas != tc.wantGas || got.Balance != tc.wantBalance || got.Shadow != tc.wantShadow {
				t.Fatalf("nested txManager blocks:\n got fees=%+v gas=%+v balance=%+v shadow=%+v\nwant fees=%+v gas=%+v balance=%+v shadow=%+v",
					got.Fees, got.Gas, got.Balance, got.Shadow, tc.wantFees, tc.wantGas, tc.wantBalance, tc.wantShadow)
			}
		})
	}
}

// TestLoad_TxManagerNestedValidation walks every validation edge of the nested txManager blocks:
// the last accepted value and the first rejected one on each side.
func TestLoad_TxManagerNestedValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		extra   []string
		wantErr string // empty: the config must load
	}{
		{name: "horizon policy", extra: []string{"fees: {policy: horizon}"}},
		{name: "unknown policy", extra: []string{"fees: {policy: fast}"}, wantErr: "fees.policy"},
		{name: "empty policy", extra: []string{`fees: {policy: ""}`}, wantErr: "fees.policy"},
		{name: "policy is case sensitive", extra: []string{"fees: {policy: Horizon}"}, wantErr: "fees.policy"},

		{name: "block time 1ms", extra: []string{"fees: {blockTimeMs: 1}"}},
		{name: "zero block time", extra: []string{"fees: {blockTimeMs: 0}"}, wantErr: "fees.blockTimeMs"},
		{name: "negative block time", extra: []string{"fees: {blockTimeMs: -1}"}, wantErr: "fees.blockTimeMs"},
		{name: "block time overflows duration", extra: []string{"fees: {blockTimeMs: 9223372036855}"}, wantErr: "fees.blockTimeMs"},

		{name: "min horizon 1", extra: []string{"fees: {minHorizonBlocks: 1}"}},
		{name: "min horizon 0", extra: []string{"fees: {minHorizonBlocks: 0}"}, wantErr: "fees.minHorizonBlocks"},
		{name: "min horizon at the most the horizons allow", extra: []string{"fees: {minHorizonBlocks: 10, pricingHorizonBlocks: 12, maxHorizonBlocks: 12}"}},
		{name: "min horizon above limit", extra: []string{"fees: {minHorizonBlocks: 13}"}, wantErr: "fees.minHorizonBlocks"},
		{name: "min horizon that would overflow", extra: []string{"fees: {minHorizonBlocks: 9223372036854775807}"}, wantErr: "fees.minHorizonBlocks"},
		{name: "pricing horizon at min plus 2", extra: []string{"fees: {minHorizonBlocks: 3, pricingHorizonBlocks: 5}"}},
		{name: "pricing horizon below min plus 2", extra: []string{"fees: {minHorizonBlocks: 4, pricingHorizonBlocks: 5}"}, wantErr: "fees.pricingHorizonBlocks"},
		{name: "max horizon equals pricing", extra: []string{"fees: {pricingHorizonBlocks: 6, maxHorizonBlocks: 6}"}},
		{name: "max horizon below pricing", extra: []string{"fees: {pricingHorizonBlocks: 6, maxHorizonBlocks: 5}"}, wantErr: "fees.maxHorizonBlocks"},
		{name: "max horizon at limit", extra: []string{"fees: {maxHorizonBlocks: 12}"}},
		{name: "max horizon above limit", extra: []string{"fees: {maxHorizonBlocks: 13}"}, wantErr: "fees.maxHorizonBlocks"},

		{name: "max head lag 0", extra: []string{"fees: {maxHeadLagBlocks: 0}"}},
		{name: "negative max head lag", extra: []string{"fees: {maxHeadLagBlocks: -1}"}, wantErr: "fees.maxHeadLagBlocks"},

		{name: "tip floor of one wei", extra: []string{"fees: {tipFloorGwei: 0.000000001}"}},
		{name: "zero tip floor", extra: []string{"fees: {tipFloorGwei: 0}"}, wantErr: "fees.tipFloorGwei"},
		{name: "negative tip floor", extra: []string{"fees: {tipFloorGwei: -0.02}"}, wantErr: "fees.tipFloorGwei"},
		{name: "non-finite tip floor", extra: []string{"fees: {tipFloorGwei: .nan}"}, wantErr: "fees.tipFloorGwei"},
		{name: "flat tip ladder", extra: []string{"fees: {tipFloorGwei: 0.2, singleFullBlockTipGwei: 0.2, congestedTipFloorGwei: 0.2, congestedTipCapGwei: 0.2}"}},
		{name: "single-full tip below floor", extra: []string{"fees: {tipFloorGwei: 0.05, singleFullBlockTipGwei: 0.04}"}, wantErr: "fees.singleFullBlockTipGwei"},
		{name: "congested floor below single-full tip", extra: []string{"fees: {singleFullBlockTipGwei: 0.3}"}, wantErr: "fees.congestedTipFloorGwei"},
		{name: "congested cap below congested floor", extra: []string{"fees: {congestedTipCapGwei: 0.1}"}, wantErr: "fees.congestedTipCapGwei"},
		{name: "infinite congested cap", extra: []string{"fees: {congestedTipCapGwei: .inf}"}, wantErr: "fees.congestedTipCapGwei"},

		{name: "one reward block", extra: []string{"fees: {congestedRewardBlocks: 1}"}},
		{name: "zero reward blocks", extra: []string{"fees: {congestedRewardBlocks: 0}"}, wantErr: "fees.congestedRewardBlocks"},
		{name: "reward blocks at fee history limit", extra: []string{"fees: {congestedRewardBlocks: 1024}"}},
		{name: "reward blocks above fee history limit", extra: []string{"fees: {congestedRewardBlocks: 1025}"}, wantErr: "fees.congestedRewardBlocks"},
		{name: "percentile 100", extra: []string{"fees: {congestedRewardPercentile: 100}"}},
		{name: "percentile above 100", extra: []string{"fees: {congestedRewardPercentile: 100.5}"}, wantErr: "fees.congestedRewardPercentile"},
		{name: "percentile 0", extra: []string{"fees: {congestedRewardPercentile: 0}"}, wantErr: "fees.congestedRewardPercentile"},
		{name: "escalate after 1 miss", extra: []string{"fees: {escalateAfterFullMisses: 1}"}},
		{name: "escalate after 0 misses", extra: []string{"fees: {escalateAfterFullMisses: 0}"}, wantErr: "fees.escalateAfterFullMisses"},
		{name: "stall after 1 miss", extra: []string{"fees: {stallAfterRoomyMisses: 1}"}},
		{name: "stall after 0 misses", extra: []string{"fees: {stallAfterRoomyMisses: 0}"}, wantErr: "fees.stallAfterRoomyMisses"},

		{name: "tipGwei under legacy", extra: []string{"tipGwei: 1"}},
		{name: "tipGwei under horizon", extra: []string{"tipGwei: 1", "fees: {policy: horizon}"}, wantErr: "txManager.tipGwei"},

		{name: "headroom at cap", extra: []string{"gas: {headroomBps: 5000, fallbackHeadroomBps: 5000}"}},
		{name: "headroom above cap", extra: []string{"gas: {headroomBps: 5001, fallbackHeadroomBps: 6000}"}, wantErr: "gas.headroomBps"},
		{name: "negative headroom", extra: []string{"gas: {headroomBps: -1}"}, wantErr: "gas.headroomBps"},
		{name: "fallback headroom equals headroom", extra: []string{"gas: {headroomBps: 800, fallbackHeadroomBps: 800}"}},
		{name: "fallback headroom below headroom", extra: []string{"gas: {headroomBps: 800, fallbackHeadroomBps: 799}"}, wantErr: "gas.fallbackHeadroomBps"},
		{name: "fallback headroom at 100%", extra: []string{"gas: {fallbackHeadroomBps: 10000}"}},
		{name: "fallback headroom above 100%", extra: []string{"gas: {fallbackHeadroomBps: 10001}"}, wantErr: "gas.fallbackHeadroomBps"},
		{name: "estimate timeout 1ms", extra: []string{"gas: {estimateTimeoutMs: 1}"}},
		{name: "zero estimate timeout", extra: []string{"gas: {estimateTimeoutMs: 0}"}, wantErr: "gas.estimateTimeoutMs"},
		{name: "estimate timeout overflows duration", extra: []string{"gas: {estimateTimeoutMs: 9223372036855}"}, wantErr: "gas.estimateTimeoutMs"},

		{name: "negative reference gas", extra: []string{"balance: {referenceGasUnits: -1}"}, wantErr: "balance.referenceGasUnits"},
		{name: "hysteresis at 100%", extra: []string{"balance: {fundingHysteresisBps: 10000}"}},
		{name: "hysteresis above 100%", extra: []string{"balance: {fundingHysteresisBps: 10001}"}, wantErr: "balance.fundingHysteresisBps"},
		{name: "negative hysteresis", extra: []string{"balance: {fundingHysteresisBps: -1}"}, wantErr: "balance.fundingHysteresisBps"},
		{name: "negative target", extra: []string{"balance: {targetEth: -0.1}"}, wantErr: "balance.targetEth"},
		{name: "non-finite target", extra: []string{"balance: {targetEth: .inf}"}, wantErr: "balance.targetEth"},

		{name: "unknown fees key", extra: []string{"fees: {tipFloor: 0.02}"}, wantErr: "field tipFloor not found"},
		{name: "unknown gas key", extra: []string{"gas: {headroom: 800}"}, wantErr: "field headroom not found"},
		{name: "unknown balance key", extra: []string{"balance: {enabled: true}"}, wantErr: "field enabled not found"},
		{name: "unknown shadow key", extra: []string{"shadow: {policy: horizon}"}, wantErr: "field policy not found"},
		{name: "nested key at txManager level", extra: []string{"tipFloorGwei: 0.02"}, wantErr: "field tipFloorGwei not found"},
		{name: "scalar where a block is expected", extra: []string{"fees: horizon"}, wantErr: "cannot unmarshal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(writeTemp(t, withTxManager(tc.extra...)))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Load: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Load error = %v, want one mentioning %q", err, tc.wantErr)
			}
		})
	}
}
