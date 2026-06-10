package bridgefacilitator

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func mustParse(t *testing.T, body string) *Config {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cfg, err := parseConfig(*doc.Content[0]) // Content[0] is the mapping node (as the two-stage decode yields)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	return cfg
}

const oneTarget = `
apiBaseUrl: https://bf.example
vault: "0x0000000000000000000000000000000000000001"
adapter: "0x0000000000000000000000000000000000000002"
`

func TestParseConfig_RedeemBatchSizeDefaults(t *testing.T) {
	cfg := mustParse(t, oneTarget)
	if cfg.RedeemBatchSize != defaultRedeemBatchSize {
		t.Fatalf("expected default %d, got %d", defaultRedeemBatchSize, cfg.RedeemBatchSize)
	}
}

func TestParseConfig_RedeemBatchSizeOverride(t *testing.T) {
	cfg := mustParse(t, oneTarget+"redeemBatchSize: 3\n")
	if cfg.RedeemBatchSize != 3 {
		t.Fatalf("expected 3, got %d", cfg.RedeemBatchSize)
	}
}
