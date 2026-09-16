package rfq

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr/funcr"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
)

const (
	inboundTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	inboundTraceID     = "4bf92f3577b34da6a3ce929d0e0e4736"
)

// spanRecorder is the slice of tracetest.SpanRecorder these assertions need.
type spanRecorder interface {
	Ended() []sdktrace.ReadOnlySpan
}

func endedSpan(t *testing.T, rec spanRecorder, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, s := range rec.Ended() {
		if s.Name() == name {
			return s
		}
	}
	t.Fatalf("span %q not ended; ended spans: %v", name, spanNames(rec))
	return nil
}

func spanNames(rec spanRecorder) []string {
	ended := rec.Ended()
	out := make([]string, 0, len(ended))
	for _, s := range ended {
		out = append(out, s.Name())
	}
	return out
}

func attr(s sdktrace.ReadOnlySpan, key string) string {
	for _, kv := range s.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.String()
		}
	}
	return ""
}

func requireSpans(t *testing.T, rec spanRecorder, want ...string) {
	t.Helper()
	got := make(map[string]bool, len(rec.Ended()))
	for _, name := range spanNames(rec) {
		got[name] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Fatalf("missing span %q; ended spans: %v", name, spanNames(rec))
		}
	}
}

func postQuote(t *testing.T, h http.Handler, body quoteRequest, traceparent string) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/quote", bytes.NewReader(encoded))
	req.Header.Set(sharedSecretHeader, testSecret)
	if traceparent != "" {
		req.Header.Set("traceparent", traceparent)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// TestServer_QuoteTracing pins the inbound quote trace: the server span continues the backend's
// trace, every stage span belongs to it, the identifiers are searchable, and the quote's span
// context is remembered so the fill that wins it can link back (spec §9.4, §12).
func TestServer_QuoteTracing(t *testing.T) {
	rec := tracetest.Install(t)
	srv := testServer()
	body := validQuoteBody()

	rr := postQuote(t, srv.handler(), body, inboundTraceparent)
	if rr.Code != http.StatusOK {
		t.Fatalf("quote = %d, want 200 (body %s)", rr.Code, rr.Body.String())
	}

	for _, s := range rec.Ended() {
		if got := s.SpanContext().TraceID().String(); got != inboundTraceID {
			t.Fatalf("span %s is in trace %s, want the inbound trace %s", s.Name(), got, inboundTraceID)
		}
	}
	requireSpans(t, rec, "POST /quote", "rfq.quote", "rfq.quote.snapshot", "rfq.quote.decide")

	server := endedSpan(t, rec, "POST /quote")
	if got := attr(server, "quote.id"); got != body.QuoteID {
		t.Fatalf("server span quote.id = %q, want %q", got, body.QuoteID)
	}
	if attr(server, "request.id") == "" {
		t.Fatalf("server span is missing request.id: %v", server.Attributes())
	}
	if got := attr(endedSpan(t, rec, "rfq.quote.decide"), "strategy.name"); got != defaultStrategyName {
		t.Fatalf("decide span strategy.name = %q, want %q", got, defaultStrategyName)
	}
	if got := attr(endedSpan(t, rec, "rfq.quote"), "adapter.address"); got != vlt.Hex() {
		t.Fatalf("quote span adapter.address = %q, want %q", got, vlt.Hex())
	}
	if _, ok := srv.links.Lookup(body.QuoteID); !ok {
		t.Fatal("quote span context not remembered for linking")
	}
}

// panicStrategy stands in for a decider that blows up mid-request — the webhook strategy calls out
// over HTTP, so this is reachable in production and is recovered by the innermost middleware.
type panicStrategy struct{}

func (panicStrategy) DecideQuote(context.Context, types.QuoteInput) (types.QuoteOutput, error) {
	panic("strategy exploded")
}

func (panicStrategy) BuildFillPlan(context.Context, types.FillInput) (*types.FillPlan, error) {
	return nil, nil
}

// A recovered panic must not leak spans: recoverPanics completes the request, so any span left open
// by the unwound stack is never exported. Every quote span has to end on that path too.
func TestServer_QuoteTracingEndsSpansOnPanic(t *testing.T) {
	rec := tracetest.Install(t)
	srv := testServer()
	srv.quotes.strategy = panicStrategy{}

	rr := postQuote(t, srv.handler(), validQuoteBody(), inboundTraceparent)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("panicking quote = %d, want 500 (body %s)", rr.Code, rr.Body.String())
	}
	requireSpans(t, rec, "POST /quote", "rfq.quote", "rfq.quote.decide")
}

