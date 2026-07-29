package bridgefacilitator

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestThreeFMetricsObserveCompleteState(t *testing.T) {
	m, err := newThreeFMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	s := &Solver{metrics: m, offers: newOfferTracker()}

	if !s.reconcileOffers(t.Context(), nil) {
		t.Fatal("empty incremental reconciliation must be complete")
	}
	if got := testutil.ToFloat64(m.lastObservation.WithLabelValues(threeFStateOffers)); got != 0 {
		t.Fatalf("incremental hydration published freshness: %v", got)
	}

	s.observeState(threeFStateOffers, 0)
	s.observeState(threeFStateActiveRequests, 2)
	s.observeState(threeFStateRedeemable, 1)
	token := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	s.observeSubmittedOffer(token, big.NewInt(1_000), big.NewInt(25))

	if got := testutil.ToFloat64(m.liveOffers); got != 0 {
		t.Fatalf("live offers = %v, want 0", got)
	}
	if got := testutil.ToFloat64(m.activeRequests); got != 2 {
		t.Fatalf("active requests = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.redeemable); got != 1 {
		t.Fatalf("redeemable = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.offerSubmissions.WithLabelValues("success")); got != 1 {
		t.Fatalf("offer submissions = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.offerAmounts.WithLabelValues(
		strings.ToLower(token.Hex()), threeFOfferPrincipal,
	)); got != 1_000 {
		t.Fatalf("offered principal = %v, want 1000", got)
	}
	if got := testutil.ToFloat64(m.offerAmounts.WithLabelValues(
		strings.ToLower(token.Hex()), threeFOfferExpectedYield,
	)); got != 25 {
		t.Fatalf("offered expected yield = %v, want 25", got)
	}
	for _, view := range []string{threeFStateOffers, threeFStateActiveRequests, threeFStateRedeemable} {
		if got := testutil.ToFloat64(m.lastObservation.WithLabelValues(view)); got <= 0 {
			t.Fatalf("last observation for %s = %v, want positive timestamp", view, got)
		}
	}
}
