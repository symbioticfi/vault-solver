package redstoneoev

import (
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	dto "github.com/prometheus/client_model/go"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

type stubStateSnapshotSource struct {
	snapshot         cachedState
	err              error
	boundaryFailures int
	calls            int
}

func (s *stubStateSnapshotSource) Snapshot(context.Context) (cachedState, error) {
	s.calls++
	if s.boundaryFailures > 0 {
		s.boundaryFailures--
		return cachedState{}, errStateRefreshBlockBoundary
	}
	return s.snapshot, s.err
}

func newOEVTestMetrics(
	t *testing.T,
	wonMetrics func() (int, time.Duration),
) (*metrics, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	metrics, err := newMetrics(reg, defaultStrategyName, wonMetrics)
	if err != nil {
		t.Fatal(err)
	}
	return metrics, reg
}

func requireOEVEvent(
	t *testing.T,
	reg *prometheus.Registry,
	event, outcome string,
	count, timestamp float64,
) {
	t.Helper()
	metricstest.RequireWorkflowEvent(t, reg, Name, event, outcome, count, timestamp)
}

func oevEventTimestamp(t *testing.T, reg *prometheus.Registry, event, outcome string) float64 {
	t.Helper()
	return metricstest.FamilyValue(t, reg, "solver_bot_workflow_last_event_timestamp", map[string]string{
		"solver": Name, "event": event, "outcome": outcome,
	})
}

func requireOEVBidAmount(
	t *testing.T,
	reg *prometheus.Registry,
	stage string,
	want float64,
) {
	t.Helper()
	metricstest.RequireWorkflowAmount(t, reg, Name, "bid", "native", stage, want)
}

func TestStateRefreshFreshnessAdvancesOnlyAfterSnapshotInstallation(t *testing.T) {
	s, _ := seededSolver(t)
	m, reg := newOEVTestMetrics(t, s.wonReservationMetrics)
	s.metrics = m
	source := &stubStateSnapshotSource{err: errors.New("rpc unavailable")}
	s.stateSource = source
	m.workflow.ObserveEventAt("state_refresh", "success", 1, time.Unix(123, 0))
	before, ok := s.state.load()
	if !ok {
		t.Fatal("seeded state missing")
	}

	if err := s.refreshState(t.Context()); err == nil {
		t.Fatal("failed state refresh returned nil error")
	}
	if got := oevEventTimestamp(t, reg, "state_refresh", "success"); got != 123 {
		t.Fatalf("freshness after failed refresh = %v, want retained 123", got)
	}
	afterFailure, ok := s.state.load()
	if !ok || !afterFailure.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatal("failed refresh replaced the last-known-good state")
	}

	installedAt := before.UpdatedAt.Add(time.Minute)
	source.err = nil
	source.snapshot = cachedState{
		Exec:    ExecutorState{Nonce: big.NewInt(8), Deposit: big.NewInt(20_000_000_000_000), Locked: false},
		Adapter: seedAdapterSnapshot(), GasLimit: 3_000_000, UpdatedAt: installedAt,
	}
	if err := s.refreshState(t.Context()); err != nil {
		t.Fatalf("successful state refresh: %v", err)
	}
	freshness := oevEventTimestamp(t, reg, "state_refresh", "success")
	if freshness <= 123 {
		t.Fatalf("freshness after successful refresh = %v, want advanced timestamp", freshness)
	}
	installed, ok := s.state.load()
	if !ok || !installed.UpdatedAt.Equal(installedAt) {
		t.Fatal("successful refresh did not install its complete snapshot")
	}
	if got := testutil.ToFloat64(m.deposit); got != 20_000_000_000_000 {
		t.Fatalf("deposit after successful refresh = %v, want applied snapshot value", got)
	}

	source.err = errors.New("second rpc failure")
	source.snapshot.UpdatedAt = installedAt.Add(time.Minute)
	if err := s.refreshState(t.Context()); err == nil {
		t.Fatal("second failed state refresh returned nil error")
	}
	if got := oevEventTimestamp(t, reg, "state_refresh", "success"); got != freshness {
		t.Fatalf("freshness after later failure = %v, want retained %v", got, freshness)
	}
	retained, ok := s.state.load()
	if !ok || !retained.UpdatedAt.Equal(installedAt) {
		t.Fatal("later failure replaced the installed snapshot")
	}
}

