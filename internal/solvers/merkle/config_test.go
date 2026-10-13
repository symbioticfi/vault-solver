package merkle

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"

	"gopkg.in/yaml.v3"
)

func configNode(t *testing.T, text string) yaml.Node {
	t.Helper()
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(text), &node); err != nil {
		t.Fatal(err)
	}
	return *node.Content[0]
}

const validConfig = `mode: simulate
stateDir: state/merkle
curator:
  url: http://127.0.0.1:1/api/v1
strategies:
  - name: rebalance
    executor: "0x0000000000000000000000000000000000000010"
`

func TestConfigValidation(t *testing.T) {
	for name, raw := range map[string]string{
		"unknown":            validConfig + "unexpected: true\n",
		"mode":               strings.Replace(validConfig, "mode: simulate", "mode: unsafe", 1),
		"duration":           validConfig + "pollInterval: -1s\n",
		"override":           validConfig + "operationTimeout: 1s\n",
		"missing strategies": "stateDir: state/merkle\ncurator: {url: 'http://127.0.0.1:1/api/v1'}\nstrategies: []\n",
		"missing curator":    strings.Replace(validConfig, "curator:\n  url: http://127.0.0.1:1/api/v1\n", "", 1),
		"manual account":     validConfig + "    account: '0x0000000000000000000000000000000000000020'\n",
		"manual tree":        validConfig + "    tree: permissions.json\n",
		"manual inputs":      validConfig + "    inputs: {0: '10'}\n",
		"zero executor":      strings.Replace(validConfig, "0000000000000000000000000000000000000010", "0000000000000000000000000000000000000000", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseConfig(configNode(t, raw)); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	cfg, err := parseConfig(configNode(t, validConfig))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.mode != "simulate" || len(cfg.strategies) != 1 || len(cfg.strategies[0].Providers) != 1 || cfg.strategies[0].Config.Account != "" || cfg.strategies[0].Config.Tree != "" {
		t.Fatalf("wrong SDK derivation workflow: %+v", cfg)
	}
}

func TestRevisionBindsDerivedIdentityAndPermissionSource(t *testing.T) {
	cfg, err := parseConfig(configNode(t, validConfig))
	if err != nil {
		t.Fatal(err)
	}
	strategy := cfg.strategies[0].Config
	strategy.Account = "0x0000000000000000000000000000000000000020"
	caller := common.HexToAddress("0x03")
	original, err := strategyRevision(strategy, cfg.curator, caller)
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*keeper.Config, *protocolio.HTTPConfig, *common.Address){
		"account": func(s *keeper.Config, _ *protocolio.HTTPConfig, _ *common.Address) {
			s.Account = common.HexToAddress("0x04").Hex()
		},
		"caller": func(_ *keeper.Config, _ *protocolio.HTTPConfig, c *common.Address) { *c = common.HexToAddress("0x05") },
		"source": func(_ *keeper.Config, c *protocolio.HTTPConfig, _ *common.Address) {
			c.URL = "http://other.invalid/api/v1"
		},
		"credential": func(_ *keeper.Config, c *protocolio.HTTPConfig, _ *common.Address) { c.APIKeyEnv = "OTHER_CURATOR_KEY" },
	} {
		t.Run(name, func(t *testing.T) {
			s, c, owner := strategy, cfg.curator, caller
			change(&s, &c, &owner)
			revision, err := strategyRevision(s, c, owner)
			if err != nil {
				t.Fatal(err)
			}
			if revision == original {
				t.Fatal("changed identity invisible to recovery")
			}
		})
	}
}

func TestCuratorTransportUsesFixedEffectivePolicy(t *testing.T) {
	var fingerprint string
	for _, tc := range []struct {
		name, fields string
		invalid      bool
	}{
		{"omitted", "", false}, {"zero", "  timeoutSeconds: 0\n  maxResponseBytes: 0\n", false}, {"fixed", "  timeoutSeconds: 10\n  maxResponseBytes: 2097152\n", false}, {"different timeout", "  timeoutSeconds: 11\n", true}, {"different bound", "  maxResponseBytes: 1048576\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := strings.Replace(validConfig, "curator:\n", "curator:\n"+tc.fields, 1)
			cfg, err := parseConfig(configNode(t, raw))
			if tc.invalid {
				if err == nil {
					t.Fatal("legacy curator transport override accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.curator.TimeoutSeconds != permissions.HostTimeoutSeconds || cfg.curator.MaxResponseBytes != permissions.HostMaxResponseBytes {
				t.Fatalf("effective curator policy %+v", cfg.curator)
			}
			got, err := strategyRevision(cfg.strategies[0].Config, cfg.curator, common.HexToAddress("0x03"))
			if err != nil {
				t.Fatal(err)
			}
			if fingerprint == "" {
				fingerprint = got
			} else if fingerprint != got {
				t.Fatal("equivalent curator policy changed fingerprint")
			}
		})
	}
}

func TestExplicitStrategyRevisionRemainsStable(t *testing.T) {
	cfg, err := parseConfig(configNode(t, validConfig))
	if err != nil {
		t.Fatal(err)
	}
	strategy := cfg.strategies[0].Config
	strategy.Account = "0x0000000000000000000000000000000000000020"
	revision, err := strategyRevision(strategy, cfg.curator, common.HexToAddress("0x03"))
	if err != nil {
		t.Fatal(err)
	}
	// Captured before optional-name support at d6ab6ffa1505.
	if revision != "0x05785a78b702963b8c5c94d28e056f167030a75517fcb475f88b97fc0003bdf1" || strategy.Fingerprint() != "0x37e6d2847e87706cb09048c6ec9f1916d44259052da72fc5513c7991539b9a0f" {
		t.Fatal("explicit strategy identity changed")
	}
}

func TestOptionalStrategyNames(t *testing.T) {
	const executor = "0xaBcDeF0000000000000000000000000000000001"
	const derived = "executor-0xabcdef0000000000000000000000000000000001"
	const other = "0x0000000000000000000000000000000000000002"
	const otherName = "executor-0x0000000000000000000000000000000000000002"
	for _, tc := range []struct {
		name, strategies string
		want             []string
	}{
		{"omitted", "  - executor: '" + executor + "'\n", []string{derived}},
		{"empty", "  - name: ''\n    executor: '" + executor + "'\n", []string{derived}},
		{"lowercase", "  - executor: '" + strings.ToLower(executor) + "'\n", []string{derived}},
		{"uppercase", "  - executor: '0x" + strings.ToUpper(executor[2:]) + "'\n", []string{derived}},
		{"ordered", "  - executor: '" + executor + "'\n  - executor: '" + other + "'\n", []string{derived, otherName}},
		{"reordered", "  - executor: '" + other + "'\n  - executor: '" + executor + "'\n", []string{otherName, derived}},
		{"custom", "  - name: Existing_Label-1\n    executor: '" + executor + "'\n", []string{"Existing_Label-1"}},
		{"separate named workflows", "  - name: first\n    executor: '" + executor + "'\n  - name: second\n    executor: '" + executor + "'\n", []string{"first", "second"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prefix, _, _ := strings.Cut(validConfig, "strategies:\n")
			cfg, err := parseConfig(configNode(t, prefix+"strategies:\n"+tc.strategies))
			if err != nil {
				t.Fatal(err)
			}
			for i, name := range tc.want {
				if cfg.strategies[i].Config.Name != name {
					t.Fatalf("name=%q want%q", cfg.strategies[i].Config.Name, name)
				}
			}
			if tc.name == "custom" && cfg.strategies[0].Config.Executor != common.HexToAddress(executor).Hex() {
				t.Fatal("explicit executor normalization changed")
			}
		})
	}
	for name, body := range map[string]string{
		"duplicate omitted": "  - executor: '" + executor + "'\n  - executor: '" + strings.ToLower(executor) + "'\n",
		"custom collision":  "  - executor: '" + executor + "'\n  - name: " + derived + "\n    executor: '" + other + "'\n",
		"duplicate custom":  "  - name: existing\n    executor: '" + executor + "'\n  - name: existing\n    executor: '" + other + "'\n",
		"invalid custom":    "  - name: '../escape'\n    executor: '" + executor + "'\n",
		"invalid executor":  "  - executor: 'invalid'\n",
		"zero executor":     "  - executor: '0x0000000000000000000000000000000000000000'\n",
	} {
		t.Run(name, func(t *testing.T) {
			prefix, _, _ := strings.Cut(validConfig, "strategies:\n")
			if _, err := parseConfig(configNode(t, prefix+"strategies:\n"+body)); err == nil {
				t.Fatal("invalid strategy identity accepted")
			}
		})
	}
}
