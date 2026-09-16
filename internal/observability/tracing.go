package observability

import (
	"context"
	"os"
	"strings"

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
	return log.WithValues("trace_id", sc.TraceID().String(), "span_id", sc.SpanID().String())
}