// A well-formed request this filler cannot quote is not a failure: the span declines instead of
// erroring, and nothing is remembered for a quote that was never served.
func TestServer_QuoteTracingDeclines(t *testing.T) {
	rec := tracetest.Install(t)
	srv := testServer()
	body := validQuoteBody()
	body.TokenInChainID = 2 // not our chain

	if rr := postQuote(t, srv.handler(), body, inboundTraceparent); rr.Code != http.StatusNoContent {
		t.Fatalf("wrong-chain quote = %d, want 204", rr.Code)
	}

	span := endedSpan(t, rec, "rfq.quote")
	if span.Status().Code == codes.Error {
		t.Fatalf("declined quote must not be an error span: %v", span.Status())
	}
	events := span.Events()
	if len(events) != 1 || events[0].Name != "declined" {
		t.Fatalf("events = %v, want one declined event", events)
	}
	if _, ok := srv.links.Lookup(body.QuoteID); ok {
		t.Fatal("a quote that was never served must not be remembered")
	}
}

// TestExecution_OrderTraceLinksToQuote pins the fill trace and its best-effort link back to the
// quote that produced the order (spec §12).
func TestExecution_OrderTraceLinksToQuote(t *testing.T) {
	rec := tracetest.Install(t)
	st, be := fillFixtures(t)
	txm := &fakeTxm{result: confirmedTxResult()}
	e := newExec(t, st, be, txm)

	// Stand in for handleQuote having served quote q1 earlier, in its own trace.
	quoteCtx, endQuote := tracer.Start(t.Context(), "rfq.quote")
	e.links.Remember(quoteCtx, "q1", time.Minute)
	endQuote(nil)
	quoteTraceID := trace.SpanContextFromContext(quoteCtx).TraceID().String()

	e.syncOnce(t.Context())

	requireSpans(t, rec,
		"rfq.execution.sync", "rfq.execution.poll", "rfq.order",
		"rfq.order.resolve", "rfq.order.plan", "rfq.order.build", "rfq.order.submit", "rfq.order.report",
	)
	order := endedSpan(t, rec, "rfq.order")
	if len(order.Links()) != 1 || order.Links()[0].SpanContext.TraceID().String() != quoteTraceID {
		t.Fatalf("order span links = %v, want one link to trace %s", order.Links(), quoteTraceID)
	}
	if got := attr(order, "order.id"); got != "o1" {
		t.Fatalf("order span order.id = %q, want o1", got)
	}
	if got := attr(order, "quote.id"); got != "q1" {
		t.Fatalf("order span quote.id = %q, want q1", got)
	}
	if got := attr(order, "quote.trace_id"); got != quoteTraceID {
		t.Fatalf("order span quote.trace_id = %q, want %s", got, quoteTraceID)
	}
	wantHash := confirmedTxResult().Hash.Hex()
	if got := attr(order, "tx.hash"); got != wantHash {
		t.Fatalf("order span tx.hash = %q, want %s", got, wantHash)
	}
	submit := endedSpan(t, rec, "rfq.order.submit")
	if got := attr(submit, "tx.hash"); got != wantHash {
		t.Fatalf("submit span tx.hash = %q, want %s", got, wantHash)
	}
	if got := attr(submit, "tx.outcome"); got != "confirmed" {
		t.Fatalf("submit span tx.outcome = %q, want confirmed", got)
	}
	if got := attr(endedSpan(t, rec, "rfq.order.plan"), "strategy.name"); got != defaultStrategyName {
		t.Fatalf("plan span strategy.name = %q, want %q", got, defaultStrategyName)
	}
}

// A quote nobody remembered (restart, eviction, tracing off) costs the link and nothing else: the
// order still fills, and the miss is named on the span (spec §12).
func TestExecution_OrderTraceRecordsLinkMiss(t *testing.T) {
	rec := tracetest.Install(t)
	st, be := fillFixtures(t)
	txm := &fakeTxm{result: confirmedTxResult()}
	e := newExec(t, st, be, txm)

	e.syncOnce(t.Context())

	order := endedSpan(t, rec, "rfq.order")
	if len(order.Links()) != 0 {
		t.Fatalf("order span links = %v, want none", order.Links())
	}
	var misses int
	for _, event := range order.Events() {
		if event.Name != "link_miss" {
			continue
		}
		misses++
		var key string
		for _, kv := range event.Attributes {
			if kv.Key == "key" {
				key = kv.Value.AsString()
			}
		}
		if key != "q1" {
			t.Fatalf("link_miss key = %q, want q1", key)
		}
	}
	if misses != 1 {
		t.Fatalf("link_miss events = %d, want 1 (events %v)", misses, order.Events())
	}
	if txm.calls != 1 {
		t.Fatalf("txm sends = %d, want the fill to proceed unlinked", txm.calls)
	}
	if rec := st.order("o1"); rec == nil || rec.Status != statusFilled {
		t.Fatalf("status = %v, want filled", rec)
	}
}

