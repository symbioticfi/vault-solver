package observability_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
)

const parentTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func TestTraceHandlerContinuesInboundTrace(t *testing.T) {
	rec := tracetest.Install(t)
	var seen trace.SpanContext
	h := observability.TraceHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = trace.SpanContextFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}), func(r *http.Request) string {
		if r.URL.Path == "/quote" {
			return "/quote"
		}
		return "other"
	})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/quote", nil)
	req.Header.Set("traceparent", parentTraceparent)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status %d", rr.Code)
	}
	if seen.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("handler did not run under the inbound trace: %v", seen)
	}
	spans := rec.Ended()
	if len(spans) != 1 || spans[0].Name() != "POST /quote" {
		t.Fatalf("spans: %v", spans)
	}
	if spans[0].Parent().SpanID().String() != "00f067aa0ba902b7" {
		t.Fatalf("parent not the inbound span: %v", spans[0].Parent())
	}
}

func TestTraceHandlerSkipsProbes(t *testing.T) {
	rec := tracetest.Install(t)
	h := observability.TraceHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }), func(*http.Request) string { return "x" })
	for _, p := range []string{"/health", "/healthz", "/ready", "/readyz", "/metrics", "/openapi.json", "/docs"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, p, nil))
	}
	if n := len(rec.Ended()); n != 0 {
		t.Fatalf("expected no spans for probes, got %d", n)
	}
}

func TestTraceTransportInjectsTraceparent(t *testing.T) {
	rec := tracetest.Install(t)
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("traceparent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	client := &http.Client{Transport: observability.TraceTransport(nil, "rfq-backend")}
	ctx, span := otel.Tracer("x").Start(t.Context(), "parent")
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/api/v1/orders", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	span.End()
	if got == "" || got[3:35] != span.SpanContext().TraceID().String() {
		t.Fatalf("traceparent %q does not carry trace %s", got, span.SpanContext().TraceID())
	}
	var sawClientSpan bool
	for _, s := range rec.Ended() {
		if s.Name() == "rfq-backend POST" && s.SpanKind() == trace.SpanKindClient {
			sawClientSpan = true
			if tracetest.Attr(s, "peer.service") != "rfq-backend" {
				t.Fatalf("peer.service missing: %v", s.Attributes())
			}
		}
	}
	if !sawClientSpan {
		t.Fatalf("client span not recorded: %v", rec.Ended())
	}
}

// A webhook URL is operator-configured and may embed a token, so no query string may reach url.full
// — while the request on the wire keeps it.
func TestTraceTransportKeepsQueryOutOfSpanURL(t *testing.T) {
	rec := tracetest.Install(t)
	var gotTarget, gotBody, gotTraceparent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.URL.RequestURI()
		gotTraceparent = r.Header.Get("traceparent")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	client := &http.Client{Transport: observability.TraceTransport(nil, "webhook")}
	req, err := http.NewRequestWithContext(
		t.Context(), http.MethodPost, srv.URL+"/hook?token=secret", strings.NewReader(`{"ping":1}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if gotTarget != "/hook?token=secret" {
		t.Fatalf("server saw %q, want the query on the wire", gotTarget)
	}
	if gotBody != `{"ping":1}` {
		t.Fatalf("server saw body %q, want the request body intact", gotBody)
	}
	span := tracetest.Ended(t, rec, "webhook POST")
	if !strings.Contains(gotTraceparent, span.SpanContext().TraceID().String()) {
		t.Fatalf("traceparent %q does not carry the client span's trace", gotTraceparent)
	}
	full := tracetest.Attr(span, "url.full")
	if strings.Contains(full, "secret") || strings.Contains(full, "?") {
		t.Fatalf("url.full = %q, want it without the query string", full)
	}
	if full != srv.URL+"/hook" {
		t.Fatalf("url.full = %q, want %q", full, srv.URL+"/hook")
	}
}

// With tracing disabled the middleware is not installed at all, so an inbound traceparent is not
// continued and no span reaches the handler. That is the deliberate trade-off for a default
// deployment that pays nothing for tracing (docs/TRACING-PLAN.md section 3).
func TestTraceHandlerDisabledStartsNoSpan(t *testing.T) {
	requireW3CPropagator(t)
	var seen trace.SpanContext
	h := observability.TraceHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = trace.SpanContextFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}), func(*http.Request) string { return "/quote" })
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/quote", nil)
	req.Header.Set("traceparent", parentTraceparent)

	h.ServeHTTP(httptest.NewRecorder(), req)

	if seen.IsValid() {
		t.Fatalf("disabled handler put a span context on the request: %v", seen)
	}
}

// The outbound side of the same trade-off: no client span, and no traceparent on the wire.
func TestTraceTransportDisabledSendsNoTraceparent(t *testing.T) {
	requireW3CPropagator(t)
	var sent http.Header
	rt := observability.TraceTransport(roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		sent = req.Header.Clone()
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	}), "peer")

	resp, err := rt.RoundTrip(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://peer.invalid/v1/quote", nil))
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = resp.Body.Close()

	if got := sent.Get("traceparent"); got != "" {
		t.Fatalf("disabled transport sent traceparent %q", got)
	}
}

// requireW3CPropagator mirrors NewTracing, which installs the propagator even when it leaves tracing
// disabled: it is the middleware, not the propagator, that the disabled path drops.
func requireW3CPropagator(t *testing.T) {
	t.Helper()
	previous := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(previous) })
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// The default deployment runs with tracing off, so the middleware on the quote path and on every
// outbound client call must cost nothing there.
func BenchmarkTraceHandlerDisabled(b *testing.B) {
	h := observability.TraceHandler(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }),
		func(*http.Request) string { return "/quote" },
	)
	req := httptest.NewRequestWithContext(b.Context(), http.MethodPost, "/quote", nil)

	b.ReportAllocs()
	for b.Loop() {
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func BenchmarkTraceTransportDisabled(b *testing.B) {
	rt := observability.TraceTransport(roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	}), "peer")
	req := httptest.NewRequestWithContext(b.Context(), http.MethodGet, "http://peer.invalid/v1/quote?token=secret", nil)

	b.ReportAllocs()
	for b.Loop() {
		resp, err := rt.RoundTrip(req)
		if err != nil {
			b.Fatal(err)
		}
		_ = resp.Body.Close()
	}
}
