package rfq

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestParallelFillPlansReserveSharedVaultCapacity(t *testing.T) {
	for _, tc := range []struct {
		name     string
		capacity int64
		second   bool
	}{
		{"enough for two", 1_800_000, true}, {"not enough for two", 1_500_000, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, backend := fillFixtures(t)
			e := newExec(t, st, backend, &fakeTxm{})
			e.discountsEnabled = false
			e.maxConcurrentOrders = 2
			e.strategy = newDefaultTestStrategy()
			e.vaults = []recoveryVault{{Adapter: vlt}}
			e.reader = &fakeRecoveryReader{
				permInv:  []solverInventory{testInventory(vlt, tIn, tOut, big.NewInt(tc.capacity), big.NewInt(1e18))},
				quoteOut: map[common.Address]*big.Int{tOut: big.NewInt(900_000)},
			}
			first, err := e.buildFillPlan(t.Context(), "o1", &executable{quoteID: "q1"}, sampleOrder(), tOut, big.NewInt(900_000))
			if err != nil || first == nil {
				t.Fatalf("first plan: %v %v", first, err)
			}
			second, _ := e.buildFillPlan(t.Context(), "o2", &executable{quoteID: "q2"}, sampleOrder(), tOut, big.NewInt(900_000))
			if (second != nil) != tc.second {
				t.Fatalf("second plan %v; expected accepted=%v", second, tc.second)
			}
			e.reservations.Delete("o1")
			e.reservations.Delete("o2")
			retry, err := e.buildFillPlan(t.Context(), "o3", &executable{quoteID: "q3"}, sampleOrder(), tOut, big.NewInt(900_000))
			if err != nil || retry == nil {
				t.Fatalf("released capacity not reusable: %v %v", retry, err)
			}
		})
	}
}

func TestUnresolvedOrderRetainsReservationUntilReconciliation(t *testing.T) {
	for _, outcome := range []txmanager.Outcome{txmanager.OutcomeConfirmed, txmanager.OutcomeSubmissionError,
		txmanager.OutcomeTrackingStopped, txmanager.OutcomeIncludedUnconfirmed, txmanager.OutcomeCancelledUnconfirmed} {
		t.Run(string(outcome), func(t *testing.T) {
			st, backend := fillFixtures(t)
			backend.order = nil
			txm := &fakeTxm{result: txmanager.Result{Outcome: outcome, Err: context.Canceled}}
			e := newExec(t, st, backend, txm)
			e.discountsEnabled = false
			e.maxConcurrentOrders = 2
			e.vaults = []recoveryVault{{Adapter: vlt}}
			e.reader = &fakeRecoveryReader{
				permInv:  []solverInventory{testInventory(vlt, tIn, tOut, big.NewInt(1_800_000), big.NewInt(1e18))},
				quoteOut: map[common.Address]*big.Int{tOut: big.NewInt(900_000)},
			}
			st.upsertQueued(queuedOrder{OrderID: "o1", QuoteID: "q1"})
			e.handleOrder(t.Context(), st.order("o1"))
			if txm.calls != 1 {
				t.Fatalf("order was not sent: %+v", st.order("o1"))
			}
			want := 1
			if outcome == txmanager.OutcomeConfirmed || outcome == txmanager.OutcomeSubmissionError {
				want = 0
			}
			if e.reservations.Len() != want {
				t.Fatalf("reservations = %d, want %d", e.reservations.Len(), want)
			}
			backend.order = &backendOrder{OrderID: "o1", OrderStatus: "filled"}
			e.reconcileTerminalStatus(t.Context(), "o1")
			if e.reservations.Len() != 0 {
				t.Fatal("terminal reconciliation retained reservation")
			}
		})
	}
}

func TestFillReservationsRejectStrategyOvercommit(t *testing.T) {
	candidate := liquidlane.QuoteCandidate{Route: liquidlane.NewRoute(1, vlt, vlt, tIn, tOut, 18, 6),
		Rate: big.NewInt(1e18), MaxAmountIn: big.NewInt(2e18), MaxAmountOut: big.NewInt(1_000_000)}
	plan := baseFillPlan()
	plan.Legs = append(plan.Legs, plan.Legs[0])
	if _, err := fillReservations(plan, []liquidlane.QuoteCandidate{candidate}); err == nil {
		t.Fatal("repeated legs exceeded route capacity")
	}
	plan.Legs = plan.Legs[:1]
	id := common.HexToHash("0x1")
	plan.Legs[0].DiscountID, candidate.DiscountID = &id, &id
	reserved, err := fillReservations(plan, []liquidlane.QuoteCandidate{candidate})
	if err != nil {
		t.Fatal(err)
	}
	candidate.DiscountID = nil
	if got := candidatesAfterReservations([]liquidlane.QuoteCandidate{candidate}, reserved); len(got) != 0 {
		t.Fatal("exact-input discount did not reserve its full capacity domain")
	}
}

