package merkle

import (
	"fmt"
	"slices"
	"strings"
	"testing"

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
curator:
  url: http://127.0.0.1:1/api/v1
strategies:
  - "0x0000000000000000000000000000000000000010"
`

func TestConfigValidation(t *testing.T) {
	for name, raw := range map[string]string{
		"removed stateDir":   validConfig + "stateDir: state/merkle\n",
		"unknown":            validConfig + "unexpected: true\n",
		"mode":               strings.Replace(validConfig, "mode: simulate", "mode: unsafe", 1),
		"duration":           validConfig + "pollInterval: -1s\n",
		"override":           validConfig + "operationTimeout: 1s\n",
		"missing strategies": "curator: {url: 'http://127.0.0.1:1/api/v1'}\nstrategies: []\n",
		"missing curator":    strings.Replace(validConfig, "curator:\n  url: http://127.0.0.1:1/api/v1\n", "", 1),
		"manual account":     strategyConfig("  - executor: '0x0000000000000000000000000000000000000010'\n    account: '0x0000000000000000000000000000000000000020'\n"),
		"manual tree":        strategyConfig("  - executor: '0x0000000000000000000000000000000000000010'\n    tree: permissions.json\n"),
		"manual inputs":      strategyConfig("  - executor: '0x0000000000000000000000000000000000000010'\n    inputs: {0: '10'}\n"),
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
	if cfg.mode != "simulate" || len(cfg.strategies) != 1 || len(cfg.strategies[0].Providers) != 0 || cfg.strategies[0].Config.Account != "" || cfg.strategies[0].Config.Tree != "" {
		t.Fatalf("wrong SDK derivation workflow: %+v", cfg)
	}
}

func TestConfigRejectsTimingFields(t *testing.T) {
	for field, value := range map[string]string{
		"pollInterval":     "15s",
		"operationTimeout": "30s",
	} {
		t.Run(field, func(t *testing.T) {
			_, err := parseConfig(configNode(t, validConfig+field+": "+value+"\n"))
			if err == nil || !strings.Contains(err.Error(), "field "+field+" not found") {
				t.Fatalf("timing field must be rejected as unknown: %v", err)
			}
		})
	}
}

func TestCuratorTransportUsesFixedEffectivePolicy(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		invalid      bool
	}{
		{"omitted", "", false},
		{"zero timeout", "  timeoutSeconds: 0\n", true},
		{"zero bound", "  maxResponseBytes: 0\n", true},
		{"API key", "  apiKeyEnv: CURATOR_TEST_TOKEN\n", true},
		{"fixed timeout", "  timeoutSeconds: 10\n", true},
		{"fixed bound", "  maxResponseBytes: 2097152\n", true},
		{"different timeout", "  timeoutSeconds: 11\n", true},
		{"different bound", "  maxResponseBytes: 1048576\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := strings.Replace(validConfig, "curator:\n", "curator:\n"+tc.fields, 1)
			cfg, err := parseConfig(configNode(t, raw))
			if tc.invalid {
				if err == nil || !strings.Contains(err.Error(), "not found in type") {
					t.Fatalf("curator transport fields must be rejected as unknown: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.curator.URL != "http://127.0.0.1:1/api/v1" {
				t.Fatalf("curator URL changed: %+v", cfg.curator)
			}
		})
	}
}

func strategyConfig(body string) string {
	prefix, _, _ := strings.Cut(validConfig, "strategies:\n")
	return prefix + "strategies:\n" + body
}

func TestScalarStrategyAddressSchema(t *testing.T) {
	const mixed = "0xaBcDeF0000000000000000000000000000000001"
	const canonical = "0xabcdef0000000000000000000000000000000001"
	const other = "0x0000000000000000000000000000000000000002"
	for _, tc := range []struct {
		name, body string
		want       []string
	}{
		{"one", "  - '" + mixed + "'\n", []string{canonical}},
		{"lowercase", "  - '" + canonical + "'\n", []string{canonical}},
		{"uppercase", "  - '0x" + strings.ToUpper(mixed[2:]) + "'\n", []string{canonical}},
		{"ordered", "  - '" + mixed + "'\n  - '" + other + "'\n", []string{canonical, other}},
		{"reordered", "  - '" + other + "'\n  - '" + mixed + "'\n", []string{other, canonical}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseConfig(configNode(t, strategyConfig(tc.body)))
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, len(cfg.strategies))
			for i, s := range cfg.strategies {
				got[i] = s.Config.Name
				if s.Config.Name != s.Config.Executor {
					t.Fatal("executor differs from identifier")
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("strategies=%v want%v", got, tc.want)
			}
		})
	}
	many := strings.Builder{}
	for i := 1; i <= 257; i++ {
		fmt.Fprintf(&many, "  - '0x%040x'\n", i)
	}
	for name, body := range map[string]string{
		"empty list": "  []\n", "too many": many.String(), "empty": "  - ''\n", "zero": "  - '0x0000000000000000000000000000000000000000'\n", "invalid": "  - invalid\n", "short": "  - '0x1'\n", "number": "  - 1\n", "null": "  - null\n", "duplicate": "  - '" + mixed + "'\n  - '" + canonical + "'\n",
		"named object": "  - name: custom\n    executor: '" + mixed + "'\n", "unnamed object": "  - executor: '" + mixed + "'\n", "label": "  - executor-" + canonical + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseConfig(configNode(t, strategyConfig(body))); err == nil {
				t.Fatal("invalid strategy accepted")
			}
		})
	}
}

func TestGenericHostRejectsThreeFConfig(t *testing.T) {
	if _, err := parseConfig(configNode(t, validConfig+"threeF: {}\n")); err == nil {
		t.Fatal("3F configuration accepted by generic host")
	}
}
