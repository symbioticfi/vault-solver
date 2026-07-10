package rfq

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"
)

func parseCfg(t *testing.T, body string) (*Config, error) {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return parseConfig(*doc.Content[0])
}

const minimalConfig = `
backendUrl: https://rfq-backend.example
backendSharedSecretEnv: RFQ_BACKEND_SHARED_SECRET
executor: "0x0000000000000000000000000000000000000010"
`

// oneAdapter is appended to make an external-mode config valid; external requires at least one adapter.
const oneAdapter = "adapters:\n  - \"0x0000000000000000000000000000000000000042\"\n"

func TestParseConfig_Defaults(t *testing.T) {
	cfg, err := parseCfg(t, minimalConfig+oneAdapter)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.ListenAddr != defaultListenAddr {
		t.Fatalf("listenAddr = %q, want %q", cfg.ListenAddr, defaultListenAddr)
	}
	if cfg.PollInterval != defaultPollInterval {
		t.Fatalf("pollInterval = %s, want %s", cfg.PollInterval, defaultPollInterval)
	}
	if cfg.OrderLimit != defaultOrderLimit {
		t.Fatalf("orderLimit = %d, want %d", cfg.OrderLimit, defaultOrderLimit)
	}
	if cfg.SolverMode != solverModeExternal {
		t.Fatalf("solverMode = %q, want %q", cfg.SolverMode, solverModeExternal)
	}
	if !cfg.restrictsToAdapters() {
		t.Fatal("external mode with a configured adapter should restrict execution to adapters")
	}
	if cfg.usesDiscounts() {
		t.Fatal("external mode should not use discounts")
	}
	if cfg.Strategy.Name != defaultStrategyName {
		t.Fatalf("strategy.name = %q, want %q", cfg.Strategy.Name, defaultStrategyName)
	}
}

func TestParseConfig_Strategy(t *testing.T) {
	cfg, err := parseCfg(t, minimalConfig+`
strategy:
  name: webhook
  config:
    url: https://strategy.example
`+oneAdapter)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Strategy.Name != "webhook" {
		t.Fatalf("strategy.name = %q, want webhook", cfg.Strategy.Name)
	}
	var raw struct {
		URL string `yaml:"url"`
	}
	if err := cfg.Strategy.Config.Decode(&raw); err != nil {
		t.Fatalf("decode strategy config: %v", err)
	}
	if raw.URL != "https://strategy.example" {
		t.Fatalf("strategy url = %q", raw.URL)
	}
}

func TestParseConfig_SolverMode(t *testing.T) {
	a := "\n" + oneAdapter
	cases := map[string]struct {
		yaml          string
		wantMode      string
		wantRestrict  bool
		wantDiscounts bool
		wantErr       bool
	}{
		"external + adapters":         {yaml: "solverMode: external" + a, wantMode: solverModeExternal, wantRestrict: true},
		"external, no adapters":       {yaml: "solverMode: external", wantErr: true},
		"internal + adapters":         {yaml: "solverMode: internal" + a, wantMode: solverModeInternal, wantDiscounts: true},
		"internal, no adapters":       {yaml: "solverMode: internal", wantMode: solverModeInternal, wantDiscounts: true},
		"default + adapters":          {yaml: a, wantMode: solverModeExternal, wantRestrict: true},
		"default, no adapters":        {wantErr: true},
		"invalid mode":                {yaml: "solverMode: hybrid", wantErr: true},
		"old whitelist flag rejected": {yaml: "adapterWhitelistEnabled: true" + a, wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, err := parseCfg(t, minimalConfig+tc.yaml+"\n")
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected the config to be rejected")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseConfig: %v", err)
			}
			if cfg.SolverMode != tc.wantMode {
				t.Fatalf("solverMode = %q, want %q", cfg.SolverMode, tc.wantMode)
			}
			if cfg.restrictsToAdapters() != tc.wantRestrict {
				t.Fatalf("restrictsToAdapters() = %v, want %v", cfg.restrictsToAdapters(), tc.wantRestrict)
			}
			if cfg.usesDiscounts() != tc.wantDiscounts {
				t.Fatalf("usesDiscounts() = %v, want %v", cfg.usesDiscounts(), tc.wantDiscounts)
			}
		})
	}
}

