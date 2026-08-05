package main

import (
	"testing"
	"time"
)

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
