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
recipeExecutor: "0x0000000000000000000000000000000000000043"
curator: {url: https://curator.example/api/v1}
connectors:
 - "0x00000000000000000000000000000000000000ab"
 - "0x0000000000000000000000000000000000000042"
`
	for _, tc := range []struct{ name, text, want string }{
		{"onchain recipe discovery", base, ""},
		{"duplicate connector", base + ` - "0x00000000000000000000000000000000000000AB"` + "\n", "duplicate connector"},
		{"zero connector", strings.ReplaceAll(base, "00000000ab", "0000000000"), "connector"},
		{"empty connectors", strings.Replace(base, `connectors:
 - "0x00000000000000000000000000000000000000ab"
 - "0x0000000000000000000000000000000000000042"`, "connectors: []", 1), ""},
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
			if cfg.RecipeExecutor != common.HexToAddress("0x43") {
				t.Fatal("local recipe executor changed")
			}
		})
	}
}
