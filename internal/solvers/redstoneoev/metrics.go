package redstoneoev

import (
	"time"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics are the OEV solver's collectors, registered on the shared Prometheus registry (served at
// the framework's /metrics). All methods are nil-safe so the solver runs unmetered when no registry
// is provided.
type metrics struct {
	auctions       prometheus.Counter
	bids           prometheus.Counter
	wins           prometheus.Counter
	failedLiq      prometheus.Counter
	skips          *prometheus.CounterVec
	hotPath        prometheus.Histogram
	gasRatio       prometheus.Histogram
	deposit        prometheus.Gauge
	callbackNative prometheus.Gauge
	depositLow     prometheus.Gauge // 1 when the deposit is below the on-chain MIN_DEPOSIT floor
}

func newMetrics(reg prometheus.Registerer) (*metrics, error) {
	m := &metrics{
		auctions:  prometheus.NewCounter(prometheus.CounterOpts{Name: "oev_auctions_total", Help: "OEV auction frames seen."}),
		bids:      prometheus.NewCounter(prometheus.CounterOpts{Name: "oev_bids_total", Help: "Bids sent (or would-bid in dry-run)."}),
		wins:      prometheus.NewCounter(prometheus.CounterOpts{Name: "oev_wins_total", Help: "Auctions won (auction-result names our callback)."}),
		failedLiq: prometheus.NewCounter(prometheus.CounterOpts{Name: "oev_failed_liquidations_total", Help: "Reverted settlements for our callback (from the WS liquidation-result frame)."}),
		skips: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "oev_skips_total", Help: "Auctions not bid on, by reason.",
		}, []string{"reason"}),
		hotPath: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "oev_hotpath_seconds", Help: "handleAuction wall-clock (the ~400ms budget).",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.4, 1},
		}),
		gasRatio: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "oev_settlement_gas_actual_predicted_ratio", Help: "Actual receipt gasUsed divided by predicted settlement gas units.",
			Buckets: []float64{0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 3, 5},
		}),
		deposit:        prometheus.NewGauge(prometheus.GaugeOpts{Name: "oev_deposit_wei", Help: "Signer's Executor deposit (wei)."}),
		callbackNative: prometheus.NewGauge(prometheus.GaugeOpts{Name: "oev_callback_native_wei", Help: "Callback contract native balance (wei)."}),
		depositLow:     prometheus.NewGauge(prometheus.GaugeOpts{Name: "oev_deposit_below_floor", Help: "1 when the Executor deposit is below the on-chain MIN_DEPOSIT floor."}),
	}
	for _, c := range []prometheus.Collector{m.auctions, m.bids, m.wins, m.failedLiq, m.skips, m.hotPath, m.gasRatio, m.deposit, m.callbackNative, m.depositLow} {
		if err := reg.Register(c); err != nil {
			return nil, errors.Errorf("redstoneoev: register metric: %w", err)
		}
	}
	return m, nil
}

func (m *metrics) auction() {
	if m != nil {
		m.auctions.Inc()
	}
}

func (m *metrics) bid() {
	if m != nil {
		m.bids.Inc()
	}
}

func (m *metrics) won() {
	if m != nil {
		m.wins.Inc()
	}
}

func (m *metrics) failed() {
	if m != nil {
		m.failedLiq.Inc()
	}
}

func (m *metrics) skip(reason string) {
	if m != nil {
		m.skips.WithLabelValues(reason).Inc()
	}
}

func (m *metrics) latency(d time.Duration) {
	if m != nil {
		m.hotPath.Observe(d.Seconds())
	}
}

func (m *metrics) settlementGas(predicted, actual uint64) {
	if m != nil && predicted > 0 {
		m.gasRatio.Observe(float64(actual) / float64(predicted))
	}
}

func (m *metrics) balances(depositWei, callbackWei float64) {
	if m != nil {
		m.deposit.Set(depositWei)
		m.callbackNative.Set(callbackWei)
	}
}

// depositBelowFloor sets the alarm gauge; "below" now means deposit < MIN_DEPOSIT.
func (m *metrics) depositBelowFloor(below bool) {
	if m != nil {
		v := 0.0
		if below {
			v = 1
		}
		m.depositLow.Set(v)
	}
}
