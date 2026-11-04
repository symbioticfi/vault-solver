package uniswapx

import (
	"math/big"
	"strings"
	"testing"
)

func rawDexFixture() *rawDexConfig {
	return &rawDexConfig{InputToken: "0x1111111111111111111111111111111111111111",
		BridgeToken: "0x6666666666666666666666666666666666666666",
		Router:      "0x2222222222222222222222222222222222222222", Quoter: "0x3333333333333333333333333333333333333333",
		Factory: "0x4444444444444444444444444444444444444444", Treasury: "0x5555555555555555555555555555555555555555",
		Outputs: []rawDexOutput{{Token: "0x7777777777777777777777777777777777777777", Fee: 100},
			{Token: "0x8888888888888888888888888888888888888888", Fee: 500}},
		MaxBridgeAmount: "100000000", MinProfitBridge: "100000", GasUnitsBudget: 600000,
		SlippageBps: 50, PlanValidity: "30s", RPCTimeout: "300ms", MaxConcurrentQuotes: 2}
}

func TestParseDexConfigBoundaries(t *testing.T) {
	for _, mode := range []string{"valid", "duplicate output", "input output", "no pool fee", "large pool fee", "negative profit", "uint256 overflow", "too much slippage", "too much gas", "too long timeout", "too long validity"} {
		t.Run(mode, func(t *testing.T) {
			raw := rawDexFixture()
			switch mode {
			case "duplicate output":
				raw.Outputs[1].Token = raw.Outputs[0].Token
			case "input output":
				raw.Outputs[1].Token = raw.InputToken
			case "no pool fee":
				raw.Outputs[0].Fee = 0
			case "large pool fee":
				raw.Outputs[0].Fee = 1 << 24
			case "negative profit":
				raw.MinProfitBridge = "-1"
			case "uint256 overflow":
				raw.MaxBridgeAmount = new(big.Int).Lsh(big.NewInt(1), 256).String()
			case "too much slippage":
				raw.SlippageBps = 1001
			case "too much gas":
				raw.GasUnitsBudget = 10000001
			case "too long timeout":
				raw.RPCTimeout = "1s"
			case "too long validity":
				raw.PlanValidity = "31s"
			}
			cfg, err := parseDexConfig(raw)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("cfg=%+v err=%v", cfg, err)
			}
		})
	}
}

const dexYAMLFixture = `dex:
  inputToken: "0x1111111111111111111111111111111111111111"
  bridgeToken: "0x6666666666666666666666666666666666666666"
  router: "0x2222222222222222222222222222222222222222"
  quoter: "0x3333333333333333333333333333333333333333"
  factory: "0x4444444444444444444444444444444444444444"
  treasury: "0x5555555555555555555555555555555555555555"
  outputs:
    - {token: "0x7777777777777777777777777777777777777777", fee: 100}
    - {token: "0x8888888888888888888888888888888888888888", fee: 500}
  maxBridgeAmount: "100000000"
  minProfitBridge: "100000"
  gasUnitsBudget: 600000
  slippageBps: 50
  planValidity: 30s
  rpcTimeout: 300ms
  maxConcurrentQuotes: 2
`

func TestDexConfigRequiresSingleStrategyAndGasOracle(t *testing.T) {
	base := strings.Replace(validUniswapXConfig, "strategy: {}", "strategy: {name: single}", 1) + dexYAMLFixture
	for _, mode := range []string{"valid", "no gas", "greedy", "missing bridge feed"} {
		raw := base
		if mode == "no gas" {
			raw = strings.Replace(raw, validUniswapXGasConfig, "", 1)
		}
		if mode == "greedy" {
			raw = strings.Replace(raw, "name: single", "name: default", 1)
		}
		if mode == "missing bridge feed" {
			raw = strings.Replace(raw, "- token: \"0x6666666666666666666666666666666666666666\"", "- token: \"0x9999999999999999999999999999999999999999\"", 1)
		}
		_, err := parseConfig(uniswapXConfigNode(t, raw))
		if (err == nil) != (mode == "valid") {
			t.Fatalf("%s err=%v", mode, err)
		}
	}
}

func TestDexDisabledWhenConfigOmitted(t *testing.T) {
	cfg, err := parseConfig(uniswapXConfigNode(t, validUniswapXConfig))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Dex != nil {
		t.Fatal("DEX must remain disabled unless explicitly configured")
	}
}
