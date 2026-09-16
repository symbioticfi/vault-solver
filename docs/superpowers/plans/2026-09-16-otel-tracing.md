# OpenTelemetry Tracing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every inbound request, outbound HTTP call, JSON-RPC call, websocket message, pipeline stage, and transaction in vault-solver is a span in one W3C-propagated trace, with identifiers as attributes, errors recorded per stage, trace ids on logs, and best-effort quote-to-fill links.

**Architecture:** A thin `internal/observability` layer (SDK bootstrap from `OTEL_*` env, a `Tracer` wrapper that stamps `solver` and records errors, `otelhttp` handler/transport wrappers, `TraceLogger`, a bounded `SpanLinks` map) is wired once in `cmd/vault-solver/run.go`. The RPC fallback transport and txmanager get spans at their existing chokepoints. Each solver then adds stage spans at the altitude of its service functions and remembers quote span contexts for later links.

**Tech Stack:** Go 1.27 (`GOTOOLCHAIN=go1.27.1`), `go.opentelemetry.io/otel` v1.46.0 (api, sdk, otlptracehttp), `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` v0.71.0, `tracetest.SpanRecorder` for tests.

**Spec:** `docs/superpowers/specs/2026-09-16-otel-tracing-design.md` — read it first; every task cites its sections.

## Global Constraints

- Toolchain: `GOTOOLCHAIN=go1.27.1`. Gate for every task: `make format && make test && make lint && GOTOOLCHAIN=go1.27.1 go build ./...` must be green before committing.
- Errors: `github.com/go-errors/errors` only (`errors.Errorf("...: %w", err)`, `errors.New`); never `fmt.Errorf` (forbidigo).
- Logging: `logr.Logger` by value; `Error` only for pageable conditions, expected skips at `V(1)`.
- Generated code under `api/**` is never edited.
- Commits: `multigit commit -s -m "type(scope): summary"` (Conventional Commits, imperative, lowercase, signed off). No AI attribution lines of any kind.
- Enablement is only `OTEL_EXPORTER_ENABLED` (values `1`,`true`,`yes`,`on`,`enabled`); all other settings come from standard `OTEL_*` env read by the SDK. No YAML changes.
- Span names are bounded (route allowlists, RPC method allowlist, fixed stage names). Identifier attributes use the neutral keys from spec §9.3 via the `observability.Attr*` constants. Never put URLs, keys, signatures, or calldata in attributes.
- Tracing never fails startup, never blocks a hot path, never errors into solver code (spec §13). `SpanLinks.Lookup` returns `(link, ok)`; a miss records a `link_miss` event and continues.
- Sentry sink promotes `trace_id` to a tag (spec §5).

---

## File map

| File | Responsibility |
|---|---|
| `internal/observability/tracing.go` (new) | `Tracing`, `NewTracing`, `tracingEnabled`, `TraceLogger` |
| `internal/observability/tracer.go` (new) | `Tracer`, `NewTracer`, `Start`, `StartLinked`, `EndFunc`, `Decline`, `SetAttributes`, `Attr*` keys |
| `internal/observability/httptrace.go` (new) | `TraceHandler`, `TraceTransport`, probe filter |
| `internal/observability/spanlinks.go` (new) | `SpanLinks` bounded TTL map |
| `internal/observability/tracetest.go` (new, `_test` helper in package) | `installSpanRecorder(t)` used by all packages' tests via `metricstest`-style helper package `observability/tracetest` |
| `internal/observability/sentry.go` | `trace_id` tag |
| `cmd/vault-solver/run.go` | wiring + shutdown |
| `internal/chain/fallback.go`, `internal/chain/trace.go` (new) | RPC span per logical request, `traceparent` on attempts |
| `internal/txmanager/txmanager.go` | `txmanager.send` / `broadcast` / `replace` / `account_poll` spans |
| Eight `*http.Client` construction sites (spec §6.2) | `TraceTransport` |
| `internal/solvers/{rfq,uniswapx,lifi,bridgefacilitator,redstoneoev}/**` | inbound handler, stage spans, links |
| `README.md`, `docs/TRACING-PLAN.md` (new), `docs/*-PLAN.md`, `CLAUDE.md`, `config/*.example.yaml`, `deploy/docker-compose.yml` | docs |

---

### Task 1: SDK bootstrap, enablement, `TraceLogger`

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/observability/tracing.go`
- Create: `internal/observability/tracetest/tracetest.go` (test helper package, like `observability/metricstest`)
- Test: `internal/observability/tracing_test.go`

**Interfaces:**
- Produces:
  ```go
  type Tracing struct { Name, Version, Commit string; Solvers []string; ChainID uint64 }
  func NewTracing(ctx context.Context, info Tracing, log logr.Logger) (shutdown func(context.Context) error, enabled bool)
  func TraceLogger(ctx context.Context, log logr.Logger) logr.Logger
  // package tracetest
  func Install(t testing.TB) *tracetest.SpanRecorder // sets a recording global provider + W3C propagator, restores on cleanup
  ```

- [ ] **Step 1: Add dependencies**

```bash
cd vault-solver
GOTOOLCHAIN=go1.27.1 go get go.opentelemetry.io/otel@v1.46.0 go.opentelemetry.io/otel/sdk@v1.46.0 \
  go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@v1.46.0 \
  go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp@v0.71.0
GOTOOLCHAIN=go1.27.1 go mod tidy
```

- [ ] **Step 2: Write the test helper package**

`internal/observability/tracetest/tracetest.go`:

```go
// Package tracetest installs a recording OpenTelemetry provider for tests.
package tracetest

import (
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// Install registers a synchronous recording TracerProvider and the W3C propagator for the test's
// lifetime and returns the recorder. Spans are available from recorder.Ended() once ended.
func Install(t testing.TB) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	prevProvider := otel.GetTracerProvider()
	prevPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	t.Cleanup(func() {
		_ = provider.Shutdown(t.Context())
		otel.SetTracerProvider(prevProvider)
		otel.SetTextMapPropagator(prevPropagator)
	})
	return recorder
}
```

- [ ] **Step 3: Write the failing tests**

`internal/observability/tracing_test.go`:

```go
package observability

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
)

