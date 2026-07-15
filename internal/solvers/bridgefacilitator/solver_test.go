package bridgefacilitator

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
)

func TestDeduplicateAdapters_PreservesSourceOrder(t *testing.T) {
	t.Parallel()

	a := common.HexToAddress("0x00000000000000000000000000000000000000A0")
	b := common.HexToAddress("0x00000000000000000000000000000000000000B0")
	c := common.HexToAddress("0x00000000000000000000000000000000000000C0")
	got := deduplicateAdapters([]common.Address{a, b, a, c, b})
	want := []common.Address{a, b, c}
	if len(got) != len(want) {
		t.Fatalf("deduplicated adapters = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("deduplicated adapter %d = %s, want %s", i, got[i].Hex(), want[i].Hex())
		}
	}
}

func TestRefreshTargets_ExplicitAdaptersSkipFactoryDiscovery(t *testing.T) {
	t.Parallel()

	adapterAddr := common.HexToAddress("0x00000000000000000000000000000000000000A0")
	vault := common.HexToAddress("0x00000000000000000000000000000000000000B0")
	signer := common.HexToAddress("0x00000000000000000000000000000000000000C0")
	asset := common.HexToAddress("0x00000000000000000000000000000000000000D0")
	c, stop := newMulticallFakeClient(t,
		abiEncodeAggregate3Results(t, abiEncodeAddress(t, vault), abiEncodeAddress(t, signer), abiEncodeBytes4(t, erc1271MagicValue)),
		abiEncodeAggregate3Results(t, abiEncodeAddress(t, asset)),
	)
	defer stop()

	s := &Solver{
		cfg: &Config{
			Targets:        []Target{{Adapter: adapterAddr}},
			AdapterFactory: common.HexToAddress("0x00000000000000000000000000000000000000F0"),
		},
		reader:     newReader(c),
		log:        logr.Discard(),
		signerAddr: signer,
		offers:     newOfferTracker(),
	}
	added, err := s.refreshTargets(t.Context())
	if err != nil {
		t.Fatalf("refreshTargets: %v", err)
	}
	if len(added) != 1 || len(s.targets) != 1 || s.targets[0].Adapter != adapterAddr {
		t.Fatalf("refresh added=%v targets=%v, want only configured adapter %s", added, s.targets, adapterAddr.Hex())
	}
}

func TestRefreshTargets_RetainsLastKnownGoodOnWholeRefreshFailure(t *testing.T) {
	t.Parallel()

	adapterAddr := common.HexToAddress("0x00000000000000000000000000000000000000A0")
	vault := common.HexToAddress("0x00000000000000000000000000000000000000B0")
	signer := common.HexToAddress("0x00000000000000000000000000000000000000C0")
	asset := common.HexToAddress("0x00000000000000000000000000000000000000D0")
	round1 := abiEncodeAggregate3Results(t, abiEncodeAddress(t, vault), abiEncodeAddress(t, signer), abiEncodeBytes4(t, erc1271MagicValue))
	round2 := abiEncodeAggregate3Results(t, abiEncodeAddress(t, asset))
	c, stop := newMulticallFakeClient(t, round1, round2, []byte{0x01})
	defer stop()

	s := &Solver{
		cfg:        &Config{Targets: []Target{{Adapter: adapterAddr}}},
		reader:     newReader(c),
		log:        logr.Discard(),
		signerAddr: signer,
		offers:     newOfferTracker(),
	}
	added, err := s.refreshTargets(t.Context())
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	if len(added) != 1 || len(s.targets) != 1 || s.targets[0].Adapter != adapterAddr {
		t.Fatalf("first refresh added=%v targets=%v", added, s.targets)
	}
	now := time.Now()
	s.offers.record(adapterAddr, 42, testOfferState(now.Add(time.Hour), big.NewInt(100)))

	if _, err := s.refreshTargets(t.Context()); err == nil {
		t.Fatal("expected the second refresh to fail")
	}
	if len(s.targets) != 1 || s.targets[0].Adapter != adapterAddr {
		t.Fatalf("targets after failed refresh = %v, want last-known-good adapter", s.targets)
	}
	if got := s.offers.liveCoverage(42, now); got.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("offer coverage after failed refresh = %s, want last-known-good 100", got)
	}
}

