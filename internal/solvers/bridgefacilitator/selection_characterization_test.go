package bridgefacilitator

import (
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/types"
	webhookstrategy "github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/webhook"
)

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
			strategy: "strategy:\n  config: {}\n",
			wantName: "default",
			wantType: "default",
		},
		{
			name:     "default",
			strategy: "strategy: {name: default, config: {}}\n",
			wantName: "default",
			wantType: "default",
		},
		{
			name: "webhook",
			strategy: `strategy:
  name: webhook
  config:
    url: https://strategy.example
`,
			wantName: "webhook",
			wantType: "webhook",
		},
		{
			name:              "unknown",
			strategy:          "strategy: {name: missing, config: {}}\n",
			wantName:          "missing",
			wantValidationErr: `strategy: unknown 3F strategy "missing" (registered: [default webhook])`,
			wantSelectionErr:  `unknown 3F strategy "missing" (registered: [default webhook])`,
		},
		{
			name:     "default rejects unknown config key",
			strategy: "strategy: {name: default, config: {unexpected: true}}\n",
			wantName: "default",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := oneTarget + test.strategy
			node := bridgeSelectionNode(t, raw)
			cfg, err := parseConfig(node)
			if err != nil {
				t.Fatalf("parseConfig: %v", err)
			}
			if cfg.Strategy.Name != test.wantName {
				t.Fatalf("strategy name = %q, want %q", cfg.Strategy.Name, test.wantName)
			}

			validationErr := validateConfig(node)
			selected, selectionErr := newStrategy(cfg.Strategy)
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
			assertBridgeStrategyType(t, selected, test.wantType)
		})
	}
	if got, want := strategyNames(), []string{"default", "webhook"}; !slices.Equal(got, want) {
		t.Fatalf("strategy catalog = %v, want %v", got, want)
	}

	t.Run("root rejects unknown key", func(t *testing.T) {
		if _, err := parseConfig(bridgeSelectionNode(t, oneTarget+"unexpected: true\n")); err == nil {
			t.Fatal("parseConfig accepted an unknown root key")
		}
	})
}

func bridgeSelectionNode(t *testing.T, raw string) yaml.Node {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &document); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	return *document.Content[0]
}

func assertBridgeStrategyType(t *testing.T, strategy types.Strategy, want string) {
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
