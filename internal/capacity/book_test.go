package capacity

import (
	"math/big"
	"testing"
)

func TestBookSharesNamespacedReservationsAcrossWorkflows(t *testing.T) {
	lifi := new(Book)
	uniswapx := lifi
	capacityID := ID("shared")

	limits := Amounts{capacityID: big.NewInt(100)}
	if _, err := lifi.Acquire(NewOwner("lifi", "order-1"), Amounts{capacityID: big.NewInt(30)}, limits); err != nil {
		t.Fatal(err)
	}
	if _, err := uniswapx.Acquire(NewOwner("uniswapx", "order-1"), Amounts{capacityID: big.NewInt(20)}, limits); err != nil {
		t.Fatal(err)
	}
	if got := lifi.Snapshot()[capacityID]; got == nil || got.Int64() != 50 {
		t.Fatalf("shared snapshot = %v, want 50", got)
	}
}

func TestBookAcquireIsAtomicAndLeaseReleasesOnce(t *testing.T) {
	book := new(Book)
	capacityID := ID("shared")
	limits := Amounts{capacityID: big.NewInt(100)}

	lease, err := book.Acquire(NewOwner("lifi", "first"), Amounts{capacityID: big.NewInt(60)}, limits)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if _, err := book.Acquire(NewOwner("uniswapx", "second"), Amounts{capacityID: big.NewInt(50)}, limits); err == nil {
		t.Fatal("overcommitted shared capacity")
	}
	if !lease.Release() || lease.Release() {
		t.Fatal("lease must release exactly once")
	}
	if _, err := book.Acquire(NewOwner("uniswapx", "second"), Amounts{capacityID: big.NewInt(50)}, limits); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
}