func TestRefreshTargets_RemovesAndReaddsWhenSignerEligibilityChanges(t *testing.T) {
	t.Parallel()

	adapterAddr := common.HexToAddress("0x00000000000000000000000000000000000000A0")
	vault := common.HexToAddress("0x00000000000000000000000000000000000000B0")
	signer := common.HexToAddress("0x00000000000000000000000000000000000000C0")
	otherSigner := common.HexToAddress("0x00000000000000000000000000000000000000C1")
	asset := common.HexToAddress("0x00000000000000000000000000000000000000D0")
	matching := abiEncodeAggregate3Results(t, abiEncodeAddress(t, vault), abiEncodeAddress(t, signer), abiEncodeBytes4(t, erc1271MagicValue))
	notMatching := abiEncodeAggregate3Results(t, abiEncodeAddress(t, vault), abiEncodeAddress(t, otherSigner), abiEncodeBytes4(t, [4]byte{0xff, 0xff, 0xff, 0xff}))
	assetRound := abiEncodeAggregate3Results(t, abiEncodeAddress(t, asset))
	c, stop := newMulticallFakeClient(t,
		matching, assetRound,
		notMatching, // unauthorized: no asset round is issued
		matching, assetRound,
		matching, assetRound,
	)
	defer stop()

	s := &Solver{
		cfg:        &Config{Targets: []Target{{Adapter: adapterAddr}}},
		reader:     newReader(c),
		log:        logr.Discard(),
		signerAddr: signer,
		offers:     newOfferTracker(),
	}
	added, err := s.refreshTargets(t.Context())
	if err != nil || len(added) != 1 || len(s.targets) != 1 {
		t.Fatalf("initial refresh added=%v targets=%v err=%v", added, s.targets, err)
	}
	now := time.Now()
	s.offers.record(adapterAddr, 42, testOfferState(now.Add(time.Hour), big.NewInt(100)))
	added, err = s.refreshTargets(t.Context())
	if err != nil || len(added) != 0 || len(s.targets) != 0 {
		t.Fatalf("removal refresh added=%v targets=%v err=%v", added, s.targets, err)
	}
	if got := s.offers.liveCoverage(42, now); got.Sign() != 0 {
		t.Fatalf("removed adapter still contributes live coverage: %s", got)
	}
	added, err = s.refreshTargets(t.Context())
	if err != nil || len(added) != 1 || len(s.targets) != 1 {
		t.Fatalf("re-add refresh added=%v targets=%v err=%v", added, s.targets, err)
	}
	added, err = s.refreshTargets(t.Context())
	if err != nil || len(added) != 0 || len(s.targets) != 1 {
		t.Fatalf("unchanged refresh added=%v targets=%v err=%v", added, s.targets, err)
	}
	if len(s.cfg.Targets) != 1 || s.cfg.Targets[0].Adapter != adapterAddr {
		t.Fatalf("static source mutated across refreshes: %v", s.cfg.Targets)
	}
}

