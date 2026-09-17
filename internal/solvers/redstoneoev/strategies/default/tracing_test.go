package defaultstrategy

import (
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

const tracingSolverName = "redstone-oev"

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
		t.Fatalf("oev.monitor span not ended; ended spans: %v", tracetest.Names(rec))
	}
	if tick.Parent().IsValid() {
		t.Fatalf("oev.monitor parent = %s, want a root span", tick.Parent().SpanID())
	}
	if got := tracetest.Attr(tick, "solver"); got != tracingSolverName {
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
		t.Fatalf("no Morpho GraphQL span nested under the tick; ended spans: %v", tracetest.Names(rec))
	}
}

// The economic skip lines run inside their own stage span, so they carry that stage's trace ids —
// once, because the context holds the base logger and Log stamps at retrieval.
func TestBundleSkipLogsCarryTraceIDOnce(t *testing.T) {
	tracetest.Install(t)
	var lines []string
	s := &Strategy{
		log:    funcr.NewJSON(func(entry string) { lines = append(lines, entry) }, funcr.Options{Verbosity: 1}),
		tracer: newStrategyTracer(tracingSolverName),
	}
	// Stands in for DecideBid, which stores the strategy logger on the context it passes down.
	ctx := observability.WithLogger(t.Context(), s.log)

	skip := s.affordableBundle(ctx, types.BidInput{
		Auction: types.AuctionSnapshot{ID: "auction-1"},
		Context: types.BidContext{
			ExecutorDeposit:    big.NewInt(1_000_000),
			ExecutorMinDeposit: big.NewInt(0),
		},
	}, pricedBundle{gasNative: big.NewInt(1), bidNative: big.NewInt(5)},
		decisionState{CallbackNative: big.NewInt(1)}, big.NewInt(1))

	if skip != types.SkipReasonCallbackBalance {
		t.Fatalf("skip = %q, want %q", skip, types.SkipReasonCallbackBalance)
	}
	var skipped string
	for _, line := range lines {
		if strings.Contains(line, `"bid skipped: callback balance cannot cover bid"`) {
			skipped = line
		}
	}
	if skipped == "" {
		t.Fatalf("no skip line was logged: %v", lines)
	}
	if n := strings.Count(skipped, `"trace_id"`); n != 1 {
		t.Fatalf("trace_id appears %d times in %s, want once", n, skipped)
	}
	if n := strings.Count(skipped, `"span_id"`); n != 1 {
		t.Fatalf("span_id appears %d times in %s, want once", n, skipped)
	}
}
