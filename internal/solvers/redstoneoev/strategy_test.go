package redstoneoev

import (
	"math/big"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

func TestNewStrategyRejectsUnknown(t *testing.T) {
	_, err := newStrategy(&Config{Strategy: StrategyConfig{Name: "bogus"}}, defaultstrategy.FactoryDeps{})
	if err == nil || !strings.Contains(err.Error(), "unknown OEV strategy") {
		t.Fatalf("error = %v, want unknown strategy", err)
	}
}

func TestNewStrategyParsesWebhookConfig(t *testing.T) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(`
url: https://strategy.example
maxRequestBytes: 2048
maxResponseBytes: 4096
`), &node); err != nil {
		t.Fatal(err)
	}
	if _, err := newStrategy(&Config{
		Strategy: StrategyConfig{Name: "webhook", Config: *node.Content[0]},
	}, defaultstrategy.FactoryDeps{}); err != nil {
		t.Fatalf("new webhook strategy: %v", err)
	}
}

func TestCheckExecutionEnvelopeRejectsSkipWithBidData(t *testing.T) {
	err := checkExecutionEnvelope(types.BidOutput{
		Decision:  types.DecisionSkip,
		BidAmount: big.NewInt(1),
	})
	if err == nil || !strings.Contains(err.Error(), "skip output") {
		t.Fatalf("error = %v, want skip output rejection", err)
	}
}

func TestCheckExecutionEnvelopeRejectsMissingOperationData(t *testing.T) {
	err := checkExecutionEnvelope(types.BidOutput{
		Decision:  types.DecisionBid,
		BidAmount: big.NewInt(1),
	})
	if err == nil || !strings.Contains(err.Error(), "empty operationData") {
		t.Fatalf("error = %v, want operationData rejection", err)
	}
}

func TestCheckExecutionEnvelopeAcceptsGenericOperationData(t *testing.T) {
	err := checkExecutionEnvelope(types.BidOutput{
		Decision:      types.DecisionBid,
		BidAmount:     big.NewInt(1),
		OperationData: []byte{1},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBoundedStrategySkipReason(t *testing.T) {
	if got := types.BoundedSkipReason(types.SkipReasonNoLegs); got != types.SkipReasonNoLegs {
		t.Fatalf("known reason = %q, want %q", got, types.SkipReasonNoLegs)
	}
	if got := types.BoundedSkipReason("remote-user-12345"); got != types.SkipReasonStrategy {
		t.Fatalf("unknown reason = %q, want %q", got, types.SkipReasonStrategy)
	}
}
