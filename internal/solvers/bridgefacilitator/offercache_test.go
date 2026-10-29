package bridgefacilitator

import (
	"math/big"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

// seed inserts an offer straight into the tracker's map, standing in for a prior reconcile.
func seed(tr *offerTracker, adapter common.Address, auction int64, expiry time.Time, principal int64) {
	tr.offers[offerKey{adapter, auction}] = offerState{expiry: expiry, principal: big.NewInt(principal)}
}

func TestOfferTracker(t *testing.T) {
	tr := newOfferTracker()
	now := time.Unix(1_000_000, 0)
	adapterA := common.Address{0xAA}
	adapterB := common.Address{0xBB}

	live := func(at time.Time, adapter common.Address, auction int64) bool {
		for _, k := range tr.liveEntries(at) {
			if k.adapter == adapter && k.auction == auction {
				return true
			}
		}
		return false
	}

	if len(tr.liveEntries(now)) != 0 {
		t.Fatal("empty tracker should report no live offers")
	}

	seed(tr, adapterA, 42, now.Add(30*time.Minute), 100)
	if !live(now, adapterA, 42) {
		t.Fatal("offer should be live before expiry")
	}
	// Dedup is per-adapter: A's offer on auction 42 must not suppress B's offer on the same auction.
	if live(now, adapterB, 42) {
		t.Fatal("an offer through adapter A must not mark adapter B's offer on the same auction as live")
	}
	if live(now.Add(31*time.Minute), adapterA, 42) {
		t.Fatal("offer should be expired after its TTL")
	}
	if live(now, adapterA, 7) {
		t.Fatal("unknown auction should not be live")
	}
}

func TestOfferTrackerLiveCoverage(t *testing.T) {
	tr := newOfferTracker()
	now := time.Unix(1_000_000, 0)
	adapterA := common.Address{0xAA}
	adapterB := common.Address{0xBB}

	if got := tr.liveCoverage(42, now); got.Sign() != 0 {
		t.Fatalf("empty tracker coverage = %s, want 0", got)
	}

	// Coverage sums principals across adapters on the same auction.
	seed(tr, adapterA, 42, now.Add(30*time.Minute), 100)
	seed(tr, adapterB, 42, now.Add(30*time.Minute), 60)
	seed(tr, adapterA, 7, now.Add(30*time.Minute), 999) // other auction, excluded
	if got := tr.liveCoverage(42, now); got.Cmp(big.NewInt(160)) != 0 {
		t.Fatalf("coverage = %s, want 160", got)
	}

	// Expired offers don't count toward coverage.
	if got := tr.liveCoverage(42, now.Add(31*time.Minute)); got.Sign() != 0 {
		t.Fatalf("coverage after expiry = %s, want 0", got)
	}
}

// TestOfferTrackerReconcileAdapter covers the wholesale replace: an adapter's cached offers are dropped
// and rebuilt from the API's live set, while other adapters are left untouched.
func TestOfferTrackerReconcileAdapter(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	exp := now.Add(time.Hour)
	adapterA := common.Address{0xAA}
	adapterB := common.Address{0xBB}

	// Seed: A holds offers on auctions 1, 2, 3; B holds one on auction 1 that A's reconcile must not touch.
	tr := newOfferTracker()
	seed(tr, adapterA, 1, exp, 100)
	seed(tr, adapterA, 2, exp, 200)
	seed(tr, adapterA, 3, exp, 300)
	seed(tr, adapterB, 1, exp, 999)

	// API for adapter A now lists only auctions 1 (new principal/expiry) and 4. Auctions 2 and 3 are gone.
	newExp := now.Add(2 * time.Hour)
	live := map[int64]offerState{
		1: {expiry: newExp, principal: big.NewInt(150), nonce: big.NewInt(7)},
		4: {expiry: newExp, principal: big.NewInt(400)},
	}
	tr.reconcileAdapter(adapterA, live)

	// A top-up re-signs with the live offer's nonce; expired or unknown offers yield none.
	if got := tr.liveNonce(adapterA, 1, now); got == nil || got.Int64() != 7 {
		t.Fatalf("liveNonce(A,1) = %v, want 7", got)
	}
	if got := tr.liveNonce(adapterA, 1, newExp); got != nil {
		t.Fatalf("liveNonce(A,1) after expiry = %v, want nil", got)
	}
	if got := tr.liveNonce(adapterA, 4, now); got != nil {
		t.Fatalf("liveNonce(A,4) with unknown nonce = %v, want nil", got)
	}
	if got := tr.liveNonce(adapterB, 4, now); got != nil {
		t.Fatalf("liveNonce(B,4) = %v, want nil", got)
	}

	want := map[offerKey]*big.Int{
		{adapterA, 1}: big.NewInt(150), // refreshed from API
		{adapterA, 4}: big.NewInt(400), // new live offer inserted
		{adapterB, 1}: big.NewInt(999), // other adapter untouched
	}
	if len(tr.offers) != len(want) {
		t.Fatalf("offers = %v, want %d entries", tr.offers, len(want))
	}
	for k, wantPrincipal := range want {
		st, ok := tr.offers[k]
		if !ok {
			t.Fatalf("missing entry %v", k)
		}
		if st.principal.Cmp(wantPrincipal) != 0 {
			t.Fatalf("%v principal = %s, want %s", k, st.principal, wantPrincipal)
		}
	}
	if _, ok := tr.offers[offerKey{adapterA, 2}]; ok {
		t.Fatal("auction 2 is no longer live and must be cleared")
	}
	if _, ok := tr.offers[offerKey{adapterA, 3}]; ok {
		t.Fatal("auction 3 is no longer live and must be cleared")
	}
	// The refreshed entry must carry the API's expiry, not the stale one.
	if got := tr.offers[offerKey{adapterA, 1}].expiry; !got.Equal(newExp) {
		t.Fatalf("auction 1 expiry = %s, want %s", got, newExp)
	}
}

func TestParseUnixTime(t *testing.T) {
	got, err := parseUnixTime("4102444800")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Unix() != 4_102_444_800 {
		t.Fatalf("got %d", got.Unix())
	}
	if _, err := parseUnixTime("not-a-number"); err == nil {
		t.Fatal("expected error on non-numeric input")
	}
}

// TestReconcileOffersDropsChainConfirmedFundedOffers: a SUCCEEDED offer keeps reserving liquidity until
// the adapter's on-chain request list confirms the loan; then it is dropped, because getMaxAssets and
// requestsLength already account for it. An open SUBMITTED offer is never dropped.
func TestReconcileOffersDropsChainConfirmedFundedOffers(t *testing.T) {
	adapter := common.Address{0xAA}
	fundedReq, unconfirmedReq, openReq := common.Address{0x83}, common.Address{0x82}, common.Address{0x84}
	expiration := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
	offer := func(id, auction int, status string, req common.Address, amount string) string {
		return `{"id":` + strconv.Itoa(id) + `,"auctionId":` + strconv.Itoa(auction) + `,"status":"` + status + `","maker":"` + lowerAddr(adapter) +
			`","requestId":"` + req.Hex() + `","amount":"` + amount + `","expectedReturn":"1","nonce":"` + strconv.Itoa(id) + `","expiration":"` + expiration + `"}`
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[` + offer(1, 83, "SUCCEEDED", fundedReq, "45000") + `,` +
			offer(2, 82, "SUCCEEDED", unconfirmedReq, "7") + `,` + offer(3, 84, "SUBMITTED", openReq, "414000") + `]`))
	}))
	defer api.Close()
	s := &Solver{
		api:    newAPIClient(api.URL, fakeSigner{}, big.NewInt(1), time.Second, logr.Discard()),
		log:    logr.Discard(),
		offers: newOfferTracker(),
		links:  observability.NewSpanLinks(),
	}
	if !s.reconcileOffers(t.Context(), []Target{{Adapter: adapter}}) {
		t.Fatal("reconcile reported incomplete")
	}
	if len(s.offers.liveEntries(time.Now())) != 3 {
		t.Fatalf("live before chain confirmation = %d, want 3 (funded offers still reserve)", len(s.offers.liveEntries(time.Now())))
	}

	s.offers.dropFunded(adapter, []common.Address{fundedReq, openReq})

	if _, ok := s.offers.offers[offerKey{adapter, 83}]; ok {
		t.Fatal("funded offer on auction 83 is on-chain and must be dropped")
	}
	if _, ok := s.offers.offers[offerKey{adapter, 82}]; !ok {
		t.Fatal("SUCCEEDED offer the chain does not list must stay live")
	}
	if got := s.offers.liveCoverage(84, time.Now()); got == nil || got.Int64() != 414000 {
		t.Fatalf("auction 84 coverage = %v, want 414000", got)
	}
}
