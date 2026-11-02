package lifi

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

func pairEventLabels(chainID int64, in, out common.Address, event, result, reason string) map[string]string {
	return map[string]string{"solver": Name, "chain_id": strconv.FormatInt(chainID, 10), "token_in": strings.ToLower(in.Hex()), "token_out": strings.ToLower(out.Hex()), "event": event, "result": result, "reason": reason}
}

func pairSeriesCount(t *testing.T, reg *prometheus.Registry) int {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() == "solver_bot_pair_events_total" {
			return len(family.GetMetric())
		}
	}
	return 0
}

func newPairTestSolver(t *testing.T) (*Solver, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	m, err := newLIFIMetrics(reg, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	cfg := testLifiConfig()
	cfg.QuoteTTL = time.Minute
	cfg.QuoteInterval = time.Second
	return &Solver{cfg: cfg, chainID: 11155111, metrics: m}, reg
}

func TestPairMetricsRequireTrustedDirectionAndRetainNewPairs(t *testing.T) {
	s, reg := newPairTestSolver(t)
	r := testQuoteRoute()
	s.setPairRoutes([]route{r})
	// No metadata cache is needed to count a structurally supported zero-capacity direction.
	s.observeStandingQuoteRejection(t.Context(), r.TokenIn, r.TokenOut, "strategy_declined", nil)
	for i := int64(1); i <= 1000; i++ {
		s.observeStandingQuoteRejection(t.Context(), common.BigToAddress(big.NewInt(i)), r.TokenOut, "arbitrary private error", nil)
	}
	s.observeStandingQuoteRejection(t.Context(), r.TokenOut, r.TokenIn, "strategy_declined", nil)
	if got := pairSeriesCount(t, reg); got != 1 {
		t.Fatalf("unsupported inputs allocated series: %d", got)
	}
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairEventLabels(s.chainID, r.TokenIn, r.TokenOut, "standing_quote", "rejected", "strategy_declined"), 1)
	next := r
	next.TokenIn = common.HexToAddress("0x1234")
	s.setPairRoutes([]route{r, next})
	s.observeStandingQuoteRejection(t.Context(), next.TokenIn, next.TokenOut, "strategy_declined", []liquidlane.Inventory{{Route: next}})
	// An inventory refresh does not reset prior counters; missing metadata does not exclude a new route.
	s.observeStandingQuoteRejection(t.Context(), r.TokenIn, r.TokenOut, "strategy_declined", nil)
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairEventLabels(s.chainID, r.TokenIn, r.TokenOut, "standing_quote", "rejected", "strategy_declined"), 2)
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairEventLabels(s.chainID, next.TokenIn, next.TokenOut, "standing_quote", "rejected", "strategy_declined"), 1)
	s.cfg.TokenPolicy = testTokenPolicy(t, tokenpolicy.Permissioned, next.TokenIn)
	s.observeStandingQuoteRejection(t.Context(), r.TokenIn, r.TokenOut, "publish_error", []liquidlane.Inventory{{Route: r}})
	if got := pairSeriesCount(t, reg); got != 2 {
		t.Fatalf("policy-denied direction allocated series: %d", got)
	}
}

func TestStandingPairMetricsCountAcknowledgedChangesAndActualRetryFailures(t *testing.T) {
	s, reg := newPairTestSolver(t)
	r := testQuoteRoute()
	s.setPairRoutes([]route{r})
	fail := false
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		var request quoteSubmissionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if fail {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		ranges := 0
		for _, q := range request.Quotes {
			ranges += len(q.Ranges)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "quotesAdded": ranges})
	}))
	defer server.Close()
	s.orders = newOrderClient(server.URL, "test-key", time.Second, s.chainID)
	q := testStandingQuote(r, 100)
	state := newQuoteState(time.Second)
	now := time.Unix(1_800_000_000, 0)
	inventory := []liquidlane.Inventory{{Route: r}}
	for range 2 {
		if _, err := s.reconcileQuotes(t.Context(), state, []types.Quote{q}, inventory, now); err != nil {
			t.Fatal(err)
		}
	}
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairEventLabels(s.chainID, r.TokenIn, r.TokenOut, "standing_quote", "published", "none"), 1)
	if calls != 1 {
		t.Fatalf("unchanged refresh published: %d", calls)
	}
	fail = true
	for range 2 {
		if _, err := s.reconcileQuotes(t.Context(), state, nil, nil, now); err == nil {
			t.Fatal("withdrawal unexpectedly succeeded")
		}
	}
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairEventLabels(s.chainID, r.TokenIn, r.TokenOut, "standing_quote", "rejected", "withdrawal_error"), 2)
	fail = false
	if _, err := s.reconcileQuotes(t.Context(), state, nil, nil, now); err != nil {
		t.Fatal(err)
	}
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairEventLabels(s.chainID, r.TokenIn, r.TokenOut, "standing_quote", "withdrawn", "none"), 1)
	fail = true
	if _, err := s.reconcileQuotes(t.Context(), state, []types.Quote{q}, inventory, now); err == nil {
		t.Fatal("publication unexpectedly succeeded")
	}
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairEventLabels(s.chainID, r.TokenIn, r.TokenOut, "standing_quote", "rejected", "publish_error"), 1)
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairEventLabels(s.chainID, r.TokenIn, r.TokenOut, "standing_quote", "published", "none"), 1)
	if calls != 5 {
		t.Fatalf("calls=%d want5", calls)
	}
}

