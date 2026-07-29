package redstoneoev

import (
	"math/big"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestLifecycleMetricsCountTransitionsOnce(t *testing.T) {
	s, _ := seededSolver(t)
	m, err := newMetrics(prometheus.NewRegistry(), defaultStrategyName, s.wonReservationCount)
	if err != nil {
		t.Fatal(err)
	}
	s.metrics = m
	bidWei := big.NewInt(123)
	m.submittedBid(bidWei)
	if got := testutil.ToFloat64(m.bidWei.WithLabelValues(oevBidSubmitted)); got != 123 {
		t.Fatalf("submitted bid wei = %v, want 123", got)
	}
	s.reserve(8, time.Now(), "auction", bidWei)

	won := marshal(AuctionResult{
		Op: "auction-result", ID: "auction",
		Data: AuctionResultData{Liquidator: seedCallback.Hex()},
	})
	s.handleMessage(t.Context(), won)
	s.handleMessage(t.Context(), won)
	if got := testutil.ToFloat64(m.wins); got != 1 {
		t.Fatalf("wins = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.bidWei.WithLabelValues(oevBidWon)); got != 123 {
		t.Fatalf("won bid wei = %v, want 123", got)
	}
	if got := testutil.ToFloat64(m.wonInflight); got != 1 {
		t.Fatalf("won inflight = %v, want 1", got)
	}

	settled := marshal(LiquidationResult{
		Op: "liquidation-result", ID: "auction",
		Data: LiquidationResultData{Success: true, Liquidator: seedCallback.Hex()},
	})
	s.handleMessage(t.Context(), settled)
	s.handleMessage(t.Context(), settled)
	if got := testutil.ToFloat64(m.settlements.WithLabelValues("success")); got != 1 {
		t.Fatalf("successful settlements = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.bidWei.WithLabelValues(oevBidSettledSuccess)); got != 123 {
		t.Fatalf("successfully settled bid wei = %v, want 123", got)
	}
	if got := testutil.ToFloat64(m.wonInflight); got != 0 {
		t.Fatalf("won inflight = %v, want 0", got)
	}

	failedBidWei := big.NewInt(456)
	s.reserve(9, time.Now(), "reordered", failedBidWei)
	failed := marshal(LiquidationResult{
		Op: "liquidation-result", ID: "reordered",
		Data: LiquidationResultData{Success: false, Liquidator: seedCallback.Hex()},
	})
	s.handleMessage(t.Context(), failed)
	if got := testutil.ToFloat64(m.wins); got != 2 {
		t.Fatalf("settlement-before-result wins = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.settlements.WithLabelValues("failed")); got != 1 {
		t.Fatalf("failed settlements = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.bidWei.WithLabelValues(oevBidSettledFailed)); got != 456 {
		t.Fatalf("failed settled bid wei = %v, want 456", got)
	}
}

func TestWonReservationTimeoutIsVisible(t *testing.T) {
	s, _ := seededSolver(t)
	m, err := newMetrics(prometheus.NewRegistry(), defaultStrategyName, s.wonReservationCount)
	if err != nil {
		t.Fatal(err)
	}
	s.metrics = m
	now := time.Now()
	s.reserve(8, now.Add(-reservationTTL-time.Second), "auction", big.NewInt(123))
	s.markReservationWon("auction")

	s.pruneReservations(7, now)

	if got := testutil.ToFloat64(m.unresolvedWinsTotal); got != 1 {
		t.Fatalf("unresolved wins = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.wonInflight); got != 0 {
		t.Fatalf("won inflight after timeout = %v, want 0", got)
	}
}
