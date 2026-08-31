// Package solver defines shared solver contracts and lifecycle execution.
package solver

import (
	"context"
	"time"

	"github.com/go-errors/errors"

	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// Deps are the shared services every solver receives. The txmanager is shared so solvers never
// race on the sending account's nonce.
type Deps struct {
	Chain     *chain.Client
	TxManager *txmanager.Manager
	Signer    signer.Signer
	Log       logr.Logger
	Metrics   *observability.Metrics
	// ReportFatal cancels the process runtime before a solver finishes draining accepted work.
	// The solver must still return the reported error from Run.
	ReportFatal func(error)
}

// Solver is a long-running strategy. Run must honor ctx cancellation and return nil (or a
// context error) on clean shutdown.
type Solver interface {
	Name() string
	Run(ctx context.Context) error
}

// ShutdownPreparer optionally reports how long a solver may keep admitting work after cancellation
// while it retires externally visible work such as active quotes.
type ShutdownPreparer interface {
	ShutdownPreparationTimeout() time.Duration
}

// Run executes a solver until it returns or ctx is cancelled. A context cancellation is treated
// as a clean shutdown, not an error.
func Run(ctx context.Context, s Solver, log logr.Logger) error {
	log = log.WithValues("solver", s.Name())
	log.Info("solver running")
	err := s.Run(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		wrapped := errors.Errorf("solver %q: %w", s.Name(), err)
		// Attribute the failure to this solver in the structured logs; the returned error still drives exit.
		log.Error(wrapped, "solver stopped with error")
		return wrapped
	}
	log.Info("solver stopped")
	return nil
}
