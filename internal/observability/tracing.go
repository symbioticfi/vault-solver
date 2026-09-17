package observability

import (
	"context"
	"os"
	"strings"
	"sync/atomic"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

// Tracing describes the process for the OpenTelemetry resource.
type Tracing struct {
	Name    string // service.name default; OTEL_SERVICE_NAME overrides
	Version string
	Commit  string
	Solvers []string
	ChainID uint64
}

const defaultServiceName = "vault-solver"

// enabled reports whether a real TracerProvider is installed. The default deployment runs with
// tracing off and must pay nothing for it, so while this is false spans are not started at all and
// the HTTP middleware is not installed. The cost is that a disabled process no longer continues an
// inbound traceparent or stamps trace ids on its logs (see docs/TRACING-PLAN.md §3).
var enabled atomic.Bool

// SetEnabled records whether tracing is on and returns the previous value. NewTracing sets it
// alongside the provider it installs; the tracetest helper sets it around a recording provider and
// restores it on cleanup.
func SetEnabled(on bool) bool { return enabled.Swap(on) }

// TracingEnabled reports whether tracing is on, for the few call sites that would otherwise build
// tracing-only values before any span exists.
func TracingEnabled() bool { return enabled.Load() }

// NewTracing installs the W3C propagator and, when OTEL_EXPORTER_ENABLED is truthy, an OTLP/HTTP
// exporting TracerProvider configured from the standard OTEL_* environment. It never fails startup:
// a setup error is logged and tracing stays disabled. The returned shutdown flushes pending spans.
func NewTracing(ctx context.Context, info Tracing, log logr.Logger) (func(context.Context) error, bool) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	noop := func(context.Context) error { return nil }
	if !tracingEnabled(os.Getenv("OTEL_EXPORTER_ENABLED")) {
		return noop, false
	}
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		log.Error(err, "tracing disabled: cannot build OTLP exporter")
		return noop, false
	}
	name := info.Name
	if name == "" {
		name = defaultServiceName
	}
	res, err := resource.New(ctx,
		resource.WithTelemetrySDK(), // telemetry.sdk.*; the option sets around it are schemaless
		resource.WithAttributes(
			semconv.ServiceName(name),
			semconv.ServiceVersion(info.Version),
			attribute.String("vault_solver.commit", info.Commit),
			attribute.String("vault_solver.solvers", strings.Join(info.Solvers, ",")),
			attribute.Int64("chain.id", int64(info.ChainID)),
		),
		resource.WithFromEnv(), // OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES win over the defaults above
	)
	if err != nil {
		log.Error(err, "tracing disabled: cannot build resource")
		return noop, false
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(provider)
	InvalidateTracers()
	SetEnabled(true)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Info("tracing export error", "err", err.Error())
	}))
	return provider.Shutdown, true
}

// tracingEnabled mirrors @symbiotic/backend-devkit: only these values switch tracing on.
func tracingEnabled(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

// TraceLogger stamps trace_id and span_id on log when ctx carries a valid span context.
func TraceLogger(ctx context.Context, log logr.Logger) logr.Logger {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return log
	}
	return stampLogger(log, sc)
}
