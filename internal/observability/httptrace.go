package observability

import (
	"context"
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
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
// The recorded url.full never carries the query string (see redactURL).
func TraceTransport(base http.RoundTripper, peer string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return otelhttp.NewTransport(redactURL{base: base},
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return peer + " " + r.Method
		}),
		otelhttp.WithSpanOptions(trace.WithAttributes(attribute.String("peer.service", peer))),
	)
}

// redactURL sits below otelhttp, which records the request URL as url.full and offers no option to
// omit the query string: an operator-configured peer URL (the webhook strategy's) may embed a token.
// otelhttp has already set the attribute by the time it calls down, so overwriting it here leaves
// the request that goes on the wire untouched.
type redactURL struct{ base http.RoundTripper }

func (t redactURL) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL != nil && req.URL.RawQuery != "" {
		redacted := *req.URL
		redacted.RawQuery = ""
		redacted.User = nil
		trace.SpanFromContext(req.Context()).
			SetAttributes(attribute.String("url.full", redacted.String()))
	}
	return t.base.RoundTrip(req)
}

// InjectTraceHeaders writes ctx's W3C trace context into h, so a connection that is not an
// http.Client request (a websocket handshake, an RPC dial) still continues the trace.
func InjectTraceHeaders(ctx context.Context, h http.Header) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(h))
}
