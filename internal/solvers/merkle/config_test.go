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