func TestRun_AllowsEmptyFactorySnapshotAtStartup(t *testing.T) {
	t.Parallel()

	countRound := abiEncodeAggregate3Results(t, abiEncodeUint256(t, 0))
	c, stop := newMulticallFakeClient(t, countRound)
	defer stop()

	s := &Solver{
		cfg: &Config{
			AdapterFactory: common.HexToAddress("0x00000000000000000000000000000000000000F0"),
			Intervals: Intervals{
				Discover:   5 * time.Millisecond,
				RedeemPoll: 5 * time.Millisecond,
				Reconcile:  5 * time.Millisecond,
			},
		},
		reader: newReader(c),
		log:    logr.Discard(),
		offers: newOfferTracker(),
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := s.Run(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run returned %v, want context deadline after staying alive", err)
	}
}

func TestRun_ExplicitEmptyAdaptersSkipFactoryDiscoveryAndFailStartup(t *testing.T) {
	t.Parallel()

	factoryAddr := common.HexToAddress("0x00000000000000000000000000000000000000F0")
	signer := common.HexToAddress("0x00000000000000000000000000000000000000C0")
	countRound := abiEncodeAggregate3Results(t, abiEncodeUint256(t, 1))
	c, stop := newMulticallFakeClient(t, countRound)
	defer stop()

	s := &Solver{
		cfg:        &Config{Targets: []Target{}, AdapterFactory: factoryAddr},
		reader:     newReader(c),
		log:        logr.Discard(),
		signerAddr: signer,
		offers:     newOfferTracker(),
	}
	want := "no configured adapter passed startup validation (must resolve and accept this solver " + signer.Hex() + " as an authorized offer signer via ERC-1271); see per-adapter warnings above"
	if err := s.Run(t.Context()); err == nil || err.Error() != want {
		t.Fatalf("Run error = %v, want %q", err, want)
	}
}

func TestRun_StaticOnlyStillFailsStartupWhenNoAdapterPassesValidation(t *testing.T) {
	t.Parallel()

	adapterAddr := common.HexToAddress("0x00000000000000000000000000000000000000A0")
	vault := common.HexToAddress("0x00000000000000000000000000000000000000B0")
	signer := common.HexToAddress("0x00000000000000000000000000000000000000C0")
	otherSigner := common.HexToAddress("0x00000000000000000000000000000000000000C1")
	c, stop := newMulticallFakeClient(t,
		// Unauthorized (isValidSignature non-magic): the adapter is dropped, so no asset round is issued.
		abiEncodeAggregate3Results(t, abiEncodeAddress(t, vault), abiEncodeAddress(t, otherSigner), abiEncodeBytes4(t, [4]byte{0xff, 0xff, 0xff, 0xff})),
	)
	defer stop()

	s := &Solver{
		cfg:        &Config{Targets: []Target{{Adapter: adapterAddr}}},
		reader:     newReader(c),
		log:        logr.Discard(),
		signerAddr: signer,
		offers:     newOfferTracker(),
	}
	want := "no configured adapter passed startup validation (must resolve and accept this solver " + signer.Hex() + " as an authorized offer signer via ERC-1271); see per-adapter warnings above"
	if err := s.Run(t.Context()); err == nil || err.Error() != want {
		t.Fatalf("Run error = %v, want %q", err, want)
	}
}

func TestRefreshTargetsAndHydrate_HydratesOnlyNewlyUsableAdapters(t *testing.T) {
	t.Parallel()

	adapterAddr := common.HexToAddress("0x00000000000000000000000000000000000000A0")
	vault := common.HexToAddress("0x00000000000000000000000000000000000000B0")
	signer := common.HexToAddress("0x00000000000000000000000000000000000000C0")
	otherSigner := common.HexToAddress("0x00000000000000000000000000000000000000C1")
	asset := common.HexToAddress("0x00000000000000000000000000000000000000D0")
	matching := abiEncodeAggregate3Results(t, abiEncodeAddress(t, vault), abiEncodeAddress(t, signer), abiEncodeBytes4(t, erc1271MagicValue))
	notMatching := abiEncodeAggregate3Results(t, abiEncodeAddress(t, vault), abiEncodeAddress(t, otherSigner), abiEncodeBytes4(t, [4]byte{0xff, 0xff, 0xff, 0xff}))
	assetRound := abiEncodeAggregate3Results(t, abiEncodeAddress(t, asset))
	c, stop := newMulticallFakeClient(t,
		matching, assetRound,
		matching, assetRound,
		notMatching, // unauthorized: no asset round is issued
		matching, assetRound,
	)
	defer stop()

	var listCalls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		listCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	s := &Solver{
		cfg:        &Config{Targets: []Target{{Adapter: adapterAddr}}},
		reader:     newReader(c),
		api:        newAPIClient(srv.URL, fakeSigner{addr: signer}, big.NewInt(11155111), time.Second, logr.Discard()),
		log:        logr.Discard(),
		signerAddr: signer,
		offers:     newOfferTracker(),
	}
	wantCalls := []int64{1, 1, 1, 2}
	for i, want := range wantCalls {
		if err := s.refreshTargetsAndHydrate(t.Context()); err != nil {
			t.Fatalf("refresh %d: %v", i, err)
		}
		if got := listCalls.Load(); got != want {
			t.Fatalf("refresh %d listOffers calls = %d, want %d", i, got, want)
		}
	}
}

func TestRefreshTargetsAndHydrate_DiscoversFactoryEntityAfterEmptyStartup(t *testing.T) {
	t.Parallel()

	factoryAddr := common.HexToAddress("0x00000000000000000000000000000000000000F0")
	adapterAddr := common.HexToAddress("0x00000000000000000000000000000000000000A0")
	vault := common.HexToAddress("0x00000000000000000000000000000000000000B0")
	signer := common.HexToAddress("0x00000000000000000000000000000000000000C0")
	asset := common.HexToAddress("0x00000000000000000000000000000000000000D0")
	c, stop := newMulticallFakeClient(t,
		abiEncodeAggregate3Results(t, abiEncodeUint256(t, 0)),
		abiEncodeAggregate3Results(t, abiEncodeUint256(t, 1)),
		abiEncodeAggregate3Results(t, abiEncodeAddress(t, adapterAddr)),
		abiEncodeAggregate3Results(t, abiEncodeAddress(t, vault), abiEncodeAddress(t, signer), abiEncodeBytes4(t, erc1271MagicValue)),
		abiEncodeAggregate3Results(t, abiEncodeAddress(t, asset)),
	)
	defer stop()

	var listCalls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		listCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	s := &Solver{
		cfg:        &Config{AdapterFactory: factoryAddr},
		reader:     newReader(c),
		api:        newAPIClient(srv.URL, fakeSigner{addr: signer}, big.NewInt(11155111), time.Second, logr.Discard()),
		log:        logr.Discard(),
		signerAddr: signer,
		offers:     newOfferTracker(),
	}
	if err := s.refreshTargetsAndHydrate(t.Context()); err != nil {
		t.Fatalf("empty startup refresh: %v", err)
	}
	if len(s.targets) != 0 || listCalls.Load() != 0 {
		t.Fatalf("empty startup targets=%v listOffers calls=%d", s.targets, listCalls.Load())
	}
	if err := s.refreshTargetsAndHydrate(t.Context()); err != nil {
		t.Fatalf("discovery refresh: %v", err)
	}
	if len(s.targets) != 1 || s.targets[0].Adapter != adapterAddr {
		t.Fatalf("discovery targets=%v, want %s", s.targets, adapterAddr.Hex())
	}
	if got := listCalls.Load(); got != 1 {
		t.Fatalf("listOffers calls = %d, want one hydration", got)
	}
}
