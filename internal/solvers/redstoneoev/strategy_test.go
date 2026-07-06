package redstoneoev

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

func TestNewStrategyRejectsUnknown(t *testing.T) {
	_, err := newStrategy(&Config{Strategy: StrategyConfig{Name: "bogus"}}, strategies.Deps{})
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
	}, strategies.Deps{}); err != nil {
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

func TestPendingAuctionsForStrategyFiltersExpired(t *testing.T) {
	now := time.Unix(1000, 0)
	got := pendingAuctionsForStrategy([]pendingAuction{
		{ID: "", SentAt: now},
		{ID: "expired", SentAt: now.Add(-reservationTTL - time.Second)},
		{ID: "pending", SentAt: now.Add(-time.Minute), Won: true},
	}, now)
	if len(got) != 1 || got[0].ID != "pending" || !got[0].Won {
		t.Fatalf("pending = %+v", got)
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
