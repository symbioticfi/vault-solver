package lifi

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/symbioticfi/vault-solver/api/lifiorder"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

func TestQuotePairSortPreservesDecimalSeparatorOrder(t *testing.T) {
	for _, tc := range []struct {
		name        string
		left, right quotePairKey
		want        int
	}{
		{name: "input prefix includes separator", left: quotePairKey{fromDecimals: 10, toDecimals: 6}, right: quotePairKey{fromDecimals: 1, toDecimals: 6}, want: -1},
		{name: "input decimals remain lexical", left: quotePairKey{fromDecimals: 18}, right: quotePairKey{fromDecimals: 6}, want: -1},
		{name: "last field has no separator", left: quotePairKey{fromDecimals: 18, toDecimals: 1}, right: quotePairKey{fromDecimals: 18, toDecimals: 10}, want: -1},
		{name: "equal fields", left: quotePairKey{fromDecimals: 18, toDecimals: 6}, right: quotePairKey{fromDecimals: 18, toDecimals: 6}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := compareQuotePairs(tc.left, tc.right); got != tc.want {
				t.Fatalf("comparison = %d, want %d", got, tc.want)
			}
		})
	}
}

type fakeQuoteSubmitter struct {
	calls [][]types.Quote
	err   error
}

func TestQuoteExpiryBoundsUsesPublishedExpiries(t *testing.T) {
	earliest, latest := quoteExpiryBounds([]types.Quote{{Expiry: 30}, {Expiry: 10}, {Expiry: 20}})
	if earliest != 10 || latest != 30 {
		t.Fatalf("expiry bounds = %d..%d, want 10..30", earliest, latest)
	}
}

type recordingQuoteStrategy struct {
	inputs []types.QuoteInput
}

func (s *recordingQuoteStrategy) DecideQuotes(
	_ context.Context,
	input types.QuoteInput,
) (types.QuoteOutput, error) {
	s.inputs = append(s.inputs, input)
	return types.QuoteOutput{}, nil
}

func (*recordingQuoteStrategy) DecideFill(context.Context, types.FillInput) (*types.FillPlan, error) {
	return nil, nil
}

func TestRefreshQuotesWithoutGasAccounting(t *testing.T) {
	cfg := testLifiConfig()
	cfg.Gas = nil
	strategy := &recordingQuoteStrategy{}
	reg := prometheus.NewRegistry()
	metrics, err := newLIFIMetrics(reg, nil, "")
	testcheck.NoError(t, err)
	feeReads := 0
	solver := &Solver{
		cfg: cfg, reader: fakeLifiReader{}, strategy: strategy, log: logr.Discard(),
		now:         func(context.Context) (time.Time, error) { return time.Unix(1_700_000_000, 0), nil },
		wallNow:     func() time.Time { return time.Unix(1_700_000_001, 0) },
		txLaneState: alwaysReadyTransactionLane(),
		metrics:     metrics,
		maxFeePerGas: func(context.Context) (*big.Int, error) {
			feeReads++
			return big.NewInt(7), nil
		},
	}

	solver.refreshQuotes(t.Context(), nil, newQuoteState(time.Second))

	if feeReads != 0 {
		t.Fatalf("max fee reads = %d, want 0", feeReads)
	}
	if len(strategy.inputs) != 1 {
		t.Fatalf("strategy quote inputs = %d, want 1", len(strategy.inputs))
	}
	input := strategy.inputs[0]
	if input.MaxFeePerGas == nil || input.MaxFeePerGas.Sign() != 0 {
		t.Fatalf("strategy max fee per gas = %v, want zero", input.MaxFeePerGas)
	}
	if input.GasSnapshot != nil || input.GasPrices != nil {
		t.Fatalf("strategy gas facts = snapshot %v prices %v, want nil", input.GasSnapshot, input.GasPrices)
	}
	metricstest.RequireExternalOperationCount(t, reg, Name, quoteRefreshOperation, "success", 1)
}

func TestFilterQuoteInventoryAppliesTokenScope(t *testing.T) {
	permissioned := common.HexToAddress("0x1111111111111111111111111111111111111111")
	permissionless := common.HexToAddress("0x2222222222222222222222222222222222222222")
	inventory := []liquidlane.Inventory{
		{Route: liquidlane.Route{TokenIn: permissioned}},
		{Route: liquidlane.Route{TokenIn: permissionless}},
	}

	tests := []struct {
		name  string
		scope tokenpolicy.Scope
		want  common.Address
	}{
		{"permissioned", tokenpolicy.Permissioned, permissioned},
		{"permissionless", tokenpolicy.Permissionless, permissionless},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterQuoteInventory(inventory, testTokenPolicy(t, tt.scope, permissioned))
			if len(filtered) != 1 || filtered[0].TokenIn != tt.want {
				t.Fatalf("filtered inventory = %+v", filtered)
			}
		})
	}
}