func TestMatchedPairMetricsScopeAndExistingInboxDedup(t *testing.T) {
	s, reg := newPairTestSolver(t)
	r := testQuoteRoute()
	s.setPairRoutes([]route{r})
	owned := testSubmittedOrder(t, s.cfg, r.TokenIn, r.TokenOut)
	owned.matchSource = "quote_exclusive_for"
	q := newOrderInbox(10)
	q.onAdmit = func(order *submittedOrder) { s.observeMatchedOrderAdmission(t.Context(), order) }
	for range 2 {
		if err := q.enqueue(owned); err != nil {
			t.Fatal(err)
		}
	}
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairEventLabels(s.chainID, r.TokenIn, r.TokenOut, "assignment", "admitted", "none"), 1)
	for _, pair := range [][2]common.Address{{r.TokenOut, r.TokenIn}, {common.HexToAddress("0xbeef"), r.TokenOut}} {
		order := *owned
		order.TokenIn, order.TokenOut = pair[0], pair[1]
		s.observeMatchedOrderAdmission(t.Context(), &order)
	}
	s.cfg.TokenPolicy = testTokenPolicy(t, tokenpolicy.Permissioned, common.HexToAddress("0xbeef"))
	s.observeMatchedOrderAdmission(t.Context(), owned)
	if got := pairSeriesCount(t, reg); got != 1 {
		t.Fatalf("unsupported admission allocated series: %d", got)
	}
	// A payload on another chain is rejected by ingress before the queue/admission observer.
	raw := mutatedTestOrderJSON(t, s.cfg, func(body map[string]any) {
		mapField(t, body, "order")["originChainId"] = "1"
		body["quote"] = map[string]any{"exclusiveFor": s.cfg.Executor.Hex()}
	})
	parsed, err := s.parseOrderMessage(t.Context(), orderMessage{Event: orderSubmitEvent, Data: raw})
	if err != nil || parsed != nil {
		t.Fatalf("foreign chain parse=%v error=%v", parsed, err)
	}
	if got := pairSeriesCount(t, reg); got != 1 {
		t.Fatalf("foreign-chain payload allocated series: %d", got)
	}
}

// Exercise the real refresh caller: a direct helper invocation with nil inventory cannot prove
// that a zero-capacity direction is actually enumerated at this observation boundary.
func TestRefreshPairMetricsIncludeResolvedZeroCapacityAndRefreshFailures(t *testing.T) {
	for _, reason := range []string{"no_inventory", "strategy_declined", "strategy_error", "lane_unavailable", "chain_time_error", "snapshot_error", "gas_price_error", "policy_denied"} {
		t.Run(reason, func(t *testing.T) {
			s, reg := newPairTestSolver(t)
			r := testQuoteRoute()
			s.setPairRoutes([]route{r, r})
			s.cfg.Gas = nil
			s.now = func(context.Context) (time.Time, error) { return time.Unix(1_800_000_000, 0), nil }
			s.wallNow = func() time.Time { return time.Unix(1_800_000_000, 0) }
			ready := true
			s.txLaneState = &testTransactionLaneState{ready: func() bool { return ready }}
			reader := rejectionTestReader{}
			strategy := rejectionTestStrategy{}
			switch reason {
			case "strategy_declined":
				reader.snapshot.Direct = []liquidlane.Inventory{{Route: r}, {Route: r}}
			case "strategy_error":
				reader.snapshot.Direct = []liquidlane.Inventory{{Route: r}}
				strategy.err = errors.New("strategy failed")
			case "lane_unavailable":
				ready = false
			case "chain_time_error":
				s.now = func(context.Context) (time.Time, error) { return time.Time{}, errors.New("clock failed") }
			case "snapshot_error":
				reader.err = errors.New("snapshot failed")
			case "gas_price_error":
				s.cfg.Gas = testLifiConfig().Gas
				s.maxFeePerGas = func(context.Context) (*big.Int, error) { return nil, errors.New("fee failed") }
			case "policy_denied":
				s.cfg.TokenPolicy = testTokenPolicy(t, tokenpolicy.Permissioned, common.HexToAddress("0xbeef"))
			}
			s.reader, s.strategy = reader, strategy
			s.refreshQuotes(t.Context(), s.currentPairRoutes(), newQuoteState(time.Second))
			if reason == "policy_denied" {
				if count := pairSeriesCount(t, reg); count != 0 {
					t.Fatalf("denied direction allocated %d series", count)
				}
				return
			}
			metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairEventLabels(s.chainID, r.TokenIn, r.TokenOut, "standing_quote", "rejected", reason), 1)
			if count := pairSeriesCount(t, reg); count != 1 {
				t.Fatalf("duplicate routes/refresh failure allocated %d series", count)
			}
		})
	}
}

