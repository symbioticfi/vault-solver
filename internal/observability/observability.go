// Package observability wires the logging backend (zap behind logr), a Prometheus metrics
// registry, and a small HTTP server exposing /metrics, /healthz, and /readyz.
package observability

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger builds the production (JSON) zap logger behind the logr interface and returns a flush
// func. When debug is true the level is lowered to Debug so logr V(1) calls are emitted; otherwise
// they're dropped. main is the only place that should reference a concrete logging backend.
func NewLogger(debug bool) (logr.Logger, func()) {
	cfg := zap.NewProductionConfig()
	if debug {
		cfg.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	}
	zl, err := cfg.Build()
	if err != nil {
		return logr.Discard(), func() {}
	}
	// Optional Sentry sink: when SENTRY_DSN is set, tee Error+ entries to Sentry. Disabled otherwise.
	sentrySink, flushSentry := initSentry()
	if sentrySink != nil {
		zl = zl.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return zapcore.NewTee(core, sentrySink)
		}))
	}
	return zapr.NewLogger(zl), func() { _ = zl.Sync(); flushSentry() }
}

// Metrics owns the Prometheus registry. Solvers register their domain collectors on Registerer();
// the framework records only integration-neutral process and external-dependency signals here.
type Metrics struct {
	registry     *prometheus.Registry
	buildInfo    *prometheus.GaugeVec
	solverInfo   *prometheus.GaugeVec
	serviceReady prometheus.GaugeFunc
	health       *Health
}

// NewMetrics creates a registry seeded with framework and standard process metrics. Readiness is
// returned as a separate framework-owned capability so solver-facing Metrics cannot mutate it.
func NewMetrics() (*Metrics, *Health) {
	reg := prometheus.NewRegistry()
	health := &Health{}
	buildInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "solver_bot",
			Name:      "build_info",
			Help:      "Build metadata; constant 1, labeled by version and commit.",
		},
		[]string{"version", "commit"},
	)
	solverInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "solver_bot",
			Name:      "solver_info",
			Help:      "Configured solver membership; constant 1 for each solver in this runtime.",
		},
		[]string{"solver"},
	)
	serviceReady := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "solver_bot",
		Name:      "service_ready",
		Help:      "1 when the process admits work through its readiness gate; 0 otherwise.",
	}, health.readyValue)
	// Standard Go runtime + process metrics, so /metrics carries CPU, memory, goroutines, GC, FDs, etc.
	reg.MustRegister(
		buildInfo,
		solverInfo,
		serviceReady,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	metrics := &Metrics{
		registry:     reg,
		buildInfo:    buildInfo,
		solverInfo:   solverInfo,
		serviceReady: serviceReady,
		health:       health,
	}
	return metrics, health
}

// Registerer lets solvers register their domain metrics on the shared registry.
func (m *Metrics) Registerer() prometheus.Registerer { return m.registry }

// SetBuildInfo records the running build's version and commit.
func (m *Metrics) SetBuildInfo(version, commit string) {
	m.buildInfo.WithLabelValues(version, commit).Set(1)
}

// SetSolvers records the bounded, config-time solver membership of this runtime.
func (m *Metrics) SetSolvers(names []string) {
	for _, name := range names {
		m.solverInfo.WithLabelValues(name).Set(1)
	}
}

// ExternalOperationOutcome is the bounded result of an external dependency operation.
// Its zero value is invalid and is recorded as ExternalOperationError.
type ExternalOperationOutcome uint8

const (
	ExternalOperationSuccess ExternalOperationOutcome = iota + 1
	ExternalOperationDegraded
	ExternalOperationSkipped
	ExternalOperationError
)

var externalOperationOutcomeLabels = [...]string{
	ExternalOperationSuccess:  "success",
	ExternalOperationDegraded: "degraded",
	ExternalOperationSkipped:  "skipped",
	ExternalOperationError:    "error",
}

// OperationObserver records one pre-bound solver/operation pair. All outcome series are also
// pre-bound, so observing cannot introduce label values at runtime.
type OperationObserver struct {
	observers [ExternalOperationError + 1]prometheus.Observer
}

// Observe records a duration against a bounded outcome. Invalid outcome values fail closed into
// the pre-bound error series instead of creating a new label value.
func (o *OperationObserver) Observe(outcome ExternalOperationOutcome, duration time.Duration) {
	if o == nil {
		return
	}
	if outcome < ExternalOperationSuccess || outcome > ExternalOperationError {
		outcome = ExternalOperationError
	}
	o.observers[outcome].Observe(duration.Seconds())
}

// OperationTimer records one operation exactly once. A canceled context maps error or degraded to
// skipped, while success keeps its terminal classification.
type OperationTimer struct {
	observer  *OperationObserver
	startedAt time.Time
	once      sync.Once
}

// StartOperation starts a timer for an already-bound operation observer.
func StartOperation(observer *OperationObserver) *OperationTimer {
	return &OperationTimer{observer: observer, startedAt: time.Now()}
}

// Finish records the terminal outcome and elapsed time. Repeated calls are ignored.
func (t *OperationTimer) Finish(ctx context.Context, outcome ExternalOperationOutcome) {
	if t == nil {
		return
	}
	t.once.Do(func() {
		ObserveOperation(ctx, t.observer, outcome, time.Since(t.startedAt))
	})
}

// ObserveOperation records a measured operation duration with the shared cancellation rule. It is
// useful when one operation's measured I/O phases are separated by work that must stay outside its timer.
func ObserveOperation(
	ctx context.Context,
	observer *OperationObserver,
	outcome ExternalOperationOutcome,
	duration time.Duration,
) {
	if ctx != nil && ctx.Err() != nil &&
		(outcome == ExternalOperationError || outcome == ExternalOperationDegraded) {
		outcome = ExternalOperationSkipped
	}
	observer.Observe(outcome, duration)
}

// Health tracks process liveness and readiness for the HTTP probes.
type Health struct {
	ready atomic.Bool
}

// SetReady marks the service ready (readyz returns 200) or not ready (503).
func (h *Health) SetReady(ready bool) { h.ready.Store(ready) }

func (h *Health) readyValue() float64 {
	if h.ready.Load() {
		return 1
	}
	return 0
}

// NewHTTPServer builds the observability HTTP server. Caller runs ListenAndServe and Shutdown.
func NewHTTPServer(addr string, m *Metrics) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, http.StatusOK, "ok")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if m.health.ready.Load() {
			writeText(w, http.StatusOK, "ready")
			return
		}
		writeText(w, http.StatusServiceUnavailable, "not ready")
	})
	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func writeText(w http.ResponseWriter, code int, body string) {
	w.WriteHeader(code)
	_, _ = w.Write([]byte(body))
}

// ServeUntil runs srv until ctx is cancelled, then shuts it down gracefully. Returns nil on a
// clean shutdown; logs (does not crash on) an unexpected serve error.
func ServeUntil(ctx context.Context, srv *http.Server, log logr.Logger) {
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	select {
	case <-ctx.Done():
	case err := <-errCh:
		log.Error(err, "observability server failed")
	}
	// Fresh context on purpose: the parent ctx is already cancelled here, so deriving from it
	// would abort the graceful drain immediately.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx) //nolint:contextcheck // fresh deadline for post-cancellation drain
}
