package defaultstrategy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

const tracingSolverName = "redstone-oev"

func spanNames(ended []sdktrace.ReadOnlySpan) []string {
	out := make([]string, 0, len(ended))
	for _, s := range ended {
		out = append(out, s.Name())
	}
	return out
}

func spanAttr(s sdktrace.ReadOnlySpan, key string) string {
	for _, kv := range s.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.String()
		}
	}
	return ""
}

// Each monitor tick roots its own trace, so the Morpho GraphQL calls it makes nest under it (spec §9.4).
func TestMonitorTickRootsTraceWithAPICalls(t *testing.T) {
	rec := tracetest.Install(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"markets":{"items":[]}}}`))
	}))
	defer srv.Close()

	loan := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	collateral := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	monitor := newAPIMonitor(logr.Discard(), Config{MorphoAPIURL: srv.URL}, 1, func() (types.AdapterSnapshot, bool) {
		return types.AdapterSnapshot{
			Loan:       loan,
			Redeemable: []types.RedeemableSnapshot{{Asset: collateral}},
		}, true
	}, newStrategyTracer(tracingSolverName))

	monitor.refresh(t.Context())

	ended := rec.Ended()
	var tick sdktrace.ReadOnlySpan
	for _, span := range ended {
		if span.Name() == "oev.monitor" {
			tick = span
		}
	}
	if tick == nil {
		t.Fatalf("oev.monitor span not ended; ended spans: %v", spanNames(ended))
	}
	if tick.Parent().IsValid() {
		t.Fatalf("oev.monitor parent = %s, want a root span", tick.Parent().SpanID())
	}
	if got := spanAttr(tick, "solver"); got != tracingSolverName {
		t.Fatalf("solver = %q, want %q", got, tracingSolverName)
	}

	nested := false
	for _, span := range ended {
		if strings.HasPrefix(span.Name(), "morpho-graphql ") &&
			span.Parent().SpanID() == tick.SpanContext().SpanID() {
			nested = true
		}
	}
	if !nested {
		t.Fatalf("no Morpho GraphQL span nested under the tick; ended spans: %v", spanNames(ended))
	}
}
