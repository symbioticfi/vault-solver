package redstoneoev

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
	webhookstrategy "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/webhook"
)

func TestBuildBidCharacterizesPendingAuctionProjection(t *testing.T) {
	s, now := solverWithPendingProjectionFacts(t)
	strategy := &recordingBidStrategy{}
	s.strategy = strategy

	if decision := s.buildBid(t.Context(), decodeAuction(t), func() time.Time { return now }); decision.skip != "" {
		t.Fatalf("buildBid skip = %q, want bid", decision.skip)
	}

	want := pendingProjectionWant()
	if !slices.Equal(strategy.input.PendingAuctions, want) {
		t.Fatalf("pending auctions = %+v, want %+v", strategy.input.PendingAuctions, want)
	}

	strategy.input.PendingAuctions[0] = types.PendingAuction{ID: "mutated"}
	next := &recordingBidStrategy{}
	s.strategy = next
	if decision := s.buildBid(t.Context(), decodeAuction(t), func() time.Time { return now }); decision.skip != "" {
		t.Fatalf("second buildBid skip = %q, want bid", decision.skip)
	}
	if !slices.Equal(next.input.PendingAuctions, want) {
		t.Fatalf("strategy input aliased reservation state: got %+v, want %+v", next.input.PendingAuctions, want)
	}
}

func TestBuildBidCharacterizesPendingAuctionsWebhookJSON(t *testing.T) {
	requestBody := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requestBody <- body
		_, _ = w.Write([]byte(`{"decision":"skip","reason":"captured"}`))
	}))
	defer server.Close()

	var node yaml.Node
	if err := yaml.Unmarshal([]byte("url: "+server.URL+"\n"), &node); err != nil {
		t.Fatalf("unmarshal webhook config: %v", err)
	}
	strategy, err := webhookstrategy.NewFromConfig(*node.Content[0])
	if err != nil {
		t.Fatalf("new webhook strategy: %v", err)
	}

	s, now := solverWithPendingProjectionFacts(t)
	s.strategy = strategy
	if decision := s.buildBid(t.Context(), decodeAuction(t), func() time.Time { return now }); decision.skip != types.SkipReasonStrategy {
		t.Fatalf("buildBid skip = %q, want %q", decision.skip, types.SkipReasonStrategy)
	}

	var wire struct {
		PendingAuctions json.RawMessage `json:"pendingAuctions"`
	}
	if err := json.Unmarshal(<-requestBody, &wire); err != nil {
		t.Fatalf("decode webhook request: %v", err)
	}
	const want = `[{"id":"alpha","sentAt":"2030-01-02T02:59:05.000000001Z","won":false,"expiresAt":"2030-01-02T03:04:05.000000001Z"},{"id":"zeta","sentAt":"2030-01-02T03:03:05Z","won":true,"expiresAt":"2030-01-02T03:08:05Z"}]`
	if string(wire.PendingAuctions) != want {
		t.Fatalf("pendingAuctions JSON = %s, want %s", wire.PendingAuctions, want)
	}
}

func solverWithPendingProjectionFacts(t *testing.T) (*Solver, time.Time) {
	t.Helper()
	s, _ := seededSolver(t)
	now := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
	state, ok := s.state.load()
	if !ok {
		t.Fatal("missing seeded state")
	}
	state.UpdatedAt = now
	s.state.store(state)

	s.reserve(8, time.Date(2030, time.January, 2, 3, 3, 5, 0, time.UTC), "zeta")
	s.markReservationWon("zeta")
	s.reserve(9, time.Date(2030, time.January, 2, 2, 59, 5, 0, time.UTC), "exact-expiry")
	s.reserve(10, time.Date(2030, time.January, 2, 2, 59, 4, 999_999_999, time.UTC), "after-expiry")
	s.reserve(11, time.Date(2030, time.January, 2, 2, 59, 5, 1, time.UTC), "alpha")
	s.reserve(12, time.Date(2030, time.January, 2, 3, 3, 5, 0, time.UTC), "")
	return s, now
}

func pendingProjectionWant() []types.PendingAuction {
	return []types.PendingAuction{
		{
			ID:        "alpha",
			SentAt:    time.Date(2030, time.January, 2, 2, 59, 5, 1, time.UTC),
			Won:       false,
			ExpiresAt: time.Date(2030, time.January, 2, 3, 4, 5, 1, time.UTC),
		},
		{
			ID:        "zeta",
			SentAt:    time.Date(2030, time.January, 2, 3, 3, 5, 0, time.UTC),
			Won:       true,
			ExpiresAt: time.Date(2030, time.January, 2, 3, 8, 5, 0, time.UTC),
		},
	}
}
