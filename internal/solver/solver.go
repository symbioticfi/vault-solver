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

// RequiresTxManager reports whether a solver uses the shared on-chain sender. Solvers default to
// requiring it; integrations whose transactions are submitted externally may opt out.
func RequiresTxManager(s Solver) bool {
	requirer, ok := s.(interface{ RequiresTxManager() bool })
	return !ok || requirer.RequiresTxManager()
}

// ShutdownPreparer optionally reports how long a solver may keep admitting work after cancellation
// while it retires externally visible work such as active quotes.
type ShutdownPreparer interface {
	ShutdownPreparationTimeout() time.Duration
}

// Factory builds a Solver from its opaque config block (decoded by the solver into its own type)
// and the shared dependencies.
type Factory func(raw yaml.Node, deps Deps) (Solver, error)

// DecodeStrict decodes a deferred solvers[].config node into out, rejecting unknown keys so typos in
// a solver's config fail fast — matching the strict top-level config decode. The framework keeps
// each solver's config opaque (yaml.Node has no KnownFields option of its own), so solvers call this
// from parseConfig instead of node.Decode to get the same typo protection.
func DecodeStrict(node yaml.Node, out any) error {
	return parse.DecodeStrict(node, out)
}

var (
	mu       sync.RWMutex
	registry = map[string]Factory{}
)

// Register associates a solver name with its factory. Intended for use from package init
// functions; panics on duplicate names (a programming error).
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if name == "" {
		panic("solver: Register called with empty name")
	}
	if f == nil {
		panic("solver: Register called with nil factory for " + name)
	}
	if _, dup := registry[name]; dup {
		panic("solver: duplicate registration for " + name)
	}
	registry[name] = f
}

// New constructs the solver registered under name.
func New(name string, raw yaml.Node, deps Deps) (Solver, error) {
	mu.RLock()
	f, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, errors.Errorf("solver: unknown solver %q (registered: %v)", name, Registered())
	}
	return f(raw, deps)
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
