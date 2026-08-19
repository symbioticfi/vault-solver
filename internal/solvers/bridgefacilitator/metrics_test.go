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

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

func newThreeFTestMetrics(t *testing.T) (*threeFMetrics, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	metrics, err := newThreeFMetrics(reg, "")
	if err != nil {
		t.Fatal(err)
	}
	return metrics, reg
}

func requireThreeFObservation(
	t *testing.T,
	reg *prometheus.Registry,
	view string,
	count, timestamp float64,
) {
	t.Helper()
	metricstest.RequireWorkflowState(t, reg, Name, view, count, timestamp)
}

func seedThreeFObservation(metrics *threeFMetrics, view string) {
	metrics.workflow.ObserveStateAt(view, 7, time.Unix(123, 0))
}

func requireThreeFEvent(
	t *testing.T,
	reg *prometheus.Registry,
	event, outcome string,
	count, timestamp float64,
) {
	t.Helper()
	metricstest.RequireWorkflowEvent(t, reg, Name, event, outcome, count, timestamp)
}

func threeFObservationTimestamp(t *testing.T, reg *prometheus.Registry, view string) float64 {
	t.Helper()
	return metricstest.FamilyValue(t, reg, "solver_bot_workflow_last_observation_timestamp", map[string]string{
		"solver": Name, "view": view,
	})
}

func TestThreeFMetricsObserveCompleteState(t *testing.T) {
	m, reg := newThreeFTestMetrics(t)
	s := &Solver{metrics: m, offers: newOfferTracker()}
	m.now = func() time.Time { return time.Unix(123, 0) }

	if !s.reconcileOffers(t.Context(), nil) {
		t.Fatal("empty incremental reconciliation must be complete")
	}
	requireThreeFObservation(t, reg, threeFStateOffers, 0, 0)

	for view, count := range map[string]int{
		threeFStateOffers: 0, threeFStateActiveRequests: 2,
		threeFStateRedeemable: 1, threeFStateTargets: 3,
	} {
		s.observeState(view, count)
	}
	token := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	s.observeSubmittedOffer(token, big.NewInt(1_000), big.NewInt(25))
	s.observeRedeemedRequests(2)

	for view, want := range map[string]float64{
		threeFStateOffers: 0, threeFStateActiveRequests: 2,
		threeFStateRedeemable: 1, threeFStateTargets: 3,
	} {
		requireThreeFObservation(t, reg, view, want, 123)
	}
	requireThreeFEvent(t, reg, threeFEventOffer, "success", 1, 123)
	requireThreeFEvent(t, reg, threeFEventRedeem, "success", 2, 123)
	for kind, want := range map[string]float64{threeFOfferPrincipal: 1_000, threeFOfferExpectedYield: 25} {
		metricstest.RequireWorkflowAmount(
			t, reg, Name, threeFEventOffer, strings.ToLower(token.Hex()), kind, want,
		)
	}
}

