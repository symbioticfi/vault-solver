package redstoneoev

import (
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"
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
	if !cfg.Sizing.AllowFullLiquidation {
		t.Fatal("example settings drifted: allowFullLiquidation must stay enabled")
	}
	if cfg.LoanEthFeed == nil {
		t.Fatal("example settings drifted: config must carry a loan↔ETH rate source")
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
				if c.MorphoAPIURL == "" {
					t.Fatal("prod profile must carry the Morpho API as its market source")
				}
				if c.BidWei.Sign() <= 0 {
					t.Fatalf("prod profile must carry a positive flat bid, got %v", c.BidWei)
				}
				if c.LoanEthFeed == nil {
					t.Fatal("prod profile must carry a rate source")
				}
			},
		},
		{
			name: "morphoApiUrl monitor: API URL + poll override",
			yaml: wsline + addrs + "morphoApiUrl: https://api.morpho.org/graphql\n" +
				feedLine + "intervals: {monitorPollMs: 10000}\n" + okBid,
			check: func(t *testing.T, c *Config) {
				t.Helper()
				if c.MorphoAPIURL != "https://api.morpho.org/graphql" || c.MonitorPoll != 10*time.Second {
					t.Fatalf("monitor profile wrong: url=%q poll=%v", c.MorphoAPIURL, c.MonitorPoll)
				}
				if c.DiscoveryMaxHealthFactor != 1.30 { // default at-risk band ceiling
					t.Fatalf("discoveryMaxHealthFactor default wrong: %v", c.DiscoveryMaxHealthFactor)
				}
			},
		},
		{
			name: "sizing: full liquidation can be disabled",
			yaml: wsline + addrs + api + feedLine + "bid: {bidEth: \"0.0005\"}\nsizing: {allowFullLiquidation: false}",
			check: func(t *testing.T, c *Config) {
				t.Helper()
				if c.Sizing.AllowFullLiquidation {
					t.Fatal("allowFullLiquidation=false was not parsed")
				}
			},
		},
		{
			name: "single adapter pinned + oracle rate source",
			yaml: wsline + addrs + api + feedLine + okBid,
			check: func(t *testing.T, c *Config) {
				t.Helper()
				if c.Adapter != adapterAddr || c.LoanEthFeed == nil {
					t.Fatalf("single-adapter profile wrong: adapter=%s feed=%v", c.Adapter, c.LoanEthFeed)
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
callback: "0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1"
adapter: "0xB5951fecFc34f56a6Ffbd62A2c61cE328E9De70b"
morphoApiUrl: https://api.morpho.org/graphql
loanEthFeed:
  ethUsd: "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2"
  loanUsd: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"
  maxAgeMs: 3600000
bid:
  bidEth: "0.0005"
  minBundleProfitBidBps: 1000
  totalBundleProfitBps: 500
  maxTxGasPriceWei: "60000000000"
sizing:
  allowFullLiquidation: true
  swapHaircutBps: 200
intervals:
  monitorPollMs: 15000
`

func TestParseConfigValid(t *testing.T) {
	cfg, err := decodeCfg(t, validCfg)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BidWei.String() != "500000000000000" { // 0.0005 ETH
		t.Fatalf("bidWei = %s", cfg.BidWei)
	}
	if !cfg.Sizing.AllowFullLiquidation || cfg.Sizing.SwapHaircutBps != 200 {
		t.Fatalf("bad sizing: %+v", cfg.Sizing)
	}
	if cfg.MorphoAPIURL != "https://api.morpho.org/graphql" || cfg.MonitorPoll != 15*time.Second {
		t.Fatalf("morphoApiUrl=%q monitorPoll=%v", cfg.MorphoAPIURL, cfg.MonitorPoll)
	}
	if cfg.MinBundleProfitBidBps != 1000 {
		t.Fatalf("minBundleProfitBidBps=%d, want 1000", cfg.MinBundleProfitBidBps)
	}
	if cfg.TotalBundleProfitBps != 500 {
		t.Fatalf("totalBundleProfitBps=%d, want 500", cfg.TotalBundleProfitBps)
	}
	if cfg.CallbackAuthTTL != defaultCallbackAuthTTL {
		t.Fatalf("callback auth TTL = %v, want %v", cfg.CallbackAuthTTL, defaultCallbackAuthTTL)
	}
}

func TestParseConfigDefaults(t *testing.T) {
	cfg, err := decodeCfg(t, `
ws: {url: "wss://x", apiKeyEnv: K}
executor: "0xfdFB1862a53a974b166d1f0D012f524Ebd2e0EbD"
callback: "0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1"
adapter: "0xB5951fecFc34f56a6Ffbd62A2c61cE328E9De70b"
morphoApiUrl: https://api.morpho.org/graphql
loanEthFeed: {ethUsd: "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", loanUsd: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"}
bid: {bidEth: "0.0001"}
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sizing.AllowFullLiquidation != defaultAllowFullLiquidation {
		t.Fatalf("defaults not applied: allowFullLiquidation=%v", cfg.Sizing.AllowFullLiquidation)
	}
	if cfg.MonitorPoll != defaultMonitorPoll || cfg.MaxTxGasPrice.Int64() != defaultMaxTxGasPrice {
		t.Fatalf("interval/gas defaults wrong")
	}
	if cfg.MaxStateAge != defaultMaxStateAge {
		t.Fatalf("maxStateAge default wrong: %v, want %v", cfg.MaxStateAge, defaultMaxStateAge)
	}
	if cfg.MaxTrackedPositions != defaultMaxTrackedPositions {
		t.Fatalf("maxTrackedPositions default wrong: %d, want %d", cfg.MaxTrackedPositions, defaultMaxTrackedPositions)
	}
	if cfg.CallbackAuthTTL != defaultCallbackAuthTTL {
		t.Fatalf("callback auth TTL default wrong: %v, want %v", cfg.CallbackAuthTTL, defaultCallbackAuthTTL)
	}
}

func TestParseConfigBidAuthTTL(t *testing.T) {
	cfg, err := decodeCfg(t, wsline+addrs+api+feedLine+`bid: {bidEth: "0.1", authTtlMs: 120000}`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CallbackAuthTTL != 2*time.Minute {
		t.Fatalf("callback auth TTL = %v, want 2m", cfg.CallbackAuthTTL)
	}
	if _, err := decodeCfg(t, wsline+addrs+api+feedLine+`bid: {bidEth: "0.1", authTtlMs: 0}`); err == nil {
		t.Fatal("expected error for zero bid.authTtlMs")
	}
}

// TestParseConfigMaxTrackedPositions pins the cap knob: unset → default; an explicit positive value is
// honored; 0 and negative are rejected (it doubles as the GraphQL `first` arg).
func TestParseConfigMaxTrackedPositions(t *testing.T) {
	t.Run("explicit positive honored", func(t *testing.T) {
		cfg, err := decodeCfg(t, wsline+addrs+api+feedLine+okBid+"maxTrackedPositions: 50\n")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.MaxTrackedPositions != 50 {
			t.Fatalf("maxTrackedPositions = %d, want 50", cfg.MaxTrackedPositions)
		}
	})
	for _, bad := range []string{"0", "-1"} {
		t.Run("rejects "+bad, func(t *testing.T) {
			if _, err := decodeCfg(t, wsline+addrs+api+feedLine+okBid+"maxTrackedPositions: "+bad+"\n"); err == nil {
				t.Fatalf("expected error for maxTrackedPositions: %s", bad)
			}
		})
	}
}

// TestParseConfigSwapHaircutZeroRespected pins the *int handling: an explicit swapHaircutBps:0 (no
// extra haircut) must survive parsing, not be silently replaced by the 2% default.
func TestParseConfigSwapHaircutZeroRespected(t *testing.T) {
	cfg, err := decodeCfg(t, wsline+addrs+api+feedLine+"bid: {bidEth: \"0.1\"}\nsizing: {swapHaircutBps: 0}")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sizing.SwapHaircutBps != 0 {
		t.Fatalf("explicit swapHaircutBps:0 should be respected, got %d", cfg.Sizing.SwapHaircutBps)
	}
	// And unset still defaults to 2%.
	cfg2, err := decodeCfg(t, wsline+addrs+api+feedLine+"bid: {bidEth: \"0.1\"}")
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Sizing.SwapHaircutBps != defaultSwapHaircut {
		t.Fatalf("unset swapHaircutBps should default to %d, got %d", defaultSwapHaircut, cfg2.Sizing.SwapHaircutBps)
	}
}

func TestParseConfigErrors(t *testing.T) {
	cases := map[string]string{
		"missing ws url":                 `ws: {apiKeyEnv: K}` + "\n" + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}",
		"missing apiKeyEnv":              `ws: {url: x}` + "\n" + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}",
		"removed positionSource":         wsline + addrs + api + feedLine + "positionSource: redstone\nbid: {bidEth: \"0.1\"}",      // unknown key: knob removed
		"removed markets key":            wsline + addrs + api + feedLine + `markets: ["` + mkt + `"]` + "\nbid: {bidEth: \"0.1\"}", // markets no longer a config field → unknown key
		"zero bid":                       wsline + addrs + api + feedLine + "bid: {bidEth: \"0\"}",
		"bad executor addr":              wsline + `executor: "0xnope"` + "\ncallback: \"0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1\"\n" + api + feedLine + "bid: {bidEth: \"0.1\"}",
		"missing loanEthFeed":            wsline + addrs + api + "bid: {bidEth: \"0.1\"}",
		"removed maxSeizeFractionBps":    wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}\nsizing: {maxSeizeFractionBps: 9000}",
		"removed maxLegsPerBid":          wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\", maxLegsPerBid: 8}",
		"removed minLegProfitLoan":       wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}\nsizing: {minLegProfitLoan: \"1\"}",
		"negative swapHaircutBps":        wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}\nsizing: {swapHaircutBps: -1}",
		"bad morphoApiUrl":               wsline + addrs + "morphoApiUrl: \"not-a-url\"\n" + feedLine + "bid: {bidEth: \"0.1\"}",
		"non-positive maxHF":             wsline + addrs + api + feedLine + "discoveryMaxHealthFactor: 0\nbid: {bidEth: \"0.1\"}",
		"removed gasBase":                wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\", gasBase: 100000}",
		"removed gasPerLeg":              wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\", gasPerLeg: 800000}",
		"removed loanPerEth":             wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\", loanPerEth: \"2500000000\"}",
		"bad loan feed age":              wsline + addrs + api + "loanEthFeed: {ethUsd: \"0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48\", loanUsd: \"0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2\", maxAgeMs: 0}\nbid: {bidEth: \"0.1\"}",
		"removed minBundleProfitLoan":    wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\", minBundleProfitLoan: \"1\"}",
		"negative minBundleProfitBidBps": wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\", minBundleProfitBidBps: -1}",
		"bad totalBundleProfitBps":       wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\", totalBundleProfitBps: 10001}",
		"zero maxTxGasPrice":             wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\", maxTxGasPriceWei: \"0\"}",
		"removed gas multiplier":         wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\", gasPriceMultiplierBps: 20000}",
		"removed priority fee":           wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\", priorityFeeWei: \"1\"}",
		"removed market poll":            wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}\nintervals: {marketPollMs: 5000}",
		"removed position poll":          wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}\nintervals: {positionPollMs: 2000}",
		"negative interval":              wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}\nintervals: {monitorPollMs: -1}",
		"removed discovery poll":         wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}\nintervals: {discoveryPollMs: 10000}",
		"removed snapshot age":           wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}\nmaxSnapshotAgeMs: 60000",
		"zero interval":                  wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}\nintervals: {opsPollMs: 0}",
		"non-positive breaker":           wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}\nbreaker: {maxFailures: 3, windowMs: 0}",
		"opsPoll >= maxStateAge":         wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}\nintervals: {opsPollMs: 60000, maxStateAgeMs: 60000}",
		"monitorPoll >= maxStateAge":     wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}\nintervals: {monitorPollMs: 90001, maxStateAgeMs: 90000}",
		"zero maxStateAge":               wsline + addrs + api + feedLine + "bid: {bidEth: \"0.1\"}\nintervals: {maxStateAgeMs: 0}",
		"zero executor addr":             wsline + "executor: \"0x0000000000000000000000000000000000000000\"\ncallback: \"0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1\"\nadapter: \"0xB5951fecFc34f56a6Ffbd62A2c61cE328E9De70b\"\n" + api + feedLine + "bid: {bidEth: \"0.1\"}",
		"zero callback addr":             wsline + "executor: \"0xfdFB1862a53a974b166d1f0D012f524Ebd2e0EbD\"\ncallback: \"0x0000000000000000000000000000000000000000\"\nadapter: \"0xB5951fecFc34f56a6Ffbd62A2c61cE328E9De70b\"\n" + api + feedLine + "bid: {bidEth: \"0.1\"}",
		"zero adapter addr":              wsline + "executor: \"0xfdFB1862a53a974b166d1f0D012f524Ebd2e0EbD\"\ncallback: \"0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1\"\nadapter: \"0x0000000000000000000000000000000000000000\"\n" + api + feedLine + "bid: {bidEth: \"0.1\"}",
	}
	for name, y := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeCfg(t, y); err == nil {
				t.Fatalf("expected error for %q", name)
			}
		})
	}
}

const (
	mkt    = "0x6209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5"
	wsline = "ws: {url: x, apiKeyEnv: K}\n"
	addrs  = "executor: \"0xfdFB1862a53a974b166d1f0D012f524Ebd2e0EbD\"\n" +
		"callback: \"0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1\"\n" +
		"adapter: \"0xB5951fecFc34f56a6Ffbd62A2c61cE328E9De70b\"\n"
	// api is the production market source (the Morpho API) appended to a valid config; markets/positions are
	// discovered at runtime, so a parseable config needs no market list.
	api      = "morphoApiUrl: https://api.morpho.org/graphql\n"
	feedLine = "loanEthFeed: {ethUsd: \"0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2\", loanUsd: \"0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48\", maxAgeMs: 3600000}\n"
	okBid    = "bid: {bidEth: \"0.0005\"}\n"
)

var adapterAddr = common.HexToAddress("0xB5951fecFc34f56a6Ffbd62A2c61cE328E9De70b")
