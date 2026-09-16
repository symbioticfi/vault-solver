package uniswapx

import (
	"net/http"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// tracer names every UniswapX span and stamps solver=uniswapx-filler on it (spec §9).
var tracer = observability.NewTracer("github.com/symbioticfi/vault-solver/internal/solvers/uniswapx", Name)

// quoteLinkTTL bounds how long a served quote's span context stays linkable from the fill that wins
// it. UniswapX quotes carry no validity on our side, so the TTL only bounds the map (spec §12).
const quoteLinkTTL = 10 * time.Minute

// uniswapxRoute labels the server span. Bounded by construction: anything but the two real routes
// is "other", and probe paths never reach it (TraceHandler filters them).
func uniswapxRoute(r *http.Request) string {
	switch r.URL.Path {
	case "/quote", "/ready":
		return r.URL.Path
	}
	return "other"
}

// quoteLink returns the span of the quote this order came from, when it is still remembered. A miss
// is an ordinary result: linking is best effort and never changes what the fill does (spec §12).
func (s *Solver) quoteLink(quoteID string) (trace.Link, bool) {
	if s.links == nil {
		return trace.Link{}, false
	}
	return s.links.Lookup(quoteID)
}
