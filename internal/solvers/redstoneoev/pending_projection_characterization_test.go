package redstoneoev

import (
	"slices"
	"testing"
	"time"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
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
