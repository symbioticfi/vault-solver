package bridgefacilitator

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	threeFStateOffers         = "offers"
	threeFStateActiveRequests = "active_requests"
	threeFStateRedeemable     = "redeemable"
	threeFOfferPrincipal      = "principal"
	threeFOfferExpectedYield  = "expected_yield"
)

type threeFMetrics struct {
	offerSubmissions *prometheus.CounterVec
	offerAmounts     *prometheus.CounterVec
	liveOffers       prometheus.Gauge
	activeRequests   prometheus.Gauge
	redeemable       prometheus.Gauge
	lastObservation  *prometheus.GaugeVec
}

func newThreeFMetrics(reg prometheus.Registerer) (*threeFMetrics, error) {
	m := &threeFMetrics{
		offerSubmissions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "threef_offer_submissions_total",
			Help: "3F offers submitted to the API by result.",
		}, []string{"result"}),
		offerAmounts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "threef_offer_amount_atomic_units_total",
			Help: "Principal and quoted expected yield in successfully submitted 3F offers, in deposit-token atomic units.",
		}, []string{"token", "kind"}),
		liveOffers: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "threef_live_offers",
			Help: "Live offers in the last complete 3F API reconciliation.",
		}),
		activeRequests: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "threef_active_requests",
			Help: "Active requests in the last complete on-chain reconciliation.",
		}),
		redeemable: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "threef_redeemable_requests",
			Help: "Redeemable requests in the last complete on-chain scan.",
		}),
		lastObservation: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "threef_last_successful_observation_timestamp",
			Help: "Unix timestamp of the last complete observation by view.",
		}, []string{"view"}),
	}
	for _, collector := range []prometheus.Collector{
		m.offerSubmissions, m.offerAmounts, m.liveOffers, m.activeRequests, m.redeemable, m.lastObservation,
	} {
		if err := reg.Register(collector); err != nil {
			return nil, errors.Errorf("bridgefacilitator: register metric: %w", err)
		}
	}
	for _, view := range []string{threeFStateOffers, threeFStateActiveRequests, threeFStateRedeemable} {
		m.lastObservation.WithLabelValues(view)
	}
	return m, nil
}

func (s *Solver) observeOfferSubmission(result string) {
	if s.metrics != nil {
		s.metrics.offerSubmissions.WithLabelValues(result).Inc()
	}
}

func (s *Solver) observeSubmittedOffer(token common.Address, principal, expectedYield *big.Int) {
	if s.metrics == nil {
		return
	}
	s.metrics.offerSubmissions.WithLabelValues("success").Inc()
	if token == (common.Address{}) {
		return
	}
	tokenLabel := strings.ToLower(token.Hex())
	addBigCounter(s.metrics.offerAmounts.WithLabelValues(tokenLabel, threeFOfferPrincipal), principal)
	addBigCounter(s.metrics.offerAmounts.WithLabelValues(tokenLabel, threeFOfferExpectedYield), expectedYield)
}

func addBigCounter(counter prometheus.Counter, amount *big.Int) {
	if amount == nil || amount.Sign() <= 0 {
		return
	}
	value, _ := new(big.Float).SetInt(amount).Float64()
	counter.Add(value)
}

func (s *Solver) observeState(view string, count int) {
	if s.metrics == nil {
		return
	}
	switch view {
	case threeFStateOffers:
		s.metrics.liveOffers.Set(float64(count))
	case threeFStateActiveRequests:
		s.metrics.activeRequests.Set(float64(count))
	case threeFStateRedeemable:
		s.metrics.redeemable.Set(float64(count))
	}
	s.metrics.lastObservation.WithLabelValues(view).SetToCurrentTime()
}
