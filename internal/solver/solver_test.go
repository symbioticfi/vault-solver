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

func TestDecodeStrict(t *testing.T) {
	type cfg struct {
		Known string `yaml:"known"`
	}
	parse := func(body string) (cfg, error) {
		var doc yaml.Node
		if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		var out cfg
		err := DecodeStrict(*doc.Content[0], &out)
		return out, err
	}

	out, err := parse("known: ok\n")
	if err != nil {
		t.Fatalf("valid config: %v", err)
	}
	if out.Known != "ok" {
		t.Fatalf("expected known=ok, got %q", out.Known)
	}

	if _, err := parse("known: ok\nunknown: typo\n"); err == nil {
		t.Fatal("expected unknown key to be rejected")
	}
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
