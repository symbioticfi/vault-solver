package redstoneoev

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func TestLifecycleMetricsCountTransitionsOnce(t *testing.T) {
	s, _ := seededSolver(t)
	m, err := newMetrics(prometheus.NewRegistry(), defaultStrategyName, s.wonReservationCount)
	if err != nil {
		t.Fatal(err)
	}
	s.metrics = m
	bidWei := big.NewInt(123)
	m.enqueuedBid(bidWei)
	if got := testutil.ToFloat64(m.bidWei.WithLabelValues(oevBidEnqueued)); got != 123 {
		t.Fatalf("enqueued bid wei = %v, want 123", got)
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
	if got := testutil.ToFloat64(m.breakerFailures); got != 1 {
		t.Fatalf("breaker failures = %v, want 1", got)
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

func TestAuctionDecisionCountsEveryParsedTerminalPathOnce(t *testing.T) {
	s, _ := seededSolver(t)
	m, err := newMetrics(prometheus.NewRegistry(), defaultStrategyName, s.wonReservationCount)
	if err != nil {
		t.Fatal(err)
	}
	s.metrics = m

	empty := decodeAuction(t)
	empty.ID = ""
	s.handleAuctionWithContext(t.Context(), marshal(empty))

	canceled := decodeAuction(t)
	canceled.ID = "canceled"
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s.handleAuctionWithContext(ctx, marshal(canceled))
	s.handleAuctionWithContext(t.Context(), marshal(canceled))

	late := decodeAuction(t)
	late.ID = "late"
	late.Timestamp = time.Now().Add(-time.Second).UnixMilli()
	late.TimeoutMs = 1
	s.handleAuctionWithContext(t.Context(), marshal(late))

	s.handleMessage(t.Context(), []byte(`{
		"op":"auction","id":"feed",
		"payload":{"ETH":"250000000000"}
	}`))
	s.handleAuctionWithContext(t.Context(), []byte(`{"op":"auction"`))

	outcomes := []string{
		skipEmptyAuctionID,
		auctionOutcomeContextCanceled,
		auctionOutcomeDuplicate,
		auctionOutcomeTooLate,
		auctionOutcomeFeedIgnored,
	}
	for _, outcome := range outcomes {
		if got := testutil.ToFloat64(m.decisions.WithLabelValues(outcome)); got != 1 {
			t.Errorf("%s decisions = %v, want 1", outcome, got)
		}
	}

	metric := &dto.Metric{}
	if err := m.hotPath.Write(metric); err != nil {
		t.Fatal(err)
	}
	if got := metric.GetHistogram().GetSampleCount(); got != uint64(len(outcomes)) {
		t.Fatalf("hot-path samples = %d, want %d", got, len(outcomes))
	}
}

func TestBuildBidClassifiesCancellationDuringStrategy(t *testing.T) {
	s, _ := seededSolver(t)
	blocking := &blockingBidStrategy{started: make(chan struct{}, 1), release: make(chan struct{})}

	auction := decodeAuction(t)
	auction.ID = "canceled-during-strategy"
	auction.Timestamp = time.Now().UnixMilli()
	setSnapshotBlockTime(t, s, auction.Timestamp)
	s.strategy = blocking
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() {
		select {
		case <-blocking.started:
			cancel()
		case <-t.Context().Done():
		}
	}()
	if decision := s.buildBidWithContext(ctx, auction, time.Now); decision.skip != auctionOutcomeContextCanceled {
		t.Fatalf("decision = %q, want %q", decision.skip, auctionOutcomeContextCanceled)
	}
}
