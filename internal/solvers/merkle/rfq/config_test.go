package rfq

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"
)

func TestAssetRecipeConfiguration(t *testing.T) {
	const base = `backendUrl: https://rfq.example
stateDir: /persistent/rfq-merkle
executor: "0x0000000000000000000000000000000000000010"
curator: {url: https://curator.example/api/v1}
recipeExecutors:
  "0x0000000000000000000000000000000000000042":
    "0x00000000000000000000000000000000000000ab": "0x0000000000000000000000000000000000000043"
    "0x00000000000000000000000000000000000000ac": "0x0000000000000000000000000000000000000044"
`
	for _, tc := range []struct{ name, text, want string }{
		{"one recipe per asset", base, ""},
		{"duplicate asset", base + `    "0x00000000000000000000000000000000000000AB": "0x0000000000000000000000000000000000000045"` + "\n", "duplicate asset"},
		{"shared executor", strings.ReplaceAll(base, "0000000044", "0000000043"), "distinct recipe executors"},
		{"shared across connectors", base + `  "0x0000000000000000000000000000000000000046":
    "0x00000000000000000000000000000000000000ad": "0x0000000000000000000000000000000000000043"
`, "distinct recipe executors"},
		{"empty connector", base + `  "0x0000000000000000000000000000000000000046": {}` + "\n", "1..64 asset recipe executors"},
		{"zero asset", strings.ReplaceAll(base, "00000000ab", "0000000000"), "asset"},
		{"zero recipe", strings.ReplaceAll(base, "0000000043", "0000000000"), "recipe executor"},
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
			routes := cfg.RecipeExecutors[common.HexToAddress("0x42")]
			if routes[common.HexToAddress("0xab")] != common.HexToAddress("0x43") || routes[common.HexToAddress("0xac")] != common.HexToAddress("0x44") {
				t.Fatal("asset recipe mapping changed")
			}
			if _, ok := routes[common.HexToAddress("0xad")]; ok {
				t.Fatal("unconfigured asset acquired a fallback recipe")
			}
		})
	}
}
