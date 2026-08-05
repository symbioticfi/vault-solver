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

	"github.com/symbioticfi/vault-solver/api/lifiorder"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

type fakeQuoteSubmitter struct {
	calls [][]types.Quote
	err   error
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
	if err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
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
	if err != nil {
		t.Fatalf("changed topology reconcile: %v", err)
	}
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
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
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
	if err != nil {
		t.Fatalf("expire uncertain pair: %v", err)
	}
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
	if err != nil {
		t.Fatalf("retry expire: %v", err)
	}
	if removed != 1 || len(state.active) != 0 || calls != 3 {
		t.Fatalf("retried expire: removed=%d active=%d calls=%d, want 1/0/3", removed, len(state.active), calls)
	}
	if len(expiries) != 3 || int64(expiries[1]) >= now.Unix() || int64(expiries[2]) >= now.Unix() {
		t.Fatalf("submitted expiries = %v, want both retry attempts expired", expiries)
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
