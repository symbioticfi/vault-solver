package bridgefacilitator

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
	if cfg.Strategy.Name != defaultStrategyName {
		t.Fatalf("strategy.name = %q, want %q", cfg.Strategy.Name, defaultStrategyName)
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

func TestParseConfigOfferTTL(t *testing.T) {
	cfg := mustParse(t, oneTarget+"intervals:\n  discover: 20m\n")
	if cfg.Intervals.OfferTTL != 40*time.Minute {
		t.Fatalf("offer TTL = %s, want 40m", cfg.Intervals.OfferTTL)
	}

	cfg = mustParse(t, oneTarget+"intervals:\n  discover: 20m\n  offerTTL: 45m\n")
	if cfg.Intervals.OfferTTL != 45*time.Minute {
		t.Fatalf("offer TTL = %s, want 45m", cfg.Intervals.OfferTTL)
	}

	if _, err := parse(t, oneTarget+"intervals:\n  discover: 20m\n  offerTTL: 19m\n"); err == nil {
		t.Fatal("expected offerTTL shorter than discover to fail")
	}
	if _, err := parse(t, oneTarget+"intervals:\n  discover: 2562047h47m16.854775807s\n"); err == nil {
		t.Fatal("expected discover too large to derive offerTTL to fail")
	}
}

func TestParseConfigOfferIntervalsAllowFractionalSeconds(t *testing.T) {
	cfg := mustParse(t, oneTarget+"intervals:\n  discover: 500ms\n  offerTTL: 750ms\n")
	if cfg.Intervals.Discover != 500*time.Millisecond || cfg.Intervals.OfferTTL != 750*time.Millisecond {
		t.Fatalf("intervals = %+v, want discover 500ms and offer TTL 750ms", cfg.Intervals)
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

func TestParseConfig_Strategy(t *testing.T) {
	cfg, err := parse(t, oneTarget+`
strategy:
  name: webhook
  config:
    url: https://strategy.example
`)
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

func TestParseConfig_RejectsEmptyAndZeroAdapters(t *testing.T) {
	if _, err := parse(t, minimalConfig); err == nil {
		t.Fatal("expected an error when no adapters are configured")
	}
	if _, err := parse(t, minimalConfig+"adapters:\n  - \"0x0000000000000000000000000000000000000000\"\n"); err == nil {
		t.Fatal("expected an error for a zero adapter address")
	}
}