func TestTracingEnabled(t *testing.T) {
	cases := map[string]bool{
		"": false, "1": true, "true": true, "TRUE": true, " yes ": true, "on": true,
		"enabled": true, "0": false, "false": false, "off": false, "garbage": false,
	}
	for in, want := range cases {
		if got := tracingEnabled(in); got != want {
			t.Errorf("tracingEnabled(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNewTracingDisabledInstallsPropagatorOnly(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_ENABLED", "")
	shutdown, enabled := NewTracing(t.Context(), Tracing{Name: "test"}, logr.Discard())
	if enabled {
		t.Fatal("expected tracing disabled")
	}
	if _, ok := otel.GetTextMapPropagator().(interface{ Fields() []string }); !ok {
		t.Fatal("propagator not installed")
	}
	if fields := otel.GetTextMapPropagator().Fields(); len(fields) == 0 {
		t.Fatal("propagator has no fields; expected traceparent/tracestate/baggage")
	}
	if err := shutdown(t.Context()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestNewTracingEnabledRegistersProvider(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_ENABLED", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1") // never reached; batcher exports async
	prev := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	shutdown, enabled := NewTracing(t.Context(), Tracing{Name: "test", Version: "v", Commit: "c", Solvers: []string{"rfq"}, ChainID: 1}, logr.Discard())
	if !enabled {
		t.Fatal("expected tracing enabled")
	}
	_, span := otel.Tracer("x").Start(t.Context(), "probe")
	if !span.SpanContext().IsValid() || !span.IsRecording() {
		t.Fatal("expected a recording span from the registered provider")
	}
	span.End()
	_ = shutdown(t.Context()) // export to a closed port fails; must not error the caller path
}

func TestTraceLogger(t *testing.T) {
	var lines []string
	log := funcr.New(func(_, args string) { lines = append(lines, args) }, funcr.Options{})
	if got := TraceLogger(t.Context(), log); got != log {
		// funcr loggers are comparable by value of their sink; a no-span context must return the input.
		t.Fatal("expected the same logger when no span is present")
	}
	tracetest.Install(t)
	ctx, span := otel.Tracer("x").Start(context.Background(), "s")
	defer span.End()
	TraceLogger(ctx, log).Info("hello")
	want := "\"trace_id\"=\"" + span.SpanContext().TraceID().String() + "\""
	if len(lines) != 1 || !contains(lines[0], want) || !contains(lines[0], "\"span_id\"") {
		t.Fatalf("log line missing trace fields: %q", lines)
	}
	_ = trace.SpanFromContext(ctx)
}

func contains(s, sub string) bool { return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

(Use `strings.Contains` instead of the two helpers; they are spelled out only so the test is self-contained. Prefer `strings`.)

- [ ] **Step 4: Run tests to verify they fail**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/observability/ -run 'TestTracing|TestNewTracing|TestTraceLogger' -v`
Expected: FAIL, undefined `tracingEnabled`, `NewTracing`, `Tracing`, `TraceLogger`.

- [ ] **Step 5: Implement `tracing.go`**

```go
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
		resource.WithAttributes(
			semconv.ServiceName(name),
			semconv.ServiceVersion(info.Version),
			attribute.String("vault_solver.commit", info.Commit),
			attribute.String("vault_solver.solvers", strings.Join(info.Solvers, ",")),
			attribute.Int64("chain.id", int64(info.ChainID)),
		),
		resource.WithFromEnv(),      // OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES win over the defaults above
		resource.WithTelemetrySDK(),
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
```

- [ ] **Step 6: Run tests, lint**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/observability/... -v && make lint`
Expected: PASS, 0 lint issues. If `resource.New` complains about a schema URL conflict from `WithFromEnv`, drop `WithTelemetrySDK()` and keep the rest; the test suite still passes.

- [ ] **Step 7: Commit**

```bash
multigit add go.mod go.sum internal/observability/tracing.go internal/observability/tracing_test.go internal/observability/tracetest/tracetest.go
multigit commit -s -m "feat(observability): bootstrap opentelemetry tracing from env"
```

---

### Task 2: `Tracer` wrapper, attribute keys, error recording

**Files:**
- Create: `internal/observability/tracer.go`
- Test: `internal/observability/tracer_test.go`

**Interfaces:**
- Produces:
  ```go
  const (
      AttrSolver         = attribute.Key("solver")
      AttrRequestID      = attribute.Key("request.id")
      AttrQuoteID        = attribute.Key("quote.id")
      AttrQuoteTraceID   = attribute.Key("quote.trace_id")
      AttrOrderID        = attribute.Key("order.id")
      AttrOrderHash      = attribute.Key("order.hash")
      AttrOrderOnchainID = attribute.Key("order.onchain_id")
      AttrOfferID        = attribute.Key("offer.id")
      AttrAuctionID      = attribute.Key("auction.id")
      AttrAdapter        = attribute.Key("adapter.address")
      AttrVault          = attribute.Key("vault.address")
      AttrRequestAddress = attribute.Key("request.address")
      AttrStrategy       = attribute.Key("strategy.name")
      AttrTxLabel        = attribute.Key("tx.label")
      AttrTxHash         = attribute.Key("tx.hash")
      AttrTxNonce        = attribute.Key("tx.nonce")
      AttrTxOutcome      = attribute.Key("tx.outcome")
      AttrTxAttempt      = attribute.Key("tx.attempt")
      AttrReasonCode     = attribute.Key("reason_code")
  )
  type EndFunc func(err error)
  type Tracer struct{ /* unexported */ }
  func NewTracer(name, solver string) *Tracer
  func (t *Tracer) Start(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, EndFunc)
  func (t *Tracer) StartLinked(ctx context.Context, name string, links []trace.Link, attrs ...attribute.KeyValue) (context.Context, EndFunc)
  func Decline(ctx context.Context, decision, reason string)
  func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue)
  ```
- `end(err)`: nil → plain End; `context.Canceled` → event `cancelled`, End; other → `RecordError`, `SetStatus(codes.Error, err.Error())`, `reason_code` attribute when `errors.As(err, &interface{ ReasonCode() string })`, End. Calling the returned `EndFunc` twice is a no-op (sync.Once).

- [ ] **Step 1: Write the failing tests**

```go
package observability

import (
	"context"
	"testing"

	"github.com/go-errors/errors"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
)

type codedErr struct{ msg string }

func (e codedErr) Error() string      { return e.msg }
func (e codedErr) ReasonCode() string { return "backend_unreachable" }

func endedSpan(t *testing.T, rec interface{ Ended() []sdktrace.ReadOnlySpan }, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, s := range rec.Ended() {
		if s.Name() == name {
			return s
		}
	}
	t.Fatalf("span %q not ended", name)
	return nil
}

func attr(s sdktrace.ReadOnlySpan, key string) string {
	for _, kv := range s.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.Emit()
		}
	}
	return ""
}

func TestTracerStartStampsSolverAndRecordsError(t *testing.T) {
	rec := tracetest.Install(t)
	tr := NewTracer("test", "rfq")
	_, end := tr.Start(context.Background(), "rfq.quote", AttrQuoteID.String("q1"))
	end(errors.Errorf("boom: %w", codedErr{"backend down"}))
	end(nil) // second call is a no-op
	s := endedSpan(t, rec, "rfq.quote")
	if attr(s, "solver") != "rfq" || attr(s, "quote.id") != "q1" {
		t.Fatalf("attributes: %v", s.Attributes())
	}
	if s.Status().Code != codes.Error {
		t.Fatalf("status = %v, want Error", s.Status())
	}
	if attr(s, "reason_code") != "backend_unreachable" {
		t.Fatalf("reason_code missing: %v", s.Attributes())
	}
	if len(s.Events()) != 1 || s.Events()[0].Name != "exception" {
		t.Fatalf("expected one exception event, got %v", s.Events())
	}
}

func TestTracerCancelledIsNotAnError(t *testing.T) {
	rec := tracetest.Install(t)
	_, end := NewTracer("test", "rfq").Start(context.Background(), "rfq.order")
	end(errors.Errorf("wrapped: %w", context.Canceled))
	s := endedSpan(t, rec, "rfq.order")
	if s.Status().Code == codes.Error {
		t.Fatal("cancellation must not set Error status")
	}
	if len(s.Events()) != 1 || s.Events()[0].Name != "cancelled" {
		t.Fatalf("expected cancelled event, got %v", s.Events())
	}
}

func TestDeclineAndSetAttributes(t *testing.T) {
	rec := tracetest.Install(t)
	ctx, end := NewTracer("test", "uniswapx").Start(context.Background(), "uniswapx.quote")
	Decline(ctx, "no_quote", "adapter_paused")
	SetAttributes(ctx, AttrTxHash.String("0xabc"))
	end(nil)
	s := endedSpan(t, rec, "uniswapx.quote")
	if s.Status().Code == codes.Error {
		t.Fatal("decline must not set Error status")
	}
	if len(s.Events()) != 1 || s.Events()[0].Name != "declined" {
		t.Fatalf("expected declined event, got %v", s.Events())
	}
	if attr(s, "tx.hash") != "0xabc" {
		t.Fatalf("SetAttributes not applied: %v", s.Attributes())
	}
}

func TestStartLinked(t *testing.T) {
	rec := tracetest.Install(t)
	tr := NewTracer("test", "rfq")
	quoteCtx, endQuote := tr.Start(context.Background(), "rfq.quote")
	endQuote(nil)
	link := LinkFromContext(quoteCtx)
	_, end := tr.StartLinked(context.Background(), "rfq.order", []trace.Link{link})
	end(nil)
	s := endedSpan(t, rec, "rfq.order")
	if len(s.Links()) != 1 || s.Links()[0].SpanContext.TraceID() != link.SpanContext.TraceID() {
		t.Fatalf("link not recorded: %v", s.Links())
	}
}

func TestNoopWithoutProvider(t *testing.T) {
	// No tracetest.Install: global provider is the no-op one. Everything must be safe and cheap.
	ctx, end := NewTracer("test", "rfq").Start(context.Background(), "rfq.quote")
	Decline(ctx, "x", "y")
	SetAttributes(ctx, AttrQuoteID.String("q"))
	end(errors.New("ignored"))
}

func BenchmarkStartEndNoop(b *testing.B) {
	tr := NewTracer("bench", "rfq")
	for b.Loop() {
		_, end := tr.Start(context.Background(), "s")
		end(nil)
	}
}

func BenchmarkStartEndRecording(b *testing.B) {
	tracetest.Install(b)
	tr := NewTracer("bench", "rfq")
	for b.Loop() {
		_, end := tr.Start(context.Background(), "s", AttrQuoteID.String("q"))
		end(nil)
	}
}
```

(`trace` import is `go.opentelemetry.io/otel/trace`; add `LinkFromContext` to the interface list below.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/observability/ -run 'TestTracer|TestDecline|TestStartLinked|TestNoop' -v`
Expected: FAIL, undefined symbols.

- [ ] **Step 3: Implement `tracer.go`**

```go
package observability

import (
	"context"
	"sync"

	"github.com/go-errors/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Span attribute keys shared by every solver (spec §9.3). Neutral names: the solver attribute
// disambiguates, so an operator searches quote.id regardless of integration.
const (
	AttrSolver         = attribute.Key("solver")
	AttrRequestID      = attribute.Key("request.id")
	AttrQuoteID        = attribute.Key("quote.id")
	AttrQuoteTraceID   = attribute.Key("quote.trace_id")
	AttrOrderID        = attribute.Key("order.id")
	AttrOrderHash      = attribute.Key("order.hash")
	AttrOrderOnchainID = attribute.Key("order.onchain_id")
	AttrOfferID        = attribute.Key("offer.id")
	AttrAuctionID      = attribute.Key("auction.id")
	AttrAdapter        = attribute.Key("adapter.address")
	AttrVault          = attribute.Key("vault.address")
	AttrRequestAddress = attribute.Key("request.address")
	AttrStrategy       = attribute.Key("strategy.name")
	AttrTxLabel        = attribute.Key("tx.label")
	AttrTxHash         = attribute.Key("tx.hash")
	AttrTxNonce        = attribute.Key("tx.nonce")
	AttrTxOutcome      = attribute.Key("tx.outcome")
	AttrTxAttempt      = attribute.Key("tx.attempt")
	AttrReasonCode     = attribute.Key("reason_code")
)

// EndFunc ends a span, recording err when non-nil. Safe to call more than once; later calls are no-ops.
type EndFunc func(err error)

// Tracer starts spans that carry the owning solver's name. Obtain one per package with NewTracer.
type Tracer struct {
	tracer trace.Tracer
	solver []attribute.KeyValue // empty for shared components
}

// NewTracer returns a Tracer for the instrumentation scope name (the package import path) that
// stamps solver on every span when solver is non-empty (shared components pass ""). Uses the global
// provider, so it is a no-op until NewTracing enables it.
func NewTracer(name, solver string) *Tracer {
	t := &Tracer{tracer: otel.Tracer(name)}
	if solver != "" {
		t.solver = []attribute.KeyValue{AttrSolver.String(solver)}
	}
	return t
}

// Raw exposes the underlying tracer for code that must hold a trace.Span across goroutines
// (txmanager keeps one span from submission to receipt).
func (t *Tracer) Raw() trace.Tracer { return t.tracer }

// Start begins a child span of ctx.
func (t *Tracer) Start(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, EndFunc) {
	return t.StartLinked(ctx, name, nil, attrs...)
}

// StartLinked begins a child span of ctx with links to earlier spans (spec §12).
func (t *Tracer) StartLinked(
	ctx context.Context, name string, links []trace.Link, attrs ...attribute.KeyValue,
) (context.Context, EndFunc) {
	opts := []trace.SpanStartOption{trace.WithAttributes(t.solver...), trace.WithAttributes(attrs...)}
	if len(links) > 0 {
		opts = append(opts, trace.WithLinks(links...))
	}
	ctx, span := t.tracer.Start(ctx, name, opts...)
	var once sync.Once
	return ctx, func(err error) { once.Do(func() { endSpan(span, err) }) }
}

func endSpan(span trace.Span, err error) {
	switch {
	case err == nil:
	case errors.Is(err, context.Canceled):
		span.AddEvent("cancelled")
	default:
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		var coded interface{ ReasonCode() string }
		if errors.As(err, &coded) {
			span.SetAttributes(AttrReasonCode.String(coded.ReasonCode()))
		}
	}
	span.End()
}

// Decline records an expected non-error outcome (no quote, not profitable, paused adapter) on the
// current span as a declined event. Status is left unset, mirroring the V(1) logging rule.
func Decline(ctx context.Context, decision, reason string) {
	trace.SpanFromContext(ctx).AddEvent("declined", trace.WithAttributes(
		attribute.String("decision", decision), attribute.String("reason", reason),
	))
}

// SetAttributes adds attributes to the current span (e.g. a tx hash learned after Send returns).
func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	trace.SpanFromContext(ctx).SetAttributes(attrs...)
}

// LinkFromContext returns a link to the span in ctx; the zero Link when there is none.
func LinkFromContext(ctx context.Context) trace.Link {
	return trace.Link{SpanContext: trace.SpanContextFromContext(ctx)}
}
```

- [ ] **Step 4: Run tests, benchmark once, lint**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/observability/ -run 'Tracer|Decline|StartLinked|Noop' -bench 'StartEnd' -benchtime 2000x -v && make lint`
Expected: PASS; the no-op benchmark reports well under 1 µs/op; record both numbers in the commit body.

- [ ] **Step 5: Commit**

```bash
multigit add internal/observability/tracer.go internal/observability/tracer_test.go
multigit commit -s -m "feat(observability): add tracer wrapper with solver attribute and error recording"
```

---

### Task 3: `TraceHandler`, `TraceTransport`

**Files:**
- Create: `internal/observability/httptrace.go`
- Test: `internal/observability/httptrace_test.go`

**Interfaces:**
- Produces:
  ```go
  func TraceHandler(next http.Handler, route func(*http.Request) string) http.Handler
  func TraceTransport(base http.RoundTripper, peer string) http.RoundTripper
  ```
- Filtered (no span) paths: `/health`, `/healthz`, `/ready`, `/readyz`, `/metrics`, `/docs`, and any path starting with `/openapi`.

- [ ] **Step 1: Write the failing tests**

```go
package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
)

const parentTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func TestTraceHandlerContinuesInboundTrace(t *testing.T) {
	rec := tracetest.Install(t)
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
	req := httptest.NewRequest(http.MethodPost, "/quote", nil)
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
	h := TraceHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }), func(*http.Request) string { return "x" })
	for _, p := range []string{"/health", "/healthz", "/ready", "/readyz", "/metrics", "/openapi.json", "/docs"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, p, nil))
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
	var client_ bool
	for _, s := range rec.Ended() {
		if s.Name() == "rfq-backend POST" && s.SpanKind() == trace.SpanKindClient {
			client_ = true
			if attr(s, "peer.service") != "rfq-backend" {
				t.Fatalf("peer.service missing: %v", s.Attributes())
			}
		}
	}
	if !client_ {
		t.Fatalf("client span not recorded: %v", rec.Ended())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/observability/ -run 'TraceHandler|TraceTransport' -v`
Expected: FAIL, undefined.

- [ ] **Step 3: Implement `httptrace.go`**

```go
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
```

- [ ] **Step 4: Run tests and lint**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/observability/... -v && make lint`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
multigit add internal/observability/httptrace.go internal/observability/httptrace_test.go
multigit commit -s -m "feat(observability): add traced http handler and transport"
```

---

### Task 4: `SpanLinks` bounded map

**Files:**
- Create: `internal/observability/spanlinks.go`
- Test: `internal/observability/spanlinks_test.go`

**Interfaces:**
- Produces:
  ```go
  type SpanLinks struct{ /* unexported */ }
  func NewSpanLinks(maxEntries int) *SpanLinks            // maxEntries <= 0 → 1024
  func (l *SpanLinks) Remember(key string, ctx context.Context, ttl time.Duration) // no-op when ctx has no valid span context or key == ""
  func (l *SpanLinks) Lookup(key string) (trace.Link, bool)
  func (l *SpanLinks) Len() int
  ```
- Keys are lowercased and trimmed. Expired entries are dropped on Lookup and during Remember sweeps. When full, the oldest entry is evicted.

- [ ] **Step 1: Write the failing tests**

```go
package observability

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
)

func TestSpanLinksRememberLookup(t *testing.T) {
	tracetest.Install(t)
	l := NewSpanLinks(2)
	ctx, span := otel.Tracer("x").Start(context.Background(), "q")
	span.End()
	l.Remember(" Q1 ", ctx, time.Minute)
	link, ok := l.Lookup("q1")
	if !ok || link.SpanContext.TraceID() != span.SpanContext().TraceID() {
		t.Fatalf("lookup = %v, %v", link, ok)
	}
	if _, ok := l.Lookup("missing"); ok {
		t.Fatal("unexpected hit")
	}
	l.Remember("", ctx, time.Minute)
	l.Remember("noctx", context.Background(), time.Minute)
	if l.Len() != 1 {
		t.Fatalf("len = %d, want 1 (empty key and no-span ctx ignored)", l.Len())
	}
}

func TestSpanLinksTTLAndEviction(t *testing.T) {
	tracetest.Install(t)
	l := NewSpanLinks(2)
	now := time.Unix(1000, 0)
	l.now = func() time.Time { return now }
	ctx, span := otel.Tracer("x").Start(context.Background(), "q")
	span.End()
	l.Remember("a", ctx, 10*time.Second)
	l.Remember("b", ctx, 10*time.Second)
	l.Remember("c", ctx, 10*time.Second) // evicts a
	if _, ok := l.Lookup("a"); ok {
		t.Fatal("a should have been evicted")
	}
	if _, ok := l.Lookup("b"); !ok {
		t.Fatal("b should remain")
	}
	now = now.Add(11 * time.Second)
	if _, ok := l.Lookup("b"); ok {
		t.Fatal("b should have expired")
	}
	if l.Len() != 1 { // c still stored until swept or looked up
		t.Fatalf("len = %d", l.Len())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/observability/ -run SpanLinks -v` → FAIL, undefined.

- [ ] **Step 3: Implement `spanlinks.go`**

```go
package observability

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"
)

const defaultSpanLinkEntries = 1024

// SpanLinks remembers span contexts by an application key (quote id, auction id, request address)
// so later work in a different context tree can link back to them (spec §12). It is process-local,
// bounded, and best effort: a miss is an ordinary (link, false) result, never an error.
type SpanLinks struct {
	mu      sync.Mutex
	max     int
	now     func() time.Time
	entries map[string]spanLinkEntry
	order   []string // insertion order for eviction
}

type spanLinkEntry struct {
	sc      trace.SpanContext
	expires time.Time
}

// NewSpanLinks creates a map holding at most maxEntries (1024 when <= 0).
func NewSpanLinks(maxEntries int) *SpanLinks {
	if maxEntries <= 0 {
		maxEntries = defaultSpanLinkEntries
	}
	return &SpanLinks{max: maxEntries, now: time.Now, entries: make(map[string]spanLinkEntry, maxEntries)}
}

// Remember stores the span context of ctx under key for ttl. Empty keys and contexts without a
// valid span context are ignored, so callers never need to check tracing state first.
func (l *SpanLinks) Remember(key string, ctx context.Context, ttl time.Duration) {
	sc := trace.SpanContextFromContext(ctx)
	key = normalizeLinkKey(key)
	if key == "" || !sc.IsValid() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.entries[key]; !exists {
		l.order = append(l.order, key)
	}
	l.entries[key] = spanLinkEntry{sc: sc, expires: l.now().Add(ttl)}
	for len(l.order) > l.max {
		oldest := l.order[0]
		l.order = l.order[1:]
		delete(l.entries, oldest)
	}
}

// Lookup returns a link to the remembered span, dropping it if expired.
func (l *SpanLinks) Lookup(key string) (trace.Link, bool) {
	key = normalizeLinkKey(key)
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[key]
	if !ok {
		return trace.Link{}, false
	}
	if !l.now().Before(entry.expires) {
		delete(l.entries, key)
		return trace.Link{}, false
	}
	return trace.Link{SpanContext: entry.sc}, true
}

// Len reports stored entries, expired or not (for tests and diagnostics).
func (l *SpanLinks) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

func normalizeLinkKey(key string) string { return strings.ToLower(strings.TrimSpace(key)) }
```

Note: `order` keeps deleted keys until they reach the front; that is fine because eviction only deletes what is still present and `Len` counts the map. If lint flags the `l.now` test assignment, keep it (unexported field set within the package test).

- [ ] **Step 4: Run tests and lint** → `GOTOOLCHAIN=go1.27.1 go test ./internal/observability/... && make lint` → PASS.

- [ ] **Step 5: Commit**

```bash
multigit add internal/observability/spanlinks.go internal/observability/spanlinks_test.go
multigit commit -s -m "feat(observability): add bounded span link map for quote-to-fill links"
```

---

### Task 5: Wire tracing in `runBot`; Sentry `trace_id` tag

**Files:**
- Modify: `cmd/vault-solver/run.go:54-73` (after `NewLogger`), and the deferred shutdown ordering
- Modify: `internal/observability/sentry.go` `eventTags` (around line 84-100)
- Test: `internal/observability/sentry_test.go` (extend the existing `eventTags` test, or add one)

- [ ] **Step 1: Write the failing Sentry test**

Find the existing test for `eventTags` in `internal/observability/sentry_test.go` and add:

```go
func TestEventTagsPromotesTraceID(t *testing.T) {
	tags := eventTags("rfq", map[string]any{"solver": "rfq", "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736"})
	if tags["trace_id"] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace_id tag missing: %v", tags)
	}
}
```

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/observability/ -run TestEventTagsPromotesTraceID` → FAIL.

- [ ] **Step 2: Implement the tag**

In `eventTags`, next to the `solver` and `label` promotion:

```go
	if traceID, _ := fields["trace_id"].(string); traceID != "" {
		tags["trace_id"] = traceID
	}
```

- [ ] **Step 3: Wire `runBot`**

In `cmd/vault-solver/run.go`, after `log, syncLog := observability.NewLogger(debug)` / `defer syncLog()` and after `solverNames` is built (move the `solverNames` loop above the logger call if needed), add:

```go
	shutdownTracing, tracingEnabled := observability.NewTracing(ctx, observability.Tracing{
		Name:    "vault-solver",
		Version: version.Version,
		Commit:  version.Commit,
		Solvers: solverNames,
		ChainID: cfg.Chain.ChainID,
	}, log)
	defer func() {
		// Fresh context on purpose: ctx is cancelled during shutdown and the flush must still run.
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(flushCtx); err != nil { //nolint:contextcheck // bounded post-cancellation flush
			log.Info("tracing shutdown incomplete", "err", err.Error())
		}
	}()
```

Add `"tracing", tracingEnabled` to the `vault-solver starting` log line. Deferred order matters: this `defer` must be registered **before** the observability server and txmanager defers so it runs **after** them (defers run LIFO), i.e. place it right after the logger defer.

- [ ] **Step 4: Build, test, lint**

Run: `GOTOOLCHAIN=go1.27.1 go build ./... && GOTOOLCHAIN=go1.27.1 go test ./cmd/... ./internal/observability/... && make lint` → PASS. Run the binary once with `OTEL_EXPORTER_ENABLED=true` against `config/sepolia.local.yaml` only if an RPC is reachable; otherwise skip (the startup log line is the check).

- [ ] **Step 5: Commit**

```bash
multigit add cmd/vault-solver/run.go internal/observability/sentry.go internal/observability/sentry_test.go
multigit commit -s -m "feat(cmd): enable opentelemetry tracing from OTEL env"
```

---

### Task 6: RPC spans in the fallback transport

**Files:**
- Modify: `internal/chain/fallback.go` (`RoundTrip` lines 47-166, `inspectRPCRequest` 175-205)
- Create: `internal/chain/trace.go`
- Test: `internal/chain/fallback_test.go` (existing file; add cases)

**Interfaces:**
- Produces (package-private):
  ```go
  var rpcTracer = observability.NewTracer("github.com/symbioticfi/vault-solver/internal/chain", "")
  type rpcRequestTrace struct { span trace.Span; once sync.Once }
  func (t *fallbackTransport) beginTrace(ctx context.Context, request rpcRequestInfo) (context.Context, *rpcRequestTrace)
  func (tr *rpcRequestTrace) attempt(endpoint string, outcome rpcOutcome)   // adds an "attempt" event
  func (tr *rpcRequestTrace) finish(outcome rpcOutcome)                    // status + End, once
  ```
- `rpcRequestInfo` gains `requestID string` (the raw JSON-RPC id for every request, not only null-fallback ones).
- Attributes: `rpc.system=jsonrpc`, `rpc.method=<boundedMethod>`, `rpc.jsonrpc.request_id`, `chain.rpc.role`, `chain.rpc.batch`. Span kind Client. Name `<boundedMethod>` (`batch`, `other`, `unknown` as today).
- RPC spans carry no `solver` attribute (the chain client is shared); `rpcTracer` uses the plain `otel.Tracer` since it needs `trace.WithSpanKind`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/chain/fallback_test.go` (reuse the file's existing helpers for building a `fallbackTransport` with `httptest` endpoints; if none fit, build one directly as the file's other tests do):

```go
func TestFallbackTransportSpans(t *testing.T) {
	rec := tracetest.Install(t)
	var gotTraceparent string
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTraceparent = r.Header.Get("traceparent")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":7,"result":"0x1"}`))
	}))
	defer good.Close()
	endpoints, _ := parseHTTPEndpoints([]string{bad.URL, good.URL})
	tr := &fallbackTransport{endpoints: endpoints, base: http.DefaultTransport, role: "read", log: logr.Discard()}
	ctx, parent := otel.Tracer("x").Start(t.Context(), "parent")
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://placeholder/", strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"eth_chainId","params":[]}`))
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	if n := len(rec.Ended()); n != 0 {
		t.Fatalf("span must not end before body close, got %d ended", n)
	}
	_ = resp.Body.Close()
	parent.End()
	var span sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		if s.Name() == "eth_chainId" {
			span = s
		}
	}
	if span == nil {
		t.Fatalf("rpc span missing: %v", rec.Ended())
	}
	if span.Parent().SpanID() != parent.SpanContext().SpanID() || span.SpanKind() != trace.SpanKindClient {
		t.Fatal("rpc span is not a client child of the caller span")
	}
	if len(span.Events()) != 2 || span.Events()[0].Name != "attempt" || span.Events()[1].Name != "attempt" {
		t.Fatalf("expected two attempt events, got %v", span.Events())
	}
	if span.Status().Code == codes.Error {
		t.Fatalf("successful request must not be Error: %v", span.Status())
	}
	if gotTraceparent == "" || !strings.Contains(gotTraceparent, parent.SpanContext().TraceID().String()) {
		t.Fatalf("traceparent not injected on attempt: %q", gotTraceparent)
	}
	if attr(span, "rpc.jsonrpc.request_id") != "7" || attr(span, "chain.rpc.role") != "read" {
		t.Fatalf("attributes: %v", span.Attributes())
	}
}

