package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics isolates the process registry from Prometheus global state.
type Metrics struct {
	registry   *prometheus.Registry
	solverInfo *prometheus.GaugeVec
	health     Health
}

func NewMetrics(version, commit string) *Metrics {
	registry := prometheus.NewRegistry()
	build := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "solver_bot",
		Name:      "build_info",
		Help:      "Build metadata; constant 1, labeled by version and commit.",
	}, []string{"version", "commit"})
	build.WithLabelValues(version, commit).Set(1)
	solvers := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "solver_bot",
		Name:      "solver_info",
		Help:      "Configured solver membership; constant 1 for each solver in this runtime.",
	}, []string{"solver"})
	metrics := &Metrics{registry: registry, solverInfo: solvers}
	ready := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "solver_bot",
		Name:      "service_ready",
		Help:      "1 when the process admits work through its readiness gate; 0 otherwise.",
	}, func() float64 {
		if metrics.health.Ready() {
			return 1
		}
		return 0
	})
	registry.MustRegister(
		build,
		solvers,
		ready,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return metrics
}

func (m *Metrics) Registerer() prometheus.Registerer {
	return m.registry
}

func (m *Metrics) SetSolvers(names []string) {
	for _, name := range names {
		m.solverInfo.WithLabelValues(name).Set(1)
	}
}

func (m *Metrics) SetReady(ready bool) { m.health.SetReady(ready) }

func (m *Metrics) Ready() bool { return m.health.Ready() }
