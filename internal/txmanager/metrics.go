package txmanager

import (
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	replacementKindReplacement  = "replacement"
	replacementKindCancellation = "cancellation"
)

// Metrics records the transaction lifecycle shared by every solver.
type Metrics struct {
	requests     *prometheus.CounterVec
	inflight     *prometheus.GaugeVec
	gasUsed      *prometheus.CounterVec
	replacements *prometheus.CounterVec
}

// NewMetrics registers transaction lifecycle collectors.
func NewMetrics(reg prometheus.Registerer) (*Metrics, error) {
	if reg == nil {
		return nil, errors.New("txmanager: metrics registerer is required")
	}
	m := &Metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "solver_bot",
			Subsystem: "txmanager",
			Name:      "requests_total",
			Help:      "Logical transaction requests by terminal outcome.",
		}, []string{"label", "outcome"}),
		inflight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "solver_bot",
			Subsystem: "txmanager",
			Name:      "inflight",
			Help:      "Accepted transaction requests awaiting a terminal result.",
		}, []string{"label"}),
		gasUsed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "solver_bot",
			Subsystem: "txmanager",
			Name:      "gas_used_total",
			Help:      "Gas used by mined transaction receipts.",
		}, []string{"label", "outcome"}),
		replacements: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "solver_bot",
			Subsystem: "txmanager",
			Name:      "replacements_total",
			Help:      "Successfully broadcast transaction replacements and cancellations.",
		}, []string{"label", "kind"}),
	}
	for _, collector := range []prometheus.Collector{
		m.requests,
		m.inflight,
		m.gasUsed,
		m.replacements,
	} {
		if err := reg.Register(collector); err != nil {
			return nil, errors.Errorf("txmanager: register metric: %w", err)
		}
	}
	return m, nil
}

func (m *Metrics) requestStarted(label string) {
	if m != nil {
		m.inflight.WithLabelValues(label).Inc()
	}
}

func (m *Metrics) requestFinished(label string, outcome Outcome, receipt *types.Receipt) {
	if m == nil {
		return
	}
	m.requests.WithLabelValues(label, string(outcome)).Inc()
	m.inflight.WithLabelValues(label).Dec()
	if receipt != nil {
		m.gasUsed.WithLabelValues(label, string(outcome)).Add(float64(receipt.GasUsed))
	}
}

func (m *Metrics) replacement(label, kind string) {
	if m != nil {
		m.replacements.WithLabelValues(label, kind).Inc()
	}
}