func TestFallbackTransportSpanErrorWhenAllFail(t *testing.T) {
	rec := tracetest.Install(t)
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer bad.Close()
	endpoints, _ := parseHTTPEndpoints([]string{bad.URL})
	tr := &fallbackTransport{endpoints: endpoints, base: http.DefaultTransport, role: "read", log: logr.Discard()}
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://placeholder/", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"eth_call","params":[]}`))
	if _, err := tr.RoundTrip(req); err == nil {
		t.Fatal("expected error")
	}
	spans := rec.Ended()
	if len(spans) != 1 || spans[0].Status().Code != codes.Error {
		t.Fatalf("expected one Error span, got %v", spans)
	}
}
```

Copy the small `attr(span, key)` helper from the observability tests into this test file (unexported helpers do not cross packages).

- [ ] **Step 2: Run to verify failure** → `GOTOOLCHAIN=go1.27.1 go test ./internal/chain/ -run FallbackTransportSpan -v` → FAIL (no spans, no traceparent).

- [ ] **Step 3: Implement `internal/chain/trace.go`**

```go
package chain

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"net/http"
)

var rpcTracer = otel.Tracer("github.com/symbioticfi/vault-solver/internal/chain")

// rpcRequestTrace is the span for one logical JSON-RPC request across endpoint attempts. It ends
// where the metrics observation finishes: on body close, or immediately on transport failure.
type rpcRequestTrace struct {
	span trace.Span
	once sync.Once
}

