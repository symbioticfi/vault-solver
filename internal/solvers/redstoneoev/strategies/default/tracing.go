package defaultstrategy

import (
	"github.com/symbioticfi/vault-solver/internal/observability"
)

// newStrategyTracer names every default-strategy span. The owning solver is passed in rather than
// imported: this package sits below the solver package and importing it would cycle (spec §9).
func newStrategyTracer(solver string) *observability.Tracer {
	return observability.NewTracer(
		"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/default", solver,
	)
}
