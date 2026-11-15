package rfq

import (
	"math/big"
	"testing"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
)

func testSpend() fillSpend {
	return fillSpend{CapacityID: liquidlane.NewCapacityID(1, vlt, tOut), Amount: big.NewInt(100), IncludedAt: 8, Deadline: 100}
}

func spendInventory(block uint64) []liquidlane.Inventory {
	return []liquidlane.Inventory{{Route: liquidlane.Route{CapacityID: testSpend().CapacityID}, MaxAssets: big.NewInt(1000), BlockNumber: block}}
}

func TestPendingFillDoesNotDisableQuotesForUnspentCapacity(t *testing.T) {
	ledger := newSpendLedger()
	spend := testSpend()
	spend.IncludedAt = 0
	if err := ledger.reserve("award", spend); err != nil {
		t.Fatal(err)
	}
	spend.Amount.SetInt64(999) // The ledger must own the amount used by concurrent quotes.
	s := &Solver{spends: ledger}
	inventory := append(spendInventory(7), liquidlane.Inventory{Route: liquidlane.Route{CapacityID: "other"}, MaxAssets: big.NewInt(500), BlockNumber: 7})
	allocated := liquidgreedy.AllocateInventoryCapacity(inventory, s.Reservations(inventory), 0)
	if len(allocated) != 2 || allocated[0].MaxAssets.Cmp(big.NewInt(900)) != 0 || allocated[1].MaxAssets.Cmp(big.NewInt(500)) != 0 {
		t.Fatalf("pending fill disabled remaining or unrelated capacity: %+v", allocated)
	}
}

func TestCanonicalFillRetainsReservationUntilSnapshotsReflectSpend(t *testing.T) {
	for _, outcome := range []string{"pending", "confirmed", "reverted", "unknown", "invalid receipt"} {
		t.Run(outcome, func(t *testing.T) {
			ledger := newSpendLedger()
			spend := testSpend()
			spend.IncludedAt = 0
			if err := ledger.reserve("award", spend); err != nil {
				t.Fatal(err)
			}
			sender := &fakeSender{busy: true}
			switch outcome {
			case "confirmed":
				sender.result = fillResult()
			case "reverted":
				sender.result = fillResult()
				sender.result.Reverted = true
			case "unknown":
				sender.pollErr = errors.New("receipt RPC unavailable")
			case "invalid receipt":
				sender.result = fillResult()
				sender.result.Execution.BlockNumber = nil
			}
			s := &Solver{sender: sender, spends: ledger, intent: &fillIntent{OrderID: "award", Spend: spend}}
			err := s.poll(t.Context())
			if (err != nil) != (outcome == "unknown" || outcome == "invalid receipt") {
				t.Fatal(err)
			}
			pending := outcome == "pending" || outcome == "unknown" || outcome == "invalid receipt"
			if (s.intent != nil) != pending || ledger.finalized("award") != (outcome == "confirmed") {
				t.Fatal("fill accounting or ownership changed incorrectly")
			}
			for _, block := range []uint64{0, 7, 8, 999} {
				inventory := spendInventory(block)
				want := int64(0)
				if pending || outcome == "confirmed" && block < 8 {
					want = 100
				}
				reserved := s.Reservations(inventory)[spend.CapacityID]
				if reserved == nil {
					reserved = new(big.Int)
				}
				if reserved.Cmp(big.NewInt(want)) != 0 {
					t.Fatalf("block %d: reservation = %s, want %d", block, reserved, want)
				}
			}
		})
	}
}

func TestFreshSnapshotPrunesSpendWithoutReopeningOlderInventory(t *testing.T) {
	ledger := newSpendLedger()
	if err := ledger.include("award", testSpend()); err != nil {
		t.Fatal(err)
	}
	s := &Solver{spends: ledger}
	// Both snapshots share physical capacity; the oldest observation controls release.
	inventory := append(spendInventory(8), spendInventory(7)...)
	if got := s.Reservations(inventory)[testSpend().CapacityID]; got == nil || got.Cmp(big.NewInt(100)) != 0 || len(ledger.spends) != 1 {
		t.Fatal("mixed snapshots released a spend too early")
	}
	if got := s.Reservations(spendInventory(8)); len(got) != 0 || len(ledger.spends) != 0 {
		t.Fatal("fresh capacity did not release and prune the canonical spend")
	}
	inventory = append(spendInventory(7), liquidlane.Inventory{Route: liquidlane.Route{CapacityID: "other"}, MaxAssets: big.NewInt(500), BlockNumber: 0})
	allocated := liquidgreedy.AllocateInventoryCapacity(inventory, s.Reservations(inventory), 0)
	if len(allocated) != 1 || allocated[0].CapacityID != "other" || allocated[0].MaxAssets.Cmp(big.NewInt(500)) != 0 {
		t.Fatal("older snapshots reopened spent cash or disabled unrelated capacity")
	}
	if len(s.Reservations(spendInventory(8))) != 0 {
		t.Fatal("fresh snapshot was disabled after an older snapshot arrived")
	}
}

func TestSpendExpiryRetainsCanonicalAccounting(t *testing.T) {
	ledger := newSpendLedger()
	if err := ledger.include("award", testSpend()); err != nil {
		t.Fatal(err)
	}
	ledger.expire(99)
	if !ledger.finalized("award") || len(ledger.spends) != 1 {
		t.Fatal("completed award expired before signed deadline")
	}
	pending := testSpend()
	pending.CapacityID = "pending"
	pending.IncludedAt = 0
	if err := ledger.reserve("pending", pending); err != nil {
		t.Fatal(err)
	}
	ledger.expire(100)
	if ledger.finalized("award") || len(ledger.spends) != 1 || ledger.spends["pending"].IncludedAt != 0 || ledger.watermarks[testSpend().CapacityID] != 8 {
		t.Fatal("expiry retained canonical details or released unresolved signed work")
	}
	s := &Solver{spends: ledger}
	for _, block := range []uint64{0, 7, 8, 999, 7} {
		inventory := append(spendInventory(block), liquidlane.Inventory{Route: liquidlane.Route{CapacityID: "other"}, MaxAssets: big.NewInt(500)})
		reserved := s.Reservations(inventory)
		if reserved[pending.CapacityID] == nil || reserved[pending.CapacityID].Cmp(pending.Amount) != 0 {
			t.Fatal("expiry released an unresolved signed reservation")
		}
		allocated := liquidgreedy.AllocateInventoryCapacity(inventory, reserved, 0)
		want := 2
		if block < 8 {
			want = 1
		}
		if len(allocated) != want || allocated[len(allocated)-1].CapacityID != "other" {
			t.Fatal("expiry reopened old capacity or disabled fresh or unrelated capacity")
		}
	}
}
