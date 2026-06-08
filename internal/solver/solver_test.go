package solver

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"
)

type fakeSolver struct{ name string }

func (f fakeSolver) Name() string { return f.name }

func (f fakeSolver) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestRegisterAndNew(t *testing.T) {
	Register("test-fake", func(yaml.Node, Deps) (Solver, error) {
		return fakeSolver{name: "test-fake"}, nil
	})

	s, err := New("test-fake", yaml.Node{}, Deps{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.Name() != "test-fake" {
		t.Fatalf("expected name test-fake, got %q", s.Name())
	}

	if _, err := New("does-not-exist", yaml.Node{}, Deps{}); err == nil {
		t.Fatal("expected unknown-solver error")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	Register("dup", func(yaml.Node, Deps) (Solver, error) { return fakeSolver{name: "dup"}, nil })
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	Register("dup", func(yaml.Node, Deps) (Solver, error) { return fakeSolver{name: "dup"}, nil })
}

func TestRunTreatsCancellationAsClean(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, fakeSolver{name: "x"}, logr.Discard()); err != nil {
		t.Fatalf("expected nil on cancellation, got %v", err)
	}
}