func (t *fallbackTransport) beginTrace(ctx context.Context, request rpcRequestInfo) (context.Context, *rpcRequestTrace) {
	ctx, span := rpcTracer.Start(ctx, request.boundedMethod,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("rpc.system", "jsonrpc"),
			attribute.String("rpc.method", request.boundedMethod),
			attribute.String("rpc.jsonrpc.request_id", request.requestID),
			attribute.String("chain.rpc.role", t.role),
			attribute.Bool("chain.rpc.batch", request.boundedMethod == "batch"),
		),
	)
	return ctx, &rpcRequestTrace{span: span}
}

func (tr *rpcRequestTrace) attempt(endpoint string, outcome rpcOutcome) {
	tr.span.AddEvent("attempt", trace.WithAttributes(
		attribute.String("endpoint", endpoint), attribute.String("outcome", string(outcome)),
	))
}

func (tr *rpcRequestTrace) finish(outcome rpcOutcome) {
	tr.once.Do(func() {
		if outcome != rpcOutcomeSuccess {
			tr.span.SetStatus(codes.Error, string(outcome))
		}
		tr.span.End()
	})
}

func injectTraceHeaders(ctx context.Context, header http.Header) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(header))
}
```

(Reorder imports with `make format`.)

- [ ] **Step 4: Modify `RoundTrip`**

Concrete edits in `internal/chain/fallback.go`:

1. In `inspectRPCRequest`, add `requestID string` to `rpcRequestInfo` and set `info.requestID = string(bytes.TrimSpace(request.ID))` right after `info` is built (before the null-fallback switch); for batches set nothing.
2. Replace lines 58-62 with:
   ```go
   request := inspectRPCRequest(body)
   method := request.boundedMethod
   ctx, requestTrace := t.beginTrace(req.Context(), request)
   ```
   and use `ctx` (not `req.Context()`) as the parent for every `context.WithTimeout(...)` in the loop and for the `req.Context().Err()` checks (`ctx.Err()`).
3. After `attempt.Host = ep.Host`, add `injectTraceHeaders(attemptCtx, attempt.Header)` where `attemptCtx` is the per-attempt timeout context.
4. Every `t.metrics.observeAttempt(t.role, endpoint, method, X)` call gets a paired `requestTrace.attempt(endpoint, X)` on the next line (three sites: null-fallback continue, the body-close closures, and the failure path).
5. Every `requestObservation.finish(X)` call gets a paired `requestTrace.finish(X)` (three sites).
6. Remove the `t.metrics == nil` special case for body wrapping (lines 130-131): always use the classified/observed body so the span ends on close even without metrics. `metrics` methods are already nil-safe. Delete the now-unused `cancelOnClose` type if nothing else uses it (check with `grep -rn cancelOnClose internal/chain`).

- [ ] **Step 5: Run the whole chain package, lint**

Run: `GOTOOLCHAIN=go1.27.1 go test -race ./internal/chain/... -v && make lint` → PASS, including the existing metrics tests (attempt counts unchanged).

- [ ] **Step 6: Commit**

```bash
multigit add internal/chain/ internal/observability/tracer.go internal/observability/tracer_test.go
multigit commit -s -m "feat(chain): trace json-rpc requests and propagate traceparent to providers"
```

---

### Task 7: txmanager spans

**Files:**
- Modify: `internal/txmanager/txmanager.go` (`job` 178-182, `Start` 398-478, `sendAsync` 515-582, `admissionFailure`, `requestLog` 666-671, `broadcast` 673-761, `complete` 763-778, `waitForPendingTransaction` `tryReplace` ~808, `monitorAccount`)
- Test: `internal/txmanager/txmanager_test.go` (existing; add one test using the file's simulated backend helpers)

**Interfaces:**
- `job` gains `span trace.Span` and `ctx context.Context` (the caller-derived span context, used only for `trace.ContextWithSpan`).
- `pendingTransaction` gains `ctx context.Context` (lifecycle ctx carrying the tx span) — used by `complete` and the receipt reader.
- Span names: `txmanager.send <label>` (kind Internal), `txmanager.broadcast`, `txmanager.replace` (attr `tx.attempt`, `tx.cancellation` bool), `txmanager.account_poll`.
- Terminal: attributes `tx.hash` (final attempt hash), `tx.nonce`, `tx.outcome`; status Error for `reverted`, `cancelled`, `submission_error`, `tracking_stopped`.

- [ ] **Step 1: Write the failing test**

Add to `internal/txmanager/txmanager_test.go`, using the package's existing simulated backend and manager constructor helpers (look for the test that sends one transaction and waits for `OutcomeConfirmed`; copy its setup):

```go
func TestSendSpansNestUnderCaller(t *testing.T) {
	rec := tracetest.Install(t)
	m, backend := newTestManager(t) // use the file's existing helper name
	ctx, parent := otel.Tracer("x").Start(t.Context(), "rfq.order.submit")
	res := m.Send(ctx, Request{To: common.Address{1}, Label: "test-fill", Solver: "rfq", GasLimit: 21000})
	parent.End()
	if res.Outcome != OutcomeConfirmed {
		t.Fatalf("outcome %v err %v", res.Outcome, res.Err)
	}
	backend.Commit() // if the helper needs explicit mining, keep the file's pattern
	var send, broadcast sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		switch s.Name() {
		case "txmanager.send test-fill":
			send = s
		case "txmanager.broadcast":
			broadcast = s
		}
	}
	if send == nil || broadcast == nil {
		t.Fatalf("missing spans: %v", rec.Ended())
	}
	if send.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatal("send span is not a child of the caller span")
	}
	if broadcast.Parent().SpanID() != send.SpanContext().SpanID() {
		t.Fatal("broadcast span is not a child of the send span")
	}
	if attr(send, "tx.outcome") != "confirmed" || attr(send, "solver") != "rfq" || attr(send, "tx.hash") != res.Hash.Hex() {
		t.Fatalf("send attributes: %v", send.Attributes())
	}
}
```

- [ ] **Step 2: Run to verify failure** → `GOTOOLCHAIN=go1.27.1 go test ./internal/txmanager/ -run TestSendSpansNestUnderCaller -v` → FAIL.

- [ ] **Step 3: Implement**

1. Package-level `var tracer = observability.NewTracer("github.com/symbioticfi/vault-solver/internal/txmanager", "")` and helper:
   ```go
   func (m *Manager) startSendSpan(ctx context.Context, req Request) (context.Context, trace.Span) {
   	return tracer.Raw().Start(ctx, "txmanager.send "+req.Label, trace.WithAttributes(
   		observability.AttrSolver.String(req.Solver), observability.AttrTxLabel.String(req.Label)))
   }
   ```
   (`Tracer.Raw()` from Task 2 returns the underlying tracer so txmanager can keep a `trace.Span` across goroutines instead of an `EndFunc`.)
2. In `sendAsync`, right after the `admissionCtx.Err()` check passes, `spanCtx, span := m.startSendSpan(ctx, req)`; every `admissionFailure(...)` return in `sendAsync` first calls `endSendSpan(span, Result{Outcome: OutcomeSubmissionError, Err: err})` — add a small helper:
   ```go
   func endSendSpan(span trace.Span, res Result) {
   	span.SetAttributes(observability.AttrTxOutcome.String(string(res.Outcome)))
   	if res.Hash != (common.Hash{}) {
   		span.SetAttributes(observability.AttrTxHash.String(res.Hash.Hex()))
   	}
   	switch res.Outcome {
   	case OutcomeReverted, OutcomeCancelled, OutcomeSubmissionError, OutcomeTrackingStopped:
   		msg := string(res.Outcome)
   		if res.Err != nil {
   			span.RecordError(res.Err)
   			msg = res.Err.Error()
   		}
   		span.SetStatus(codes.Error, msg)
   	}
   	span.End()
   }
   ```
   The `try` early returns (`return nil, false`) also end the span with `OutcomeSubmissionError` and no error (span status Error is acceptable there? No: a busy lane is not an error. End it with a `declined` event via `observability.Decline(spanCtx, "not_admitted", "lane_busy")` and `span.End()` without status).
   Enqueue `job{req: cloneRequest(req), res: res, admissionStarted: admissionStarted, span: span}`.
3. In `Start`'s dequeue branch: `spanCtx := trace.ContextWithSpan(ctx, j.span)`; the `notAdmittedResult` returns call `endSendSpan(j.span, result)`; call `m.broadcast(spanCtx, j.req)`; on broadcast error `endSendSpan(j.span, Result{...})` before `j.res <-`; on success `pending.ctx = trace.ContextWithSpan(lifecycleCtx, j.span)` and `pending.span = j.span`, then `m.complete(pending.ctx, pending)`.
4. In `broadcast`: `ctx, end := tracer.Start(ctx, "txmanager.broadcast")` at the top (after the CancelAt deadline wrap so cancellation is preserved), `defer func() { end(err) }()` with a named `err` return — rename the function's return values to `(pending *pendingTransaction, err error)`; after `signAndSend` set `observability.SetAttributes(ctx, AttrTxHash.String(hash.Hex()), AttrTxNonce.Int64(int64(nonce)))` on the **send** span too: `trace.SpanFromContext(ctx)` is the broadcast span, so also set them on `j.span` via the parent: simplest is to set on both using `trace.SpanFromContext(ctx)` (broadcast) and let `endSendSpan` set `tx.hash` from the result. Use `log := observability.TraceLogger(ctx, m.requestLog(req))` at the top and replace the four `m.requestLog(req)` calls inside `broadcast` with `log`; set `pendingTransaction.log = log`.
5. In `complete`: after `pending.deliver(outcome)`, `endSendSpan(pending.span, outcome)`; also `pending.span.SetAttributes(AttrTxNonce.Int64(int64(pending.nonce)))`.
6. In `waitForPendingTransaction`'s `tryReplace(cancellation bool)`: wrap the replacement in `rctx, end := tracer.Start(ctx, "txmanager.replace", AttrTxAttempt.Int(len(pending.attempts)+1), attribute.Bool("tx.cancellation", cancellation))` and `end(err)` with the replacement's error; pass `rctx` to the RPC calls made inside (fee refresh, `signAndSend`).
7. In `monitorAccount`: wrap each poll iteration body in `pollCtx, end := tracer.Start(ctx, "txmanager.account_poll")` … `end(err)`.
8. Receipt reads already take `ctx`; because `complete` runs on `pending.ctx`, `startReceiptReader(ctx)` reads become RPC child spans automatically.

- [ ] **Step 4: Run the package with race, lint**

Run: `GOTOOLCHAIN=go1.27.1 go test -race ./internal/txmanager/... && make lint` → PASS. Watch for goroutine leaks flagged by existing tests: spans hold no goroutines.

- [ ] **Step 5: Commit**

```bash
multigit add internal/txmanager/ internal/observability/tracer.go
multigit commit -s -m "feat(txmanager): trace transaction lifecycle from submission to receipt"
```

---

### Task 8: Outbound HTTP clients

**Files (modify, one line each unless noted):**
- `internal/solvers/rfq/backend.go:60-67` — `Transport: observability.TraceTransport(backendRequestTransport{base: http.DefaultTransport}, "rfq-backend")`
- `internal/liquidlane/discounts/client.go:67-68` — `NewClient` builds `&http.Client{Timeout: 10 * time.Second, Transport: observability.TraceTransport(nil, "rfq-discounts")}`
- `internal/solvers/bridgefacilitator/apiclient.go:34-44` — `Transport: observability.TraceTransport(nil, "3f-api")`
- `internal/solvers/lifi/orderclient.go:32-37` — `Transport: observability.TraceTransport(nil, "lifi-order-server")`
- `internal/solvers/uniswapx/orderclient.go:36-51` — `Transport: observability.TraceTransport(responseLimitTransport{next: http.DefaultTransport, limit: ...}, "uniswapx-api")`
- `internal/solvers/redstoneoev/strategies/default/morphoapi.go:77-82` — `Transport: observability.TraceTransport(nil, "morpho-graphql")`
- `internal/webhook/client.go:206-218` — `Transport: observability.TraceTransport(nil, "webhook")`
- Tests: `internal/webhook/client_test.go` and `internal/solvers/rfq/backend_test.go` (existing files; add one assertion each)

- [ ] **Step 1: Write the failing tests**

In `internal/webhook/client_test.go`, next to an existing `PostJSON` test using `httptest`:

```go
func TestPostJSONCarriesTraceparent(t *testing.T) {
	tracetest.Install(t)
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("traceparent")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	client := NewClient(Config{URL: srv.URL, Timeout: time.Second}) // match the file's existing constructor call
	ctx, span := otel.Tracer("x").Start(t.Context(), "parent")
	var out struct{}
	if err := client.PostJSON(ctx, map[string]string{"a": "b"}, &out); err != nil {
		t.Fatal(err)
	}
	span.End()
	if !strings.Contains(got, span.SpanContext().TraceID().String()) {
		t.Fatalf("traceparent %q", got)
	}
}
```

In `internal/solvers/rfq/backend_test.go`, in the existing test that asserts `X-Request-Id` is sent, also capture `traceparent` and assert it is non-empty when the test wraps the call in a span (install `tracetest` first). Both requests must carry **both** headers: this proves the transport composition kept `backendRequestTransport`.

- [ ] **Step 2: Run to verify failure** → both FAIL (no `traceparent`).

- [ ] **Step 3: Apply the seven edits** listed under Files. Keep every existing timeout, `CheckRedirect`, header, and limit exactly as is; only the `Transport` field changes (or is added).

- [ ] **Step 4: Test, lint** → `GOTOOLCHAIN=go1.27.1 go test ./internal/webhook/... ./internal/liquidlane/... ./internal/solvers/... && make lint` → PASS.

- [ ] **Step 5: Commit**

```bash
multigit add internal/solvers internal/liquidlane internal/webhook
multigit commit -s -m "feat: propagate trace context on every outbound http client"
```

---

### Task 9: RFQ solver — inbound span, stage spans, link

**Files:**
- Modify: `internal/solvers/rfq/server.go:60-116`, `middleware.go` (no change to request id; add attribute), `quote.go:95-177`, `execution.go:86-300, 555`, `solver.go` (construct `SpanLinks`), `metrics.go:179` (`routeLabel` reused)
- Test: `internal/solvers/rfq/server_test.go`, `execution_test.go` (existing; add cases)

**Interfaces:**
- `var tracer = observability.NewTracer("github.com/symbioticfi/vault-solver/internal/solvers/rfq", Name)`
- `server` gains `links *observability.SpanLinks`; `executionService` gains the same pointer (shared instance created in the factory: `observability.NewSpanLinks(0)`).
- Spans and attributes exactly as spec §9.4 "rfq".

- [ ] **Step 1: Write the failing tests**

In `server_test.go`, extend the existing successful `/quote` test:

```go
	rec := tracetest.Install(t)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	// ... existing request/response assertions ...
	names := map[string]bool{}
	for _, s := range rec.Ended() {
		names[s.Name()] = true
		if s.SpanContext().TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Fatalf("span %s outside inbound trace", s.Name())
		}
	}
	for _, want := range []string{"POST /quote", "rfq.quote", "rfq.quote.decide"} {
		if !names[want] {
			t.Fatalf("missing span %s in %v", want, names)
		}
	}
	// quote.id and request.id on the server span
	for _, s := range rec.Ended() {
		if s.Name() == "POST /quote" && (attr(s, "quote.id") == "" || attr(s, "request.id") == "") {
			t.Fatalf("identifiers missing on server span: %v", s.Attributes())
		}
	}
	if _, ok := srv.links.Lookup(quoteID); !ok {
		t.Fatal("quote span context not remembered for linking")
	}
