package solver

import (
	"context"
	"strings"
	"testing"

	"github.com/go-errors/errors"
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
	isolateRegistry(t)
	Register("test-fake", Registration{
		Factory: func(yaml.Node, Deps) (Solver, error) {
			return fakeSolver{name: "test-fake"}, nil
		},
		ValidateConfig: func(yaml.Node) error { return nil },
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
	isolateRegistry(t)
	registration := Registration{
		Factory:        func(yaml.Node, Deps) (Solver, error) { return fakeSolver{name: "dup"}, nil },
		ValidateConfig: func(yaml.Node) error { return nil },
	}
	Register("dup", registration)
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	Register("dup", registration)
}

func TestRegistrationMetadata(t *testing.T) {
	isolateRegistry(t)
	validated := false
	Register("metadata", Registration{
		Factory: func(yaml.Node, Deps) (Solver, error) {
			return fakeSolver{name: "metadata"}, nil
		},
		ValidateConfig: func(yaml.Node) error {
			validated = true
			return nil
		},
		ExternallySubmitted: true,
	})
	if err := ValidateConfig("metadata", yaml.Node{}); err != nil {
		t.Fatalf("ValidateConfig: %v", err)
	}
	if !validated {
		t.Fatal("config validator was not called")
	}
	requires, err := RequiresTxManager("metadata")
	if err != nil {
		t.Fatalf("RequiresTxManager: %v", err)
	}
	if requires {
		t.Fatal("externally submitted solver must not require txManager")
	}
}

func isolateRegistry(t *testing.T) {
	t.Helper()
	mu.Lock()
	previous := registry
	registry = make(map[string]Registration)
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		registry = previous
		mu.Unlock()
	})
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
