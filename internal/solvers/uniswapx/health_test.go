package uniswapx

import (
	"testing"
	"time"
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
	if !solver.ready() {
		t.Fatal("fresh solver should be ready")
	}
	solver.beginFillPlanning()
	if solver.ready() {
		t.Fatal("solver planning a fill should not be ready")
	}
	solver.endFillPlanning()
	solver.quoteState.Store(&quoteState{epoch: solver.quoteEpoch.Load(), expiresAt: now.Add(30 * time.Second)})
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
