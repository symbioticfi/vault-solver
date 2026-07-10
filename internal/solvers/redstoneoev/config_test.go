package redstoneoev

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"

	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/default"
)

type exampleSolverEntry struct {
	Name   string    `yaml:"name"`
	Config yaml.Node `yaml:"config"`
}

type exampleConfigFile struct {
	Solvers []exampleSolverEntry `yaml:"solvers"`
}

// TestExampleConfigParses loads the committed Sepolia profile and runs its solver block through
// parseConfig, so the example can't drift out of sync with the parser/validation.
func TestExampleConfigParses(t *testing.T) {
	data, err := os.ReadFile("../../../config/redstone-oev.example.yaml")
	if err != nil {
		t.Fatalf("read example config: %v", err)
	}
	var top exampleConfigFile
	if err := yaml.Unmarshal(data, &top); err != nil {
		t.Fatal(err)
	}
	if len(top.Solvers) != 1 || top.Solvers[0].Name != Name {
		t.Fatalf("example must define exactly the %q solver, got %+v", Name, top.Solvers)
	}
	cfg, err := parseConfig(top.Solvers[0].Config)
	if err != nil {
		t.Fatalf("example config failed to parse: %v", err)
	}
	// Full liquidation is the production default; disabling it is the explicit fallback if settlement
	// routing ever has issues with full-collateral/bad-debt cases.
	strategyCfg := parseDefaultStrategyConfigForTest(t, cfg)
	if !strategyCfg.Sizing.AllowFullLiquidation {
		t.Fatal("example settings drifted: allowFullLiquidation must stay enabled")
	}
	if strategyCfg.LoanEthFeed == nil {
		t.Fatal("example settings drifted: default strategy config must carry a loan↔ETH rate source")
	}
}

func decodeCfg(t *testing.T, y string) (*Config, error) {
	t.Helper()
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(y), &node); err != nil {
		t.Fatal(err)
	}
	// The framework hands the solver the `config:` sub-node; here y is that node's content.
	return parseConfig(node)
}

func strategyBlock(extra string) string {
	return strategyConfigBlock("    morphoApiUrl: https://api.morpho.org/graphql\n    bid: {bidEth: \"0.0005\"}\n" + extra)
}

func strategyConfigBlock(body string) string {
	return "strategy:\n  name: default\n  config:\n    loanEthFeed: {ethUsd: \"0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2\", loanUsd: \"0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48\", maxAgeMs: 3600000}\n" + body
}

func parseDefaultStrategyConfigForTest(t *testing.T, c *Config) defaultstrategy.Config {
	t.Helper()
	cfg, err := defaultstrategy.ParseConfig(c.Strategy.Config)
	if err != nil {
		t.Fatalf("parse default strategy config: %v", err)
	}
	cfg.Adapter = c.Adapter
	return cfg
}