```

In `execution_test.go`, extend the test that polls an open order and submits it (the one using the fake backend + fake txmanager): install the recorder, pre-`Remember` a quote span under the order's `quoteId`, run one `syncOnce`, then assert the `rfq.order` span has one link with that trace id, has `order.id`/`quote.id`/`quote.trace_id` attributes, and `rfq.order.submit` carries `tx.hash`. Add a second case where nothing was remembered: the `rfq.order` span has zero links and one `link_miss` event, and the order still submits.

- [ ] **Step 2: Run to verify failure** → FAIL.

- [ ] **Step 3: Implement**

1. `server.handler()`: return `observability.TraceHandler(http.MaxBytesHandler(h, maxRequestBytes), func(r *http.Request) string { return routeLabel(r.URL.Path) })`.
2. `handleQuote`: first line `observability.SetAttributes(ctx, observability.AttrRequestID.String(requestID(ctx)), observability.AttrQuoteID.String(in.Body.QuoteID))`; use `log := observability.TraceLogger(ctx, s.log)` for the two log lines in the function. After a successful non-nil `decision.response`: `s.links.Remember(in.Body.QuoteID, ctx, quoteLinkTTL)` where `quoteLinkTTL` = the quote's validity: compute from the response's deadline field if present (`time.Until(deadline) + time.Minute`), else `10 * time.Minute`. Put the constant next to `maxRequestBytes`.
3. `quoteService.quote`: `ctx, end := tracer.Start(ctx, "rfq.quote", AttrAdapter.String(adapter.Hex()))` once the adapter is known (or at the top with the attribute set later via `SetAttributes`); `defer func() { end(err) }()` with a named error return. Wrap the helpers it calls: chain snapshot read → `rfq.quote.snapshot`; discount lookup → `rfq.quote.discounts`; strategy `DecideQuote` → `rfq.quote.decide` with `AttrStrategy.String(strategyName)`; signing → `rfq.quote.sign`. Where the function sets `quoteDecisionNoQuote` (or any non-error non-quote outcome), call `observability.Decline(ctx, "no_quote", string(outcome))`. `badRequestError` is a caller error, not a solver error: end that path with `Decline(ctx, "bad_request", ...)` and `end(nil)` so the span is not Error (the HTTP status still says 400).
4. `executionService.syncOnce`: `ctx, end := tracer.Start(ctx, "rfq.execution.sync")`, `defer end(err)`; `pollOpenOrders` → child `rfq.execution.poll`.
5. `handleOrder(ctx, ...)`: build links:
   ```go
   attrs := []attribute.KeyValue{observability.AttrOrderID.String(orderID), observability.AttrQuoteID.String(local.QuoteID)}
   var links []trace.Link
   if link, ok := e.links.Lookup(local.QuoteID); ok {
   	links = append(links, link)
   	attrs = append(attrs, observability.AttrQuoteTraceID.String(link.SpanContext.TraceID().String()))
   }
   ctx, end := tracer.StartLinked(ctx, "rfq.order", links, attrs...)
   if len(links) == 0 {
   	trace.SpanFromContext(ctx).AddEvent("link_miss", trace.WithAttributes(attribute.String("key", local.QuoteID)))
   }
   log := observability.TraceLogger(ctx, e.log).WithValues("orderId", orderID, "quoteId", local.QuoteID)
   if len(links) > 0 { log = log.WithValues("quoteTraceId", links[0].SpanContext.TraceID().String()) }
   ```
   Pass `log` down instead of `e.log` inside the order path. Stages: `resolveExecutable` → `rfq.order.resolve`; strategy `BuildFillPlan` → `rfq.order.plan` (+`strategy.name`); calldata build → `rfq.order.build`; `txm.Send` → `rfq.order.submit`, then `observability.SetAttributes(ctx, AttrTxHash.String(res.Hash.Hex()), AttrTxOutcome.String(string(res.Outcome)))` on the submit span **and** on the `rfq.order` span (keep the order ctx in a variable); status reconcile → `rfq.order.report`. Skips (already filled, adapter mismatch, expired) → `Decline`.
6. Factory/`solver.go`: create `links := observability.NewSpanLinks(0)` and pass it to both the server and the execution service.

- [ ] **Step 4: Test, lint** → `GOTOOLCHAIN=go1.27.1 go test -race ./internal/solvers/rfq/... && make lint` → PASS.

- [ ] **Step 5: Commit**

```bash
multigit add internal/solvers/rfq
multigit commit -s -m "feat(rfq): trace quote and fill pipelines and link fills to quotes"
```

---

### Task 10: UniswapX solver

**Files:**
- Modify: `internal/solvers/uniswapx/server.go:25-150`, `quote_refresh.go`, `polling.go:123-190`, `order.go:70-83, 333`, `execution.go:36-130, 185, 255, 422`, `solver.go` (links instance)
- Test: `server_test.go`, `polling_test.go` / `execution_test.go` (existing; extend)

**Interfaces:**
- `resolvedOrder` gains `span trace.SpanContext` (set in `pollSource` from the `uniswapx.order.track` span).
- `Solver` gains `links *observability.SpanLinks`.
- Route function for `TraceHandler`: `/quote` → `/quote`, `/ready` → `/ready`, else `other` (probes filtered by the handler anyway).

- [ ] **Step 1: Write the failing tests**

`server_test.go`: same shape as Task 9's server test — inbound `traceparent`, expect spans `POST /quote`, `uniswapx.quote`, `uniswapx.quote.decide` in that trace, `quote.id`/`request.id` on the server span, and `s.links.Lookup(quoteID)` hit after a 200.

`execution_test.go` (or the polling test that pushes a `resolvedOrder` through `fillLoop`): install the recorder; remember a quote span under `order.QuoteID`; run the poll → fill; assert `uniswapx.order.track` has one link to it and `order.hash`, `quote.id`, `quote.trace_id`; assert `uniswapx.fill`'s parent is the track span (same trace id) and `uniswapx.fill.complete` carries `tx.hash`. Miss case: no link, one `link_miss` event, fill still runs.

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement**

1. `newQuoteHTTPServer`: `Handler: observability.TraceHandler(recoverQuoteServer(mux, s.log), uniswapxRoute)` with
   ```go
   func uniswapxRoute(r *http.Request) string {
   	switch r.URL.Path {
   	case "/quote", "/ready":
   		return r.URL.Path
   	}
   	return "other"
   }
   ```
2. `quoteHandler`: once the body is decoded, `observability.SetAttributes(r.Context(), AttrRequestID.String(request.RequestID), AttrQuoteID.String(request.QuoteID))`; `log := observability.TraceLogger(r.Context(), s.log)` for the handler's log lines. After a 200 response is written: `s.links.Remember(request.QuoteID, r.Context(), 10*time.Minute)` (UniswapX quotes have no explicit validity on our side; 10 min bounds the map).
3. `Solver.quote`: `ctx, end := tracer.Start(ctx, "uniswapx.quote", ...)`, strategy call under `uniswapx.quote.decide` with `strategy.name`; declines via `Decline`.
4. `quote_refresh.go` refresh function: root span `uniswapx.quote_refresh`.
5. `pollSource`: root span `uniswapx.orders.poll` per poll; per accepted order (around `trackExclusive` + `claim`): `uniswapx.order.track` with `order.hash`, `quote.id`, links from `s.links.Lookup(order.QuoteID)` (+ `quote.trace_id`, or `link_miss` event), then `order.span = trace.SpanContextFromContext(orderCtx)` before `out <- order`, and end the track span right after enqueue.
6. `fillLoop` → `startFill(order)`: `ctx := trace.ContextWithSpanContext(baseCtx, order.span)`; `ctx, end := tracer.Start(ctx, "uniswapx.fill", AttrOrderHash.String(order.Hash.Hex()), AttrQuoteID.String(order.QuoteID))`; stages `uniswapx.fill.plan` (strategy, `strategy.name`), `uniswapx.fill.build`, `uniswapx.fill.submit` around `SendAsync`; keep the fill ctx on `pendingUniswapFill` so `completePendingFill` can run `uniswapx.fill.complete` under it and set `tx.hash`/`tx.outcome` on both the complete and fill spans, then `end(err)` the fill span there (the fill span lives until completion; store its `EndFunc` on `pendingUniswapFill`).
7. `log := observability.TraceLogger(ctx, s.log)` at each root/stage that logs.

- [ ] **Step 4: Test, lint** → `GOTOOLCHAIN=go1.27.1 go test -race ./internal/solvers/uniswapx/... && make lint` → PASS.

- [ ] **Step 5: Commit**

```bash
multigit add internal/solvers/uniswapx
multigit commit -s -m "feat(uniswapx): trace quotes, order polling, and fills"
```

---

### Task 11: LiFi solver

**Files:**
- Modify: `internal/solvers/lifi/quotes.go:46-431`, `wsclient.go:88-147`, `execution.go:361-620`, `order.go:47-64`, `submission.go:29-174`, `planning.go:380`, `deposit_retry.go`, `order_retry.go`
- Test: `quotes_test.go`, `execution_test.go` (existing; extend)

**Interfaces:**
- `submittedOrder` gains `span trace.SpanContext` set in `parseOrderMessage`.
- No `SpanLinks` in LiFi (spec §12: not linked).

- [ ] **Step 1: Write the failing tests**

`execution_test.go`: in the test that feeds an `orderMessage` through the inbox to the worker with a fake order client and txmanager, install the recorder and assert: a span `lifi.order.<event>` with `order.id`, `order.onchain_id`, `quote.id`; a span `lifi.order.process` whose trace id equals the message span's; `lifi.order.submit` with `tx.hash`.
`quotes_test.go`: in the refresh test, assert spans `lifi.quotes.refresh` → `lifi.quotes.decide` → `lifi.quotes.reconcile`, and that the order-server HTTP request carries `traceparent` (the fake server captures it).

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement**

1. `wsclient.go` dial (`:92`): wrap in `ctx, end := tracer.Start(ctx, "lifi.feed.connect")`, `observability` inject via `otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(headers))` before `DialContext`, `end(err)` after.
2. `parseOrderMessage`: after the order is parsed, `ctx, end := tracer.Start(feedCtx, "lifi.order."+msg.Event, AttrOrderID.String(o.OrderID), AttrOrderOnchainID.String(o.OnChainOrderID), AttrQuoteID.String(o.QuoteID))`; `o.span = trace.SpanContextFromContext(ctx)`; `end(nil)` once enqueued (the message span is short; the worker span is its child).
3. `runOrderWorker` per order: `ctx := trace.ContextWithSpanContext(workCtx, order.span)`; `ctx, end := tracer.Start(ctx, "lifi.order.process", AttrOrderID..., AttrQuoteID...)`; `log := observability.TraceLogger(ctx, s.log)`; stages: `lifi.order.plan` (`planning.go`), `lifi.order.reserve` (re-entered on each `order_retry` pop: start a fresh `lifi.order.reserve` span with `tx.attempt` = retry count, parent = the same order span context), `lifi.order.deposit` (same for `deposit_retry`, with `tx.attempt`), `lifi.order.submit` around `SendAsync` (set `tx.hash`/`tx.outcome` when the result arrives, on submit and process spans), `lifi.order.complete`. The process span ends when the order reaches a terminal state in the worker (store its `EndFunc` alongside the pending fill state, mirroring Task 10).
4. `quoteLoop` / `refreshQuotes`: root `lifi.quotes.refresh` per cycle, `lifi.quotes.decide` around the strategy call (`strategy.name`), `lifi.quotes.reconcile` around `state.reconcile` (its HTTP calls are children), `lifi.quotes.suspend` around `suspendQuotes`.
5. Declines (no inventory, pair skipped, order rejected as unsupported) → `Decline`.

- [ ] **Step 4: Test, lint** → `GOTOOLCHAIN=go1.27.1 go test -race ./internal/solvers/lifi/... && make lint` → PASS.

- [ ] **Step 5: Commit**

```bash
multigit add internal/solvers/lifi
multigit commit -s -m "feat(lifi): trace quote publishing and order handling"
```

---

### Task 12: 3F solver

**Files:**
- Modify: `internal/solvers/bridgefacilitator/solver.go:182-384`, `offer.go:17-74`, `apiclient.go:58-91`, `redeemer.go:19-100`, `auctionview.go`
- Test: `solver_test.go` / `redeemer_test.go` (existing; extend)

**Interfaces:**
- `Solver` gains `links *observability.SpanLinks` (created in the factory).
- Keys: `req:<request address hex lowercase>` and `auction:<adapter hex>:<auction id>`; TTL = offer expiration + 1 h.

- [ ] **Step 1: Write the failing tests**

In the existing discover-and-offer test with the fake 3F API: install the recorder; assert spans `3f.sync` → `3f.offers.reconcile`, and per auction `3f.auction` (with `auction.id`, `adapter.address`, `request.address`) → `3f.offer.decide` → `3f.offer.build` → `3f.offer.submit`; after a successful submit, `s.links.Lookup("req:"+requestAddr)` hits. In the redeem test: pre-remember under the request key, run `redeemAll`, assert `3f.redeem` has one link and `offer.linked_count=1`, `3f.redeem.submit` has `tx.hash`; miss case has zero links and a `link_miss` event.

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement**

1. `discoverAndOffer`: root `3f.sync`; `reconcileOffers` → `3f.offers.reconcile`; inside the per-auction loop: `3f.auction` with the three attributes; auction view build → `3f.auction.view`; strategy → `3f.offer.decide` (`strategy.name`); `buildSignedOffer` → `3f.offer.build`; `submitOfferIfLaneReady`/`createOffer` → `3f.offer.submit`; on success `s.links.Remember("req:"+req.Hex(), ctx, time.Until(expiration)+time.Hour)` and `s.links.Remember(fmt.Sprintf("auction:%s:%d", adapter.Hex(), auctionID), ctx, sameTTL)` (use `strconv`, not `fmt`, if forbidigo objects). Lane-not-ready and strategy declines → `Decline`.
2. `reconcileOffers`: for each listed offer whose status changed, `Lookup("auction:...")` and stamp `quoteTraceId` on that log line (no span link needed there).
3. `redeemAll` → root `3f.redeem`; `readyToRedeem` → `3f.redeem.read`; `redeemReady`: collect links for each request address via `Lookup("req:"+addr)`, start `3f.redeem.submit` with `StartLinked` and `offer.linked_count`, `link_miss` events for misses (one event per missing key), `Send` inside, then `tx.hash`/`tx.outcome` attributes.

- [ ] **Step 4: Test, lint** → `GOTOOLCHAIN=go1.27.1 go test -race ./internal/solvers/bridgefacilitator/... && make lint` → PASS.

- [ ] **Step 5: Commit**

```bash
multigit add internal/solvers/bridgefacilitator
multigit commit -s -m "feat(3f): trace offer and redeem pipelines and link redeems to offers"
```

---

### Task 13: RedStone OEV solver

**Files:**
- Modify: `internal/solvers/redstoneoev/wsclient.go:120-150`, `auction.go:45-240, 301-373`, `strategies/default/monitor.go`, `strategies/default/strategy.go` (stage spans around candidates/size/economics/bundle), `factory.go`
- Test: `auction_test.go` (existing; extend)

**Interfaces:**
- `Solver` gains `links *observability.SpanLinks`; key = normalized auction id; TTL = `reservationTTL`.
- Log key harmonization: the bid path's `"auction"` and the result path's `"id"` both become `"auctionId"`.

- [ ] **Step 1: Write the failing tests**

In the auction handling test with a fake ws client: install the recorder; feed an `AuctionMessage`, assert `oev.auction` (with `auction.id`) → `oev.auction.bid` → strategy stage spans → `oev.auction.send`; then feed an `AuctionResultMessage` with the same id and assert `oev.auction.result` has one link to the `oev.auction` span and `auction.id`. Feed a result for an unknown id: zero links, one `link_miss` event, handler still updates reservations. Assert the dropped-send path (`Send` returns false) produces a `declined` event on `oev.auction.send`.

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement**

1. `wsclient.go` dial: `oev.feed.connect` span + header injection, as in Task 11.
2. `handleAuction`: `ctx, end := tracer.Start(ctx, "oev.auction", AttrAuctionID.String(a.ID))`; `log := TraceLogger(ctx, s.log).WithValues("auctionId", a.ID)`; `buildBidWithContext` → `oev.auction.bid`; inside the default strategy, wrap candidates/sizing/economics/bundle in `oev.auction.candidates`, `oev.auction.size`, `oev.auction.economics`, `oev.auction.bundle`; `ws.Send` → `oev.auction.send`, `Decline(ctx, "dropped", "send_queue_full")` when it returns false; after a successful send `s.links.Remember(a.ID, ctx, reservationTTL)`; every `tooLate`/`bidExpired`/no-candidate path → `Decline`.
3. `handleAuctionResult`, `handleLiquidationResult`, `handleBlacklisted`: `StartLinked(ctx, "oev.auction.result"|"oev.liquidation.result"|"oev.blacklisted", links, AttrAuctionID...)` with `link_miss` on a miss; `tx.hash` on the liquidation result when present; logs use `auctionId`.
4. `monitor.go` tick → root `oev.monitor`.
5. Rename log keys `"auction"` → `"auctionId"` and `"id"` → `"auctionId"` on the touched lines only; update any test that greps those keys.

- [ ] **Step 4: Test, lint** → `GOTOOLCHAIN=go1.27.1 go test -race ./internal/solvers/redstoneoev/... && make lint` → PASS.

- [ ] **Step 5: Commit**

```bash
multigit add internal/solvers/redstoneoev
multigit commit -s -m "feat(oev): trace auctions and link results to bids"
```

---

### Task 14: Docs, config comments, compose, plan sync

**Files:**
- Modify: `README.md` (Observability section, ~line 365), `deploy/docker-compose.yml` (env comments), `config/3f.example.yaml`, `config/lifi.example.yaml`, `config/redstone-oev.example.yaml`, `config/rfq.example.yaml`, `config/uniswapx.example.yaml` (one comment line each under `observability:`), `CLAUDE.md` (logging paragraph), `docs/RFQ-PLAN.md:467-471`
- Create: `docs/TRACING-PLAN.md`
- Modify: `docs/3F-PLAN.md`, `docs/LIFI-PLAN.md`, `docs/OEV-PLAN.md`, `docs/UNISWAPX-PLAN.md`, `docs/TXMANAGER-PLAN.md` (one pointer line each in their architecture section)

- [ ] **Step 1: README**

Under `## Observability`, add a subsection:

