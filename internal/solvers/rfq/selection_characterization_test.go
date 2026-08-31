package rfq

import (
	"reflect"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/default"
	webhookstrategy "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/webhook"
)

func TestStrategySelectionCharacterization(t *testing.T) {
	tests := []struct {
		name       string
		spec       StrategyConfig
		wantType   reflect.Type
		wantError  string
		wantReject bool
	}{
		{
			name:     "default",
			spec:     StrategyConfig{Name: defaultstrategy.Name, Config: strategyConfigNode(t, "{}")},
			wantType: reflect.TypeFor[*defaultstrategy.Strategy](),
		},
		{
			name: "webhook",
			spec: StrategyConfig{
				Name:   webhookstrategy.Name,
				Config: strategyConfigNode(t, "url: https://strategy.example"),
			},
			wantType: reflect.TypeFor[*webhookstrategy.Strategy](),
		},
		{
			name: "default rejects unknown config key",
			spec: StrategyConfig{
				Name:   defaultstrategy.Name,
				Config: strategyConfigNode(t, "unexpected: true"),
			},
			wantReject: true,
		},
		{
			name:      "unknown",
			spec:      StrategyConfig{Name: "missing", Config: strategyConfigNode(t, "{}")},
			wantError: `unknown RFQ strategy "missing" (registered: [default webhook])`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validationErr := validateStrategyConfig(test.spec)
			selected, selectionErr := newStrategy(test.spec)
			if (validationErr == nil) != (selectionErr == nil) {
				t.Fatalf("validation error = %v, selection error = %v", validationErr, selectionErr)
			}
			if test.wantError != "" {
				if validationErr == nil || validationErr.Error() != test.wantError {
					t.Fatalf("validation error = %v, want %q", validationErr, test.wantError)
				}
				if selectionErr == nil || selectionErr.Error() != test.wantError {
					t.Fatalf("selection error = %v, want %q", selectionErr, test.wantError)
				}
				return
			}
			if test.wantReject {
				if validationErr == nil || selectionErr == nil {
					t.Fatalf("validation error = %v, selection error = %v; want strict rejection", validationErr, selectionErr)
				}
				return
			}
			if validationErr != nil || selectionErr != nil {
				t.Fatalf("validation error = %v, selection error = %v", validationErr, selectionErr)
			}
			if got := reflect.TypeOf(selected); got != test.wantType {
				t.Fatalf("strategy type = %v, want %v", got, test.wantType)
			}
		})
	}

	if got, want := strategyNames(), []string{"default", "webhook"}; !slices.Equal(got, want) {
		t.Fatalf("strategy catalog = %v, want %v", got, want)
	}
}

func strategyConfigNode(t *testing.T, raw string) yaml.Node {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &document); err != nil {
		t.Fatalf("unmarshal strategy config: %v", err)
	}
	return *document.Content[0]
}
