package uniswapx

import (
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"
)

const validUniswapXGasConfig = `gas:
  nativeUsdFeed: "0x5555555555555555555555555555555555555555"
  nativeMaxAge: 1h
  tokenUsdFeeds:
    - token: "0x6666666666666666666666666666666666666666"
      feed: "0x7777777777777777777777777777777777777777"
      maxAge: 2h
`

const validUniswapXConfig = `
reactor: "0x1111111111111111111111111111111111111111"
executor: "0x2222222222222222222222222222222222222222"
adapters:
  - "0x4444444444444444444444444444444444444444"
tokensToQuote: all
quoteServer:
orderServer:
  baseUrl: https://api.uniswap.org/v2
  apiKeyEnv: UNISWAP_API_KEY
  sources:
    exclusiveV2: true
    publicV2: true
` + validUniswapXGasConfig + `strategy: {}
`

func TestParseConfigDefaultsAndSources(t *testing.T) {
	cfg, err := parseConfig(uniswapXConfigNode(t, validUniswapXConfig))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.QuoteServer.ListenAddress != defaultListenAddress ||
		cfg.QuoteServer.HTTPTimeout != defaultHTTPTimeout ||
		cfg.QuoteServer.RefreshInterval != defaultRefreshInterval ||
		cfg.QuoteServer.QuoteTTL != defaultQuoteTTL ||
		cfg.OrderServer.PollInterval != defaultPollInterval ||
		cfg.Strategy.Name != defaultStrategyName ||
		cfg.SolverMode != solverModeExternal ||
		cfg.usesDiscounts() {
		t.Fatalf("defaults were not applied: %+v", cfg)
	}
	if !cfg.OrderServer.Sources.ExclusiveV2 || !cfg.OrderServer.Sources.PublicV2 {
		t.Fatalf("sources = %+v", cfg.OrderServer.Sources)
	}
	if cfg.Gas == nil {
		t.Fatal("gas config = nil")
	}
	feed := cfg.Gas.TokenUSDFeeds[common.HexToAddress("0x6666666666666666666666666666666666666666")]
	if feed.MaxAge != 2*time.Hour {
		t.Fatalf("token feed max age = %s, want 2h", feed.MaxAge)
	}
}