func TestThreeFBacklogNonemptySinceTimestampTracksAuthoritativeTransitions(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := newThreeFMetrics(reg, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := testutil.GatherAndCompare(reg, strings.NewReader(`# HELP threef_backlog_nonempty_since_timestamp Unix timestamp when this process first observed a continuous non-empty 3F backlog in complete authoritative snapshots by view; 0 before the first authoritative non-empty observation or after an authoritative empty snapshot. Pair with solver_bot_workflow_last_observation_timestamp; resets on process restart; not an item age.
# TYPE threef_backlog_nonempty_since_timestamp gauge
threef_backlog_nonempty_since_timestamp{view="active_requests"} 0
threef_backlog_nonempty_since_timestamp{view="redeemable"} 0
`), "threef_backlog_nonempty_since_timestamp"); err != nil {
		t.Fatalf("initial backlog metric: %v", err)
	}

	now := time.Unix(100, 0)
	m.now = func() time.Time { return now }
	s := &Solver{metrics: m}
	active := m.backlogNonemptySince.WithLabelValues(threeFStateActiveRequests)
	redeemable := m.backlogNonemptySince.WithLabelValues(threeFStateRedeemable)

	// Neither a partial target source nor an incomplete child scan can start the process-local clock.
	s.observeTargetDerivedState(threeFStateActiveRequests, 3, true)
	s.targetsAuthoritative = true
	s.observeTargetDerivedState(threeFStateRedeemable, 2, false)
	metricstest.RequireValue(t, active, 0)
	metricstest.RequireValue(t, redeemable, 0)

	s.observeTargetDerivedState(threeFStateActiveRequests, 3, true)
	metricstest.RequireValue(t, active, 100)

	// A later complete non-empty observation and an incomplete empty observation both retain the
	// original start of the continuously observed non-empty condition.
	now = time.Unix(200, 0)
	s.observeTargetDerivedState(threeFStateActiveRequests, 5, true)
	s.observeTargetDerivedState(threeFStateActiveRequests, 0, false)
	metricstest.RequireValue(t, active, 100)

	// A partial parent also cannot clear an established condition.
	s.targetsAuthoritative = false
	s.observeTargetDerivedState(threeFStateActiveRequests, 0, true)
	metricstest.RequireValue(t, active, 100)

	s.targetsAuthoritative = true
	s.observeTargetDerivedState(threeFStateActiveRequests, 0, true)
	metricstest.RequireValue(t, active, 0)
	s.observeTargetDerivedState(threeFStateActiveRequests, 1, true)
	s.observeTargetDerivedState(threeFStateRedeemable, 1, true)
	metricstest.RequireValue(t, active, 200)
	metricstest.RequireValue(t, redeemable, 200)
}

func TestRefreshTargetsPublishesOnlyAuthoritativeTargetSnapshots(t *testing.T) {
	t.Run("authorized target", func(t *testing.T) {
		adapter := common.HexToAddress("0x00000000000000000000000000000000000000A0")
		vault := common.HexToAddress("0x00000000000000000000000000000000000000B0")
		signer := common.HexToAddress("0x00000000000000000000000000000000000000C0")
		asset := common.HexToAddress("0x00000000000000000000000000000000000000D0")
		client, stop := newMulticallFakeClient(t,
			abiEncodeAggregate3Results(t,
				abiEncodeAddress(t, vault),
				abiEncodeAddress(t, signer),
				abiEncodeBytes4(t, erc1271MagicValue),
			),
			abiEncodeAggregate3Results(t, abiEncodeAddress(t, asset)),
		)
		defer stop()
		m, reg := newThreeFTestMetrics(t)
		s := &Solver{
			cfg: &Config{Targets: []Target{{Adapter: adapter}}}, reader: newReader(client, common.Address{}),
			log: logr.Discard(), signerAddr: signer, offers: newOfferTracker(), metrics: m,
		}

		if _, err := s.refreshTargets(t.Context()); err != nil {
			t.Fatalf("refresh targets: %v", err)
		}
		if got := threeFObservationTimestamp(t, reg, threeFStateTargets); got <= 0 {
			t.Fatalf("target freshness = %v, want positive timestamp", got)
		}
		metricstest.RequireFamilyValue(t, reg, "solver_bot_workflow_observed_items", map[string]string{
			"solver": Name, "view": threeFStateTargets,
		}, 1)
		if !s.targetsAuthoritative {
			t.Fatal("complete target refresh marked non-authoritative")
		}
	})

	t.Run("successful empty factory", func(t *testing.T) {
		client, stop := newMulticallFakeClient(t, abiEncodeAggregate3Results(t, abiEncodeUint256(t, 0)))
		defer stop()
		m, reg := newThreeFTestMetrics(t)
		seedThreeFObservation(m, threeFStateTargets)
		s := &Solver{
			cfg:    &Config{AdapterFactory: common.HexToAddress("0x00000000000000000000000000000000000000F0")},
			reader: newReader(client, common.Address{}), log: logr.Discard(), offers: newOfferTracker(), metrics: m,
		}

		if _, err := s.refreshTargets(t.Context()); err != nil {
			t.Fatalf("refresh empty targets: %v", err)
		}
		if got := threeFObservationTimestamp(t, reg, threeFStateTargets); got <= 123 {
			t.Fatalf("empty target freshness = %v, want timestamp advanced from 123", got)
		}
		metricstest.RequireFamilyValue(t, reg, "solver_bot_workflow_observed_items", map[string]string{
			"solver": Name, "view": threeFStateTargets,
		}, 0)
		if !s.targetsAuthoritative {
			t.Fatal("successful empty target refresh marked non-authoritative")
		}
	})

	t.Run("failed refresh retains metric", func(t *testing.T) {
		adapter := common.HexToAddress("0x00000000000000000000000000000000000000A0")
		client, stop := newMulticallFakeClient(t, []byte{0x01})
		defer stop()
		m, reg := newThreeFTestMetrics(t)
		seedThreeFObservation(m, threeFStateTargets)
		s := &Solver{
			cfg: &Config{Targets: []Target{{Adapter: adapter}}}, reader: newReader(client, common.Address{}),
			log: logr.Discard(), offers: newOfferTracker(), metrics: m, targetsAuthoritative: true,
		}

		if _, err := s.refreshTargets(t.Context()); err == nil {
			t.Fatal("failed refresh returned nil error")
		}
		requireThreeFObservation(t, reg, threeFStateTargets, 7, 123)
		if s.targetsAuthoritative {
			t.Fatal("failed target refresh retained authoritative provenance")
		}
	})

	t.Run("partial resolution installs safe subset but retains metric", func(t *testing.T) {
		adapter0 := common.HexToAddress("0x00000000000000000000000000000000000000A0")
		adapter1 := common.HexToAddress("0x00000000000000000000000000000000000000A1")
		vault0 := common.HexToAddress("0x00000000000000000000000000000000000000B0")
		signer := common.HexToAddress("0x00000000000000000000000000000000000000C0")
		asset0 := common.HexToAddress("0x00000000000000000000000000000000000000D0")
		round1 := abiEncodeAggregate3CallResults(t, []chain.CallResult{
			{Success: true, ReturnData: abiEncodeAddress(t, vault0)},
			{Success: true, ReturnData: abiEncodeAddress(t, signer)},
			{Success: true, ReturnData: abiEncodeBytes4(t, erc1271MagicValue)},
			{Success: false},
			{Success: true, ReturnData: abiEncodeAddress(t, signer)},
			{Success: true, ReturnData: abiEncodeBytes4(t, erc1271MagicValue)},
		})
		round2 := abiEncodeAggregate3Results(t, abiEncodeAddress(t, asset0))
		client, stop := newMulticallFakeClient(t, round1, round2)
		defer stop()
		m, reg := newThreeFTestMetrics(t)
		seedThreeFObservation(m, threeFStateTargets)
		s := &Solver{
			cfg:    &Config{Targets: []Target{{Adapter: adapter0}, {Adapter: adapter1}}},
			reader: newReader(client, common.Address{}), log: logr.Discard(), signerAddr: signer,
			offers: newOfferTracker(), metrics: m,
		}

		if _, err := s.refreshTargets(t.Context()); err != nil {
			t.Fatalf("partial refresh: %v", err)
		}
		if len(s.targets) != 1 || s.targets[0].Adapter != adapter0 {
			t.Fatalf("runtime targets = %+v, want safe adapter %s", s.targets, adapter0.Hex())
		}
		requireThreeFObservation(t, reg, threeFStateTargets, 7, 123)
		if s.targetsAuthoritative {
			t.Fatal("partial target refresh marked authoritative")
		}
	})
}

func TestTargetDerivedMetricsRequireAuthoritativeTargetSnapshot(t *testing.T) {
	m, reg := newThreeFTestMetrics(t)
	views := []string{threeFStateOffers, threeFStateActiveRequests, threeFStateRedeemable}
	for _, view := range views {
		seedThreeFObservation(m, view)
	}
	s := &Solver{
		cfg:        &Config{Targets: []Target{}},
		metrics:    m,
		operations: m.operations,
		offers:     newOfferTracker(),
		laneReady:  func() bool { return true },
		log:        logr.Discard(),
	}

	s.discoverAndOffer(t.Context())
	s.reconcile(t.Context())
	s.redeemAll(t.Context())
	for _, view := range views {
		requireThreeFObservation(t, reg, view, 7, 123)
	}

	s.targetsAuthoritative = true
	s.discoverAndOffer(t.Context())
	s.reconcile(t.Context())
	s.redeemAll(t.Context())
	for _, view := range views {
		metricstest.RequireFamilyValue(t, reg, "solver_bot_workflow_observed_items", map[string]string{
			"solver": Name, "view": view,
		}, 0)
		if got := threeFObservationTimestamp(t, reg, view); got <= 123 {
			t.Errorf("authoritative-empty %s freshness = %v, want timestamp advanced from 123", view, got)
		}
	}
	if _, err := s.refreshTargets(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{
		targetRefreshOperation, offerRefreshOperation,
		activeRequestRefreshOperation, redeemableRefreshOperation,
	} {
		metricstest.RequireExternalOperationCount(t, reg, Name, operation, "success", 1)
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
						"id": 1, "auctionId": 2, "status": tc.status, "maker": "0x00000000000000000000000000000000000000a0",
						"requestId": "0x00000000000000000000000000000000000000b0", "asset": nil, "vault": nil,
						"amount": tc.amount, "expectedReturn": "1", "nonce": "1", "expiration": tc.expiration, "signature": nil,
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

			metrics, reg := newThreeFTestMetrics(t)
			seedThreeFObservation(metrics, threeFStateOffers)
			adapterAddr := common.HexToAddress("0x00000000000000000000000000000000000000a0")
			s := &Solver{
				api:                  newAPIClient(srv.URL, fakeSigner{}, big.NewInt(1), time.Second, logr.Discard()),
				reader:               newReader(c, common.Address{}),
				log:                  logr.Discard(),
				offers:               newOfferTracker(),
				metrics:              metrics,
				targets:              []Target{{Adapter: adapterAddr}},
				targetsAuthoritative: true,
				laneReady:            func() bool { return true },
			}

			s.discoverAndOffer(t.Context())

			requireThreeFObservation(t, reg, threeFStateOffers, 7, 123)
			if tc.name == "empty status" && len(s.offers.liveEntries(time.Now())) != 1 {
				t.Fatal("empty-status offer was dropped from conservative live coverage")
			}
		})
	}
}
