package uniswapx

import (
	"math/big"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func TestReadyRequiresFreshDeliveryAndQuoteState(t *testing.T) {
	now := time.Now()
	solver := &Solver{cfg: &Config{
		QuoteServer: QuoteServerConfig{QuoteTTL: 30 * time.Second},
		OrderServer: OrderServerConfig{PollInterval: time.Second},
	}}
	solver.lastExclusivePoll.Store(now.Unix())
	if solver.ready() {
		t.Fatal("solver without a quote state should not be ready")
	}
	solver.quoteState.Store(&quoteState{epoch: solver.quoteEpoch.Load(), expiresAt: now.Add(30 * time.Second)})
	if solver.ready() {
		t.Fatal("solver without quote inventory should not be ready")
	}
	solver.quoteState.Store(&quoteState{
		epoch: solver.quoteEpoch.Load(), expiresAt: now.Add(30 * time.Second),
		inventory: []liquidlane.Inventory{{}},
	})
	if !solver.ready() {
		t.Fatal("fresh solver should be ready")
	}
	txm := &executionTestTxManager{unavailable: true}
	solver.txm = txm
	if solver.ready() {
		t.Fatal("solver with a paused transaction nonce lane should not be ready")
	}
	txm.unavailable = false
	if !solver.ready() {
		t.Fatal("solver did not become ready after the transaction nonce lane resumed")
	}
	txm.busy = true
	if solver.ready() {
		t.Fatal("solver with a busy shared transaction nonce lane should not be ready")
	}
	txm.busy = false
	if !solver.ready() {
		t.Fatal("solver did not become ready after the shared transaction nonce lane became idle")
	}
	solver.beginFillPlanning()
	if solver.ready() {
		t.Fatal("solver planning a fill should not be ready")
	}
	solver.endFillPlanning()
	solver.quoteState.Store(&quoteState{
		epoch: solver.quoteEpoch.Load(), expiresAt: now.Add(30 * time.Second),
		inventory: []liquidlane.Inventory{{}},
	})
	solver.warmupUntil.Store(now.Add(time.Minute).Unix())
	if solver.ready() {
		t.Fatal("warmup solver should not be ready")
	}
	solver.warmupUntil.Store(0)
	solver.lastExclusivePoll.Store(now.Add(-time.Minute).Unix())
	if solver.ready() {
		t.Fatal("solver with stale exclusive delivery should not be ready")
	}
}

func TestReadinessMetricIsEvaluatedAtCollection(t *testing.T) {
	now := time.Now()
	solver := &Solver{cfg: &Config{
		QuoteServer: QuoteServerConfig{QuoteTTL: 30 * time.Second},
		OrderServer: OrderServerConfig{PollInterval: time.Second},
	}}
	solver.lastExclusivePoll.Store(now.Unix())
	solver.quoteState.Store(&quoteState{
		epoch: solver.quoteEpoch.Load(), expiresAt: now.Add(time.Minute),
		inventory: []liquidlane.Inventory{{}},
	})
	metrics := newUniswapXTestMetrics(t, solver)

	if got := testutil.ToFloat64(metrics.ready); got != 1 {
		t.Fatalf("ready metric = %v, want 1", got)
	}
	solver.quoteState.Store(&quoteState{
		epoch: solver.quoteEpoch.Load(), expiresAt: now.Add(-time.Minute),
		inventory: []liquidlane.Inventory{{}},
	})
	if got := testutil.ToFloat64(metrics.ready); got != 0 {
		t.Fatalf("stale ready metric = %v, want 0", got)
	}
}

func TestBlockUntilMetricIncludesEveryTimeBasedBlocker(t *testing.T) {
	solver := &Solver{}
	metrics := newUniswapXTestMetrics(t, solver)
	solver.blockUntil.Store(10)
	solver.localBlockUntil.Store(20)
	solver.exclusiveBlockUntil.Store(30)
	solver.warmupUntil.Store(40)

	if got := testutil.ToFloat64(metrics.blockUntil); got != 40 {
		t.Fatalf("block until metric = %v, want 40", got)
	}
	solver.warmupUntil.Store(0)
	if got := testutil.ToFloat64(metrics.blockUntil); got != 30 {
		t.Fatalf("block until metric without warmup = %v, want 30", got)
	}
}

func TestStateMetricsAreEvaluatedAtCollection(t *testing.T) {
	solver := &Solver{}
	metrics := newUniswapXTestMetrics(t, solver)
	solver.lastExclusivePoll.Store(123)
	if !solver.capacity.Set("order", liquidlane.CapacityReservations{
		liquidlane.CapacityID("capacity"): big.NewInt(1),
	}) {
		t.Fatal("capacity reservation was not stored")
	}

	if got := testutil.ToFloat64(metrics.exclusivePoll); got != 123 {
		t.Fatalf("exclusive poll metric = %v, want 123", got)
	}
	if got := testutil.ToFloat64(metrics.pendingFills); got != 1 {
		t.Fatalf("pending fills metric = %v, want 1", got)
	}
	if !solver.capacity.Delete("order") {
		t.Fatal("capacity reservation was not released")
	}
	if got := testutil.ToFloat64(metrics.pendingFills); got != 0 {
		t.Fatalf("released pending fills metric = %v, want 0", got)
	}
}