func TestStateRefreshExternalOperationOutcomes(t *testing.T) {
	s, _ := seededSolver(t)
	metrics, reg := newOEVTestMetrics(t, s.wonReservationMetrics)
	s.metrics = metrics
	s.stateRefreshObserver = metrics.workflow.Operation(stateRefreshOperation)
	snapshot, ok := s.state.load()
	if !ok {
		t.Fatal("seed state missing")
	}
	source := &stubStateSnapshotSource{snapshot: snapshot}
	s.stateSource = source

	if err := s.refreshState(t.Context()); err != nil {
		t.Fatalf("successful refresh: %v", err)
	}
	source.err = errStateRefreshBlockBoundary
	if err := s.refreshState(t.Context()); !errors.Is(err, errStateRefreshBlockBoundary) {
		t.Fatalf("block-boundary refresh error = %v", err)
	}
	metricstest.RequireExternalOperationCount(t, reg, Name, stateRefreshOperation, "success", 1)
	metricstest.RequireExternalOperationCount(t, reg, Name, stateRefreshOperation, "degraded", 1)
}

func TestStateRefreshBlockBoundaryWithoutCacheIsError(t *testing.T) {
	metrics, reg := newOEVTestMetrics(t, nil)
	s := &Solver{
		stateSource:          &stubStateSnapshotSource{err: errStateRefreshBlockBoundary},
		stateRefreshObserver: metrics.workflow.Operation(stateRefreshOperation),
	}
	if err := s.refreshState(t.Context()); !errors.Is(err, errStateRefreshBlockBoundary) {
		t.Fatalf("block-boundary refresh error = %v", err)
	}
	metricstest.RequireExternalOperationCount(t, reg, Name, stateRefreshOperation, "error", 1)
	metricstest.RequireExternalOperationCount(t, reg, Name, stateRefreshOperation, "degraded", 0)
}

func TestStateRefreshBoundaryRetryIsBounded(t *testing.T) {
	for _, test := range []struct {
		name             string
		boundaryFailures int
		wantErr          bool
	}{
		{"one boundary", 1, false},
		{"repeated boundaries", 3, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _ := seededSolver(t)
			snapshot, _ := s.state.load()
			source := &stubStateSnapshotSource{snapshot: snapshot, boundaryFailures: test.boundaryFailures}
			s.stateSource = source
			err := s.refreshStateWithBoundaryRetry(t.Context())
			if (err != nil) != test.wantErr || test.wantErr && !errors.Is(err, errStateRefreshBlockBoundary) {
				t.Fatalf("refresh error = %v, want boundary error %t", err, test.wantErr)
			}
			if source.calls != 2 {
				t.Fatalf("snapshot calls = %d, want initial attempt plus one retry", source.calls)
			}
		})
	}
}

func TestOEVBoundedLifecycleSeriesArePreinitialized(t *testing.T) {
	_, reg := newOEVTestMetrics(t, nil)
	for _, outcome := range auctionDecisionOutcomes {
		requireOEVEvent(t, reg, "auction", outcome, 0, 0)
	}
	for _, outcome := range append(bidLifecycleStages[:], oevBidUnresolved) {
		requireOEVEvent(t, reg, "bid", outcome, 0, 0)
	}
	for _, stage := range bidLifecycleStages {
		requireOEVBidAmount(t, reg, stage, 0)
	}
	requireOEVEvent(t, reg, "breaker", "failure", 0, 0)
	requireOEVEvent(t, reg, "state_refresh", "success", 0, 0)
}

func TestLifecycleMetricsCountTransitionsOnce(t *testing.T) {
	s, _ := seededSolver(t)
	m, reg := newOEVTestMetrics(t, s.wonReservationMetrics)
	s.metrics = m
	m.now = func() time.Time { return time.Unix(123, 0) }
	bidWei := big.NewInt(123)
	m.enqueuedBid(bidWei)
	requireOEVEvent(t, reg, "bid", oevBidEnqueued, 1, 123)
	requireOEVBidAmount(t, reg, oevBidEnqueued, 123)
	s.reserve(8, time.Now(), "auction", bidWei)

	won := marshal(AuctionResult{
		Op: "auction-result", ID: "auction",
		Data: AuctionResultData{Liquidator: seedCallback.Hex()},
	})
	s.handleMessage(t.Context(), won)
	s.handleMessage(t.Context(), won)
	requireOEVEvent(t, reg, "bid", oevBidWon, 1, 123)
	requireOEVBidAmount(t, reg, oevBidWon, 123)
	metricstest.RequireValue(t, m.wonInflight, 1)

	settled := marshal(LiquidationResult{
		Op: "liquidation-result", ID: "auction",
		Data: LiquidationResultData{Success: true, Liquidator: seedCallback.Hex()},
	})
	s.handleMessage(t.Context(), settled)
	s.handleMessage(t.Context(), settled)
	requireOEVEvent(t, reg, "bid", oevBidSettledSuccess, 1, 123)
	requireOEVBidAmount(t, reg, oevBidSettledSuccess, 123)
	metricstest.RequireValue(t, m.wonInflight, 0)

	failedBidWei := big.NewInt(456)
	s.reserve(9, time.Now(), "reordered", failedBidWei)
	failed := marshal(LiquidationResult{
		Op: "liquidation-result", ID: "reordered",
		Data: LiquidationResultData{Success: false, Liquidator: seedCallback.Hex()},
	})
	s.handleMessage(t.Context(), failed)
	requireOEVEvent(t, reg, "bid", oevBidWon, 2, 123)
	requireOEVEvent(t, reg, "bid", oevBidSettledFailed, 1, 123)
	requireOEVBidAmount(t, reg, oevBidSettledFailed, 456)
	requireOEVEvent(t, reg, "breaker", "failure", 1, 123)
}

