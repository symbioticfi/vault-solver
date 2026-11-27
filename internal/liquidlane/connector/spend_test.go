package connector

import (
	"math/big"
	"slices"
	"testing"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
)

func testCapacityID() liquidlane.CapacityID {
	return liquidlane.NewCapacityID(1, vlt, tOut)
}

func testSpend() Spend {
	return Spend{Amounts: liquidlane.CapacityReservations{testCapacityID(): big.NewInt(100)}, IncludedAt: 8, Deadline: 100}
}

func spendInventory(block uint64) []liquidlane.Inventory {
	return []liquidlane.Inventory{{Route: liquidlane.Route{CapacityID: testCapacityID()}, MaxAssets: big.NewInt(1000), BlockNumber: block}}
}

func TestPendingFillDoesNotDisableQuotesForUnspentCapacity(t *testing.T) {
	ledger := NewSpendLedger()
	spend := testSpend()
	spend.IncludedAt = 0
	if err := ledger.Reserve("award", spend); err != nil {
		t.Fatal(err)
	}
	spend.Amounts[testCapacityID()].SetInt64(999) // The ledger must own the amount used by concurrent quotes.
	inventory := append(spendInventory(7), liquidlane.Inventory{Route: liquidlane.Route{CapacityID: "other"}, MaxAssets: big.NewInt(500), BlockNumber: 7})
	allocated := liquidgreedy.AllocateInventoryCapacity(inventory, ledger.Reservations(inventory), 0)
	if len(allocated) != 2 || allocated[0].MaxAssets.Cmp(big.NewInt(900)) != 0 || allocated[1].MaxAssets.Cmp(big.NewInt(500)) != 0 {
		t.Fatalf("pending fill disabled remaining or unrelated capacity: %+v", allocated)
	}
}

func TestQueuedReservationTracksSubmissionAndExpiry(t *testing.T) {
	ledger := NewSpendLedger()
	queued := testSpend()
	queued.IncludedAt = 0
	if err := ledger.ReserveQueued("award", queued); err != nil {
		t.Fatal(err)
	}
	if len(ledger.ReservationsExcluding(nil, "award")) != 0 {
		t.Fatal("award could not use its own virtual reservation")
	}
	actual := queued
	actual.Amounts = liquidlane.CapacityReservations{testCapacityID(): big.NewInt(110)}
	if err := ledger.Reserve("award", actual); err != nil {
		t.Fatal(err)
	}
	if got := ledger.Reservations(nil)[testCapacityID()]; got == nil || got.Int64() != 110 {
		t.Fatalf("submitted spend must replace the estimate, got %v", got)
	}
	ledger.Release("award")
	if got := ledger.Reservations(nil)[testCapacityID()]; got == nil || got.Int64() != 100 {
		t.Fatalf("open award lost its virtual reservation, got %v", got)
	}
	if err := ledger.Reserve("award", actual); err != nil {
		t.Fatal(err)
	}
	if expired := ledger.Expire(queued.Deadline); !slices.Equal(expired, []Expired{{ID: "award", Deadline: 100, Queued: true}}) {
		t.Fatalf("expiry reported %+v, want the queued award", expired)
	}
	if got := ledger.Reservations(nil)[testCapacityID()]; ledger.Queued("award") || got == nil || got.Int64() != 110 {
		t.Fatalf("expiry must release the estimate and retain signed capacity: %v", got)
	}
	actual.IncludedAt = 8
	if err := ledger.Include("award", actual); err != nil {
		t.Fatal(err)
	}
	if ledger.Queued("award") || len(ledger.Reservations(spendInventory(8))) != 0 {
		t.Fatal("confirmed fill remained virtually reserved against a fresh snapshot")
	}
}