// TestConfigProfiles is the deployment matrix: each representative operating configuration must parse and
// validate, and produce the Config the operator expects. This is the offline proof that "the various
// configurations are all operable as expected" — every mode/combination the solver supports, exercised
// through the real parser+validator (the on-chain behavior of each is the operator's live runbook).
func TestConfigProfiles(t *testing.T) {
	cases := []struct {
		name  string
		yaml  string
		check func(*testing.T, *Config)
	}{
		{
			// Production: the Morpho API is the market source + a flat bid, with a loan↔ETH rate source so
			// the bundle-level after-cost profitability gate is active.
			name: "prod: API snapshot / flat bid",
			yaml: wsline + addrs + api + feedLine + okBid,
			check: func(t *testing.T, c *Config) {
				t.Helper()
				strategyCfg := parseDefaultStrategyConfigForTest(t, c)
				if strategyCfg.MorphoAPIURL == "" {
					t.Fatal("prod profile must carry the Morpho API as its market source")
				}
				if strategyCfg.BidWei.Sign() <= 0 {
					t.Fatalf("prod profile must carry a positive flat bid, got %v", strategyCfg.BidWei)
				}
				if strategyCfg.LoanEthFeed == nil {
					t.Fatal("prod profile must carry a rate source")
				}
			},
		},
		{
			name: "morphoApiUrl monitor: API URL + poll override",
			yaml: wsline + addrs + strategyBlock("    monitorPollMs: 10000\n") + feedLine,
			check: func(t *testing.T, c *Config) {
				t.Helper()
				strategyCfg := parseDefaultStrategyConfigForTest(t, c)
				if strategyCfg.MorphoAPIURL != "https://api.morpho.org/graphql" || strategyCfg.MonitorPoll != 10*time.Second {
					t.Fatalf("monitor profile wrong: url=%q poll=%v", strategyCfg.MorphoAPIURL, strategyCfg.MonitorPoll)
				}
				if strategyCfg.DiscoveryMaxHealthFactor != 1.30 { // default at-risk band ceiling
					t.Fatalf("discoveryMaxHealthFactor default wrong: %v", strategyCfg.DiscoveryMaxHealthFactor)
				}
			},
		},
		{
			name: "sizing: full liquidation can be disabled",
			yaml: wsline + addrs + strategyBlock("    sizing: {allowFullLiquidation: false}\n") + feedLine,
			check: func(t *testing.T, c *Config) {
				t.Helper()
				if parseDefaultStrategyConfigForTest(t, c).Sizing.AllowFullLiquidation {
					t.Fatal("allowFullLiquidation=false was not parsed")
				}
			},
		},
		{
			name: "single adapter pinned + oracle rate source",
			yaml: wsline + addrs + api + feedLine + okBid,
			check: func(t *testing.T, c *Config) {
				t.Helper()
				strategyCfg := parseDefaultStrategyConfigForTest(t, c)
				if strategyCfg.Adapter != adapterAddr || strategyCfg.LoanEthFeed == nil {
					t.Fatalf("single-adapter profile wrong: adapter=%s feed=%v", strategyCfg.Adapter, strategyCfg.LoanEthFeed)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := decodeCfg(t, tc.yaml)
			if err != nil {
				t.Fatalf("profile failed to parse: %v", err)
			}
			tc.check(t, cfg)
		})
	}
}

const validCfg = `
ws:
  url: wss://dev-rwa-sepolia.oev.a.redstone.finance
  apiKeyEnv: OEV_REDSTONE_API_KEY
executor: "0xfdFB1862a53a974b166d1f0D012f524Ebd2e0EbD"
adapter: "0xB5951fecFc34f56a6Ffbd62A2c61cE328E9De70b"
callback: "0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1"
strategy:
  name: default
  config:
    loanEthFeed:
      ethUsd: "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2"
      loanUsd: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"
      maxAgeMs: 3600000
    morphoApiUrl: https://api.morpho.org/graphql
    bid:
      bidEth: "0.0005"
      minBundleProfitBidBps: 1000
      totalBundleProfitBps: 500
    monitorPollMs: 15000
    sizing:
      allowFullLiquidation: true
      swapHaircutBps: 200
maxTxGasPriceWei: "60000000000"
maxBidWei: "1000000000000000"
`

func TestParseConfigValid(t *testing.T) {
	cfg, err := decodeCfg(t, validCfg)
	if err != nil {
		t.Fatal(err)
	}
	strategyCfg := parseDefaultStrategyConfigForTest(t, cfg)
	if strategyCfg.BidWei.String() != "500000000000000" { // 0.0005 ETH
		t.Fatalf("bidWei = %s", strategyCfg.BidWei)
	}
	if cfg.Callback != common.HexToAddress("0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1") {
		t.Fatalf("callback = %s", cfg.Callback.Hex())
	}
	if !strategyCfg.Sizing.AllowFullLiquidation || strategyCfg.Sizing.SwapHaircutBps != 200 {
		t.Fatalf("bad sizing: %+v", strategyCfg.Sizing)
	}
	if strategyCfg.MorphoAPIURL != "https://api.morpho.org/graphql" || strategyCfg.MonitorPoll != 15*time.Second {
		t.Fatalf("morphoApiUrl=%q monitorPoll=%v", strategyCfg.MorphoAPIURL, strategyCfg.MonitorPoll)
	}
	if strategyCfg.MinBundleProfitBidBps != 1000 {
		t.Fatalf("minBundleProfitBidBps=%d, want 1000", strategyCfg.MinBundleProfitBidBps)
	}
	if strategyCfg.TotalBundleProfitBps != 500 {
		t.Fatalf("totalBundleProfitBps=%d, want 500", strategyCfg.TotalBundleProfitBps)
	}
	if strategyCfg.CallbackAuthTTL != time.Minute {
		t.Fatalf("callback auth TTL = %v, want %v", strategyCfg.CallbackAuthTTL, time.Minute)
	}
	if cfg.MaxBidWei == nil || cfg.MaxBidWei.String() != "1000000000000000" {
		t.Fatalf("maxBidWei = %v, want 1000000000000000", cfg.MaxBidWei)
	}
}

func TestParseConfigDefaults(t *testing.T) {
	cfg, err := decodeCfg(t, `
ws: {url: "wss://x", apiKeyEnv: K}
executor: "0xfdFB1862a53a974b166d1f0D012f524Ebd2e0EbD"
adapter: "0xB5951fecFc34f56a6Ffbd62A2c61cE328E9De70b"
callback: "0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1"
strategy:
  name: default
  config:
    loanEthFeed: {ethUsd: "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", loanUsd: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"}
    morphoApiUrl: https://api.morpho.org/graphql
    bid: {bidEth: "0.0001"}
`)
	if err != nil {
		t.Fatal(err)
	}
	strategyCfg := parseDefaultStrategyConfigForTest(t, cfg)
	if !strategyCfg.Sizing.AllowFullLiquidation {
		t.Fatalf("defaults not applied: allowFullLiquidation=%v", strategyCfg.Sizing.AllowFullLiquidation)
	}
	if strategyCfg.MonitorPoll != 10*time.Second || cfg.MaxTxGasPrice.Int64() != defaultMaxTxGasPrice {
		t.Fatalf("interval/gas defaults wrong")
	}
	if cfg.ExecutorStateMaxAge != defaultExecutorStateMaxAge {
		t.Fatalf("executorStateMaxAge default wrong: %v, want %v", cfg.ExecutorStateMaxAge, defaultExecutorStateMaxAge)
	}
	if cfg.MaxBidWei != nil {
		t.Fatalf("maxBidWei default = %s, want nil", cfg.MaxBidWei)
	}
	if strategyCfg.MaxTrackedPositions != 10_000 {
		t.Fatalf("maxTrackedPositions default wrong: %d, want %d", strategyCfg.MaxTrackedPositions, 10_000)
	}
	if strategyCfg.CallbackAuthTTL != time.Minute {
		t.Fatalf("callback auth TTL default wrong: %v, want %v", strategyCfg.CallbackAuthTTL, time.Minute)
	}
}

func TestParseConfigRequiresBidCapForWebhook(t *testing.T) {
	_, err := decodeCfg(t, `
ws: {url: "wss://x", apiKeyEnv: K}
executor: "0xfdFB1862a53a974b166d1f0D012f524Ebd2e0EbD"
adapter: "0xB5951fecFc34f56a6Ffbd62A2c61cE328E9De70b"
callback: "0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1"
strategy:
  name: webhook
  config: {url: "https://strategy.example"}
`)
	if err == nil || !strings.Contains(err.Error(), "maxBidWei is required") {
		t.Fatalf("error = %v, want required webhook bid cap", err)
	}
}

func TestParseConfigBidAuthTTL(t *testing.T) {
	cfg, err := decodeCfg(t, wsline+addrs+strategyConfigBlock("    morphoApiUrl: https://api.morpho.org/graphql\n    bid: {bidEth: \"0.0005\", authTtlMs: 120000}\n")+feedLine)
	if err != nil {
		t.Fatal(err)
	}
	if got := parseDefaultStrategyConfigForTest(t, cfg).CallbackAuthTTL; got != 2*time.Minute {
		t.Fatalf("callback auth TTL = %v, want 2m", got)
	}
	cfg, err = decodeCfg(t, wsline+addrs+strategyConfigBlock("    morphoApiUrl: https://api.morpho.org/graphql\n    bid: {bidEth: \"0.0005\", authTtlMs: 0}\n")+feedLine)
	if err == nil {
		_, err = defaultstrategy.ParseConfig(cfg.Strategy.Config)
	}
	if err == nil {
		t.Fatal("expected error for zero strategy.config.bid.authTtlMs")
	}
}

// TestParseConfigMaxTrackedPositions pins the cap knob: unset → default; an explicit positive value is
// honored; 0 and negative are rejected (it doubles as the GraphQL `first` arg).
func TestParseConfigMaxTrackedPositions(t *testing.T) {
	t.Run("explicit positive honored", func(t *testing.T) {
		cfg, err := decodeCfg(t, wsline+addrs+strategyBlock("    maxTrackedPositions: 50\n")+feedLine)
		if err != nil {
			t.Fatal(err)
		}
		if got := parseDefaultStrategyConfigForTest(t, cfg).MaxTrackedPositions; got != 50 {
			t.Fatalf("maxTrackedPositions = %d, want 50", got)
		}
	})
	for _, bad := range []string{"0", "-1"} {
		t.Run("rejects "+bad, func(t *testing.T) {
			cfg, err := decodeCfg(t, wsline+addrs+strategyBlock("    maxTrackedPositions: "+bad+"\n")+feedLine)
			if err == nil {
				_, err = defaultstrategy.ParseConfig(cfg.Strategy.Config)
			}
			if err == nil {
				t.Fatalf("expected error for maxTrackedPositions: %s", bad)
			}
		})
	}
}

// TestParseConfigSwapHaircutZeroRespected pins the *int handling: an explicit swapHaircutBps:0 (no
// extra haircut) must survive parsing, not be silently replaced by the 2% default.
func TestParseConfigSwapHaircutZeroRespected(t *testing.T) {
	cfg, err := decodeCfg(t, wsline+addrs+strategyBlock("    sizing: {swapHaircutBps: 0}\n")+feedLine)
	if err != nil {
		t.Fatal(err)
	}
	if got := parseDefaultStrategyConfigForTest(t, cfg).Sizing.SwapHaircutBps; got != 0 {
		t.Fatalf("explicit swapHaircutBps:0 should be respected, got %d", got)
	}
	// And unset still defaults to 2%.
	cfg2, err := decodeCfg(t, wsline+addrs+api+feedLine)
	if err != nil {
		t.Fatal(err)
	}
	if got := parseDefaultStrategyConfigForTest(t, cfg2).Sizing.SwapHaircutBps; got != 200 {
		t.Fatalf("unset swapHaircutBps should default to %d, got %d", 200, got)
	}
}

func TestParseConfigErrors(t *testing.T) {
	cases := map[string]string{
		"missing ws url":                 `ws: {apiKeyEnv: K}` + "\n" + addrs + api + feedLine,
		"missing apiKeyEnv":              `ws: {url: x}` + "\n" + addrs + api + feedLine,
		"missing adapter":                wsline + `executor: "0xfdFB1862a53a974b166d1f0D012f524Ebd2e0EbD"` + "\n" + api + feedLine,
		"removed positionSource":         wsline + addrs + api + feedLine + "positionSource: redstone\n",      // unknown key: knob removed
		"removed markets key":            wsline + addrs + api + feedLine + `markets: ["` + mkt + `"]` + "\n", // markets no longer a config field → unknown key
		"zero bid":                       wsline + addrs + strategyConfigBlock("    morphoApiUrl: https://api.morpho.org/graphql\n    bid: {bidEth: \"0\"}\n") + feedLine,
		"bad executor addr":              wsline + `executor: "0xnope"` + "\n" + api + feedLine,
		"missing loanEthFeed":            wsline + addrs + "strategy:\n  name: default\n  config:\n    morphoApiUrl: https://api.morpho.org/graphql\n    bid: {bidEth: \"0.0005\"}\n",
		"removed maxSeizeFractionBps":    wsline + addrs + api + feedLine + "sizing: {maxSeizeFractionBps: 9000}",
		"removed maxLegsPerBid":          wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\", maxLegsPerBid: 8}",
		"removed minLegProfitLoan":       wsline + addrs + api + feedLine + "sizing: {minLegProfitLoan: \"1\"}",
		"negative swapHaircutBps":        wsline + addrs + strategyBlock("    sizing: {swapHaircutBps: -1}\n") + feedLine,
		"bad morphoApiUrl":               wsline + addrs + strategyConfigBlock("    morphoApiUrl: \"not-a-url\"\n    bid: {bidEth: \"0.0005\"}\n") + feedLine,
		"non-positive maxHF":             wsline + addrs + strategyBlock("    discoveryMaxHealthFactor: 0\n") + feedLine,
		"removed gasBase":                wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\", gasBase: 100000}",
		"removed gasPerLeg":              wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\", gasPerLeg: 800000}",
		"removed loanPerEth":             wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\", loanPerEth: \"2500000000\"}",
		"bad loan feed age":              wsline + addrs + "strategy:\n  name: default\n  config:\n    loanEthFeed: {ethUsd: \"0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48\", loanUsd: \"0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2\", maxAgeMs: 0}\n    morphoApiUrl: https://api.morpho.org/graphql\n    bid: {bidEth: \"0.0005\"}\n",
		"zero loan feed":                 wsline + addrs + "strategy:\n  name: default\n  config:\n    loanEthFeed: {ethUsd: \"0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48\", loanUsd: \"0x0000000000000000000000000000000000000000\"}\n    morphoApiUrl: https://api.morpho.org/graphql\n    bid: {bidEth: \"0.0005\"}\n",
		"zero eth feed":                  wsline + addrs + "strategy:\n  name: default\n  config:\n    loanEthFeed: {ethUsd: \"0x0000000000000000000000000000000000000000\", loanUsd: \"0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48\"}\n    morphoApiUrl: https://api.morpho.org/graphql\n    bid: {bidEth: \"0.0005\"}\n",
		"removed minBundleProfitLoan":    wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\", minBundleProfitLoan: \"1\"}",
		"negative minBundleProfitBidBps": wsline + addrs + strategyConfigBlock("    morphoApiUrl: https://api.morpho.org/graphql\n    bid: {bidEth: \"0.1\", minBundleProfitBidBps: -1}\n") + feedLine,
		"bad totalBundleProfitBps":       wsline + addrs + strategyConfigBlock("    morphoApiUrl: https://api.morpho.org/graphql\n    bid: {bidEth: \"0.1\", totalBundleProfitBps: 10001}\n") + feedLine,
		"zero maxTxGasPrice":             wsline + addrs + api + feedLine + "maxTxGasPriceWei: \"0\"",
		"zero maxBidWei":                 wsline + addrs + api + feedLine + "maxBidWei: \"0\"",
		"bad maxBidWei":                  wsline + addrs + api + feedLine + "maxBidWei: nope",
		"removed gas multiplier":         wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\", gasPriceMultiplierBps: 20000}",
		"removed priority fee":           wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\", priorityFeeWei: \"1\"}",
		"removed market poll":            wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}\nintervals: {marketPollMs: 5000}",
		"removed position poll":          wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}\nintervals: {positionPollMs: 2000}",
		"negative monitor poll":          wsline + addrs + strategyBlock("    monitorPollMs: -1\n") + feedLine,
		"removed discovery poll":         wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}\nintervals: {discoveryPollMs: 10000}",
		"removed snapshot age":           wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}\nmaxSnapshotAgeMs: 60000",
		"zero interval":                  wsline + addrs + api + feedLine + "intervals: {opsPollMs: 0}",
		"non-positive breaker":           wsline + addrs + api + feedLine + "breaker: {maxFailures: 3, windowMs: 0}",
		"opsPoll >= executorStateMaxAge": wsline + addrs + api + feedLine + "intervals: {opsPollMs: 60000, executorStateMaxAgeMs: 60000}",
		"monitorPoll >= maxStateAge":     wsline + addrs + strategyBlock("    monitorPollMs: 90001\n    maxStateAgeMs: 90000\n") + feedLine,
		"zero strategy maxStateAge":      wsline + addrs + strategyBlock("    maxStateAgeMs: 0\n") + feedLine,
		"zero executor state max age":    wsline + addrs + api + feedLine + "intervals: {executorStateMaxAgeMs: 0}",
		"zero executor addr":             wsline + "executor: \"0x0000000000000000000000000000000000000000\"\n" + api + feedLine,
		"zero callback addr":             wsline + "executor: \"0xfdFB1862a53a974b166d1f0D012f524Ebd2e0EbD\"\nadapter: \"0xB5951fecFc34f56a6Ffbd62A2c61cE328E9De70b\"\ncallback: \"0x0000000000000000000000000000000000000000\"\n" + api + feedLine,
		"callback in strategy config":    wsline + addrs + "strategy:\n  name: default\n  config:\n    callback: \"0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1\"\n    loanEthFeed: {ethUsd: \"0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2\", loanUsd: \"0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48\", maxAgeMs: 3600000}\n    morphoApiUrl: https://api.morpho.org/graphql\n    bid: {bidEth: \"0.0005\"}\n" + feedLine,
		"zero adapter addr":              wsline + "executor: \"0xfdFB1862a53a974b166d1f0D012f524Ebd2e0EbD\"\nadapter: \"0x0000000000000000000000000000000000000000\"\n" + api + feedLine,
		"adapter in strategy config":     wsline + addrs + "strategy:\n  name: default\n  config:\n    adapter: \"0xB5951fecFc34f56a6Ffbd62A2c61cE328E9De70b\"\n    loanEthFeed: {ethUsd: \"0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2\", loanUsd: \"0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48\", maxAgeMs: 3600000}\n    morphoApiUrl: https://api.morpho.org/graphql\n    bid: {bidEth: \"0.0005\"}\n" + feedLine,
	}
	for name, y := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, err := decodeCfg(t, y)
			if err == nil {
				_, err = defaultstrategy.ParseConfig(cfg.Strategy.Config)
			}
			if err == nil {
				t.Fatalf("expected error for %q", name)
			}
		})
	}
}

const (
	mkt    = "0x6209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5"
	wsline = "ws: {url: x, apiKeyEnv: K}\n"
	addrs  = "executor: \"0xfdFB1862a53a974b166d1f0D012f524Ebd2e0EbD\"\nadapter: \"0xB5951fecFc34f56a6Ffbd62A2c61cE328E9De70b\"\ncallback: \"0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1\"\n"
	// api is the production market source (the Morpho API) appended to a valid config; markets/positions are
	// discovered at runtime, so a parseable config needs no market list.
	api      = "strategy:\n  name: default\n  config:\n    loanEthFeed: {ethUsd: \"0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2\", loanUsd: \"0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48\", maxAgeMs: 3600000}\n    morphoApiUrl: https://api.morpho.org/graphql\n    bid: {bidEth: \"0.0005\"}\n"
	feedLine = ""
	okBid    = ""
)

var adapterAddr = common.HexToAddress("0xB5951fecFc34f56a6Ffbd62A2c61cE328E9De70b")