```markdown
### OpenTelemetry tracing

Tracing is off unless `OTEL_EXPORTER_ENABLED` is `1`, `true`, `yes`, `on`, or `enabled` (the same
switch the RFQ backend uses). Everything else is the standard OpenTelemetry environment, read by the
SDK: `OTEL_EXPORTER_OTLP_ENDPOINT` (default `http://localhost:4318`, OTLP over HTTP/protobuf),
`OTEL_SERVICE_NAME` (default `vault-solver`; set it per deployment, e.g. `vault-solver-rfq`),
`OTEL_RESOURCE_ATTRIBUTES`, `OTEL_TRACES_SAMPLER` / `OTEL_TRACES_SAMPLER_ARG` (default
`parentbased_always_on`; use `parentbased_traceidratio` to thin background-loop traces without
dropping backend-initiated ones), `OTEL_EXPORTER_OTLP_HEADERS`, and the `OTEL_BSP_*` batch settings.
`OTEL_TRACES_EXPORTER` and `OTEL_EXPORTER_OTLP_PROTOCOL` are ignored: the exporter is always OTLP/HTTP.

An inbound `/quote` continues the backend's trace; every outbound HTTP call, JSON-RPC call, and
transaction carries `traceparent` onward. Log lines under traced work carry `trace_id` and
`span_id`, and Sentry events are tagged with `trace_id`. Spans carry `solver`, `quote.id`,
`order.id`, `tx.hash`, and the other identifiers listed in `docs/TRACING-PLAN.md`. A fill links back
to the quote that produced it when the process still remembers that quote (best effort, in-memory);
after a restart the fill simply starts a new trace.