// A fill we refuse to send — here the backend resolved a discount to an adapter the plan never
// quoted — is an expected skip, so it declines on the order span instead of erroring the trace.
func TestExecution_OrderTraceDeclinesRefusedFill(t *testing.T) {
	rec := tracetest.Install(t)
	st, be := fillFixtures(t)
	h := common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000000ab")
	be.discount = &resolveDiscountResponse{
		DiscountID: h.Hex(),
		Discount: discountTerms{
			Adapter:       "0x00000000000000000000000000000000000000aa", // not the quoted leg's adapter
			TokenToRedeem: tIn.Hex(), Discount: "500",
			Signer:   "0x00000000000000000000000000000000000000a1",
			Protocol: "0x00000000000000000000000000000000000000a2",
			Nonce:    "1", Deadline: 4_102_444_800,
		},
		SignerSignature: "0xaa", ProtocolDeadline: 4_102_444_800, ProtocolSignature: "0xbb",
	}
	txm := &fakeTxm{result: confirmedTxResult()}
	e := newExec(t, st, be, txm)
	e.strategy = fixedFillStrategy{plan: discountFillPlan(h)}

	e.syncOnce(t.Context())

	if txm.calls != 0 {
		t.Fatalf("txm sends = %d, want none for a refused fill", txm.calls)
	}
	order := endedSpan(t, rec, "rfq.order")
	if order.Status().Code == codes.Error {
		t.Fatalf("a refused fill must not be an error span: %v", order.Status())
	}
	if !hasEvent(order, "declined") {
		t.Fatalf("order span has no declined event: %v", order.Events())
	}
	if build := endedSpan(t, rec, "rfq.order.build"); build.Status().Code == codes.Error {
		t.Fatalf("build span must not be an error: %v", build.Status())
	}
}

func hasEvent(s sdktrace.ReadOnlySpan, name string) bool {
	for _, event := range s.Events() {
		if event.Name == name {
			return true
		}
	}
	return false
}

// Trace loggers are derived from the base logger at each span-starting site, never from an
// already-derived one: re-deriving appends a second trace_id/span_id pair to every line.
func TestExecution_OrderLogsCarryTraceIDOnce(t *testing.T) {
	tracetest.Install(t)
	st, be := fillFixtures(t)
	var lines []string
	e := newExec(t, st, be, &fakeTxm{result: confirmedTxResult()})
	e.log = funcr.NewJSON(func(entry string) { lines = append(lines, entry) }, funcr.Options{Verbosity: 1})

	e.syncOnce(t.Context())

	if len(lines) == 0 {
		t.Fatal("no log output captured")
	}
	var sawOrderLine bool
	for _, line := range lines {
		if n := strings.Count(line, `"trace_id"`); n > 1 {
			t.Fatalf("trace_id appears %d times in %s", n, line)
		}
		if n := strings.Count(line, `"span_id"`); n > 1 {
			t.Fatalf("span_id appears %d times in %s", n, line)
		}
		if strings.Contains(line, `"orderId"`) && strings.Contains(line, `"trace_id"`) {
			sawOrderLine = true
		}
	}
	if !sawOrderLine {
		t.Fatalf("no order-path line carried trace_id: %v", lines)
	}
}

// Tracing is off by default: no provider is installed here, so every span is a no-op and the fill
// must behave exactly as it does without tracing.
func TestExecution_FillsWithTracingDisabled(t *testing.T) {
	st, be := fillFixtures(t)
	txm := &fakeTxm{result: confirmedTxResult()}
	e := newExec(t, st, be, txm)

	e.syncOnce(t.Context())

	if rec := st.order("o1"); rec == nil || rec.Status != statusFilled {
		t.Fatalf("status = %v, want filled", rec)
	}
}
