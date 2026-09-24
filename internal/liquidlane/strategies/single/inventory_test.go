package single

import (
	"math/big"
	"testing"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func TestAllocateInventoryCapacity(t *testing.T) {
	for _, test := range []struct {
		name       string
		reserveBps int
		pending    int64
		want       []int64
	}{
		{"alternatives share full budget", 0, 0, []int64{100, 100, 40, 50}},
		{"own limit and pending reservations", 1_000, 30, []int64{60, 60, 36, 45}},
		{"pending limits every alternative", 1_000, 70, []int64{20, 20, 20, 45}},
		{"exhausted domain", 1_000, 90, []int64{45}},
		{"overreserved domain", 0, 110, []int64{50}},
	} {
		t.Run(test.name, func(t *testing.T) {
			inventory := []liquidlane.Inventory{
				{Route: liquidlane.Route{ID: "a", CapacityID: "shared"}, MaxAssets: big.NewInt(100)},
				{Route: liquidlane.Route{ID: "b", CapacityID: "shared"}, MaxAssets: big.NewInt(100)},
				{Route: liquidlane.Route{ID: "limited", CapacityID: "shared"}, MaxAssets: big.NewInt(40)},
				{Route: liquidlane.Route{ID: "independent", CapacityID: "other"}, MaxAssets: big.NewInt(50)},
				{Route: liquidlane.Route{ID: "invalid", CapacityID: "shared"}},
			}
			pending := big.NewInt(test.pending)
			result := AllocateInventoryCapacity(inventory, liquidlane.CapacityReservations{"shared": pending}, test.reserveBps)
			if len(result) != len(test.want) {
				t.Fatalf("got %d candidates, want %d", len(result), len(test.want))
			}
			for i, want := range test.want {
				if result[i].MaxAssets.Int64() != want {
					t.Fatalf("candidate %s capacity=%s, want %d", result[i].ID, result[i].MaxAssets, want)
				}
				result[i].MaxAssets.SetInt64(0)
			}
			for i, want := range []int64{100, 100, 40, 50} {
				if inventory[i].MaxAssets.Int64() != want {
					t.Fatal("mutated source capacity")
				}
			}
			if pending.Int64() != test.pending {
				t.Fatal("mutated reservation")
			}
		})
	}
}
