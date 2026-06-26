package bridgefacilitator

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestSelectBestAdapter(t *testing.T) {
	cand := func(n byte, p int64) adapterSizing {
		return adapterSizing{target: Target{Adapter: common.Address{n}}, principal: big.NewInt(p)}
	}

	t.Run("picks the largest principal", func(t *testing.T) {
		best, ok := selectBestAdapter([]adapterSizing{cand(1, 100), cand(2, 300), cand(3, 200)})
		if !ok || best.target.Adapter != (common.Address{2}) || best.principal.Int64() != 300 {
			t.Fatalf("best = %+v ok=%v, want adapter 0x02 / 300", best, ok)
		}
	})

	t.Run("no candidates", func(t *testing.T) {
		if _, ok := selectBestAdapter(nil); ok {
			t.Fatal("expected ok=false for no candidates")
		}
	})

	t.Run("ties keep config order", func(t *testing.T) {
		best, ok := selectBestAdapter([]adapterSizing{cand(1, 200), cand(2, 200)})
		if !ok || best.target.Adapter != (common.Address{1}) {
			t.Fatalf("tie should keep the first (0x01), got %v", best.target.Adapter)
		}
	})
}
