package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/symbioticfi/vault-solver/internal/config"
)

func TestWatchReadinessTracksLaneState(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	changes := make(chan struct{}, 1)
	states := make(chan bool, 3)
	var ready atomic.Bool
	ready.Store(true)
	go watchReadiness(ctx, changes, ready.Load, func(state bool) { states <- state })

	ready.Store(false)
	changes <- struct{}{}
	expectReadyState(t, states, false)
	ready.Store(true)
	changes <- struct{}{}
	expectReadyState(t, states, true)
	cancel()
	expectReadyState(t, states, false)
}

func expectReadyState(t *testing.T, states <-chan bool, want bool) {
	t.Helper()
	select {
	case got := <-states:
		if got != want {
			t.Fatalf("ready state = %t, want %t", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("ready state did not change to %t", want)
	}
}

func TestMonitorTransactionDrainForcesStopAfterTimeout(t *testing.T) {
	shutdown := make(chan struct{})
	solversDone := make(chan struct{})
	stopped := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		monitorTransactionDrain(shutdown, solversDone, time.Millisecond, func() {
			stopped <- struct{}{}
		})
	}()

	close(shutdown)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("transaction drain did not force stop after timeout")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("transaction drain monitor did not return after timeout")
	}
}

func TestMonitorTransactionDrainStopsWhenSolversFinish(t *testing.T) {
	shutdown := make(chan struct{})
	solversDone := make(chan struct{})
	stopped := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		monitorTransactionDrain(shutdown, solversDone, time.Hour, func() {
			stopped <- struct{}{}
		})
	}()

	close(shutdown)
	close(solversDone)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("transaction drain monitor did not stop with solvers")
	}
	select {
	case <-stopped:
		t.Fatal("transaction manager was stopped after solvers drained")
	default:
	}
}

func TestTransactionDrainBudget(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy string
		ri     int
		block  int
		want   time.Duration
	}{
		{name: "legacy keeps one replacement interval", policy: config.FeePolicyLegacy, ri: 30_000, block: 12_000, want: 5*time.Second + 5*time.Minute + 30*time.Second},
		{name: "legacy ignores the block time", policy: config.FeePolicyLegacy, ri: 1_000, block: 12_000, want: 5*time.Second + 5*time.Minute + time.Second},
		{name: "horizon with the replacement interval above a block", policy: config.FeePolicyHorizon, ri: 15_000, block: 12_000, want: 5*time.Second + 5*time.Minute + 15*time.Second},
		{name: "horizon with a block above the replacement interval", policy: config.FeePolicyHorizon, ri: 5_000, block: 12_000, want: 5*time.Second + 5*time.Minute + 12*time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.TxManagerConfig{PendingTimeoutMs: 300_000, ReplacementIntervalMs: tc.ri}
			cfg.Fees.Policy, cfg.Fees.BlockTimeMs = tc.policy, tc.block
			if got := transactionDrainBudget(cfg, 5*time.Second); got != tc.want {
				t.Fatalf("drain budget = %s, want %s", got, tc.want)
			}
		})
	}
}