func (f *fakeQuoteSubmitter) submitQuotes(_ context.Context, quotes []types.Quote) error {
	copyOfQuotes := append([]types.Quote(nil), quotes...)
	f.calls = append(f.calls, copyOfQuotes)
	return f.err
}

func TestQuoteStatePublishesAndReplacesChangedTopology(t *testing.T) {
	routeItem := testQuoteRoute()
	state := newQuoteState(30 * time.Second)
	submitter := &fakeQuoteSubmitter{}
	now := time.Unix(1_800_000_000, 0)

	first := testStandingQuote(routeItem, 1_000)
	removed, err := state.reconcile(context.Background(), submitter, []types.Quote{first}, now)
	testcheck.NoError(t, err, "first reconcile: %v")
	if removed != 0 || len(submitter.calls) != 1 || len(submitter.calls[0][0].Ranges) == 0 {
		t.Fatalf("initial reconcile: removed=%d calls=%#v", removed, submitter.calls)
	}

	second := testStandingQuote(routeItem, 1_000)
	second.Ranges[0].Quote = "0.98"
	if _, err := state.reconcile(context.Background(), submitter, []types.Quote{second}, now); err != nil {
		t.Fatalf("same topology reconcile: %v", err)
	}
	if len(submitter.calls) != 2 || submitter.calls[1][0].Ranges[0].Quote != "0.98" {
		t.Fatalf("changed price calls = %#v", submitter.calls)
	}

	changed := testStandingQuote(routeItem, 2_000)
	removed, err = state.reconcile(context.Background(), submitter, []types.Quote{changed}, now)
	testcheck.NoError(t, err, "changed topology reconcile: %v")
	if removed != 0 || len(submitter.calls) != 3 || submitter.calls[2][0].Ranges[0].MaxAmount.String() != "2000" {
		t.Fatalf("changed topology: removed=%d calls=%#v", removed, submitter.calls)
	}
}

func TestQuoteStateSkipsUnchangedPairUntilRenewalWindow(t *testing.T) {
	routeItem := testQuoteRoute()
	state := newQuoteState(30 * time.Second)
	submitter := &fakeQuoteSubmitter{}
	now := time.Unix(1_800_000_000, 0)
	quote := testStandingQuote(routeItem, 1_000)

	if _, err := state.reconcile(context.Background(), submitter, []types.Quote{quote}, now); err != nil {
		t.Fatalf("publish: %v", err)
	}
	refreshed := testStandingQuote(routeItem, 1_000)
	refreshed.Expiry += 60
	if _, err := state.reconcile(context.Background(), submitter, []types.Quote{refreshed}, now.Add(30*time.Second)); err != nil {
		t.Fatalf("unchanged: %v", err)
	}
	if len(submitter.calls) != 1 {
		t.Fatalf("unchanged pair was republished: calls=%d", len(submitter.calls))
	}
	if _, err := state.reconcile(context.Background(), submitter, []types.Quote{refreshed}, now.Add(90*time.Second)); err != nil {
		t.Fatalf("renew: %v", err)
	}
	if len(submitter.calls) != 2 || submitter.calls[1][0].Ranges[0].MaxAmount.String() != "1000" {
		t.Fatalf("renew calls = %#v", submitter.calls)
	}
}

func TestQuoteStateNeedsRenewalWithoutNewBlock(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	state := newQuoteState(30 * time.Second)
	state.active[quotePairKey{}] = quotePairState{expiry: now.Add(time.Minute).Unix()}

	if state.needsRenewal(now) {
		t.Fatal("quote entered renewal window too early")
	}
	if !state.needsRenewal(now.Add(30 * time.Second)) {
		t.Fatal("quote should renew by wall clock even without a new block")
	}
}

func TestShouldRefreshQuotesInBlockMode(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	state := newQuoteState(30 * time.Second)
	state.active[quotePairKey{}] = quotePairState{expiry: now.Add(time.Minute).Unix()}
	solver := &Solver{
		cfg:     &Config{QuoteRefreshMode: quoteRefreshModeBlock},
		reader:  fakeLifiReader{latestBlock: 10},
		wallNow: func() time.Time { return now },
		log:     logr.Discard(),
	}
	lastBlock := uint64(10)

	if solver.shouldRefreshQuotes(context.Background(), state, &lastBlock) {
		t.Fatal("unchanged block outside renewal window should not refresh")
	}
	solver.reader = fakeLifiReader{latestBlock: 11}
	if !solver.shouldRefreshQuotes(context.Background(), state, &lastBlock) || lastBlock != 11 {
		t.Fatalf("new block was not observed: lastBlock=%d", lastBlock)
	}
	state.active[quotePairKey{}] = quotePairState{expiry: now.Add(30 * time.Second).Unix()}
	solver.reader = fakeLifiReader{latestBlockErr: errors.New("rpc unavailable")}
	if !solver.shouldRefreshQuotes(context.Background(), state, &lastBlock) {
		t.Fatal("renewal should proceed when the block-number read fails")
	}
}

