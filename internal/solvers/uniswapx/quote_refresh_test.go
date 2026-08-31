package uniswapx

import (
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquiddiscounts "github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

type quoteModeReader struct {
	chainReader

	now            time.Time
	resolved       []liquidlane.Route
	resolveErr     error
	resolveFn      func([]common.Address) ([]liquidlane.Route, error)
	adapters       []common.Address
	snapshotRoutes []liquidlane.Route
	snapshot       snapshot
	gasErr         error
}

func (r *quoteModeReader) latestBlockTime(context.Context) (time.Time, error) {
	return r.now, nil
}

func (r *quoteModeReader) ResolveRoutes(
	_ context.Context,
	adapters []common.Address,
) ([]liquidlane.Route, error) {
	r.adapters = append(r.adapters, adapters...)
	if r.resolveFn != nil {
		return r.resolveFn(adapters)
	}
	return append([]liquidlane.Route(nil), r.resolved...), r.resolveErr
}

func (r *quoteModeReader) ValidateGasTokens([]liquidlane.Route) error {
	return r.gasErr
}

func (r *quoteModeReader) Quote(
	_ context.Context,
	routes []liquidlane.Route,
	_ common.Address,
	_ time.Time,
) (snapshot, error) {
	r.snapshotRoutes = append([]liquidlane.Route(nil), routes...)
	return r.snapshot, nil
}

func TestRefreshQuoteStateInternalDiscountScopes(t *testing.T) {
	now := time.Unix(1_000, 0)
	advertised := testDiscountRoute()
	configured := liquidlane.NewRoute(
		1,
		common.HexToAddress("0x9999999999999999999999999999999999999999"),
		common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		advertised.TokenIn,
		advertised.TokenOut,
		advertised.TokenInDecimals,
		advertised.TokenOutDecimals,
	)
	inventory := func(route liquidlane.Route) liquidlane.Inventory {
		item := liquidlane.DirectInventory(route, big.NewInt(100), big.NewInt(100))
		item.AdapterMinDiscount = new(big.Int)
		return item
	}

	t.Run("no configured adapters is discount only", func(t *testing.T) {
		listed := &liquiddiscounts.List{Discounts: []liquiddiscounts.ListItem{
			testDiscountOffer(advertised, now.Add(time.Minute), "100", "100"),
		}}
		provider := &fakeDiscountProvider{list: listed}
		reader := &quoteModeReader{
			now: now, resolved: []liquidlane.Route{advertised},
			snapshot: snapshot{
				Direct:   []liquidlane.Inventory{inventory(advertised)},
				Physical: []liquidlane.Inventory{inventory(advertised)},
			},
		}
		solver := quoteModeSolver(reader, provider)

		if err := solver.refreshQuoteState(t.Context(), nil); err != nil {
			t.Fatalf("refreshQuoteState: %v", err)
		}
		state := solver.quoteState.Load()
		if state == nil || len(state.inventory) != 1 || state.inventory[0].DiscountID == nil ||
			state.inventory[0].Adapter != advertised.Adapter {
			t.Fatalf("discount-only quote state = %+v", state)
		}
		if len(reader.adapters) != 1 || reader.adapters[0] != advertised.Adapter ||
			len(reader.snapshotRoutes) != 1 {
			t.Fatalf("dynamic quote routes: adapters=%+v routes=%+v", reader.adapters, reader.snapshotRoutes)
		}
		if reads := solver.txm.(*executionTestTxManager).maxFeeReads; reads != 0 {
			t.Fatalf("max fee reads = %d, want 0 with gas accounting disabled", reads)
		}
	})

	t.Run("configured adapters scope quotes", func(t *testing.T) {
		listed := &liquiddiscounts.List{Discounts: []liquiddiscounts.ListItem{
			testDiscountOffer(configured, now.Add(time.Minute), "100", "100"),
			testDiscountOffer(advertised, now.Add(time.Minute), "100", "100"),
		}}
		provider := &fakeDiscountProvider{list: listed}
		reader := &quoteModeReader{
			now: now,
			snapshot: snapshot{
				Direct:   []liquidlane.Inventory{inventory(configured)},
				Physical: []liquidlane.Inventory{inventory(configured)},
			},
		}
		solver := quoteModeSolver(reader, provider)
		solver.cfg.Adapters = []common.Address{configured.Adapter}

		if err := solver.refreshQuoteState(t.Context(), []liquidlane.Route{configured}); err != nil {
			t.Fatalf("refreshQuoteState: %v", err)
		}
		state := solver.quoteState.Load()
		if state == nil || len(state.inventory) != 2 {
			t.Fatalf("configured quote state = %+v", state)
		}
		for _, item := range state.inventory {
			if item.Adapter != configured.Adapter {
				t.Fatalf("unconfigured adapter reached quote state: %+v", item)
			}
		}
		if len(reader.adapters) != 0 || len(reader.snapshotRoutes) != 1 ||
			reader.snapshotRoutes[0].Adapter != configured.Adapter {
			t.Fatalf("configured quote scope: adapters=%+v routes=%+v", reader.adapters, reader.snapshotRoutes)
		}
	})

	t.Run("advertised token must be active on chain", func(t *testing.T) {
		inactive := advertised
		active := advertised
		active.TokenIn = common.HexToAddress("0x7777777777777777777777777777777777777777")
		active.ID = liquidlane.NewRouteID(1, active.Adapter, active.TokenIn, active.TokenOut)
		provider := &fakeDiscountProvider{list: &liquiddiscounts.List{Discounts: []liquiddiscounts.ListItem{
			testDiscountOffer(inactive, now.Add(time.Minute), "100", "100"),
		}}}
		reader := &quoteModeReader{now: now, resolved: []liquidlane.Route{active}}
		solver := quoteModeSolver(reader, provider)

		if err := solver.refreshQuoteState(t.Context(), nil); err != nil {
			t.Fatalf("refreshQuoteState: %v", err)
		}
		state := solver.quoteState.Load()
		if state == nil || len(state.inventory) != 0 || len(reader.snapshotRoutes) != 0 {
			t.Fatalf("inactive advertised token reached quote state: state=%+v routes=%+v", state, reader.snapshotRoutes)
		}
	})

	t.Run("dynamic resolution failure keeps configured discounts", func(t *testing.T) {
		dynamic := configured
		dynamic.TokenIn = common.HexToAddress("0x8888888888888888888888888888888888888888")
		dynamic.ID = liquidlane.NewRouteID(1, dynamic.Adapter, dynamic.TokenIn, dynamic.TokenOut)
		provider := &fakeDiscountProvider{list: &liquiddiscounts.List{Discounts: []liquiddiscounts.ListItem{
			testDiscountOffer(configured, now.Add(time.Minute), "100", "100"),
			testDiscountOffer(dynamic, now.Add(time.Minute), "100", "100"),
		}}}
		reader := &quoteModeReader{
			now: now, resolveErr: errors.New("dynamic adapter resolution failed"),
			snapshot: snapshot{
				Direct:   []liquidlane.Inventory{inventory(configured)},
				Physical: []liquidlane.Inventory{inventory(configured)},
			},
		}
		solver := quoteModeSolver(reader, provider)
		solver.cfg.Adapters = []common.Address{configured.Adapter}

		if err := solver.refreshQuoteState(t.Context(), []liquidlane.Route{configured}); err != nil {
			t.Fatalf("refreshQuoteState: %v", err)
		}
		state := solver.quoteState.Load()
		if state == nil || len(state.inventory) != 2 || state.inventory[1].DiscountID == nil {
			t.Fatalf("configured discount was lost after dynamic resolution failure: %+v", state)
		}
	})
}

func TestRefreshQuoteStateInternalDiscountFailureFallsBackToDirect(t *testing.T) {
	now := time.Unix(1_000, 0)
	route := testDiscountRoute()
	direct := liquidlane.DirectInventory(route, big.NewInt(100), big.NewInt(100))
	provider := &fakeDiscountProvider{listErr: errors.New("discount backend unavailable")}
	reader := &quoteModeReader{
		now: now,
		snapshot: snapshot{
			Direct: []liquidlane.Inventory{direct}, Physical: []liquidlane.Inventory{direct},
		},
	}
	solver := quoteModeSolver(reader, provider)
	solver.cfg.Adapters = []common.Address{route.Adapter}

	if err := solver.refreshQuoteState(t.Context(), []liquidlane.Route{route}); err != nil {
		t.Fatalf("refreshQuoteState: %v", err)
	}
	state := solver.quoteState.Load()
	if state == nil || len(state.inventory) != 1 || state.inventory[0].DiscountID != nil {
		t.Fatalf("direct fallback state = %+v", state)
	}
}

func TestResolveAdvertisedRoutesIsolatesInvalidAdapter(t *testing.T) {
	now := time.Unix(1_000, 0)
	good := testDiscountRoute()
	bad := good
	bad.Adapter = common.HexToAddress("0x9999999999999999999999999999999999999999")
	bad.ID = liquidlane.NewRouteID(1, bad.Adapter, bad.TokenIn, bad.TokenOut)
	reader := &quoteModeReader{now: now}
	reader.resolveFn = func(adapters []common.Address) ([]liquidlane.Route, error) {
		if len(adapters) > 1 {
			return nil, errors.New("batch contains invalid adapter")
		}
		if adapters[0] == bad.Adapter {
			return nil, errors.New("invalid adapter")
		}
		return []liquidlane.Route{good}, nil
	}
	solver := quoteModeSolver(reader, &fakeDiscountProvider{})
	listed := &liquiddiscounts.List{Discounts: []liquiddiscounts.ListItem{
		testDiscountOffer(good, now.Add(time.Minute), "100", "100"),
		testDiscountOffer(bad, now.Add(time.Minute), "100", "100"),
	}}

	routes := solver.resolveAdvertisedRoutes(t.Context(), listed, nil, now, advertisedRouteFilter{})
	if len(routes) != 1 || routes[0].ID != good.ID {
		t.Fatalf("resolved routes = %+v, want only good adapter", routes)
	}
}

func TestRefreshQuoteStateInternalWithoutRoutesPublishesEmptyUnreadyState(t *testing.T) {
	now := time.Unix(1_000, 0)
	reader := &quoteModeReader{now: now}
	solver := quoteModeSolver(reader, &fakeDiscountProvider{list: &liquiddiscounts.List{}})

	if err := solver.refreshQuoteState(t.Context(), nil); err != nil {
		t.Fatalf("refreshQuoteState: %v", err)
	}
	state := solver.quoteState.Load()
	if state == nil || len(state.inventory) != 0 {
		t.Fatalf("empty quote state = %+v", state)
	}
	solver.lastExclusivePoll.Store(time.Now().Unix())
	if solver.ready() {
		t.Fatal("solver with empty quote inventory should not be ready")
	}
}

func TestRefreshQuoteStateSkipsDynamicRouteWithoutGasFeed(t *testing.T) {
	now := time.Unix(1_000, 0)
	route := testDiscountRoute()
	provider := &fakeDiscountProvider{list: &liquiddiscounts.List{Discounts: []liquiddiscounts.ListItem{
		testDiscountOffer(route, now.Add(time.Minute), "100", "100"),
	}}}
	reader := &quoteModeReader{
		now: now, resolved: []liquidlane.Route{route},
		gasErr: errors.New("missing token USD feed"),
	}
	solver := quoteModeSolver(reader, provider)

	if err := solver.refreshQuoteState(t.Context(), nil); err != nil {
		t.Fatalf("refreshQuoteState: %v", err)
	}
	state := solver.quoteState.Load()
	if state == nil || len(state.inventory) != 0 || len(reader.snapshotRoutes) != 0 {
		t.Fatalf("missing-feed quote state=%+v routes=%+v", state, reader.snapshotRoutes)
	}
}

func TestPublishQuoteStateDoesNotRetainConcurrentlyInvalidatedState(t *testing.T) {
	const iterations = 10_000

	solver := &Solver{}
	expiresAt := time.Now().Add(time.Minute)
	for range iterations {
		epoch := solver.quoteEpoch.Load()
		candidate := &quoteState{
			inventory: []liquidlane.Inventory{{}},
			expiresAt: expiresAt,
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Go(func() {
			<-start
			solver.publishQuoteState(epoch, candidate)
		})
		wg.Go(func() {
			<-start
			solver.invalidateQuotes()
		})
		close(start)
		wg.Wait()

		currentEpoch := solver.quoteEpoch.Load()
		if current := solver.quoteState.Load(); current != nil && current.epoch != currentEpoch {
			t.Fatalf("published stale epoch %d while current epoch is %d", current.epoch, currentEpoch)
		}
	}
}

func quoteModeSolver(reader chainReader, provider liquiddiscounts.Provider) *Solver {
	return &Solver{
		cfg: &Config{
			SolverMode: solverModeInternal,
			Discounts:  &DiscountConfig{HTTPTimeout: time.Second},
			QuoteServer: QuoteServerConfig{
				QuoteTTL: time.Minute,
			},
		},
		reader:    reader,
		txm:       &executionTestTxManager{},
		discounts: provider,
		log:       logr.Discard(),
	}
}
