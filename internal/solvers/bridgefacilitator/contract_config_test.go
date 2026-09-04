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

const oneFactory = `
apiBaseUrl: https://bf.example
adapterFactory: "0x0000000000000000000000000000000000000003"
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

func TestParseConfig_OfferExpiryBufferDefaults(t *testing.T) {
	cfg := mustParse(t, oneTarget)
	if cfg.OfferExpiryBuffer != defaultOfferExpiryBuffer {
		t.Fatalf("offerExpiryBuffer = %s, want default %s", cfg.OfferExpiryBuffer, defaultOfferExpiryBuffer)
	}
}

func TestParseConfig_OfferExpiryBufferOverride(t *testing.T) {
	cfg := mustParse(t, oneTarget+"offerExpiryBuffer: 6h\n")
	if cfg.OfferExpiryBuffer != 6*time.Hour {
		t.Fatalf("offerExpiryBuffer = %s, want 6h", cfg.OfferExpiryBuffer)
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

func TestParseConfig_AdapterFactory(t *testing.T) {
	cfg := mustParse(t, oneFactory)
	want := common.HexToAddress("0x0000000000000000000000000000000000000003")
	if cfg.AdapterFactory != want {
		t.Fatalf("adapter factory = %s, want %s", cfg.AdapterFactory.Hex(), want.Hex())
	}
	if cfg.Targets != nil {
		t.Fatalf("static targets = %+v, want nil when adapters is omitted", cfg.Targets)
	}
}

func TestParseConfig_ExplicitEmptyAdaptersRemainAuthoritative(t *testing.T) {
	cfg := mustParse(t, oneFactory+"adapters: []\n")
	if cfg.Targets == nil || len(cfg.Targets) != 0 {
		t.Fatalf("static targets = %+v, want a present empty list", cfg.Targets)
	}
}

func TestParseConfig_StaticAndFactorySources(t *testing.T) {
	cfg := mustParse(t, oneTarget+`adapterFactory: "0x0000000000000000000000000000000000000003"
`)
	if len(cfg.Targets) != 1 {
		t.Fatalf("static targets = %+v, want one", cfg.Targets)
	}
	if cfg.AdapterFactory != common.HexToAddress("0x0000000000000000000000000000000000000003") {
		t.Fatalf("adapter factory = %s", cfg.AdapterFactory.Hex())
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

func TestParseConfig_RejectsEmptyAndZeroAdapterSources(t *testing.T) {
	if _, err := parse(t, minimalConfig); err == nil {
		t.Fatal("expected an error when neither adapters nor adapterFactory is configured")
	}
	if _, err := parse(t, minimalConfig+"adapters:\n  - \"0x0000000000000000000000000000000000000000\"\n"); err == nil {
		t.Fatal("expected an error for a zero adapter address")
	}
	if _, err := parse(t, minimalConfig+"adapterFactory: \"0x0000000000000000000000000000000000000000\"\n"); err == nil {
		t.Fatal("expected an error for a zero adapter factory address")
	}
}
