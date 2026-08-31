package redstoneoev

import (
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/default"
	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
	webhookstrategy "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/webhook"
)

const oevSelectionBase = `
ws: {url: "wss://oev.example", apiKeyEnv: OEV_API_KEY}
executor: "0x1111111111111111111111111111111111111111"
adapter: "0x2222222222222222222222222222222222222222"
callback: "0x3333333333333333333333333333333333333333"
`

const oevDefaultStrategyConfig = `
  config:
    morphoApiUrl: https://api.morpho.example/graphql
    bid: {bidEth: "0.0001"}
`

func TestStrategySelectionCharacterization(t *testing.T) {
	tests := []struct {
		name              string
		strategy          string
		wantName          string
		wantType          string
		wantValidationErr string
		wantSelectionErr  string
	}{
		{
			name:     "omitted name selects default",
			strategy: "strategy:" + oevDefaultStrategyConfig,
			wantName: "default",
			wantType: "default",
		},
		{
			name:     "default",
			strategy: "strategy:\n  name: default" + oevDefaultStrategyConfig,
			wantName: "default",
			wantType: "default",
		},
		{
			name: "webhook",
			strategy: `strategy:
  name: webhook
  config:
    url: https://strategy.example
maxBidWei: "1"
`,
			wantName: "webhook",
			wantType: "webhook",
		},
		{
			name:              "unknown without bid cap",
			strategy:          "strategy: {name: missing, config: {}}\n",
			wantName:          "missing",
			wantValidationErr: `strategy: unknown OEV strategy "missing" (registered: [default webhook])`,
			wantSelectionErr:  `unknown OEV strategy "missing" (registered: [default webhook])`,
		},
		{
			name: "default rejects unknown config key",
			strategy: `strategy:
  name: default
  config:
    morphoApiUrl: https://api.morpho.example/graphql
    bid: {bidEth: "0.0001"}
    unexpected: true
`,
			wantName: "default",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := oevSelectionNode(t, oevSelectionBase+test.strategy)
			cfg, err := parseConfig(node)
			if err != nil {
				t.Fatalf("parseConfig: %v", err)
			}
			if cfg.Strategy.Name != test.wantName {
				t.Fatalf("strategy name = %q, want %q", cfg.Strategy.Name, test.wantName)
			}

			validationErr := ValidateConfig(node)
			selected, selectionErr := newStrategy(cfg, oevSelectionDeps())
			if (validationErr == nil) != (selectionErr == nil) {
				t.Fatalf("validation error = %v, selection error = %v", validationErr, selectionErr)
			}
			if test.wantValidationErr != "" {
				if validationErr == nil || validationErr.Error() != test.wantValidationErr {
					t.Fatalf("validation error = %v, want %q", validationErr, test.wantValidationErr)
				}
				if selectionErr == nil || selectionErr.Error() != test.wantSelectionErr {
					t.Fatalf("selection error = %v, want %q", selectionErr, test.wantSelectionErr)
				}
				return
			}
			if test.wantType == "" {
				if validationErr == nil || selectionErr == nil {
					t.Fatalf("validation error = %v, selection error = %v; want strict rejection", validationErr, selectionErr)
				}
				return
			}
			if validationErr != nil || selectionErr != nil {
				t.Fatalf("validation error = %v, selection error = %v", validationErr, selectionErr)
			}
			assertOEVStrategyType(t, selected, test.wantType)
		})
	}
	if got, want := strategyNames(), []string{"default", "webhook"}; !slices.Equal(got, want) {
		t.Fatalf("strategy catalog = %v, want %v", got, want)
	}

	t.Run("root rejects unknown key", func(t *testing.T) {
		raw := oevSelectionBase + "strategy:" + oevDefaultStrategyConfig + "unexpected: true\n"
		if _, err := parseConfig(oevSelectionNode(t, raw)); err == nil {
			t.Fatal("parseConfig accepted an unknown root key")
		}
	})
}

func TestStrategyBidCapPolicyCharacterization(t *testing.T) {
	defaultConfig := "strategy:" + oevDefaultStrategyConfig
	webhookConfig := "strategy: {name: webhook, config: {url: https://strategy.example}}\n"
	tests := []struct {
		name    string
		config  string
		wantErr string
	}{
		{name: "default without maxBidWei", config: defaultConfig},
		{
			name:    "webhook without maxBidWei",
			config:  webhookConfig,
			wantErr: "maxBidWei is required for webhook strategy",
		},
		{name: "webhook with positive maxBidWei", config: webhookConfig + "maxBidWei: \"1\"\n"},
		{
			name:    "unknown without maxBidWei reaches selector",
			config:  "strategy: {name: missing, config: {}}\n",
			wantErr: `strategy: unknown OEV strategy "missing" (registered: [default webhook])`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateConfig(oevSelectionNode(t, oevSelectionBase+test.config))
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("validateConfig: %v", err)
				}
				return
			}
			if err == nil || err.Error() != test.wantErr {
				t.Fatalf("validateConfig error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func oevSelectionNode(t *testing.T, raw string) yaml.Node {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &document); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	return *document.Content[0]
}

func oevSelectionDeps() defaultstrategy.FactoryDeps {
	return defaultstrategy.FactoryDeps{
		Log:      logr.Discard(),
		ChainID:  1,
		Adapter:  common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Callback: common.HexToAddress("0x3333333333333333333333333333333333333333"),
		LoadAdapterSnapshot: func() (strategytypes.AdapterSnapshot, bool) {
			return strategytypes.AdapterSnapshot{}, false
		},
	}
}

func assertOEVStrategyType(t *testing.T, strategy strategytypes.Strategy, want string) {
	t.Helper()
	switch want {
	case "default":
		if _, ok := strategy.(*defaultstrategy.Strategy); !ok {
			t.Fatalf("strategy type = %T, want *defaultstrategy.Strategy", strategy)
		}
	case "webhook":
		if _, ok := strategy.(*webhookstrategy.Strategy); !ok {
			t.Fatalf("strategy type = %T, want *webhookstrategy.Strategy", strategy)
		}
	default:
		t.Fatalf("unsupported expected strategy type %q", want)
	}
}
