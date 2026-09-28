package greedy

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func TestFilterLiveInventoryRemovesExpiredAndDuplicateCandidates(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	direct := liquidlane.DirectInventory(
		liquidlane.Route{ID: "direct"}, big.NewInt(100), big.NewInt(1),
	)
	discountID := common.HexToHash("0x01")
	private := liquidlane.DiscountInventory(
		liquidlane.Route{ID: "private"}, big.NewInt(100), big.NewInt(1), discountID, now.Add(time.Minute),
	)
	expired := liquidlane.DirectInventory(
		liquidlane.Route{ID: "expired"}, big.NewInt(100), big.NewInt(1),
	)
	expired.ValidUntil = now

	got := FilterLiveInventory([]liquidlane.Inventory{direct, direct, private, expired}, now)
	if len(got) != 2 || got[0].ID != direct.ID || got[1].ID != private.ID {
		t.Fatalf("filtered inventory = %+v", got)
	}
}

func TestReserveInventoryCapacity(t *testing.T) {
	for _, test := range []struct {
		name       string
		reserveBps int
		pending    int64
		want       []int64
	}{
		{"alternatives share full budget", 0, 0, []int64{100, 100, 40, 50}},
		{"own limit and pending reservations", 1_000, 30, []int64{60, 60, 6, 45}},
		{"pending limits every alternative", 1_000, 70, []int64{20, 20, 45}},
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
			result := ReserveInventoryCapacity(inventory, liquidlane.CapacityReservations{"shared": pending}, test.reserveBps)
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
