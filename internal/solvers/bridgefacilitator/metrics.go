package bridgefacilitator

import (
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics are the 3F solver's collectors, registered on the shared Prometheus registry (served at
// the framework's /metrics). All methods are nil-safe so the solver runs unmetered when no registry
// is provided.
type metrics struct {
	discoverPasses       prometheus.Counter
	auctionsDiscovered   prometheus.Counter
	offersSubmitted      prometheus.Counter
	offerSubmitFailures  prometheus.Counter
	cacheLoaded          prometheus.Counter
	cacheRebuildFailures prometheus.Counter
	liveOffers           prometheus.Gauge
	redeemableRequests   prometheus.Counter
	redeemedRequests     prometheus.Counter
	redeemScanFailures   prometheus.Counter
	redeemTxFailures     prometheus.Counter
}

func newMetrics(reg prometheus.Registerer) (*metrics, error) {
	m := &metrics{
		discoverPasses: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "threef_discover_passes_total",
			Help: "Successful 3F auction discovery passes.",
		}),
		auctionsDiscovered: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "threef_auctions_discovered_total",
			Help: "Auctions returned by the 3F API across successful discovery passes.",
		}),
		offersSubmitted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "threef_offers_submitted_total",
			Help: "3F offers successfully submitted.",
		}),
		offerSubmitFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "threef_offer_submit_failures_total",
			Help: "3F offer submission attempts that failed at the API boundary.",
		}),
		cacheLoaded: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "threef_offer_cache_loaded_total",
			Help: "Live offers loaded from the 3F API into the local dedup cache.",
		}),
		cacheRebuildFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "threef_offer_cache_rebuild_failures_total",
			Help: "Per-adapter 3F offer-cache rebuild failures.",
		}),
		liveOffers: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "threef_live_offers",
			Help: "Current number of live offers in the local dedup cache.",
		}),
		redeemableRequests: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "threef_redeemable_requests_total",
			Help: "Redeemable 3F requests found across redeem scans before batch capping.",
		}),
		redeemedRequests: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "threef_redeemed_requests_total",
			Help: "3F requests successfully finalized on-chain.",
		}),
		redeemScanFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "threef_redeem_scan_failures_total",
			Help: "Redeem scans that failed before building a finalize transaction.",
		}),
		redeemTxFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "threef_redeem_tx_failures_total",
			Help: "Finalize-request transaction attempts that failed.",
		}),
	}
	for _, c := range []prometheus.Collector{
		m.discoverPasses, m.auctionsDiscovered, m.offersSubmitted, m.offerSubmitFailures,
		m.cacheLoaded, m.cacheRebuildFailures, m.liveOffers, m.redeemableRequests,
		m.redeemedRequests, m.redeemScanFailures, m.redeemTxFailures,
	} {
		if err := reg.Register(c); err != nil {
			return nil, errors.Errorf("bridgefacilitator: register metric: %w", err)
		}
	}
	return m, nil
}

func (m *metrics) observeDiscover(auctions int) {
	if m != nil {
		m.discoverPasses.Inc()
		m.auctionsDiscovered.Add(float64(auctions))
	}
}

func (m *metrics) incOfferSubmitted() {
	if m != nil {
		m.offersSubmitted.Inc()
	}
}

func (m *metrics) incOfferSubmitFailed() {
	if m != nil {
		m.offerSubmitFailures.Inc()
	}
}

func (m *metrics) addCacheLoaded(live int) {
	if m != nil {
		m.cacheLoaded.Add(float64(live))
	}
}

func (m *metrics) incCacheRebuildFailed() {
	if m != nil {
		m.cacheRebuildFailures.Inc()
	}
}

func (m *metrics) setLiveOffers(live int) {
	if m != nil {
		m.liveOffers.Set(float64(live))
	}
}

func (m *metrics) addRedeemable(found int) {
	if m != nil {
		m.redeemableRequests.Add(float64(found))
	}
}

func (m *metrics) addRedeemed(count int) {
	if m != nil {
		m.redeemedRequests.Add(float64(count))
	}
}

func (m *metrics) incRedeemScanFailed() {
	if m != nil {
		m.redeemScanFailures.Inc()
	}
}

func (m *metrics) incRedeemTxFailed() {
	if m != nil {
		m.redeemTxFailures.Inc()
	}
}
