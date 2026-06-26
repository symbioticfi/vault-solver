package bridgefacilitator

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestSelectOffers(t *testing.T) {
	// cand builds a candidate adapter keyed by its first address byte with the given capacity.
	cand := func(n byte, capacity int64) adapterSizing {
		return adapterSizing{
			off:      &adapterOffering{target: Target{Adapter: common.Address{n}}},
			capacity: big.NewInt(capacity),
		}
	}
	adapterOf := func(o adapterOffer) byte { return o.off.target.Adapter[0] }

	t.Run("largest first, clamp the last to the remainder", func(t *testing.T) {
		offers := selectOffers([]adapterSizing{cand(1, 50), cand(2, 80), cand(3, 30)}, big.NewInt(100))
		// ranked by capacity: 2(80), 1(50), 3(30). Fill 100: 2→80 (rem 20), 1→20 (rem 0); 3 unused.
		if len(offers) != 2 {
			t.Fatalf("want 2 offers, got %d", len(offers))
		}
		if adapterOf(offers[0]) != 2 || offers[0].principal.Int64() != 80 {
			t.Errorf("offer0 = adapter %d / %s, want 2 / 80", adapterOf(offers[0]), offers[0].principal)
		}
		if adapterOf(offers[1]) != 1 || offers[1].principal.Int64() != 20 {
			t.Errorf("offer1 = adapter %d / %s, want 1 / 20", adapterOf(offers[1]), offers[1].principal)
		}
	})

	t.Run("a single adapter covers a small request (highest capacity wins)", func(t *testing.T) {
		offers := selectOffers([]adapterSizing{cand(1, 80), cand(2, 70)}, big.NewInt(10))
		if len(offers) != 1 || adapterOf(offers[0]) != 1 || offers[0].principal.Int64() != 10 {
			t.Fatalf("want a single offer adapter 1 / 10, got %d offers (%+v)", len(offers), offers)
		}
	})

	t.Run("partial coverage when total capacity is short of the request", func(t *testing.T) {
		offers := selectOffers([]adapterSizing{cand(1, 30), cand(2, 20)}, big.NewInt(100))
		if len(offers) != 2 {
			t.Fatalf("want 2 offers, got %d", len(offers))
		}
		sum := new(big.Int).Add(offers[0].principal, offers[1].principal)
		if sum.Int64() != 50 { // both offer their full capacity; 50 < 100 stays uncovered
			t.Fatalf("sum = %s, want 50", sum)
		}
	})

	t.Run("equal capacity keeps config order", func(t *testing.T) {
		offers := selectOffers([]adapterSizing{cand(1, 50), cand(2, 50)}, big.NewInt(50))
		if len(offers) != 1 || adapterOf(offers[0]) != 1 {
			t.Fatalf("tie should keep the first candidate (0x01), got %d offers (first %d)", len(offers), adapterOf(offers[0]))
		}
	})

	t.Run("no candidates", func(t *testing.T) {
		if offers := selectOffers(nil, big.NewInt(100)); len(offers) != 0 {
			t.Fatalf("expected no offers, got %d", len(offers))
		}
	})
}