func TestQuoteStateRemovesPairWhenStrategyStopsQuoting(t *testing.T) {
	routeItem := testQuoteRoute()
	state := newQuoteState(30 * time.Second)
	submitter := &fakeQuoteSubmitter{}
	now := time.Unix(1_800_000_000, 0)
	if _, err := state.reconcile(context.Background(), submitter, []types.Quote{testStandingQuote(routeItem, 1_000)}, now); err != nil {
		t.Fatalf("publish: %v", err)
	}
	submitter.calls = nil

	removed, err := state.reconcile(context.Background(), submitter, nil, now)
	testcheck.NoError(t, err, "remove: %v")
	if removed != 1 || len(submitter.calls) != 1 || len(submitter.calls[0]) != 1 ||
		len(submitter.calls[0][0].Ranges) == 0 || submitter.calls[0][0].Expiry >= now.Unix() {
		t.Fatalf("remove: removed=%d calls=%#v", removed, submitter.calls)
	}
}

func TestQuoteStateExpiresPairAfterUnknownPublishOutcome(t *testing.T) {
	routeItem := testQuoteRoute()
	state := newQuoteState(30 * time.Second)
	submitter := &fakeQuoteSubmitter{err: errors.New("lost response")}
	now := time.Unix(1_800_000_000, 0)

	if _, err := state.reconcile(
		context.Background(),
		submitter,
		[]types.Quote{testStandingQuote(routeItem, 1_000)},
		now,
	); err == nil {
		t.Fatal("publish unexpectedly succeeded")
	}
	if len(state.active) != 1 {
		t.Fatalf("uncertain active pairs = %d, want 1", len(state.active))
	}
	for _, pair := range state.active {
		if pair.expiry != 0 {
			t.Fatalf("uncertain pair expiry = %d, want forced renewal", pair.expiry)
		}
	}

	submitter.err = nil
	submitter.calls = nil
	removed, err := state.reconcile(context.Background(), submitter, nil, now)
	testcheck.NoError(t, err, "expire uncertain pair: %v")
	if removed != 1 || len(state.active) != 0 || len(submitter.calls) != 1 ||
		len(submitter.calls[0]) != 1 || submitter.calls[0][0].Expiry >= now.Unix() {
		t.Fatalf(
			"expire uncertain pair: removed=%d active=%d calls=%#v",
			removed,
			len(state.active),
			submitter.calls,
		)
	}
}

func TestQuoteStateRetriesExpireAfterPartialSubmitAcknowledgement(t *testing.T) {
	var calls int
	var expiries []int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var dto lifiorder.SubmitQuotesDto
		if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
			t.Errorf("decode submit quotes: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if len(dto.Quotes) != 1 || len(dto.Quotes[0].Ranges) != 1 {
			t.Errorf("submitted quotes = %#v, want one quote with one range", dto.Quotes)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		calls++
		expiries = append(expiries, dto.Quotes[0].Expiry)
		w.Header().Set("Content-Type", "application/json")
		if calls == 2 {
			_, _ = w.Write([]byte(`{"status":"success","quotesAdded":0}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","quotesAdded":1}`))
	}))
	defer srv.Close()

	state := newQuoteState(30 * time.Second)
	client := newOrderClient(srv.URL, "test-key", time.Second, 11155111)
	now := time.Unix(1_800_000_000, 0)
	quote := testStandingQuote(testQuoteRoute(), 1_000)
	if _, err := state.reconcile(context.Background(), client, []types.Quote{quote}, now); err != nil {
		t.Fatalf("publish: %v", err)
	}

	removed, err := state.reconcile(context.Background(), client, nil, now)
	if err == nil || !strings.Contains(err.Error(), "quotesAdded 0, want 1") {
		t.Fatalf("first expire error = %v, want acknowledgement mismatch", err)
	}
	if removed != 1 || len(state.active) != 1 {
		t.Fatalf("failed expire: removed=%d active=%d, want 1/1", removed, len(state.active))
	}

	removed, err = state.reconcile(context.Background(), client, nil, now)
	testcheck.NoError(t, err, "retry expire: %v")
	if removed != 1 || len(state.active) != 0 || calls != 3 {
		t.Fatalf("retried expire: removed=%d active=%d calls=%d, want 1/0/3", removed, len(state.active), calls)
	}
	if len(expiries) != 3 || int64(expiries[1]) >= now.Unix() || int64(expiries[2]) >= now.Unix() {
		t.Fatalf("submitted expiries = %v, want both retry attempts expired", expiries)
	}
}

