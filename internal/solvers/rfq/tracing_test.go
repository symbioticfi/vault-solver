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
	"github.com/go-errors/errors"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

const (
	inboundTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	inboundTraceID     = "4bf92f3577b34da6a3ce929d0e0e4736"
)

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
	tracetest.RequireSpans(t, rec, "POST /quote", "rfq.quote", "rfq.quote.snapshot", "rfq.quote.decide")

	server := tracetest.Ended(t, rec, "POST /quote")
	if got := tracetest.Attr(server, "quote.id"); got != body.QuoteID {
		t.Fatalf("server span quote.id = %q, want %q", got, body.QuoteID)
	}
	if tracetest.Attr(server, "request.id") == "" {
		t.Fatalf("server span is missing request.id: %v", server.Attributes())
	}
	if got := tracetest.Attr(tracetest.Ended(t, rec, "rfq.quote.decide"), "strategy.name"); got != defaultStrategyName {
		t.Fatalf("decide span strategy.name = %q, want %q", got, defaultStrategyName)
	}
	if got := tracetest.Attr(tracetest.Ended(t, rec, "rfq.quote"), "adapter.address"); got != vlt.Hex() {
		t.Fatalf("quote span adapter.address = %q, want %q", got, vlt.Hex())
	}
	if _, ok := srv.links.Lookup(body.QuoteID); !ok {
		t.Fatal("quote span context not remembered for linking")
	}
}

