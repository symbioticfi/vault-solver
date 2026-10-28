package threef

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"gopkg.in/yaml.v3"
)

const validConfig = `mode: simulate
stateDir: state/threef-merkle
executor: "0x0000000000000000000000000000000000000010"
curator:
  url: http://127.0.0.1:1/api/v1
url: https://bf.3f.xyz
apiKeyEnv: THREEF_CURATOR_API_KEY
broker: "0x0000000000000000000000000000000000000040"
`

func configNode(t *testing.T, text string) yaml.Node {
	t.Helper()
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(text), &n); err != nil {
		t.Fatal(err)
	}
	return *n.Content[0]
}
func TestConfigValidation(t *testing.T) {
	for name, raw := range map[string]string{
		"default simulate": strings.Replace(validConfig, "mode: simulate\n", "", 1),
		"send":             strings.Replace(validConfig, "mode: simulate", "mode: send", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseConfig(configNode(t, raw)); err != nil {
				t.Fatal(err)
			}
		})
	}
	for name, raw := range map[string]string{
		"mode":           strings.Replace(validConfig, "mode: simulate", "mode: unsafe", 1),
		"state":          strings.Replace(validConfig, "stateDir: state/threef-merkle\n", "", 1),
		"zero executor":  strings.Replace(validConfig, "0000000000000000000000000000000000000010", "0000000000000000000000000000000000000000", 1),
		"zero broker":    strings.Replace(validConfig, "0000000000000000000000000000000000000040", "0000000000000000000000000000000000000000", 1),
		"curator":        strings.Replace(validConfig, "http://127.0.0.1:1/api/v1", "", 1),
		"url":            strings.Replace(validConfig, "https://bf.3f.xyz", "https://bf.3f.xyz?key=private", 1),
		"key":            strings.Replace(validConfig, "THREEF_CURATOR_API_KEY", "invalid-env-name", 1),
		"inline key":     validConfig + "apiKey: secret\n",
		"manual amount":  validConfig + "principal: '100'\n",
		"manual account": validConfig + "account: '0x20'\n",
		"strategies":     validConfig + "strategies: []\n",
		"nested threeF":  validConfig + "threeF: {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseConfig(configNode(t, raw)); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
func TestRecoveryIdentity(t *testing.T) {
	cfg, err := parseConfig(configNode(t, validConfig))
	if err != nil {
		t.Fatal(err)
	}
	strategy := keeper.Config{Name: cfg.Executor, Executor: cfg.Executor, Account: common.HexToAddress("0x20").Hex(), Interval: pollInterval}
	caller := common.HexToAddress("0x30")
	original, err := strategyRevision(strategy, cfg, caller)
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*config, *keeper.Config, *common.Address){
		"key":      func(c *config, _ *keeper.Config, _ *common.Address) { c.APIKeyEnv = "ANOTHER_CURATOR_KEY" },
		"broker":   func(c *config, _ *keeper.Config, _ *common.Address) { c.Broker = common.HexToAddress("0x41").Hex() },
		"API":      func(c *config, _ *keeper.Config, _ *common.Address) { c.URL = "https://other.example" },
		"curator":  func(c *config, _ *keeper.Config, _ *common.Address) { c.Curator.URL = "https://other.example" },
		"account":  func(_ *config, s *keeper.Config, _ *common.Address) { s.Account = common.HexToAddress("0x21").Hex() },
		"executor": func(_ *config, s *keeper.Config, _ *common.Address) { s.Executor = common.HexToAddress("0x11").Hex() },
		"caller":   func(_ *config, _ *keeper.Config, c *common.Address) { *c = common.HexToAddress("0x31") },
	} {
		t.Run(name, func(t *testing.T) {
			c, s, owner := cfg, strategy, caller
			change(&c, &s, &owner)
			got, err := strategyRevision(s, c, owner)
			if err != nil || got == original {
				t.Fatalf("changed configuration escaped recovery: %v", err)
			}
		})
	}
}

func TestConfigRequiresOneExecutorAndRejectsArray(t *testing.T) {
	const entry = `executor: "0x0000000000000000000000000000000000000010"`
	for name, tc := range map[string]struct{ value, want string }{
		"missing": {"", "executor"}, "zero": {`executor: "0x0000000000000000000000000000000000000000"`, "executor"},
		"malformed": {`executor: "invalid"`, "executor"}, "array": {`executors: ["0x0000000000000000000000000000000000000010"]`, "field executors not found"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseConfig(configNode(t, strings.Replace(validConfig, entry, tc.value, 1)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted %q got %v", tc.want, err)
			}
		})
	}
}

func TestEligibilityWorkBudgetConfiguration(t *testing.T) {
	for _, value := range []string{"1", "256", "131072"} {
		if _, err := parseConfig(configNode(t, validConfig+"maxEligibilityChecksPerTick: "+value+"\n")); err != nil {
			t.Fatalf("valid work budget %s: %v", value, err)
		}
	}
	for _, value := range []string{"-1", "0", "131073"} {
		if _, err := parseConfig(configNode(t, validConfig+"maxEligibilityChecksPerTick: "+value+"\n")); err == nil {
			t.Fatalf("invalid work budget %s accepted", value)
		}
	}
}

func TestEligibilitySchedulingDoesNotChangeRecoveryIdentity(t *testing.T) {
	cfg, err := parseConfig(configNode(t, validConfig))
	if err != nil {
		t.Fatal(err)
	}
	strategy := keeper.Config{Name: cfg.Executor, Executor: cfg.Executor, Account: common.HexToAddress("0x20").Hex(), Interval: pollInterval}
	caller := common.HexToAddress("0x30")
	original, err := strategyRevision(strategy, cfg, caller)
	if err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{1, 131072} {
		cfg.MaxEligibilityChecksPerTick = limit
		changed, err := strategyRevision(strategy, cfg, caller)
		if err != nil || changed != original {
			t.Fatalf("scheduling-only cap changed durable recovery identity: %v", err)
		}
	}
}
