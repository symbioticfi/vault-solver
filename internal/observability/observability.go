// Package observability wires the logging backend (zap behind logr), a Prometheus metrics
// registry, and a small HTTP server exposing /metrics, /healthz, and /readyz.
package observability

import (
	"context"
	"net/http"
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

	"github.com/symbioticfi/vault-solver/internal/httpserver"
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

// Metrics owns the Prometheus registry. Solvers register their own collectors on Registerer();
// the framework only records build info here so the package stays solver-agnostic.
type Metrics struct {
	registry  *prometheus.Registry
	buildInfo *prometheus.GaugeVec
}

// NewMetrics creates a registry seeded with build-info.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	buildInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "solver_bot",
			Name:      "build_info",
			Help:      "Build metadata; constant 1, labeled by version and commit.",
		},
		[]string{"version", "commit"},
	)
	// Standard Go runtime + process metrics, so /metrics carries CPU, memory, goroutines, GC, FDs, etc.
	reg.MustRegister(buildInfo, collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return &Metrics{registry: reg, buildInfo: buildInfo}
}

// Registerer lets solvers register their domain metrics on the shared registry.
func (m *Metrics) Registerer() prometheus.Registerer { return m.registry }

// SetBuildInfo records the running build's version and commit.
func (m *Metrics) SetBuildInfo(version, commit string) {
	m.buildInfo.WithLabelValues(version, commit).Set(1)
}

// Health tracks process liveness and readiness for the HTTP probes.
type Health struct {
	ready atomic.Bool
}

// SetReady marks the service ready (readyz returns 200) or not ready (503).
func (h *Health) SetReady(ready bool) { h.ready.Store(ready) }

// NewHTTPServer builds the observability HTTP server. Caller runs ServeUntil.
func NewHTTPServer(addr string, m *Metrics, h *Health) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, http.StatusOK, "ok")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if h.ready.Load() {
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

const shutdownTimeout = 5 * time.Second

// ServeUntil runs srv until ctx is cancelled and returns listener or shutdown failures.
func ServeUntil(ctx context.Context, srv *http.Server) error {
	if err := httpserver.ServeUntil(ctx, srv, shutdownTimeout); err != nil {
		return errors.Errorf("observability server: %w", err)
	}
	return nil
}
