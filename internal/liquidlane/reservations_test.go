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
