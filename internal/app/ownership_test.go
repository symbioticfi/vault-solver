package app

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/observability"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

type heldService struct{ started, draining, finish chan struct{} }

func (*heldService) Name() string { return "held" }
func (s *heldService) Run(ctx context.Context) error {
	close(s.started)
	<-ctx.Done()
	close(s.draining)
	<-s.finish
	return ctx.Err()
}

type observedLane struct{ stopped chan struct{} }

func (l *observedLane) Start(ctx context.Context) { <-ctx.Done(); close(l.stopped) }
func (*observedLane) LaneReady() bool             { return true }
func (*observedLane) SubscribeLaneState() (<-chan struct{}, func()) {
	return make(chan struct{}), func() {}
}

func TestSolverDrainsBeforeLaneStops(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	s := &heldService{started: make(chan struct{}), draining: make(chan struct{}), finish: make(chan struct{})}
	lane := &observedLane{stopped: make(chan struct{})}
	_, health := observability.NewMetrics()
	done := make(chan error, 1)
	go func() {
		done <- supervise(ctx, cancel, []service{{s, logr.Discard()}}, lane, health, time.Second, logr.Discard())
	}()
	<-s.started
	cancel(context.Canceled)
	<-s.draining
	select {
	case <-lane.stopped:
		t.Fatal("lane stopped before solver drained")
	default:
	}
	close(s.finish)
	testcheck.NoError(t, <-done)
	select {
	case <-lane.stopped:
	default:
		t.Fatal("lane leaked after shutdown")
	}
}