func TestFreshSnapshotPrunesSpendWithoutReopeningOlderInventory(t *testing.T) {
	ledger := NewSpendLedger()
	if err := ledger.Include("award", testSpend()); err != nil {
		t.Fatal(err)
	}
	// Both snapshots share physical capacity; the oldest observation controls release.
	inventory := append(spendInventory(8), spendInventory(7)...)
	if got := ledger.Reservations(inventory)[testCapacityID()]; got == nil || got.Cmp(big.NewInt(100)) != 0 || len(ledger.spends) != 1 {
		t.Fatal("mixed snapshots released a spend too early")
	}
	if got := ledger.Reservations(spendInventory(8)); len(got) != 0 || len(ledger.spends) != 0 {
		t.Fatal("fresh capacity did not release and prune the canonical spend")
	}
	inventory = append(spendInventory(7), liquidlane.Inventory{Route: liquidlane.Route{CapacityID: "other"}, MaxAssets: big.NewInt(500), BlockNumber: 0})
	allocated := liquidgreedy.AllocateInventoryCapacity(inventory, ledger.Reservations(inventory), 0)
	if len(allocated) != 1 || allocated[0].CapacityID != "other" || allocated[0].MaxAssets.Cmp(big.NewInt(500)) != 0 {
		t.Fatal("older snapshots reopened spent cash or disabled unrelated capacity")
	}
	if len(ledger.Reservations(spendInventory(8))) != 0 {
		t.Fatal("fresh snapshot was disabled after an older snapshot arrived")
	}
}

func TestSpendExpiryRetainsCanonicalAccounting(t *testing.T) {
	ledger := NewSpendLedger()
	if err := ledger.Include("award", testSpend()); err != nil {
		t.Fatal(err)
	}
	if expired := ledger.Expire(99); len(expired) != 0 {
		t.Fatalf("expiry before the deadline reported %+v", expired)
	}
	if !ledger.Finalized("award") || len(ledger.spends) != 1 {
		t.Fatal("completed award expired before signed deadline")
	}
	pending := testSpend()
	pending.Amounts = liquidlane.CapacityReservations{"pending": big.NewInt(100)}
	pending.IncludedAt = 0
	if err := ledger.Reserve("pending", pending); err != nil {
		t.Fatal(err)
	}
	// The pending fill has no completed entry: only the included award retires.
	if expired := ledger.Expire(100); !slices.Equal(expired, []Expired{{ID: "award", Deadline: 100}}) {
		t.Fatalf("expiry reported %+v, want the completed award", expired)
	}
	if ledger.Finalized("award") || len(ledger.spends) != 1 || ledger.spends["pending"].IncludedAt != 0 || ledger.watermarks[testCapacityID()] != 8 {
		t.Fatal("expiry retained canonical details or released unresolved signed work")
	}
	for _, block := range []uint64{0, 7, 8, 999, 7} {
		inventory := append(spendInventory(block), liquidlane.Inventory{Route: liquidlane.Route{CapacityID: "other"}, MaxAssets: big.NewInt(500)})
		reserved := ledger.Reservations(inventory)
		if reserved["pending"] == nil || reserved["pending"].Cmp(pending.Amounts["pending"]) != 0 {
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

func TestNilLedgerReservesNothing(t *testing.T) {
	var ledger *SpendLedger
	ledger.Release("award")
	if ledger.Reservations(spendInventory(7)) != nil || ledger.Finalized("award") || ledger.Include("award", testSpend()) != nil ||
		ledger.Reserve("award", testSpend()) != nil {
		t.Fatal("nil ledger tracked a spend")
	}
	if ledger.Expire(100) != nil {
		t.Fatal("nil ledger expired a spend")
	}
}

func TestSplitFillPrunesEachCapacityAgainstItsOwnSnapshot(t *testing.T) {
	ledger := NewSpendLedger()
	spend := testSpend()
	spend.Amounts["other"] = big.NewInt(45)
	if err := ledger.Include("split", spend); err != nil {
		t.Fatal(err)
	}
	inventory := append(spendInventory(8), liquidlane.Inventory{Route: liquidlane.Route{CapacityID: "other"}, MaxAssets: big.NewInt(90), BlockNumber: 7})
	got := ledger.Reservations(inventory)
	if len(got) != 1 || got["other"] == nil || got["other"].Cmp(big.NewInt(45)) != 0 {
		t.Fatalf("wrong split reservations after one fresh snapshot: %v", got)
	}
	inventory[1].BlockNumber = 8
	if got = ledger.Reservations(inventory); len(got) != 0 {
		t.Fatalf("fresh capacity should release both legs: %v", got)
	}
}
