package defaultstrategy

import (
	"testing"
	"time"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

func TestDecisionReservationsLifecycle(t *testing.T) {
	now := time.Unix(1000, 0)
	var reservations decisionReservations

	reservations.reserve("auction-1")
	live := reservations.reconcile([]types.PendingAuction{{ID: "auction-1", ExpiresAt: now.Add(time.Minute)}}, now, now)
	if !live {
		t.Fatal("pending auction must block another bid")
	}

	resolved := reservations.reconcile(nil, now.Add(time.Second), now)
	if !resolved {
		t.Fatal("reservation released before a post-resolution callback balance refresh")
	}
	refreshed := reservations.reconcile(nil, now.Add(2*time.Second), now.Add(2*time.Second))
	if refreshed {
		t.Fatalf("reservation not released after balance refresh: %+v", refreshed)
	}
}

func TestDecisionReservationsDropUnsentDecision(t *testing.T) {
	var reservations decisionReservations
	reservations.reserve("auction-1")
	got := reservations.reconcile(nil, time.Unix(1000, 0), time.Unix(1000, 0))
	if got {
		t.Fatalf("decision never observed as pending must be released: %+v", got)
	}
}
