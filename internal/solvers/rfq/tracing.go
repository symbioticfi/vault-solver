package rfq

import (
	"context"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// tracer names every RFQ span and stamps solver=rfq-filler on it (spec §9).
var tracer = observability.NewTracer("github.com/symbioticfi/vault-solver/internal/solvers/rfq", Name)

// traceAdapter records the adapter a plan fills through, when every leg shares one. A multi-adapter
// plan has no single address to report, so the attribute is left off rather than made ambiguous.
func traceAdapter(ctx context.Context, legs []fillLeg) {
	if len(legs) == 0 {
		return
	}
	adapter := legs[0].Adapter
	for _, leg := range legs[1:] {
		if leg.Adapter != adapter {
			return
		}
	}
	observability.SetAttributes(ctx, observability.AttrAdapter.String(adapter.Hex()))
}