func TestLIFIMetricsRecordSuccessfulRefresh(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := newLIFIMetrics(reg, newOrderFeed(OrderServerConfig{}, "", logr.Discard()), "")
	testcheck.NoError(t, err)
	quoteRoute := testQuoteRoute()
	state := newQuoteState(time.Minute)
	state.active = indexQuotePairs([]types.Quote{
		testStandingQuote(quoteRoute, 1_000),
		testStandingQuote(quoteRoute, 2_000),
		testStandingQuote(quoteRoute, 3_000),
	})
	(&Solver{metrics: metrics}).observeQuoteRefresh(state)
	if _, err := reg.Gather(); err != nil {
		t.Fatalf("gather quote metrics: %v", err)
	}

	firstRefresh := metricstest.FamilyValue(t, reg, "lifi_last_successful_refresh_timestamp", nil)
	metricstest.RequireFamilyValue(t, reg, "lifi_active_quotes", nil, 3)
	metricstest.RequireFamilyValue(t, reg, "lifi_active_quote_ranges", nil, 3)
	metricstest.RequireFamilyValue(t, reg, "lifi_active_quote_max_input_atomic_units", map[string]string{
		"token_in": strings.ToLower(quoteRoute.TokenIn.Hex()), "token_out": strings.ToLower(quoteRoute.TokenOut.Hex()),
		"token_in_decimals": "6", "token_out_decimals": "6",
	}, 3000)
	if firstRefresh <= 0 {
		t.Fatal("missing successful refresh timestamp")
	}
	(&Solver{metrics: metrics}).observeQuoteRefresh(newQuoteState(time.Minute))
	metricstest.RequireFamilyValue(t, reg, "lifi_active_quotes", nil, 0)
	metricstest.RequireFamilyValue(t, reg, "lifi_active_quote_ranges", nil, 0)
	if testutil.CollectAndCount(metrics.quotes, "lifi_active_quote_max_input_atomic_units") != 0 ||
		metricstest.FamilyValue(t, reg, "lifi_last_successful_refresh_timestamp", nil) < firstRefresh {
		t.Fatal("unexpected empty quote metrics")
	}
}

func TestSuspendQuotesPublishesRetiredQuoteMetrics(t *testing.T) {
	var submissions int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var dto lifiorder.SubmitQuotesDto
		if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
			t.Errorf("decode submit quotes: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		ranges := 0
		for _, quote := range dto.Quotes {
			ranges += len(quote.Ranges)
		}
		submissions++
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"status": "success", "quotesAdded": ranges,
		}); err != nil {
			t.Errorf("encode submit response: %v", err)
		}
	}))
	defer srv.Close()

	reg := prometheus.NewRegistry()
	metrics, err := newLIFIMetrics(reg, nil, "")
	testcheck.NoError(t, err)
	now := time.Unix(1_800_000_000, 0)
	client := newOrderClient(srv.URL, "test-key", time.Second, 11155111)
	state := newQuoteState(30 * time.Second)
	if _, err := state.reconcile(
		t.Context(),
		client,
		[]types.Quote{testStandingQuote(testQuoteRoute(), 1_000)},
		now,
	); err != nil {
		t.Fatalf("publish quote: %v", err)
	}
	metrics.quotes.observe(state)
	observed := *metrics.quotes.current.Load()
	observed.updatedAt = 1
	metrics.quotes.current.Store(&observed)
	solver := &Solver{
		orders: client, metrics: metrics, wallNow: func() time.Time { return now }, log: logr.Discard(),
	}

	solver.suspendQuotes(t.Context(), state)

	if submissions != 2 {
		t.Fatalf("quote submissions = %d, want publish plus suspension", submissions)
	}
	if got := state.activeQuoteCount(); got != 0 {
		t.Fatalf("active quote state = %d, want 0", got)
	}
	if metricstest.FamilyValue(t, reg, "lifi_active_quotes", nil) != 0 ||
		metricstest.FamilyValue(t, reg, "lifi_last_successful_refresh_timestamp", nil) <= 1 {
		t.Fatal("unexpected suspended quote metrics")
	}
}

func testQuoteRoute() route {
	return liquidlane.NewRoute(
		11155111,
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
		common.HexToAddress("0x3333333333333333333333333333333333333333"),
		common.HexToAddress("0x4444444444444444444444444444444444444444"),
		6,
		6,
	)
}

func testStandingQuote(route route, maxAmount int64) types.Quote {
	return types.Quote{
		FromAsset: route.TokenIn, ToAsset: route.TokenOut,
		FromDecimals: route.TokenInDecimals, ToDecimals: route.TokenOutDecimals,
		Ranges: []types.QuoteRange{{
			MinAmount: big.NewInt(100), MaxAmount: big.NewInt(maxAmount), Quote: "0.99",
		}},
		Expiry: 1_800_000_120,
	}
}
