package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatchReadinessTracksLaneState(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	states := make(chan bool, 3)
	var ready atomic.Bool
	go watchReadiness(ctx, time.Millisecond, ready.Load, func(state bool) { states <- state })

	expectReadyState(t, states, false)
	ready.Store(true)
	expectReadyState(t, states, true)
	cancel()
	expectReadyState(t, states, false)
}

func TestWatchReadinessReadsCompletedReconciliationWithoutSignal(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	states := make(chan bool, 2)
	go watchReadiness(ctx, time.Hour, func() bool { return true }, func(state bool) { states <- state })
	expectReadyState(t, states, true)
	cancel()
	expectReadyState(t, states, false)
}

func TestWatchReadinessDropsTxlessReadiness(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	states := make(chan bool, 2)
	go watchReadiness(ctx, time.Hour, func() bool { return true }, func(state bool) { states <- state })
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
