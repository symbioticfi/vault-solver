// Package solver defines the generic, solver-agnostic framework: the Solver interface, a
// name->factory registry, shared dependencies, and a thin run engine. Concrete strategies live in
// their own packages under internal/solvers and self-register via Register in an init function.
package solver

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/go-errors/errors"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/parse"
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

// Factory builds a Solver from its opaque config block and shared runtime dependencies.
type Factory func(raw yaml.Node, deps Deps) (Solver, error)

// ConfigValidator performs integration-owned config validation without runtime dependencies or I/O.
type ConfigValidator func(raw yaml.Node) error

// Registration describes one integration's construction and offline configuration contract.
type Registration struct {
	Factory             Factory
	ValidateConfig      ConfigValidator
	ExternallySubmitted bool
}

// DecodeStrict decodes a deferred solvers[].config node into out, rejecting unknown keys so typos in
// a solver's config fail fast — matching the strict top-level config decode. The framework keeps
// each solver's config opaque (yaml.Node has no KnownFields option of its own), so solvers call this
// from parseConfig instead of node.Decode to get the same typo protection.
func DecodeStrict(node yaml.Node, out any) error {
	return parse.DecodeStrict(node, out)
}

var (
	mu       sync.RWMutex
	registry = map[string]Registration{}
)

// Register associates a solver name with its runtime and offline config contracts. It is intended
// for package init functions and panics on incomplete or duplicate registrations.
func Register(name string, registration Registration) {
	mu.Lock()
	defer mu.Unlock()
	if name == "" {
		panic("solver: Register called with empty name")
	}
	if registration.Factory == nil {
		panic("solver: Register called with nil factory for " + name)
	}
	if registration.ValidateConfig == nil {
		panic("solver: Register called with nil config validator for " + name)
	}
	if _, dup := registry[name]; dup {
		panic("solver: duplicate registration for " + name)
	}
	registry[name] = registration
}

// New constructs the solver registered under name.
func New(name string, raw yaml.Node, deps Deps) (Solver, error) {
	registration, ok := lookup(name)
	if !ok {
		return nil, unknownSolverError(name)
	}
	return registration.Factory(raw, deps)
}

// ValidateConfig validates one opaque solver config without constructing runtime dependencies.
func ValidateConfig(name string, raw yaml.Node) error {
	registration, ok := lookup(name)
	if !ok {
		return unknownSolverError(name)
	}
	if err := registration.ValidateConfig(raw); err != nil {
		return errors.Errorf("solver %q config: %w", name, err)
	}
	return nil
}

// RequiresTxManager reports the registered integration's transaction-submission mode.
func RequiresTxManager(name string) (bool, error) {
	registration, ok := lookup(name)
	if !ok {
		return false, unknownSolverError(name)
	}
	return !registration.ExternallySubmitted, nil
}

func lookup(name string) (Registration, bool) {
	mu.RLock()
	defer mu.RUnlock()
	registration, ok := registry[name]
	return registration, ok
}

func unknownSolverError(name string) error {
	return errors.Errorf("solver: unknown solver %q (registered: %v)", name, Registered())
}

// Registered returns the sorted list of registered solver names (for diagnostics).
func Registered() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
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