type pairScopeReader struct {
	fakeLifiReader

	read func(context.Context) ([]route, error)
}

func (r pairScopeReader) resolveRoutes(ctx context.Context, _ []common.Address) ([]route, error) {
	return r.read(ctx)
}

func TestPairScopeRemovalReaddAndZeroInventory(t *testing.T) {
	s, reg := newPairTestSolver(t)
	r := testQuoteRoute()
	changedOutput, changedUnits, added := r, r, r
	changedOutput.TokenOut = common.HexToAddress("0x5678")
	changedUnits.TokenInDecimals++
	added.TokenIn = common.HexToAddress("0x9876")
	trading := []route{r}
	var catalog []route
	s.reader = pairScopeReader{read: func(context.Context) ([]route, error) { return catalog, nil }}
	for _, tc := range []struct {
		routes    []route
		supported bool
	}{
		{[]route{r}, true}, {nil, false}, {[]route{changedOutput}, false},
		{[]route{changedUnits}, false}, {[]route{r, added}, true}, {[]route{r}, true},
	} {
		catalog = slices.Clone(tc.routes)
		s.refreshPairRoutes(t.Context(), trading)
		stale := []liquidlane.Inventory{{Route: r}}
		if got := s.pairScopeSource(r.TokenIn, r.TokenOut, stale) != ""; got != tc.supported {
			t.Fatalf("scope=%v want=%v catalog=%v", got, tc.supported, tc.routes)
		}
		if !slices.Equal(catalog, tc.routes) || !slices.Equal(trading, []route{r}) {
			t.Fatal("observation mutated reader or trading routes")
		}
		if s.pairScopeSource(r.TokenOut, r.TokenIn, nil) != "" || s.pairScopeSource(added.TokenIn, added.TokenOut, nil) != "" {
			t.Fatal("reverse or newly advertised trading direction admitted")
		}
		s.observeStandingQuoteRejection(t.Context(), r.TokenIn, r.TokenOut, "no_inventory", stale)
	}
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairEventLabels(s.chainID, r.TokenIn, r.TokenOut, "standing_quote", "rejected", "no_inventory"), 3)
	if pairSeriesCount(t, reg) != 1 {
		t.Fatal("route refresh reset or expanded counter inventory")
	}
}

func TestPairScopeDiscoveryLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name      string
		delay     time.Duration
		err       error
		callsAt37 int32
	}{
		{"slow success", 1200 * time.Millisecond, nil, 2}, // Four serial300ms reads exceeded the old1s budget.
		{"error", 0, errors.New("offline"), 3}, {"hung", time.Minute, nil, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s, reg := newPairTestSolver(t)
				s.cfg.QuoteTTL, s.cfg.QuoteInterval = 36*time.Second, time.Second
				r := testQuoteRoute()
				var calls atomic.Int32
				s.reader = pairScopeReader{read: func(ctx context.Context) ([]route, error) {
					calls.Add(1)
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(tc.delay):
					}
					return []route{r}, tc.err
				}}
				s.setPairRoutes([]route{r})
				original := s.pairRoutes.Load()
				s.observeStandingQuoteRejection(t.Context(), r.TokenIn, r.TokenOut, "no_inventory", nil)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				done := make(chan struct{})
				go func() { defer close(done); s.observePairRoutes(ctx, []route{r}) }()
				time.Sleep(11 * time.Second)
				synctest.Wait()
				if calls.Load() != 0 {
					t.Fatal("discovery duplicated startup or used block cadence")
				}
				time.Sleep(26 * time.Second)
				synctest.Wait()
				renewed := tc.name == "slow success"
				if calls.Load() != tc.callsAt37 || (s.pairRoutes.Load() != original) != renewed {
					t.Fatal("read budget, renewal or after-completion spacing regressed")
				}
				if renewed {
					time.Sleep(40 * time.Second)
					synctest.Wait()
				} // Beyond two proofTTLs.
				if (s.pairScopeSource(r.TokenIn, r.TokenOut, nil) != "") != renewed {
					t.Fatal("proof renewal/expiry regressed")
				}
				before := calls.Load()
				for i := int64(1); i <= 1000; i++ {
					s.observeStandingQuoteRejection(ctx, common.BigToAddress(big.NewInt(i)), r.TokenOut, "no_inventory", nil)
				}
				if before > 6 || calls.Load() != before || pairSeriesCount(t, reg) != 1 {
					t.Fatal("request-time discovery or untrusted series growth")
				}
				cancel()
				synctest.Wait()
				select {
				case <-done:
				default:
					t.Fatal("optional discovery ignored cancellation")
				}
			})
		})
	}
}
