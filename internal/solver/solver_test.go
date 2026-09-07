package solver

import (
	"context"
	"strings"
	"testing"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/parse"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
	"gopkg.in/yaml.v3"
)

type fakeSolver struct{ name string }

func (f fakeSolver) Name() string { return f.name }

func (f fakeSolver) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestDecodeStrict(t *testing.T) {
	type cfg struct {
		Known string `yaml:"known"`
	}
	parse := func(body string) (cfg, error) {
		var doc yaml.Node
		testcheck.NoError(t, yaml.Unmarshal([]byte(body), &doc), "unmarshal: %v")
		var out cfg
		err := parse.DecodeStrict(*doc.Content[0], &out)
		return out, err
	}

	out, err := parse("known: ok\n")
	testcheck.NoError(t, err, "valid config: %v")
	if out.Known != "ok" {
		t.Fatalf("expected known=ok, got %q", out.Known)
	}

	if _, err := parse("known: ok\nunknown: typo\n"); err == nil {
		t.Fatal("expected unknown key to be rejected")
	}
}

type txManagerIndependentSolver struct{ fakeSolver }

func (txManagerIndependentSolver) RequiresTxManager() bool { return false }

func TestRequiresTxManagerDefaultsToSafe(t *testing.T) {
	if !RequiresTxManager(fakeSolver{name: "default"}) {
		t.Fatal("solver without an explicit capability must require txManager")
	}
	if RequiresTxManager(txManagerIndependentSolver{fakeSolver{name: "external"}}) {
		t.Fatal("externally submitted solver must not require txManager")
	}
}

func TestRunTreatsCancellationAsClean(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	testcheck.NoError(t, Run(ctx, fakeSolver{name: "x"}, logr.Discard()), "expected nil on cancellation, got %v")
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