func TestDirectReservationAllowsSurplusInput(t *testing.T) {
	candidate := liquidlane.QuoteCandidate{Route: liquidlane.NewRoute(1, vlt, vlt, tIn, tOut, 18, 6),
		Rate: big.NewInt(1e18), MaxAmountIn: big.NewInt(700_000_000_000_000_000), MaxAmountOut: big.NewInt(700_000)}
	plan := baseFillPlan()
	plan.Legs[0].AmountOut = big.NewInt(700_000)
	reserved, err := fillReservations(plan, []liquidlane.QuoteCandidate{candidate})
	if err != nil {
		t.Fatal(err)
	}
	if reserved[liquidlane.RouteCapacityID(candidate.Route)].Cmp(big.NewInt(700_000)) != 0 {
		t.Fatal("direct exact-output swap did not reserve its capped output")
	}
	id := common.HexToHash("0x1")
	plan.Legs[0].DiscountID, candidate.DiscountID = &id, &id
	if _, err := fillReservations(plan, []liquidlane.QuoteCandidate{candidate}); err == nil {
		t.Fatal("exact-input discount may not absorb input beyond capacity")
	}
}

// A receipt can finish while an RPC is returning a capacity snapshot from before
// that receipt. Removing the completed reservation must not revive spent output.
type releasingCandidateReader struct {
	fillReader

	release func()
}

func (r releasingCandidateReader) readQuoteCandidates(
	ctx context.Context, inventory []solverInventory, tokenIn, tokenOut common.Address, amountIn *big.Int,
) ([]liquidlane.QuoteCandidate, error) {
	candidates, err := r.fillReader.readQuoteCandidates(ctx, inventory, tokenIn, tokenOut, amountIn)
	r.release()
	return candidates, err
}

func TestCapacitySnapshotRetainsConcurrentCompletedReservation(t *testing.T) {
	st, backend := fillFixtures(t)
	e := newExec(t, st, backend, &fakeTxm{})
	e.discountsEnabled, e.maxConcurrentOrders = false, 2
	e.strategy = newDefaultTestStrategy()
	e.vaults = []recoveryVault{{Adapter: vlt}}
	inv := testInventory(vlt, tIn, tOut, big.NewInt(1_500_000), big.NewInt(1e18))
	reader := &fakeRecoveryReader{permInv: []solverInventory{inv}, quoteOut: map[common.Address]*big.Int{tOut: big.NewInt(900_000)}}
	e.reader = releasingCandidateReader{fillReader: reader, release: func() { e.reservations.Delete("pending") }}
	reserved := liquidlane.CapacityReservations{liquidlane.RouteCapacityID(inv.Route): big.NewInt(900_000)}
	e.reservations.Set("pending", reserved)
	plan, err := e.buildFillPlan(t.Context(), "o2", &executable{quoteID: "q2"}, sampleOrder(), tOut, big.NewInt(900_000))
	if err == nil && plan != nil {
		t.Fatal("plan reused capacity spent during its read")
	}
	e.reservations.Set("pending", reserved)
	quotes := &quoteService{reader: e.reader, reservations: &e.reservations}
	candidates, err := quotes.snapshotCandidates(t.Context(), []solverInventory{inv}, strategyRequest{TokenIn: tIn, TokenOut: tOut, Amount: big.NewInt(1e18)})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].MaxAmountOut.Cmp(big.NewInt(600_000)) != 0 {
		t.Fatalf("quote reused capacity spent during its read: %+v", candidates)
	}
}

func TestPendingDiscountCannotBeReusedByAnotherOrder(t *testing.T) {
	id := common.HexToHash("0x1")
	candidate := liquidlane.QuoteCandidate{Route: liquidlane.NewRoute(1, vlt, vlt, tIn, tOut, 18, 6),
		DiscountID: &id, Rate: big.NewInt(1e18), MaxAmountIn: big.NewInt(1e18), MaxAmountOut: big.NewInt(1_000_000)}
	available := candidatesAfterReservations([]liquidlane.QuoteCandidate{candidate}, liquidlane.CapacityReservations{
		liquidlane.CapacityID("discount:" + id.Hex()): big.NewInt(1),
	})
	if len(available) != 0 {
		t.Fatal("a pending discount was offered to another order")
	}
}
