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

func TestParseConfig_Defaults(t *testing.T) {
	cfg, err := parse(t, minimalConfig)
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
	if cfg.AdapterWhitelistEnabled {
		t.Fatal("adapterWhitelistEnabled should default to false")
	}
}

func TestParseConfig_AdapterWhitelistFlag(t *testing.T) {
	// Enabling requires at least one vaults entry (parseConfig rejects an empty whitelist).
	vaults := `
vaults:
  - address: "0x0000000000000000000000000000000000000041"
    adapter: "0x0000000000000000000000000000000000000042"
    asset: "0x0000000000000000000000000000000000000043"
`
	cases := map[string]struct {
		yaml string
		want bool
	}{
		"explicit true":  {yaml: "adapterWhitelistEnabled: true" + vaults, want: true},
		"explicit false": {yaml: "adapterWhitelistEnabled: false", want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, err := parse(t, minimalConfig+tc.yaml+"\n")
			if err != nil {
				t.Fatalf("parseConfig: %v", err)
			}
			if cfg.AdapterWhitelistEnabled != tc.want {
				t.Fatalf("adapterWhitelistEnabled = %v, want %v", cfg.AdapterWhitelistEnabled, tc.want)
			}
		})
	}
}

func TestParseConfig_Overrides(t *testing.T) {
	cfg, err := parse(t, minimalConfig+`
listenAddr: ":9000"
pollIntervalMs: 1500
orderLimit: 5
reactor: "0x0000000000000000000000000000000000000030"
`)
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

func TestParseConfig_Vaults(t *testing.T) {
	cfg, err := parse(t, minimalConfig+`
vaults:
  - address: "0x0000000000000000000000000000000000000041"
    adapter: "0x0000000000000000000000000000000000000042"
    asset: "0x0000000000000000000000000000000000000043"
`)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if len(cfg.Vaults) != 1 {
		t.Fatalf("vaults = %d, want 1", len(cfg.Vaults))
	}
	v := cfg.Vaults[0]
	if v.Vault != common.HexToAddress("0x0000000000000000000000000000000000000041") ||
		v.Adapter != common.HexToAddress("0x0000000000000000000000000000000000000042") ||
		v.AssetHint != common.HexToAddress("0x0000000000000000000000000000000000000043") {
		t.Fatalf("vault entry not parsed: %+v", v)
	}
}

func TestParseConfig_BadVault(t *testing.T) {
	cases := map[string]string{
		"bad adapter address": `
vaults:
  - address: "0x0000000000000000000000000000000000000041"
    adapter: "not-an-address"
    asset: "0x0000000000000000000000000000000000000043"
`,
		// Zero addresses feed the adapter whitelist; a placeholder must fail at startup.
		"zero adapter address": `
vaults:
  - address: "0x0000000000000000000000000000000000000041"
    adapter: "0x0000000000000000000000000000000000000000"
    asset: "0x0000000000000000000000000000000000000043"
`,
		"zero vault address": `
vaults:
  - address: "0x0000000000000000000000000000000000000000"
    adapter: "0x0000000000000000000000000000000000000042"
    asset: "0x0000000000000000000000000000000000000043"
`,
		"zero asset address": `
vaults:
  - address: "0x0000000000000000000000000000000000000041"
    adapter: "0x0000000000000000000000000000000000000042"
    asset: "0x0000000000000000000000000000000000000000"
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
		"whitelist enabled without vaults": minimalConfig + `
adapterWhitelistEnabled: true
`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parse(t, body); err == nil {
				t.Fatalf("expected an error for %q", name)
			}
		})
	}
}