func TestParseConfig_QuoteScopesToAdapters(t *testing.T) {
	a := "\n" + oneAdapter
	cases := map[string]struct {
		yaml         string
		wantQuote    bool
		wantRestrict bool
	}{
		"external + adapters":   {yaml: "solverMode: external" + a, wantQuote: true, wantRestrict: true},
		"internal + adapters":   {yaml: "solverMode: internal" + a, wantQuote: true},
		"internal, no adapters": {yaml: "solverMode: internal"},
		"default + adapters":    {yaml: a, wantQuote: true, wantRestrict: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, err := parseCfg(t, minimalConfig+tc.yaml+"\n")
			if err != nil {
				t.Fatalf("parseConfig: %v", err)
			}
			if cfg.quoteScopesToAdapters() != tc.wantQuote {
				t.Fatalf("quoteScopesToAdapters() = %v, want %v", cfg.quoteScopesToAdapters(), tc.wantQuote)
			}
			if cfg.restrictsToAdapters() != tc.wantRestrict {
				t.Fatalf("restrictsToAdapters() = %v, want %v", cfg.restrictsToAdapters(), tc.wantRestrict)
			}
		})
	}
}

func TestParseConfig_UnknownKeyRejected(t *testing.T) {
	if _, err := parseCfg(t, minimalConfig+"pollIntervalMs: 100\nordreLimit: 5\n"); err == nil {
		t.Fatal("expected a typo'd key to be rejected")
	}
}

func TestParseConfig_Overrides(t *testing.T) {
	cfg, err := parseCfg(t, minimalConfig+`
listenAddr: ":9000"
pollIntervalMs: 1500
orderLimit: 5
reactor: "0x0000000000000000000000000000000000000030"
`+oneAdapter)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.ListenAddr != ":9000" ||
		cfg.PollInterval != 1500*time.Millisecond || cfg.OrderLimit != 5 {
		t.Fatalf("overrides not applied: %+v", cfg)
	}
	if cfg.Reactor == [20]byte{} {
		t.Fatalf("reactor not parsed")
	}
}

func TestParseConfig_Adapters(t *testing.T) {
	cfg, err := parseCfg(t, minimalConfig+oneAdapter)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if len(cfg.Adapters) != 1 {
		t.Fatalf("adapters = %d, want 1", len(cfg.Adapters))
	}
	v := cfg.Adapters[0]
	if v.Adapter != common.HexToAddress("0x0000000000000000000000000000000000000042") {
		t.Fatalf("adapter entry not parsed: %+v", v)
	}
	if v.Vault != (common.Address{}) || v.Asset != (common.Address{}) {
		t.Fatalf("vault/asset should be unresolved before startup: %+v", v)
	}
}

func TestParseConfig_BadAdapter(t *testing.T) {
	cases := map[string]string{
		"bad adapter address": `
adapters:
  - "not-an-address"
`,
		"zero adapter address": `
adapters:
  - "0x0000000000000000000000000000000000000000"
`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseCfg(t, minimalConfig+body); err == nil {
				t.Fatalf("expected an error for %q", name)
			}
		})
	}
}

func TestParseConfig_Errors(t *testing.T) {
	cases := map[string]string{ //nolint:gosec // G101 false positive: YAML test fixtures, not credentials.
		"missing backendUrl": `
backendSharedSecretEnv: S
executor: "0x0000000000000000000000000000000000000010"
`,
		"missing sharedSecretEnv": `
backendUrl: https://x
executor: "0x0000000000000000000000000000000000000010"
`,
		"bad executor": `
backendUrl: https://x
backendSharedSecretEnv: S
executor: "not-an-address"
`,
		"zero executor": `
backendUrl: https://x
backendSharedSecretEnv: S
executor: "0x0000000000000000000000000000000000000000"
solverMode: internal
`,
		"external mode requires adapters":       minimalConfig + "solverMode: external\n",
		"removed discountsEnabled key rejected": minimalConfig + "discountsEnabled: true\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseCfg(t, body); err == nil {
				t.Fatalf("expected an error for %q", name)
			}
		})
	}
}
