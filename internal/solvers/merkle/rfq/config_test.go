package rfq

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"
)

func TestConnectorConfigurationUsesOnchainRecipes(t *testing.T) {
	const base = `backendUrl: https://rfq.example
stateDir: /persistent/rfq-merkle
executor: "0x0000000000000000000000000000000000000010"
recipeExecutors: ["0x0000000000000000000000000000000000000043"]
curator: {url: https://curator.example/api/v1}
`
	for _, tc := range []struct{ name, text, want string }{
		{"onchain recipe discovery", base, ""},
		{"multiple executors", strings.Replace(base, `recipeExecutors: ["0x0000000000000000000000000000000000000043"]`, `recipeExecutors: ["0x0000000000000000000000000000000000000043", "0x0000000000000000000000000000000000000044"]`, 1), ""},
		{"duplicate executor", strings.Replace(base, `recipeExecutors: ["0x0000000000000000000000000000000000000043"]`, `recipeExecutors: ["0x0000000000000000000000000000000000000043", "0x0000000000000000000000000000000000000043"]`, 1), "distinct"},
		{"empty list", strings.Replace(base, `recipeExecutors: ["0x0000000000000000000000000000000000000043"]`, `recipeExecutors: []`, 1), "recipeExecutors"},
		{"obsolete connector list", base + "connectors: []\n", "field connectors not found"},
		{"scalar executor rejected", base + "recipeExecutor: 0x43\n", "field recipeExecutor not found"},
		{"missing recipe executor", strings.Replace(base, `recipeExecutors: ["0x0000000000000000000000000000000000000043"]`+"\n", "", 1), "recipeExecutor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var node yaml.Node
			if err := yaml.Unmarshal([]byte(tc.text), &node); err != nil {
				t.Fatal(err)
			}
			cfg, err := parseConfig(node)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("wanted %s: %v", tc.want, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(cfg.RecipeExecutors) == 0 || cfg.RecipeExecutors[0] != common.HexToAddress("0x43") {
				t.Fatal("local recipe executor changed")
			}
		})
	}
}
