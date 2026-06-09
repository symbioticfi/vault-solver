package rfq

import (
	"testing"
	"time"

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
adapter: "0x0000000000000000000000000000000000000020"
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

func TestParseConfig_Errors(t *testing.T) {
	cases := map[string]string{ //nolint:gosec // G101 false positive: YAML test fixtures, not credentials.
		"missing backendUrl": `
backendSharedSecretEnv: S
executor: "0x0000000000000000000000000000000000000010"
adapter: "0x0000000000000000000000000000000000000020"
`,
		"missing sharedSecretEnv": `
backendUrl: https://x
executor: "0x0000000000000000000000000000000000000010"
adapter: "0x0000000000000000000000000000000000000020"
`,
		"bad executor": `
backendUrl: https://x
backendSharedSecretEnv: S
executor: "not-an-address"
adapter: "0x0000000000000000000000000000000000000020"
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
