package observability

import (
	"context"
	"net/http"
	"net/url"
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
// The recorded url.full never carries the query string (see stripQuery).
func TraceTransport(base http.RoundTripper, peer string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return stripQuery{next: otelhttp.NewTransport(restoreQuery{base: base},
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return peer + " " + r.Method
		}),
		otelhttp.WithSpanOptions(trace.WithAttributes(attribute.String("peer.service", peer))),
	)}
}

// strippedQuery carries the request URL past otelhttp, which clones the request before handing it to
// the transport below it.
type strippedQuery struct{}

// stripQuery hides the query string from otelhttp, which records the request URL as url.full and
// offers no option to omit it: an operator-configured peer URL (the webhook strategy's) may embed a
// token. Userinfo needs no such care — otelhttp already strips that from url.full.
type stripQuery struct{ next http.RoundTripper }

func (t stripQuery) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil || req.URL.RawQuery == "" {
		return t.next.RoundTrip(req)
	}
	stripped := req.Clone(context.WithValue(req.Context(), strippedQuery{}, req.URL))
	stripped.URL.RawQuery = ""
	return t.next.RoundTrip(stripped)
}

// restoreQuery puts the real URL back on the request that goes on the wire, keeping everything
// otelhttp set on it (traceparent, the body wrapper it measures).
type restoreQuery struct{ base http.RoundTripper }

func (t restoreQuery) RoundTrip(req *http.Request) (*http.Response, error) {
	original, ok := req.Context().Value(strippedQuery{}).(*url.URL)
	if !ok {
		return t.base.RoundTrip(req)
	}
	restored := req.Clone(req.Context())
	restored.URL = original
	return t.base.RoundTrip(restored)
}
