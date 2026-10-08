package merkle

import (
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
stateDir: state/merkle
strategies:
  - name: rebalance
    executor: "0x0000000000000000000000000000000000000010"
    account: "0x0000000000000000000000000000000000000020"
    tree: permissions.json
    inputs: {0: "10"}
`

func TestConfigValidation(t *testing.T) {
	for _, tc := range []struct{ name, extra string }{
		{"unknown field", "unexpected: true\n"}, {"mode", "mode: unsafe\n"}, {"duration", "pollInterval: -1s\n"}, {"missing tree", "strategies: []\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseConfig(configNode(t, invalidConfig(tc.name, tc.extra))); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	cfg, err := parseConfig(configNode(t, validConfig))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.mode != "simulate" || len(cfg.strategies) != 1 || cfg.strategies[0].Providers == nil {
		t.Fatalf("missing runnable SDK workflow: %+v", cfg)
	}
}

func invalidConfig(name, extra string) string {
	switch name {
	case "mode":
		return strings.Replace(validConfig, "mode: simulate", strings.TrimSpace(extra), 1)
	case "missing tree":
		return "mode: simulate\nstateDir: state/merkle\nstrategies: []\n"
	default:
		return validConfig + extra
	}
}

func TestWorkflowRevisionIncludesAmountsAndRejectsOverflow(t *testing.T) {
	first, err := parseConfig(configNode(t, validConfig))
	if err != nil {
		t.Fatal(err)
	}
	changed, err := parseConfig(configNode(t, strings.Replace(validConfig, `"10"`, `"11"`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if first.strategies[0].Config.Fingerprint() == changed.strategies[0].Config.Fingerprint() {
		t.Fatal("amount change invisible to active workflow recovery")
	}
	for _, value := range []string{"-1", "not-a-number", "115792089237316195423570985008687907853269984665640564039457584007913129639936"} {
		if _, err := parseConfig(configNode(t, strings.Replace(validConfig, `"10"`, `"`+value+`"`, 1))); err == nil {
			t.Fatal("invalid uint256 accepted")
		}
	}
}
