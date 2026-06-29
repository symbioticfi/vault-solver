package bridgefacilitator

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestOfferTracker(t *testing.T) {
	tr := newOfferTracker()
	now := time.Unix(1_000_000, 0)
	adapterA := common.Address{0xAA}
	adapterB := common.Address{0xBB}

	if tr.hasLive(adapterA, 42, now) {
		t.Fatal("empty tracker should report no live offer")
	}

	tr.record(adapterA, 42, now.Add(30*time.Minute), big.NewInt(100))
	if !tr.hasLive(adapterA, 42, now) {
		t.Fatal("offer should be live before expiry")
	}
	// Dedup is per-adapter: A's offer on auction 42 must not suppress B's offer on the same auction.
	if tr.hasLive(adapterB, 42, now) {
		t.Fatal("an offer through adapter A must not mark adapter B's offer on the same auction as live")
	}
	if tr.hasLive(adapterA, 42, now.Add(31*time.Minute)) {
		t.Fatal("offer should be expired after its TTL")
	}
	if tr.hasLive(adapterA, 7, now) {
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
	tr.record(adapterA, 42, now.Add(30*time.Minute), big.NewInt(100))
	tr.record(adapterB, 42, now.Add(30*time.Minute), big.NewInt(60))
	tr.record(adapterA, 7, now.Add(30*time.Minute), big.NewInt(999)) // other auction, excluded
	if got := tr.liveCoverage(42, now); got.Cmp(big.NewInt(160)) != 0 {
		t.Fatalf("coverage = %s, want 160", got)
	}

	// Expired offers don't count toward coverage.
	if got := tr.liveCoverage(42, now.Add(31*time.Minute)); got.Sign() != 0 {
		t.Fatalf("coverage after expiry = %s, want 0", got)
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