func TestParseConfigAllowsMissingGas(t *testing.T) {
	raw := strings.Replace(validUniswapXConfig, validUniswapXGasConfig, "", 1)
	cfg, err := parseConfig(uniswapXConfigNode(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Gas != nil {
		t.Fatalf("gas config = %#v, want nil", cfg.Gas)
	}
}

func TestParseConfigValidatesOrderServerURL(t *testing.T) {
	tests := map[string]struct {
		baseURL string
		wantErr bool
	}{
		"https":           {baseURL: "https://api.uniswap.org/v2"},
		"loopback IPv4":   {baseURL: "http://127.0.0.1:8080/v2"},
		"loopback IPv6":   {baseURL: "http://[::1]:8080/v2"},
		"localhost":       {baseURL: "http://localhost:8080/v2"},
		"remote HTTP":     {baseURL: "http://api.uniswap.org/v2", wantErr: true},
		"relative URL":    {baseURL: "/v2", wantErr: true},
		"unsupported URL": {baseURL: "ftp://api.uniswap.org/v2", wantErr: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			raw := strings.Replace(validUniswapXConfig, "https://api.uniswap.org/v2", test.baseURL, 1)
			_, err := parseConfig(uniswapXConfigNode(t, raw))
			if (err != nil) != test.wantErr {
				t.Fatalf("parseConfig() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestParseConfigSolverMode(t *testing.T) {
	discounts := `discounts:
  baseUrl: https://backend.example
`
	withoutAdapters := strings.Replace(
		validUniswapXConfig,
		`adapters:
  - "0x4444444444444444444444444444444444444444"
`,
		"",
		1,
	)
	tests := map[string]struct {
		base          string
		suffix        string
		wantMode      string
		wantDiscounts bool
		wantRestrict  bool
		wantQuote     bool
		wantError     string
	}{
		"default external": {
			base: validUniswapXConfig, wantMode: solverModeExternal, wantRestrict: true, wantQuote: true,
		},
		"default external without adapters": {
			base: withoutAdapters, wantError: `solverMode "external" requires at least one adapters entry`,
		},
		"explicit external": {
			base: validUniswapXConfig, suffix: "solverMode: external\n",
			wantMode: solverModeExternal, wantRestrict: true, wantQuote: true,
		},
		"explicit external without adapters": {
			base: withoutAdapters, suffix: "solverMode: external\n",
			wantError: `solverMode "external" requires at least one adapters entry`,
		},
		"internal with adapters": {
			base: validUniswapXConfig, suffix: "solverMode: internal\n" + discounts,
			wantMode: solverModeInternal, wantDiscounts: true, wantQuote: true,
		},
		"internal without adapters": {
			base: withoutAdapters, suffix: "solverMode: internal\n" + discounts,
			wantMode: solverModeInternal, wantDiscounts: true,
		},
		"invalid": {
			base: validUniswapXConfig, suffix: "solverMode: hybrid\n", wantError: "solverMode: must be",
		},
		"internal without discounts": {
			base: validUniswapXConfig, suffix: "solverMode: internal\n",
			wantError: "discounts is required in internal solverMode",
		},
		"external with discounts": {
			base: validUniswapXConfig, suffix: "solverMode: external\n" + discounts,
			wantError: "discounts requires internal solverMode",
		},
		"default with discounts": {
			base: validUniswapXConfig, suffix: discounts, wantError: "discounts requires internal solverMode",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cfg, err := parseConfig(uniswapXConfigNode(t, test.base+test.suffix))
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("parseConfig() error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.SolverMode != test.wantMode || cfg.usesDiscounts() != test.wantDiscounts {
				t.Fatalf(
					"solverMode = %q, usesDiscounts = %t",
					cfg.SolverMode,
					cfg.usesDiscounts(),
				)
			}
			if cfg.restrictsToAdapters() != test.wantRestrict {
				t.Fatalf(
					"restrictsToAdapters() = %t, want %t",
					cfg.restrictsToAdapters(),
					test.wantRestrict,
				)
			}
			if cfg.quoteScopesToAdapters() != test.wantQuote {
				t.Fatalf(
					"quoteScopesToAdapters() = %t, want %t",
					cfg.quoteScopesToAdapters(),
					test.wantQuote,
				)
			}
		})
	}
}

func TestParseConfigDiscounts(t *testing.T) {
	raw := validUniswapXConfig + `solverMode: internal
discounts:
  baseUrl: https://backend.example
  httpTimeout: 3s
  minimumValidity: 20s
`
	cfg, err := parseConfig(uniswapXConfigNode(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Discounts == nil || cfg.Discounts.HTTPTimeout != 3*time.Second ||
		cfg.Discounts.MinimumValidity != 20*time.Second {
		t.Fatalf("discount config = %+v", cfg.Discounts)
	}

	raw = strings.Replace(raw, "https://backend.example", "http://backend.example", 1)
	if _, err := parseConfig(uniswapXConfigNode(t, raw)); err == nil {
		t.Fatal("expected unsafe discounts URL rejection")
	}
}

func TestParseConfigRejectsUnsafeOrderPolling(t *testing.T) {
	tests := map[string]string{
		"exclusive source disabled": strings.Replace(validUniswapXConfig, "exclusiveV2: true", "exclusiveV2: false", 1),
		"polls faster than 6 RPS": strings.Replace(
			validUniswapXConfig, "apiKeyEnv: UNISWAP_API_KEY", "apiKeyEnv: UNISWAP_API_KEY\n  pollInterval: 166ms", 1,
		),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseConfig(uniswapXConfigNode(t, raw)); err == nil {
				t.Fatal("expected config rejection")
			}
		})
	}
}

func TestParseConfigRejectsQuoteTTLWithoutRefreshHeadroom(t *testing.T) {
	raw := strings.Replace(validUniswapXConfig, "quoteServer:\n", `quoteServer:
  refreshInterval: 20s
  quoteTtl: 30s
`, 1)
	if _, err := parseConfig(uniswapXConfigNode(t, raw)); err == nil ||
		!strings.Contains(err.Error(), "quoteServer.quoteTtl must be at least twice refresh interval") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseConfigRejectsLegacyLimitSource(t *testing.T) {
	raw := strings.Replace(validUniswapXConfig, "publicV2: true", `publicV2: true
    limit:
      reactor: "0x8888888888888888888888888888888888888888"
      executor: "0x2222222222222222222222222222222222222222"`, 1)
	if _, err := parseConfig(uniswapXConfigNode(t, raw)); err == nil ||
		!strings.Contains(err.Error(), "field limit not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseConfigRejectsUnknownFields(t *testing.T) {
	if _, err := parseConfig(uniswapXConfigNode(t, validUniswapXConfig+"unknown: true\n")); err == nil {
		t.Fatal("expected strict decode rejection")
	}
}

func TestParseConfigRejectsLegacyCosignerPin(t *testing.T) {
	raw := strings.Replace(
		validUniswapXConfig,
		"executor: \"0x2222222222222222222222222222222222222222\"",
		"executor: \"0x2222222222222222222222222222222222222222\"\n"+
			"cosigner: \"0x3333333333333333333333333333333333333333\"",
		1,
	)
	if _, err := parseConfig(uniswapXConfigNode(t, raw)); err == nil {
		t.Fatal("expected legacy cosigner pin rejection")
	}
}

func uniswapXConfigNode(t *testing.T, raw string) yaml.Node {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &document); err != nil {
		t.Fatal(err)
	}
	return *document.Content[0]
}
