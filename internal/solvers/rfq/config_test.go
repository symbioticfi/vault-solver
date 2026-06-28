package rfq

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"
)

func parse(t *testing.T, body string) (*Config, error) {
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

// oneAdapter is appended to make an external-mode config valid — external requires at least one adapter.
const oneAdapter = "adapters:\n  - \"0x0000000000000000000000000000000000000042\"\n"

func TestParseConfig_Defaults(t *testing.T) {
	cfg, err := parse(t, minimalConfig+oneAdapter)
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
		t.Fatalf("solverMode = %q, want %q (default)", cfg.SolverMode, solverModeExternal)
	}
	if !cfg.restrictsToAdapters() {
		t.Fatal("external mode with a configured adapter should restrict to adapters")
	}
	if cfg.usesDiscounts() {
		t.Fatal("usesDiscounts should default to false (external mode: discounts API is internal-only)")
	}
}

func TestParseConfig_SolverMode(t *testing.T) {
	a := "\n" + oneAdapter
	cases := map[string]struct {
		yaml                              string
		wantMode                          string
		wantWhitelist, wantDiscounts, err bool
	}{
		"external + adapters → restrict, no discounts":    {yaml: "solverMode: external" + a, wantMode: "external", wantWhitelist: true, wantDiscounts: false},
		"external, no adapters → error":                   {yaml: "solverMode: external", err: true},
		"internal + adapters → discounts on, no restrict": {yaml: "solverMode: internal" + a, wantMode: "internal", wantWhitelist: false, wantDiscounts: true},
		"internal, no adapters → discounts on (optional)": {yaml: "solverMode: internal", wantMode: "internal", wantWhitelist: false, wantDiscounts: true},
		"default (unset) + adapters → external":           {yaml: a, wantMode: "external", wantWhitelist: true, wantDiscounts: false},
		"default (unset), no adapters → error":            {yaml: "", err: true},
		"invalid mode → error":                            {yaml: "solverMode: hybrid", err: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, err := parse(t, minimalConfig+tc.yaml+"\n")
			if tc.err {
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
			if cfg.restrictsToAdapters() != tc.wantWhitelist {
				t.Fatalf("restrictsToAdapters() = %v, want %v", cfg.restrictsToAdapters(), tc.wantWhitelist)
			}
			if cfg.usesDiscounts() != tc.wantDiscounts {
				t.Fatalf("usesDiscounts() = %v, want %v", cfg.usesDiscounts(), tc.wantDiscounts)
			}
		})
	}
}

// TestParseConfig_QuoteScopesToAdapters pins the quote-vs-execution scoping split: the QUOTE path scopes
// to configured adapters in BOTH external and internal mode (quoteScopesToAdapters), while execution
// scoping (restrictsToAdapters) stays external-only. The internal+adapters row is the new behavior — an
// internal-mode filler advertises quotes only for its own adapter universe without restricting filling.
func TestParseConfig_QuoteScopesToAdapters(t *testing.T) {
	a := "\n" + oneAdapter
	type want struct {
		quoteScope   bool // quoteScopesToAdapters()
		execRestrict bool // restrictsToAdapters()
	}
	cases := map[string]struct {
		yaml string
		want want
	}{
		"external + adapters → quote scoped, exec restricted": {
			yaml: "solverMode: external" + a,
			want: want{quoteScope: true, execRestrict: true},
		},
		"internal + adapters → quote scoped, exec unrestricted": {
			yaml: "solverMode: internal" + a,
			want: want{quoteScope: true, execRestrict: false},
		},
		"internal, no adapters → neither scoped": {
			yaml: "solverMode: internal",
			want: want{quoteScope: false, execRestrict: false},
		},
		"default (unset) + adapters → quote scoped, exec restricted": {
			yaml: a,
			want: want{quoteScope: true, execRestrict: true},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, err := parse(t, minimalConfig+tc.yaml+"\n")
			if err != nil {
				t.Fatalf("parseConfig: %v", err)
			}
			if cfg.quoteScopesToAdapters() != tc.want.quoteScope {
				t.Fatalf("quoteScopesToAdapters() = %v, want %v", cfg.quoteScopesToAdapters(), tc.want.quoteScope)
			}
			if cfg.restrictsToAdapters() != tc.want.execRestrict {
				t.Fatalf("restrictsToAdapters() = %v, want %v", cfg.restrictsToAdapters(), tc.want.execRestrict)
			}
			// Quote scoping must always be a superset of execution scoping (filling never scopes when
			// quoting doesn't).
			if tc.want.execRestrict && !tc.want.quoteScope {
				t.Fatal("invariant broken: execution restricted but quote not scoped")
			}
		})
	}
}

func TestParseConfig_UnknownKeyRejected(t *testing.T) {
	if _, err := parse(t, minimalConfig+"pollIntervalMs: 100\nordreLimit: 5\n"); err == nil {
		t.Fatal("expected a typo'd key to be rejected")
	}
}

func TestParseConfig_Overrides(t *testing.T) {
	cfg, err := parse(t, minimalConfig+`
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
	cfg, err := parse(t, minimalConfig+`
adapters:
  - "0x0000000000000000000000000000000000000042"
`)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if len(cfg.Adapters) != 1 {
		t.Fatalf("adapters = %d, want 1", len(cfg.Adapters))
	}
	v := cfg.Adapters[0]
	// Only Adapter comes from config; Vault/Asset are resolved on-chain at startup (zero here).
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
		// A zero adapter feeds the whitelist; a placeholder must fail at startup.
		"zero adapter address": `
adapters:
  - "0x0000000000000000000000000000000000000000"
`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parse(t, minimalConfig+body); err == nil {
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
		// External mode (the default) has no discounts fallback, so an empty adapter list is rejected.
		"external mode (default) requires adapters":  minimalConfig,
		"external mode (explicit) requires adapters": minimalConfig + "solverMode: external\n",
		// Old flags folded into solverMode — a config still carrying them must fail (unknown key).
		"removed adapterWhitelistEnabled key rejected": minimalConfig + "adapterWhitelistEnabled: true\n",
		"removed discountsEnabled key rejected":        minimalConfig + "discountsEnabled: true\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parse(t, body); err == nil {
				t.Fatalf("expected an error for %q", name)
			}
		})
	}
}
