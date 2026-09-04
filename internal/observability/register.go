package observability

import (
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
)

// RegisterCollectors registers one subsystem's collectors and adds ownership to failures.
func RegisterCollectors(reg prometheus.Registerer, owner string, collectors ...prometheus.Collector) error {
	for _, collector := range collectors {
		if err := reg.Register(collector); err != nil {
			return errors.Errorf("%s: register metric: %w", owner, err)
		}
	}
	return nil
}
