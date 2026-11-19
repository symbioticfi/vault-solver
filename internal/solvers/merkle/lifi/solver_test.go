package lifi

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler/fillertest"
)

const validConfig = `
orderServer:
  baseUrl: https://order.example
  wsUrl: wss://order.example
  apiKeyEnv: LIFI_SOLVER_API_KEY
inputSettler: "0x2222222222222222222222222222222222222222"
outputSettler: "0x3333333333333333333333333333333333333333"
executor: "0x4444444444444444444444444444444444444444"
privateDiscountsUrl: https://rfq.example
recipeExecutors:
  - "0x4343434343434343434343434343434343434343"
curator:
  url: https://curator.example
`

// The lifi-merkle config binds the engine's executor to the Merkle lane and carries no adapter
// settings; the example profile parses.
func TestConfig(t *testing.T) {
	parse := func(raw string) error {
		var document yaml.Node
		if err := yaml.Unmarshal([]byte(raw), &document); err != nil {
			t.Fatal(err)
		}
		cfg, lane, err := parseConfig(*document.Content[0])
		if err == nil && (lane.Executor != cfg.Executor || len(lane.RecipeExecutors) != 1 || lane.Curator.URL == "" || lane.QuoteOnly ||
			cfg.DiscountsURL == "" || cfg.Gas != nil || len(cfg.Adapters) != 0) {
			t.Fatalf("lane = %+v engine = %+v", lane, cfg)
		}
		return err
	}
	if err := parse(validConfig); err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string]string{
		"adapters":          validConfig + "adapters: []\n",
		"solver mode":       validConfig + "solverMode: internal\n",
		"gas accounting":    validConfig + "gas: {}\n",
		"liquidity lens":    validConfig + "liquidityLens: \"0x5555555555555555555555555555555555555555\"\n",
		"webhook strategy":  validConfig + "strategy:\n  name: webhook\n",
		"missing curator":   strings.Replace(validConfig, "  url: https://curator.example\n", "", 1),
		"missing recipes":   strings.Replace(validConfig, "recipeExecutors:\n  - \"0x4343434343434343434343434343434343434343\"\n", "", 1),
		"missing discounts": strings.Replace(validConfig, "privateDiscountsUrl: https://rfq.example\n", "", 1),
	} {
		if parse(raw) == nil {
			t.Fatalf("%s: invalid config accepted", name)
		}
	}
	if _, lane, err := parseConfig(fillertest.ExampleConfig(t, "../../../../config/lifi-merkle.example.yaml")); err != nil || lane.Validate() != nil {
		t.Fatalf("example config: lane = %+v err=%v", lane, err)
	}
}
