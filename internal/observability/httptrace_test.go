package observability_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
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