// The access log is the natural join key for an inbound request, so it has to carry the server
// span's ids. Run puts the solver logger on the quote server's BaseContext; stand in for that here.
func TestServer_AccessLogCarriesTraceIDOnce(t *testing.T) {
	tracetest.Install(t)
	srv := testServer()
	log, capture := tracetest.CaptureLogs(t, 0)

	encoded, err := json.Marshal(validQuoteBody())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequestWithContext(
		observability.WithLogger(t.Context(), log), http.MethodPost, "/quote", bytes.NewReader(encoded),
	)
	req.Header.Set(sharedSecretHeader, testSecret)
	req.Header.Set("traceparent", inboundTraceparent)
	srv.handler().ServeHTTP(httptest.NewRecorder(), req)

	lines := capture()
	tracetest.RequireTraceIDsOnce(t, lines)
	var sawRequestLine bool
	for _, line := range lines {
		if !strings.Contains(line, `"msg":"request"`) {
			continue
		}
		sawRequestLine = true
		if !strings.Contains(line, `"trace_id":"`+inboundTraceID+`"`) {
			t.Fatalf("access log line carries no inbound trace_id: %s", line)
		}
	}
	if !sawRequestLine {
		t.Fatalf("no access log line was written: %v", lines)
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
	tracetest.RequireSpans(t, rec, "POST /quote", "rfq.quote", "rfq.quote.decide")
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

	span := tracetest.Ended(t, rec, "rfq.quote")
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

	tracetest.RequireSpans(t, rec,
		"rfq.execution.sync", "rfq.execution.poll", "rfq.order",
		"rfq.order.resolve", "rfq.order.plan", "rfq.order.build", "rfq.order.submit", "rfq.order.report",
	)
	order := tracetest.Ended(t, rec, "rfq.order")
	if len(order.Links()) != 1 || order.Links()[0].SpanContext.TraceID().String() != quoteTraceID {
		t.Fatalf("order span links = %v, want one link to trace %s", order.Links(), quoteTraceID)
	}
	if got := tracetest.Attr(order, "order.id"); got != "o1" {
		t.Fatalf("order span order.id = %q, want o1", got)
	}
	if got := tracetest.Attr(order, "quote.id"); got != "q1" {
		t.Fatalf("order span quote.id = %q, want q1", got)
	}
	if got := tracetest.Attr(order, "quote.trace_id"); got != quoteTraceID {
		t.Fatalf("order span quote.trace_id = %q, want %s", got, quoteTraceID)
	}
	wantHash := confirmedTxResult().Hash.Hex()
	if got := tracetest.Attr(order, "tx.hash"); got != wantHash {
		t.Fatalf("order span tx.hash = %q, want %s", got, wantHash)
	}
	submit := tracetest.Ended(t, rec, "rfq.order.submit")
	if got := tracetest.Attr(submit, "tx.hash"); got != wantHash {
		t.Fatalf("submit span tx.hash = %q, want %s", got, wantHash)
	}
	if got := tracetest.Attr(submit, "tx.outcome"); got != "confirmed" {
		t.Fatalf("submit span tx.outcome = %q, want confirmed", got)
	}
	if got := tracetest.Attr(tracetest.Ended(t, rec, "rfq.order.plan"), "strategy.name"); got != defaultStrategyName {
		t.Fatalf("plan span strategy.name = %q, want %q", got, defaultStrategyName)
	}
}

// A submission the manager rejected before broadcasting has no transaction: the outcome is recorded,
// tx.hash is left off rather than stamped as the zero hash.
func TestExecution_SubmissionErrorOmitsTxHash(t *testing.T) {
	rec := tracetest.Install(t)
	st, be := fillFixtures(t)
	txm := &fakeTxm{result: txmanager.Result{
		Outcome: txmanager.OutcomeSubmissionError,
		Err:     errors.New("insufficient funds for gas * price + value"),
	}}
	e := newExec(t, st, be, txm)

	e.syncOnce(t.Context())

	if txm.calls != 1 {
		t.Fatalf("txm sends = %d, want 1", txm.calls)
	}
	for _, name := range []string{"rfq.order", "rfq.order.submit"} {
		span := tracetest.Ended(t, rec, name)
		tracetest.RequireNoAttr(t, span, "tx.hash") // a transaction that never went out has none
		tracetest.RequireAttr(t, span, "tx.outcome", string(txmanager.OutcomeSubmissionError))
	}
	if got := tracetest.Ended(t, rec, "rfq.order.submit").Status().Code; got != codes.Error {
		t.Fatalf("submit span status = %v, want Error", got)
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

	order := tracetest.Ended(t, rec, "rfq.order")
	if len(order.Links()) != 0 {
		t.Fatalf("order span links = %v, want none", order.Links())
	}
	if key, misses := tracetest.EventAttr(order, "link_miss", "key"); misses != 1 || key != "q1" {
		t.Fatalf("link_miss events = %d with key %q, want 1 with q1", misses, key)
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
	order := tracetest.Ended(t, rec, "rfq.order")
	if order.Status().Code == codes.Error {
		t.Fatalf("a refused fill must not be an error span: %v", order.Status())
	}
	if !tracetest.HasEvent(order, "declined") {
		t.Fatalf("order span has no declined event: %v", order.Events())
	}
	if build := tracetest.Ended(t, rec, "rfq.order.build"); build.Status().Code == codes.Error {
		t.Fatalf("build span must not be an error: %v", build.Status())
	}
}

// Trace loggers are derived from the base logger at each span-starting site, never from an
// already-derived one: re-deriving appends a second trace_id/span_id pair to every line.
func TestExecution_OrderLogsCarryTraceIDOnce(t *testing.T) {
	tracetest.Install(t)
	st, be := fillFixtures(t)
	e := newExec(t, st, be, &fakeTxm{result: confirmedTxResult()})
	log, capture := tracetest.CaptureLogs(t, 1)
	e.log = log

	// solver.Run stores the solver logger on the context it hands the poll loop; stand in for it.
	e.syncOnce(observability.WithLogger(t.Context(), e.log))

	lines := capture()
	if len(lines) == 0 {
		t.Fatal("no log output captured")
	}
	tracetest.RequireTraceIDsOnce(t, lines)
	var sawOrderLine bool
	for _, line := range lines {
		if strings.Contains(line, `"orderId"`) && strings.Contains(line, `"trace_id"`) {
			sawOrderLine = true
		}
	}
	if !sawOrderLine {
		t.Fatalf("no order-path line carried trace_id: %v", lines)
	}
}

// The quote pipeline logs through the context logger now, so its decline lines carry the rfq.quote
// span's ids — and carry them exactly once.
func TestQuote_DeclineLogsCarryTraceIDOnce(t *testing.T) {
	tracetest.Install(t)
	srv := testServer()
	log, capture := tracetest.CaptureLogs(t, 1)
	srv.quotes.log = log
	body := validQuoteBody()
	body.TokenInChainID = 2 // not our chain

	if _, err := srv.quotes.quote(t.Context(), &body); err != nil {
		t.Fatalf("quote: %v", err)
	}

	lines := capture()
	tracetest.RequireTraceIDsOnce(t, lines)
	var sawDecline bool
	for _, line := range lines {
		if strings.Contains(line, "declining quote: not quotable") && strings.Contains(line, `"trace_id"`) {
			sawDecline = true
		}
	}
	if !sawDecline {
		t.Fatalf("the quote decline line carried no trace_id: %v", lines)
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
