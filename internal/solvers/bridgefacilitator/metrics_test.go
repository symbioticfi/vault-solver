package bridgefacilitator

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetrics_RecordLifecycle(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := newMetrics(reg)
	if err != nil {
		t.Fatalf("newMetrics: %v", err)
	}

	m.observeDiscover(3)
	m.incOfferSubmitted()
	m.incOfferSubmitFailed()
	m.addCacheLoaded(2)
	m.incCacheRebuildFailed()
	m.setLiveOffers(4)
	m.addRedeemable(5)
	m.addRedeemed(3)
	m.incRedeemScanFailed()
	m.incRedeemTxFailed()

	if got := testutil.ToFloat64(m.discoverPasses); got != 1 {
		t.Fatalf("threef_discover_passes_total = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.auctionsDiscovered); got != 3 {
		t.Fatalf("threef_auctions_discovered_total = %v, want 3", got)
	}
	if got := testutil.ToFloat64(m.offersSubmitted); got != 1 {
		t.Fatalf("threef_offers_submitted_total = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.offerSubmitFailures); got != 1 {
		t.Fatalf("threef_offer_submit_failures_total = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.cacheLoaded); got != 2 {
		t.Fatalf("threef_offer_cache_loaded_total = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.cacheRebuildFailures); got != 1 {
		t.Fatalf("threef_offer_cache_rebuild_failures_total = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.liveOffers); got != 4 {
		t.Fatalf("threef_live_offers = %v, want 4", got)
	}
	if got := testutil.ToFloat64(m.redeemableRequests); got != 5 {
		t.Fatalf("threef_redeemable_requests_total = %v, want 5", got)
	}
	if got := testutil.ToFloat64(m.redeemedRequests); got != 3 {
		t.Fatalf("threef_redeemed_requests_total = %v, want 3", got)
	}
	if got := testutil.ToFloat64(m.redeemScanFailures); got != 1 {
		t.Fatalf("threef_redeem_scan_failures_total = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.redeemTxFailures); got != 1 {
		t.Fatalf("threef_redeem_tx_failures_total = %v, want 1", got)
	}
}

func TestMetrics_NilSafe(t *testing.T) {
	var m *metrics
	m.observeDiscover(1)
	m.incOfferSubmitted()
	m.incOfferSubmitFailed()
	m.addCacheLoaded(1)
	m.incCacheRebuildFailed()
	m.setLiveOffers(1)
	m.addRedeemable(1)
	m.addRedeemed(1)
	m.incRedeemScanFailed()
	m.incRedeemTxFailed()
}
