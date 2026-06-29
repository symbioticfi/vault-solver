package bridgefacilitator

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"
)

func parse(t *testing.T, body string) (*Config, error) {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return parseConfig(*doc.Content[0]) // Content[0] is the mapping node (as the two-stage decode yields)
}

func mustParse(t *testing.T, body string) *Config {
	t.Helper()
	cfg, err := parse(t, body)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	return cfg
}

const minimalConfig = `
apiBaseUrl: https://bf.example
`

const oneTarget = `
apiBaseUrl: https://bf.example
adapters:
  - "0x0000000000000000000000000000000000000002"
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

func TestParseConfig_UnknownKeyRejected(t *testing.T) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(oneTarget+"redeemBatchSiez: 3\n"), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, err := parseConfig(*doc.Content[0]); err == nil {
		t.Fatal("expected a typo'd key to be rejected")
	}
}

func TestParseConfig_InvalidDurationRejected(t *testing.T) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(oneTarget+"intervals:\n  discover: \"1 hour\"\n"), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, err := parseConfig(*doc.Content[0]); err == nil {
		t.Fatal("expected an invalid duration to be rejected")
	}
}

func TestParseConfig_ZeroAdapterRejected(t *testing.T) {
	body := `
apiBaseUrl: https://bf.example
adapters:
  - "0x0000000000000000000000000000000000000000"
`
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, err := parseConfig(*doc.Content[0]); err == nil {
		t.Fatal("expected zero adapter address to be rejected")
	}
}

func TestParseConfig_AdaptersList(t *testing.T) {
	cfg, err := parse(t, minimalConfig+"adapters:\n  - \"0x0000000000000000000000000000000000000042\"\n  - \"0x0000000000000000000000000000000000000043\"\n")
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if len(cfg.Targets) != 2 ||
		cfg.Targets[0].Adapter != common.HexToAddress("0x0000000000000000000000000000000000000042") ||
		cfg.Targets[1].Adapter != common.HexToAddress("0x0000000000000000000000000000000000000043") {
		t.Fatalf("targets = %+v", cfg.Targets)
	}
}

func TestParseConfig_RejectsEmptyAndZeroAdapters(t *testing.T) {
	if _, err := parse(t, minimalConfig); err == nil {
		t.Fatal("expected an error when no adapters are configured")
	}
	if _, err := parse(t, minimalConfig+"adapters:\n  - \"0x0000000000000000000000000000000000000000\"\n"); err == nil {
		t.Fatal("expected an error for a zero adapter address")
	}
}