Tracing never blocks a quote or a fill: spans are exported in the background from a bounded queue,
export failures are logged at Info, and a bad `OTEL_*` setting disables tracing at startup instead
of failing it.
```

Add the two `OTEL_*` lines to the existing `docker run` / compose examples next to `SENTRY_DSN`.

- [ ] **Step 2: Config and compose comments**

Under each example's `observability:` block, add: `# Tracing: set env OTEL_EXPORTER_ENABLED=true and OTEL_EXPORTER_OTLP_ENDPOINT (see README).` In `deploy/docker-compose.yml`, add commented `# OTEL_EXPORTER_ENABLED=true` / `# OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318` lines in the env section.

- [ ] **Step 3: `docs/TRACING-PLAN.md`**

Write it from the spec: §1 purpose, §2 enablement (copy the spec §3 table), §3 architecture (framework helpers, boundaries, txmanager, root loops), §4 span naming and attribute conventions (spec §9), §5 per-solver stage table (spec §9.4), §6 linking (spec §12, including the LiFi "not linked" reasoning and the restart behaviour), §7 overhead rules (spec §13), §10 TODO: `- [ ] backend returns quote trace id on order list (would replace the RFQ in-memory link)`, `- [ ] non-HTTP RPC dial path untraced`, `- [ ] observability listener untraced by design`.

