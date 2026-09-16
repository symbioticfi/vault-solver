package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

const parentTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func TestTraceHandlerContinuesInboundTrace(t *testing.T) {
	rec := installRecorder(t)
	var seen trace.SpanContext
	h := TraceHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	rec := installRecorder(t)
	h := TraceHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }), func(*http.Request) string { return "x" })
	for _, p := range []string{"/health", "/healthz", "/ready", "/readyz", "/metrics", "/openapi.json", "/docs"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, p, nil))
	}
	if n := len(rec.Ended()); n != 0 {
		t.Fatalf("expected no spans for probes, got %d", n)
	}
}

func TestTraceTransportInjectsTraceparent(t *testing.T) {
	rec := installRecorder(t)
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("traceparent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	client := &http.Client{Transport: TraceTransport(nil, "rfq-backend")}
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
			if attr(s, "peer.service") != "rfq-backend" {
				t.Fatalf("peer.service missing: %v", s.Attributes())
			}
		}
	}
	if !sawClientSpan {
		t.Fatalf("client span not recorded: %v", rec.Ended())
	}
}
