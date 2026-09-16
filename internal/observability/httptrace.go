package observability

import (
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TraceHandler extracts W3C trace context from inbound requests and wraps next in a server span
// named "<METHOD> <route>". route must return a bounded label, never the raw path. Probe and docs
// paths produce no span.
func TraceHandler(next http.Handler, route func(*http.Request) string) http.Handler {
	return otelhttp.NewHandler(next, "http.server",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + route(r)
		}),
		otelhttp.WithFilter(func(r *http.Request) bool { return !isProbePath(r.URL.Path) }),
	)
}

func isProbePath(p string) bool {
	switch p {
	case "/health", "/healthz", "/ready", "/readyz", "/metrics", "/docs":
		return true
	}
	return strings.HasPrefix(p, "/openapi")
}

// TraceTransport wraps base (nil means http.DefaultTransport) so every request runs in a client span
// named "<peer> <METHOD>" and carries traceparent. peer is a short integration name, never a URL.
func TraceTransport(base http.RoundTripper, peer string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return otelhttp.NewTransport(base,
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return peer + " " + r.Method
		}),
		otelhttp.WithSpanOptions(trace.WithAttributes(attribute.String("peer.service", peer))),
	)
}
