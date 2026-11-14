package rfq

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
)

func TestNativeQuoteDiscoveryFiltersAdapters(t *testing.T) {
	backend := &fakeBackend{discounts: &discountsResponse{Discounts: []discountListItem{
		{Adapter: vlt.Hex()}, {Adapter: tIn.Hex(), DiscountPrecision: "100000000"},
		{Adapter: "invalid"}, {Adapter: (common.Address{}).Hex()},
	}}}
	warmer := &recordingQuoteWarmer{fakeRecoveryReader: &fakeRecoveryReader{}}
	s := &Solver{exec: &executionService{backend: backend, reader: warmer}}
	if err := s.refreshNativeQuoteAdapters(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(warmer.adapters) != 1 || warmer.adapters[0] != vlt {
		t.Fatalf("discovery adapters = %v", warmer.adapters)
	}
}

type recordingQuoteWarmer struct {
	*fakeRecoveryReader

	adapters []common.Address
	notified chan struct{}
}

func (r *recordingQuoteWarmer) warmQuoteAdapters(_ context.Context, adapters []common.Address) error {
	r.adapters = adapters
	if r.notified != nil {
		select {
		case r.notified <- struct{}{}:
		default:
		}
	}
	return nil
}

func TestNativeQuoteDiscoveryRefreshesOutsideRequestsAndStops(t *testing.T) {
	backend := &fakeBackend{discounts: &discountsResponse{Discounts: []discountListItem{{Adapter: vlt.Hex()}}}}
	warmer := &recordingQuoteWarmer{fakeRecoveryReader: &fakeRecoveryReader{}, notified: make(chan struct{}, 1)}
	s := &Solver{cfg: &Config{PollInterval: time.Millisecond}, exec: &executionService{backend: backend, reader: warmer}}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); s.refreshNativeQuoteAdaptersLoop(ctx) }()
	select {
	case <-warmer.notified:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("background discovery did not refresh without a quote")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("background discovery did not stop")
	}
	if backend.listCalls == 0 || len(warmer.adapters) != 1 || warmer.adapters[0] != vlt {
		t.Fatal("background discovery lost the listed adapter")
	}
}

type unavailableDiscoveryBackend struct{ *fakeBackend }

func (*unavailableDiscoveryBackend) listDiscounts(context.Context) (*discountsResponse, error) {
	return nil, errors.New("backend unavailable")
}

func TestQuoteOnlyInitializesNativeAccountsBeforeServing(t *testing.T) {
	s := &Solver{
		cfg:    &Config{SolverMode: solverModeInternal, QuoteOnly: true},
		exec:   &executionService{backend: &unavailableDiscoveryBackend{}, reader: &fakeRecoveryReader{}},
		server: testServer(),
	}
	err := s.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "initialize quote accounts: discover quote adapters: backend unavailable") {
		t.Fatalf("startup error = %v", err)
	}
}

func TestNativeQuoteCachePublishesImmutableSnapshots(t *testing.T) {
	r := &reader{}
	r.setQuoteAdapters([]recoveryVault{{Adapter: vlt, Vault: vlt, TokenOut: tOut, TokenOutDecimals: 6}})
	var group sync.WaitGroup
	for range 3 {
		group.Go(func() {
			for range 100 {
				r.setQuoteAdapters([]recoveryVault{{Adapter: tIn, Vault: tIn, TokenOut: tOut, TokenOutDecimals: 6}})
			}
		})
		group.Go(func() {
			for range 100 {
				_, account, ok := r.cachedQuoteRoute(vlt, tIn, tOut, 6)
				if !ok || account != vlt {
					t.Error("refresh lost cached account")
				}
			}
		})
	}
	group.Wait()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	// Already cached accounts must not be re-read; no RPC client is present.
	if err := r.warmQuoteAdapters(ctx, []common.Address{vlt, tIn}); err != nil {
		t.Fatal(err)
	}
}
