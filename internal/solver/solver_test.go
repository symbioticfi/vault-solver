package solver

import (
	"context"
	"strings"
	"testing"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

type fakeSolver struct{ name string }

func (f fakeSolver) Name() string { return f.name }

func (f fakeSolver) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestRunTreatsCancellationAsClean(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, fakeSolver{name: "x"}, logr.Discard()); err != nil {
		t.Fatalf("expected nil on cancellation, got %v", err)
	}
}

type failingSolver struct {
	name string
	err  error
}

func (f failingSolver) Name() string              { return f.name }
func (f failingSolver) Run(context.Context) error { return f.err }

func TestRunWrapsNonCancellationError(t *testing.T) {
	sentinel := errors.New("startup failed")
	err := Run(context.Background(), failingSolver{name: "3f", err: sentinel}, logr.Discard())
	if err == nil {
		t.Fatal("expected a non-nil error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("error should wrap the solver's error, got %v", err)
	}
	if !strings.Contains(err.Error(), `"3f"`) {
		t.Fatalf("error should name the solver, got %v", err)
	}
}