- [ ] **Step 4: Plan pointers and fixes**

In each solver plan's architecture section add: `Tracing: spans, attributes, and quote-to-fill links for this solver are specified in docs/TRACING-PLAN.md §5–§6.` In `docs/RFQ-PLAN.md`, replace the "OpenTelemetry — intentionally not ported" bullet with: `- **OpenTelemetry.** Ported as fleet-wide tracing; see docs/TRACING-PLAN.md. The TS filler's unused @opentelemetry deps were never the reference; the backend's devkit setup was.` In `CLAUDE.md`'s Go style logging bullet add one sentence: `Spans are started at I/O boundaries, loop ticks, and pipeline stages through observability.NewTracer; whoever starts a span derives that scope's logger with observability.TraceLogger so lines carry trace_id.`

- [ ] **Step 5: Final gate on the whole repo**

Run: `make format && make test && make lint && GOTOOLCHAIN=go1.27.1 go build ./...` → all green. Run `GOTOOLCHAIN=go1.27.1 go test ./internal/observability/ -bench StartEnd -run xxx -benchtime 2000x` and paste the two numbers into the commit body.

- [ ] **Step 6: Commit**

```bash
multigit add README.md CLAUDE.md AGENTS.md docs config deploy
multigit commit -s -m "docs: describe opentelemetry tracing and sync plans"
```

---

## Self-review

- **Spec coverage:** §3 → T1/T14; §4 → T1–T5; §5 → T1, T5, T7, T9–T13; §6.1 → T3, T9, T10; §6.2 → T8; §6.3 → T6; §6.4 → T11, T13; §7 → T7; §8/§9.4 → T9–T13 (+ `txmanager.account_poll` in T7); §9.1/§9.3 → T2; §10 → T14; §11 → per-task tests + benchmark in T2; §12 → T4, T9, T10, T12, T13 (LiFi deliberately none); §13 → T1 (never-fail startup, batcher), T2 (no-op benchmark), T4 (bounded map).
- **Type consistency:** `NewTracer(name, solver string) *Tracer`; `Start`/`StartLinked` return `(context.Context, EndFunc)`; `Tracer.Raw()` added in T7 for txmanager's long-lived span; `SpanLinks.Remember(key, ctx, ttl)` / `Lookup(key) (trace.Link, bool)`; attribute constants `observability.Attr*` used by name everywhere; `tracetest.Install(t)` from `internal/observability/tracetest`.
- **Placeholders:** none; solver tasks name the functions, span names, attributes, and link keys, and the stage lists come from spec §9.4.
