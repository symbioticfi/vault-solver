package defaultstrategy

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

func TestDecisionReservationsLifecycle(t *testing.T) {
	now := time.Unix(1000, 0)
	position := selectedLeg{MarketId: common.Hash{31: 1}, Borrower: common.Address{19: 2}}
	priced := pricedBundle{bidNative: big.NewInt(10), gasNative: big.NewInt(20), selectedLegs: []selectedLeg{position}}
	var reservations decisionReservations

	reservations.reserve("auction-1", priced)
	live := reservations.reconcile([]types.PendingAuction{{ID: "auction-1", ExpiresAt: now.Add(time.Minute)}}, now, now)
	if live.bidNative.Int64() != 10 || live.gasNative.Int64() != 20 {
		t.Fatalf("live reservation = bid %s gas %s", live.bidNative, live.gasNative)
	}
	if _, ok := live.positions[positionKey{market: position.MarketId, borrower: position.Borrower}]; !ok {
		t.Fatal("reserved position missing")
	}

	resolved := reservations.reconcile(nil, now.Add(time.Second), now)
	if resolved.bidNative.Int64() != 10 {
		t.Fatal("reservation released before a post-resolution callback balance refresh")
	}
	refreshed := reservations.reconcile(nil, now.Add(2*time.Second), now.Add(2*time.Second))
	if refreshed.bidNative.Sign() != 0 || refreshed.gasNative.Sign() != 0 || len(refreshed.positions) != 0 {
		t.Fatalf("reservation not released after balance refresh: %+v", refreshed)
	}
}

func TestDecisionReservationsDropUnsentDecision(t *testing.T) {
	var reservations decisionReservations
	reservations.reserve("auction-1", pricedBundle{bidNative: big.NewInt(10), gasNative: big.NewInt(20)})
	got := reservations.reconcile(nil, time.Unix(1000, 0), time.Unix(1000, 0))
	if got.bidNative.Sign() != 0 || got.gasNative.Sign() != 0 {
		t.Fatalf("decision never observed as pending must be released: %+v", got)
	}
}

func TestFilterReservedPositionsKeepsIndependentLegs(t *testing.T) {
	reserved := scoredLeg{bundleLeg: bundleLeg{selectedLeg: selectedLeg{
		MarketId: common.Hash{31: 1}, Borrower: common.Address{19: 1},
	}}}
	independent := scoredLeg{bundleLeg: bundleLeg{selectedLeg: selectedLeg{
		MarketId: common.Hash{31: 2}, Borrower: common.Address{19: 2},
	}}}
	got := filterReservedPositions([]scoredLeg{reserved, independent}, map[positionKey]struct{}{
		{market: reserved.MarketId, borrower: reserved.Borrower}: {},
	})
	if len(got) != 1 || got[0].MarketId != independent.MarketId || got[0].Borrower != independent.Borrower {
		t.Fatalf("filtered legs = %+v", got)
	}
}
