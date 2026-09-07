package liquidlane

import (
	"math/big"
	"testing"
)

func TestCapacityLedgerAggregatesAndReleasesClonedReservations(t *testing.T) {
	var ledger CapacityLedger
	first := CapacityReservations{"shared": big.NewInt(30)}
	if !ledger.Set("first", first) || !ledger.Set("second", CapacityReservations{"shared": big.NewInt(20)}) {
		t.Fatal("expected reservations to change ledger")
	}
	first["shared"].SetInt64(1)
	if got := ledger.Snapshot()["shared"]; got == nil || got.Int64() != 50 || ledger.Len() != 2 {
		t.Fatalf("snapshot = %v, len = %d", ledger.Snapshot(), ledger.Len())
	}
	if got := ledger.SnapshotExcluding("first")["shared"]; got == nil || got.Int64() != 20 {
		t.Fatalf("snapshot excluding first = %v, want shared=20", ledger.SnapshotExcluding("first"))
	}
	if !ledger.Delete("first") || ledger.Delete("missing") {
		t.Fatal("unexpected delete result")
	}
	if got := ledger.Snapshot()["shared"]; got == nil || got.Int64() != 20 || ledger.Len() != 1 {
		t.Fatalf("snapshot after release = %v, len = %d", ledger.Snapshot(), ledger.Len())
	}
}

func TestCapacityLedgerReplacementAndSnapshotIsolation(t *testing.T) {
	var ledger CapacityLedger
	ledger.Set("a", CapacityReservations{"x": big.NewInt(4), "y": big.NewInt(2)})
	ledger.Set("b", CapacityReservations{"x": big.NewInt(3)})
	ledger.Set("a", CapacityReservations{"x": big.NewInt(8)})
	got := ledger.Snapshot()
	if len(got) != 1 || got["x"].Int64() != 11 {
		t.Fatalf("replacement totals: %v", got)
	}
	got["x"].SetInt64(999)
	if excluded := ledger.SnapshotExcluding("a"); excluded["x"].Int64() != 3 {
		t.Fatalf("exclusion: %v", excluded)
	}
	ledger.Delete("a")
	ledger.Delete("b")
	if remaining := ledger.Snapshot(); len(remaining) != 0 {
		t.Fatalf("released totals: %v", remaining)
	}
}
