package observability

import (
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
)

// MetricGroup owns construction of one component's collectors. Publish attaches
// the complete group in one registry operation, so failed startup cannot leave
// a subset registered. Construction is single-threaded; collectors own updates.
type MetricGroup struct {
	*prometheus.Registry

	prefix string
	err    error
}

func NewMetricGroup(prefix string) *MetricGroup {
	return &MetricGroup{Registry: prometheus.NewRegistry(), prefix: prefix}
}

func (g *MetricGroup) Counter(name, help string, labels ...string) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: g.prefix + name, Help: help}, labels)
	g.Add(c)
	return c
}

func (g *MetricGroup) Gauge(name, help string, labels ...string) *prometheus.GaugeVec {
	c := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: g.prefix + name, Help: help}, labels)
	g.Add(c)
	return c
}

func (g *MetricGroup) Histogram(name, help string, buckets []float64, labels ...string) *prometheus.HistogramVec {
	c := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: g.prefix + name, Help: help, Buckets: buckets,
	}, labels)
	g.Add(c)
	return c
}

func (g *MetricGroup) Add(collectors ...prometheus.Collector) {
	for _, collector := range collectors {
		if err := g.Register(collector); err != nil {
			g.err = errors.Join(g.err, err)
		}
	}
}

func (g *MetricGroup) Publish(parent prometheus.Registerer) error {
	if g.err != nil {
		return errors.Errorf("construct metric group: %w", g.err)
	}
	if parent == nil {
		return errors.New("metric group registerer is required")
	}
	if err := parent.Register(g.Registry); err != nil {
		return errors.Errorf("publish metric group: %w", err)
	}
	return nil
}
