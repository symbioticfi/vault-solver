package lifi

import (
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
	"gopkg.in/yaml.v3"
)

func TestParseConfigValid(t *testing.T) {
	cfg := parseConfigYAML(t, `
orderServer:
  baseUrl: https://order.example
  wsUrl: wss://order.example
  apiKeyEnv: LIFI_SOLVER_API_KEY
inputSettler: "0x2222222222222222222222222222222222222222"
outputSettler: "0x3333333333333333333333333333333333333333"
executor: "0x4444444444444444444444444444444444444444"
adapters:
  - "0x5555555555555555555555555555555555555555"
tokensToQuote: permissioned
permissionedTokens:
  - "0x6666666666666666666666666666666666666666"
gas:
  nativeUsdFeed: "0x7777777777777777777777777777777777777777"
  nativeMaxAge: 30m
  tokenUsdFeeds:
    - token: "0x6666666666666666666666666666666666666666"
      feed: "0x8888888888888888888888888888888888888888"
      maxAge: 1h
strategy:
  name: default
quoteIntervalMs: 45000
quoteTtl: 90s
quoteRefreshMode: block
`)

	if cfg.OrderServer.BaseURL != "https://order.example" {
		t.Fatalf("baseURL = %q", cfg.OrderServer.BaseURL)
	}
	if cfg.OrderServer.WSURL != "wss://order.example" {
		t.Fatalf("wsURL = %q", cfg.OrderServer.WSURL)
	}
	if cfg.OrderServer.HTTPTimeout != defaultHTTPTimeout {
		t.Fatalf("httpTimeout = %s", cfg.OrderServer.HTTPTimeout)
	}
	if cfg.QuoteInterval != 45*time.Second {
		t.Fatalf("quoteInterval = %s", cfg.QuoteInterval)
	}
	if cfg.QuoteTTL != 90*time.Second {
		t.Fatalf("quoteTTL = %s", cfg.QuoteTTL)
	}
	if cfg.QuoteRefreshMode != quoteRefreshModeBlock {
		t.Fatalf("quoteRefreshMode = %q", cfg.QuoteRefreshMode)
	}
	if cfg.Strategy.Name != "default" {
		t.Fatalf("strategy = %q", cfg.Strategy.Name)
	}
	permissioned := common.HexToAddress("0x6666666666666666666666666666666666666666")
	if cfg.Gas.NativeUSDFeed.MaxAge != 30*time.Minute ||
		cfg.Gas.TokenUSDFeeds[permissioned].MaxAge != time.Hour {
		t.Fatalf("gas oracle config = %+v", cfg.Gas)
	}
	if cfg.SolverMode != solverModeExternal || cfg.usesDiscounts() {
		t.Fatalf("solver mode = %q discounts=%v", cfg.SolverMode, cfg.usesDiscounts())
	}
	if !cfg.TokenPolicy.RequiresSingleRoute(permissioned) {
		t.Fatalf("token scope = %q", cfg.TokenPolicy.Scope())
	}
	if _, err := newStrategy(cfg.Strategy); err != nil {
		t.Fatalf("newStrategy: %v", err)
	}
}

