package lifi

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

type fakeQuoteSubmitter struct {
	calls [][]types.Quote
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
	return nil
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

func TestCapacityChangesRequestImmediateQuoteRefresh(t *testing.T) {
	solver := &Solver{quoteRefresh: make(chan struct{}, 1)}
	reservations := liquidlane.CapacityReservations{"capacity-1": big.NewInt(400)}

	solver.reserve("order-1", reservations)
	select {
	case <-solver.quoteRefresh:
	default:
		t.Fatal("reservation did not request quote refresh")
	}
	if got := solver.capacity.Snapshot()["capacity-1"]; got == nil || got.String() != "400" {
		t.Fatalf("reserved capacity = %v, want 400", got)
	}

	solver.releaseReservation("order-1")
	select {
	case <-solver.quoteRefresh:
	default:
		t.Fatal("reservation release did not request quote refresh")
	}
	if got := solver.capacity.Snapshot()["capacity-1"]; got != nil {
		t.Fatalf("released capacity = %v, want nil", got)
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

func TestLIFIMetricsRecordSuccessfulRefresh(t *testing.T) {
	metrics, err := newLIFIMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	(&Solver{metrics: metrics}).observeQuoteRefresh(3)

	if got := testutil.ToFloat64(metrics.activeQuotes); got != 3 {
		t.Fatalf("active quotes = %v, want 3", got)
	}
	if got := testutil.ToFloat64(metrics.lastRefresh); got <= 0 {
		t.Fatalf("last refresh = %v, want a current timestamp", got)
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
