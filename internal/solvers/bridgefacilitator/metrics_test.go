package bridgefacilitator

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
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

	for view, want := range map[string]float64{
		threeFStateOffers:         0,
		threeFStateActiveRequests: 2,
		threeFStateRedeemable:     1,
	} {
		if got := testutil.ToFloat64(m.observedItems.WithLabelValues(view)); got != want {
			t.Fatalf("observed items for %s = %v, want %v", view, got, want)
		}
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

func TestDiscoverAndOfferMalformedOfferRetainsLastCompleteMetric(t *testing.T) {
	t.Parallel()

	validExpiration := new(big.Int).SetInt64(time.Now().Add(time.Hour).Unix()).String()
	tests := []struct {
		name       string
		status     string
		amount     string
		expiration string
	}{
		{"empty status", "", "100", validExpiration},
		{"malformed expiration", "CREATED", "100", "not-a-unix-timestamp"},
		{"malformed amount", "CREATED", "not-a-uint256", validExpiration},
		{"negative amount", "CREATED", "-1", validExpiration},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/auction":
					_ = json.NewEncoder(w).Encode([]any{})
				case "/v1/offer":
					_ = json.NewEncoder(w).Encode([]map[string]any{{
						"id":             1,
						"auctionId":      2,
						"status":         tc.status,
						"maker":          "0x00000000000000000000000000000000000000a0",
						"requestId":      "0x00000000000000000000000000000000000000b0",
						"asset":          nil,
						"vault":          nil,
						"amount":         tc.amount,
						"expectedReturn": "1",
						"nonce":          "1",
						"expiration":     tc.expiration,
						"signature":      nil,
					}})
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			liquidityRound := abiEncodeAggregate3Results(t,
				abiEncodeUint256(t, 1_000),
				abiEncodeUint256(t, 0),
				abiEncodeUint256(t, 0),
				abiEncodeUint256(t, 1_000),
				abiEncodeUint256(t, 0),
			)
			c, stop := newMulticallFakeClient(t, liquidityRound)
			defer stop()

			metrics, err := newThreeFMetrics(prometheus.NewRegistry())
			if err != nil {
				t.Fatal(err)
			}
			metrics.observedItems.WithLabelValues(threeFStateOffers).Set(7)
			metrics.lastObservation.WithLabelValues(threeFStateOffers).Set(123)
			adapterAddr := common.HexToAddress("0x00000000000000000000000000000000000000a0")
			s := &Solver{
				api:     newAPIClient(srv.URL, fakeSigner{}, big.NewInt(1), time.Second, logr.Discard()),
				reader:  newReader(c, common.Address{}),
				log:     logr.Discard(),
				offers:  newOfferTracker(),
				metrics: metrics,
				targets: []Target{{Adapter: adapterAddr}},
			}

			s.discoverAndOffer(t.Context())

			if got := testutil.ToFloat64(metrics.observedItems.WithLabelValues(threeFStateOffers)); got != 7 {
				t.Fatalf("offers gauge = %v, want retained 7", got)
			}
			if got := testutil.ToFloat64(metrics.lastObservation.WithLabelValues(threeFStateOffers)); got != 123 {
				t.Fatalf("offers freshness = %v, want retained 123", got)
			}
			if tc.name == "empty status" && len(s.offers.liveEntries(time.Now())) != 1 {
				t.Fatal("empty-status offer was dropped from conservative live coverage")
			}
		})
	}
}