func TestOldestWonInflightAgeTracksFirstObservedWin(t *testing.T) {
	s, _ := seededSolver(t)
	now := time.Unix(1_781_243_340, 0)
	reg := prometheus.NewRegistry()
	m, err := newMetrics(reg, "webhook", func() (int, time.Duration) {
		return s.wonReservationMetricsAt(now)
	})
	if err != nil {
		t.Fatal(err)
	}
	metricstest.RequireValue(t, m.oldestWonInflight, 0)
	s.reserve(7, now, "future-win", nil)
	if _, transitioned := s.markReservationWon("future-win", now.Add(time.Second)); !transitioned {
		t.Fatal("future-dated reservation did not transition to won")
	}
	metricstest.RequireValue(t, m.oldestWonInflight, 0)
	s.releaseReservationByAuction("future-win")

	// An older pending reservation is not a local win and must not influence either gauge.
	s.reserve(8, now.Add(-time.Minute), "pending", nil)
	s.reserve(9, now.Add(-time.Minute), "newer-win", nil)
	s.reserve(10, now.Add(-time.Minute), "older-win", nil)
	if _, transitioned := s.markReservationWon("newer-win", now.Add(-10*time.Second)); !transitioned {
		t.Fatal("newer reservation did not transition to won")
	}
	if _, transitioned := s.markReservationWon("older-win", now.Add(-30*time.Second)); !transitioned {
		t.Fatal("older reservation did not transition to won")
	}
	// A replay is deduped by the lifecycle transition and must not replace the original wonAt.
	if _, transitioned := s.markReservationWon("older-win", now.Add(-time.Second)); transitioned {
		t.Fatal("duplicate win transitioned the reservation twice")
	}

	metricstest.RequireValue(t, m.wonInflight, 2)
	metricstest.RequireValue(t, m.oldestWonInflight, 30)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	var strategy string
	for _, family := range families {
		if family.GetName() != "oev_oldest_won_inflight_age_seconds" {
			continue
		}
		for _, label := range family.GetMetric()[0].GetLabel() {
			if label.GetName() == "strategy" {
				strategy = label.GetValue()
			}
		}
	}
	if strategy != "webhook" {
		t.Fatalf("oldest won inflight strategy label = %q, want webhook", strategy)
	}

	s.releaseReservationByAuction("older-win")
	metricstest.RequireValue(t, m.oldestWonInflight, 10)
	s.releaseReservationByAuction("newer-win")
	metricstest.RequireValue(t, m.oldestWonInflight, 0)
	metricstest.RequireValue(t, m.wonInflight, 0)
}

func TestWonInflightMetricsAreRaceSafeDuringLifecycleUpdates(t *testing.T) {
	s, _ := seededSolver(t)
	now := time.Unix(1_781_243_340, 0)
	m, err := newMetrics(prometheus.NewRegistry(), defaultStrategyName, func() (int, time.Duration) {
		return s.wonReservationMetricsAt(now)
	})
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for i := range 1_000 {
			s.reserve(uint64(i+1), now, "auction", nil)
			s.markReservationWon("auction", now.Add(-time.Second))
			s.releaseReservationByAuction("auction")
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for range 1_000 {
			testutil.ToFloat64(m.wonInflight)
			testutil.ToFloat64(m.oldestWonInflight)
		}
	}()
	close(start)
	wg.Wait()

	metricstest.RequireValue(t, m.wonInflight, 0)
	metricstest.RequireValue(t, m.oldestWonInflight, 0)
}

func TestWonReservationTimeoutIsVisible(t *testing.T) {
	s, _ := seededSolver(t)
	m, reg := newOEVTestMetrics(t, s.wonReservationMetrics)
	s.metrics = m
	now := time.Now()
	s.reserve(8, now.Add(-reservationTTL-time.Second), "auction", big.NewInt(123))
	if _, transitioned := s.markReservationWon("auction", now); !transitioned {
		t.Fatal("reservation did not transition to won")
	}

	s.pruneReservations(7, now)

	metricstest.RequireWorkflowEventCount(t, reg, Name, "bid", oevBidUnresolved, 1)
	metricstest.RequireValue(t, m.wonInflight, 0)
}

func TestAuctionDecisionCountsEveryParsedTerminalPathOnce(t *testing.T) {
	s, _ := seededSolver(t)
	m, reg := newOEVTestMetrics(t, s.wonReservationMetrics)
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
		metricstest.RequireWorkflowEventCount(t, reg, Name, "auction", outcome, 1)
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