func TestParseConfigRejectsLegacySolverAddress(t *testing.T) {
	_, err := parseConfig(parseYAMLNode(t, `solverAddress: "0x1111111111111111111111111111111111111111"`))
	if err == nil || !strings.Contains(err.Error(), "solverAddress") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseConfigTokenScope(t *testing.T) {
	const base = `
orderServer:
  baseUrl: https://order.example
  wsUrl: wss://order.example
  apiKeyEnv: LIFI_SOLVER_API_KEY
inputSettler: "0x2222222222222222222222222222222222222222"
outputSettler: "0x3333333333333333333333333333333333333333"
executor: "0x4444444444444444444444444444444444444444"
adapters:
  - "0x5555555555555555555555555555555555555555"
permissionedTokens:
  - "0x6666666666666666666666666666666666666666"
gas:
  nativeUsdFeed: "0x7777777777777777777777777777777777777777"
  nativeMaxAge: 30m
  tokenUsdFeeds:
    - token: "0x6666666666666666666666666666666666666666"
      feed: "0x8888888888888888888888888888888888888888"
      maxAge: 1h
`
	permissioned := common.HexToAddress("0x6666666666666666666666666666666666666666")
	all := parseConfigYAML(t, base)
	if all.TokenPolicy.Scope() != tokenpolicy.All || all.TokenPolicy.RequiresSingleRoute(permissioned) {
		t.Fatalf("default policy = %q", all.TokenPolicy.Scope())
	}
	permissionedOnly := parseConfigYAML(t, base+"tokensToQuote: permissioned\n")
	if !permissionedOnly.TokenPolicy.Allows(permissioned) ||
		!permissionedOnly.TokenPolicy.RequiresSingleRoute(permissioned) {
		t.Fatal("permissioned scope did not admit and constrain configured token")
	}
	if _, err := parseConfig(parseYAMLNode(t, base+"tokensToQuote: bogus\n")); err == nil {
		t.Fatal("expected invalid tokensToQuote error")
	}
}

func TestParseConfigEnablesPrivateDiscountsOnlyInInternalMode(t *testing.T) {
	base := `
orderServer:
  baseUrl: https://order.example
  wsUrl: wss://order.example
  apiKeyEnv: LIFI_SOLVER_API_KEY
inputSettler: "0x2222222222222222222222222222222222222222"
outputSettler: "0x3333333333333333333333333333333333333333"
executor: "0x4444444444444444444444444444444444444444"
adapters:
  - "0x5555555555555555555555555555555555555555"
gas:
  nativeUsdFeed: "0x7777777777777777777777777777777777777777"
  nativeMaxAge: 30m
  tokenUsdFeeds:
    - token: "0x6666666666666666666666666666666666666666"
      feed: "0x8888888888888888888888888888888888888888"
      maxAge: 1h
`
	cfg := parseConfigYAML(t, base+"solverMode: internal\nprivateDiscountsUrl: https://rfq.example\n")
	if !cfg.usesDiscounts() || cfg.DiscountsURL != "https://rfq.example" {
		t.Fatalf("config = %+v", cfg)
	}

	for _, raw := range []string{
		base + "solverMode: internal\n",
		base + "privateDiscountsUrl: https://rfq.example\n",
	} {
		if _, err := parseConfig(parseYAMLNode(t, raw)); err == nil {
			t.Fatalf("expected mode/url validation error for %q", raw)
		}
	}
}

func TestParseConfigRejectsMissingAPIKeyEnv(t *testing.T) {
	_, err := parseConfig(parseYAMLNode(t, `
inputSettler: "0x2222222222222222222222222222222222222222"
outputSettler: "0x3333333333333333333333333333333333333333"
executor: "0x4444444444444444444444444444444444444444"
adapters:
  - "0x5555555555555555555555555555555555555555"
`))
	if err == nil || !strings.Contains(err.Error(), "orderServer.apiKeyEnv is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseConfigRequiresOrderServerEndpoints(t *testing.T) {
	const base = `
orderServer:
  apiKeyEnv: LIFI_SOLVER_API_KEY
%s
inputSettler: "0x2222222222222222222222222222222222222222"
outputSettler: "0x3333333333333333333333333333333333333333"
executor: "0x4444444444444444444444444444444444444444"
adapters:
  - "0x5555555555555555555555555555555555555555"
`
	for _, tc := range []struct {
		name     string
		endpoint string
		want     string
	}{
		{name: "base URL", endpoint: "  wsUrl: wss://order.example", want: "orderServer.baseUrl is required"},
		{name: "websocket URL", endpoint: "  baseUrl: https://order.example", want: "orderServer.wsUrl is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseConfig(parseYAMLNode(t, strings.Replace(base, "%s", tc.endpoint, 1)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestParseConfigRequiresGasOracleFeeds(t *testing.T) {
	_, err := parseConfig(parseYAMLNode(t, `
orderServer:
  baseUrl: https://order.example
  wsUrl: wss://order.example
  apiKeyEnv: LIFI_SOLVER_API_KEY
inputSettler: "0x2222222222222222222222222222222222222222"
outputSettler: "0x3333333333333333333333333333333333333333"
executor: "0x4444444444444444444444444444444444444444"
adapters:
  - "0x5555555555555555555555555555555555555555"
`))
	if err == nil || !strings.Contains(err.Error(), "gas.nativeUsdFeed") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseGasConfigRequiresPerFeedMaxAge(t *testing.T) {
	const address = "0x7777777777777777777777777777777777777777"
	for _, tc := range []struct {
		name string
		raw  liquidlanegas.RawConfig
		want string
	}{
		{
			name: "native",
			raw: liquidlanegas.RawConfig{
				NativeUSDFeed: address,
				TokenUSDFeeds: []liquidlanegas.RawTokenFeed{{Token: address, Feed: address, MaxAge: "1h"}},
			},
			want: "gas.nativeMaxAge is required",
		},
		{
			name: "token",
			raw: liquidlanegas.RawConfig{
				NativeUSDFeed: address,
				NativeMaxAge:  "1h",
				TokenUSDFeeds: []liquidlanegas.RawTokenFeed{{Token: address, Feed: address}},
			},
			want: "gas.tokenUsdFeeds[0].maxAge is required",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := liquidlanegas.ParseConfig(tc.raw)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestParseConfigDefaultsToBlockPollingAndShortTTL(t *testing.T) {
	cfg := parseConfigYAML(t, `
orderServer:
  baseUrl: https://order.example
  wsUrl: wss://order.example
  apiKeyEnv: LIFI_SOLVER_API_KEY
inputSettler: "0x2222222222222222222222222222222222222222"
outputSettler: "0x3333333333333333333333333333333333333333"
executor: "0x4444444444444444444444444444444444444444"
adapters:
  - "0x5555555555555555555555555555555555555555"
gas:
  nativeUsdFeed: "0x7777777777777777777777777777777777777777"
  nativeMaxAge: 30m
  tokenUsdFeeds:
    - token: "0x6666666666666666666666666666666666666666"
      feed: "0x8888888888888888888888888888888888888888"
      maxAge: 1h
`)
	if cfg.QuoteInterval != time.Second {
		t.Fatalf("quoteInterval = %s", cfg.QuoteInterval)
	}
	if cfg.QuoteRefreshMode != quoteRefreshModeBlock {
		t.Fatalf("quoteRefreshMode = %q", cfg.QuoteRefreshMode)
	}
	if cfg.QuoteTTL != 36*time.Second {
		t.Fatalf("quoteTTL = %s", cfg.QuoteTTL)
	}
}

func TestParseConfigRejectsQuoteTTLBelowTwiceRefreshInterval(t *testing.T) {
	_, err := parseConfig(parseYAMLNode(t, `
orderServer:
  baseUrl: https://order.example
  wsUrl: wss://order.example
  apiKeyEnv: LIFI_SOLVER_API_KEY
inputSettler: "0x2222222222222222222222222222222222222222"
outputSettler: "0x3333333333333333333333333333333333333333"
executor: "0x4444444444444444444444444444444444444444"
adapters:
  - "0x5555555555555555555555555555555555555555"
quoteIntervalMs: 30000
quoteTtl: 30s
`))
	if err == nil || !strings.Contains(err.Error(), "quoteTtl must be at least twice quote interval") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseConfigRejectsDuplicateAdapters(t *testing.T) {
	_, err := parseConfig(parseYAMLNode(t, `
orderServer:
  baseUrl: https://order.example
  wsUrl: wss://order.example
  apiKeyEnv: LIFI_SOLVER_API_KEY
inputSettler: "0x2222222222222222222222222222222222222222"
outputSettler: "0x3333333333333333333333333333333333333333"
executor: "0x4444444444444444444444444444444444444444"
adapters:
  - "0x5555555555555555555555555555555555555555"
  - "0x5555555555555555555555555555555555555555"
`))
	if err == nil || !strings.Contains(err.Error(), "duplicate adapter") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseConfigRejectsInvalidPermissionedTokens(t *testing.T) {
	base := `
orderServer:
  baseUrl: https://order.example
  wsUrl: wss://order.example
  apiKeyEnv: LIFI_SOLVER_API_KEY
inputSettler: "0x2222222222222222222222222222222222222222"
outputSettler: "0x3333333333333333333333333333333333333333"
executor: "0x4444444444444444444444444444444444444444"
adapters:
  - "0x5555555555555555555555555555555555555555"
permissionedTokens:
`
	for _, entries := range []string{
		`  - "0x0000000000000000000000000000000000000000"`,
		`  - "0x6666666666666666666666666666666666666666"
  - "0x6666666666666666666666666666666666666666"`,
	} {
		if _, err := parseConfig(parseYAMLNode(t, base+entries+"\n")); err == nil {
			t.Fatalf("expected permissionedTokens validation error for:\n%s", entries)
		}
	}
}

func parseConfigYAML(t *testing.T, raw string) *Config {
	t.Helper()
	cfg, err := parseConfig(parseYAMLNode(t, raw))
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	return cfg
}

func parseYAMLNode(t *testing.T, raw string) yaml.Node {
	t.Helper()
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &node); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	if len(node.Content) != 1 {
		t.Fatalf("unexpected yaml document content len %d", len(node.Content))
	}
	return *node.Content[0]
}

func testTokenPolicy(t *testing.T, scope tokenpolicy.Scope, tokens ...common.Address) tokenpolicy.Policy {
	t.Helper()
	policy, err := tokenpolicy.New(scope, tokens)
	if err != nil {
		t.Fatalf("tokenpolicy.New: %v", err)
	}
	return policy
}
