package rfq

import (
	"context"
	"math/big"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func vltFillCapacity() liquidlane.CapacityID {
	return liquidlane.RouteCapacityID(testInventory(vlt, tIn, tOut, maxUint256(), maxUint256()).Route)
}

func reservedOn(st *store, capacityID liquidlane.CapacityID) *big.Int {
	if amount := st.pendingReservations("")[capacityID]; amount != nil {
		return amount
	}
	return new(big.Int)
}

func TestQuoteSubtractsWonOrderReservations(t *testing.T) {
	body := validQuoteBody()
	parsed, err := body.toStrategy(1)
	if err != nil || parsed == nil {
		t.Fatalf("parse quote body: %v", err)
	}
	capacityID := liquidlane.RouteCapacityID(parsed.inv[0].Route)

	for _, tc := range []struct {
		name     string
		reserved *big.Int
		want     int
	}{
		{name: "free capacity quotes", want: http.StatusOK},
		{name: "fully reserved capacity declines", reserved: big.NewInt(10_000_000), want: http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := testServer()
			reservations := liquidlane.CapacityReservations{}
			reservations.Add(capacityID, tc.reserved)
			srv.quotes.reservations = func(string) liquidlane.CapacityReservations { return reservations }

			rr := do(t, srv.handler(), http.MethodPost, "/quote", testSecret, validQuoteBody())
			if rr.Code != tc.want {
				t.Fatalf("quote status = %d, want %d (body %s)", rr.Code, tc.want, rr.Body.String())
			}
		})
	}
}

func TestPollReservesWonOrderBeforeSubmission(t *testing.T) {
	st, be := fillFixtures(t)
	txm := &fakeTxm{result: confirmedTxResult()}
	e := newExec(t, st, be, txm)

	e.syncOnce(t.Context())

	if txm.calls != 0 {
		t.Fatalf("poll sent %d transactions, want none", txm.calls)
	}
	if got := reservedOn(st, vltFillCapacity()); got.Cmp(big.NewInt(900000)) != 0 {
		t.Fatalf("reserved = %s, want the plan's 900000 output", got)
	}
}

func TestReservationLifecycleFollowsOrderOutcome(t *testing.T) {
	for _, tc := range []struct {
		name     string
		result   txmanager.Result
		settled  string
		status   orderStatus
		reserved bool
	}{
		{name: "filled releases", result: confirmedTxResult(), settled: "filled", status: statusFilled},
		{name: "revert releases", result: txmanager.Result{
			Hash: common.HexToHash("0xbad"), Outcome: txmanager.OutcomeReverted, Err: errors.New("reverted"),
		}, settled: "open", status: statusFailed},
		{name: "cancellation retry keeps", result: confirmedCancellation(), settled: "open",
			status: statusRetryWaiting, reserved: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, be := fillFixtures(t)
			be.order.OrderStatus = tc.settled
			e := newExec(t, st, be, &fakeTxm{result: tc.result})

			syncCycle(t.Context(), e)

			if rec := st.order("o1"); rec == nil || rec.Status != tc.status {
				t.Fatalf("order = %+v, want status %s", rec, tc.status)
			}
			if got := st.reserved("o1"); got != tc.reserved {
				t.Fatalf("reserved = %v, want %v", got, tc.reserved)
			}
		})
	}
}

func TestFillPlanningSubtractsOtherWonOrders(t *testing.T) {
	st, be := fillFixtures(t)
	st.upsertQueued(queuedOrder{OrderID: "o1", QuoteID: "q1"})
	st.upsertQueued(queuedOrder{OrderID: "other", QuoteID: "q2"})
	held := liquidlane.CapacityReservations{}
	held.Add(vltFillCapacity(), maxUint256())
	if !st.reserve("other", held) {
		t.Fatal("reserve other order")
	}
	txm := &fakeTxm{result: confirmedTxResult()}
	e := newExec(t, st, be, txm)
	e.strategy = newDefaultTestStrategy()

	for _, o := range st.ordersAwaitingSubmission() {
		if o.OrderID == "o1" {
			e.handleOrder(t.Context(), o)
		}
	}

	if txm.calls != 0 {
		t.Fatalf("filled through capacity another won order holds (%d sends)", txm.calls)
	}
	if rec := st.order("o1"); rec == nil || rec.Status != statusFailed {
		t.Fatalf("order = %+v, want failed for lack of unreserved capacity", rec)
	}
}

func TestStoreReservationNeverOutlivesOrder(t *testing.T) {
	st := newStore(func() time.Time { return time.Unix(0, 0) })
	held := liquidlane.CapacityReservations{}
	held.Add(vltFillCapacity(), big.NewInt(1))
	if st.reserve("missing", held) {
		t.Fatal("reserved an unknown order")
	}
	st.upsertQueued(queuedOrder{OrderID: "o1", QuoteID: "q1"})
	if !st.reserve("o1", held) {
		t.Fatal("did not reserve an active order")
	}
	st.markStatus("o1", statusExpired, common.Hash{}, "")
	if st.reserved("o1") {
		t.Fatal("terminal order kept its reservation")
	}
	if st.reserve("o1", held) {
		t.Fatal("reserved a terminal order")
	}
}

func TestFillPlanReservations(t *testing.T) {
	id := common.HexToHash("0xab")
	inventory := testInventory(vlt, tIn, tOut, maxUint256(), maxUint256())
	direct := liquidlane.QuoteCandidate{Route: inventory.Route}
	discounted := liquidlane.QuoteCandidate{Route: inventory.Route, DiscountID: &id}
	capacityOnly := func(amount int64) liquidlane.CapacityReservations {
		out := liquidlane.CapacityReservations{}
		out.Add(vltFillCapacity(), big.NewInt(amount))
		return out
	}

	for _, tc := range []struct {
		name       string
		legs       []fillLeg
		candidates []liquidlane.QuoteCandidate
		want       liquidlane.CapacityReservations
		wantErr    bool
	}{
		{
			name:       "direct leg reserves its output",
			legs:       []fillLeg{{Adapter: vlt, AmountOut: big.NewInt(5)}},
			candidates: []liquidlane.QuoteCandidate{direct},
			want:       capacityOnly(5),
		},
		{
			// The adapter never consumes a discount nonce, so only vault capacity is held.
			name:       "discount leg reserves capacity, not the discount",
			legs:       []fillLeg{{Adapter: vlt, AmountOut: big.NewInt(7), DiscountID: &id}},
			candidates: []liquidlane.QuoteCandidate{direct, discounted},
			want:       capacityOnly(7),
		},
		{
			name:       "legs on one vault accumulate",
			legs:       []fillLeg{{Adapter: vlt, AmountOut: big.NewInt(2)}, {Adapter: vlt, AmountOut: big.NewInt(3), DiscountID: &id}},
			candidates: []liquidlane.QuoteCandidate{direct, discounted},
			want:       capacityOnly(5),
		},
		{
			name:       "unmatched leg fails closed",
			legs:       []fillLeg{{Adapter: vlt, AmountOut: big.NewInt(1), DiscountID: &id}},
			candidates: []liquidlane.QuoteCandidate{direct},
			wantErr:    true,
		},
		{name: "empty plan fails", candidates: []liquidlane.QuoteCandidate{direct}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := fillPlanReservations(&fillPlan{Legs: tc.legs}, tc.candidates)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("reservations = %v, want %v", got, tc.want)
			}
			for capacityID, amount := range tc.want {
				if got[capacityID] == nil || got[capacityID].Cmp(amount) != 0 {
					t.Fatalf("reservations = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// lockedBackend serializes the fake backend, which the poll loop and submitter now share.
type lockedBackend struct {
	mu    sync.Mutex
	inner *fakeBackend
}

func (b *lockedBackend) setOpen(orders []backendOrder) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.inner.open = orders
}

func (b *lockedBackend) listOpenOrders(ctx context.Context, filler string, limit int) ([]backendOrder, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.inner.listOpenOrders(ctx, filler, limit)
}

func (b *lockedBackend) getExecutableOrder(ctx context.Context, orderID, filler string) (*backendOrder, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.inner.getExecutableOrder(ctx, orderID, filler)
}

func (b *lockedBackend) getOrder(ctx context.Context, orderID string) (*backendOrder, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.inner.getOrder(ctx, orderID)
}

func (b *lockedBackend) resolveDiscount(ctx context.Context, discountID string) (*resolveDiscountResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.inner.resolveDiscount(ctx, discountID)
}

func (b *lockedBackend) listDiscounts(ctx context.Context) (*discountsResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.inner.listDiscounts(ctx)
}

type blockingTxm struct {
	entered chan struct{}
	release chan struct{}
}

func (b *blockingTxm) Send(context.Context, txmanager.Request) txmanager.Result {
	b.entered <- struct{}{}
	<-b.release
	return confirmedTxResult()
}

// A fill occupying the lane must not stop the poll loop: an order won meanwhile is reserved, and
// quotes see both commitments, before the first transaction finishes.
func TestPollReservesWhileFillIsInFlight(t *testing.T) {
	st, inner := fillFixtures(t)
	be := &lockedBackend{inner: inner}
	txm := &blockingTxm{entered: make(chan struct{}, 2), release: make(chan struct{})}
	e := newExec(t, st, be, txm)
	e.pollInterval = 5 * time.Millisecond

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.run(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()

	select {
	case <-txm.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first fill never reached the transaction manager")
	}
	filler := "0x0000000000000000000000000000000000000010"
	be.setOpen([]backendOrder{
		{OrderID: "o1", OrderStatus: "open", QuoteID: "q1", Filler: &filler},
		{OrderID: "o2", OrderStatus: "open", QuoteID: "q2", Filler: &filler},
	})

	deadline := time.Now().Add(5 * time.Second)
	for !st.reserved("o2") {
		if time.Now().After(deadline) {
			t.Fatal("order won during an in-flight fill was never reserved")
		}
		time.Sleep(time.Millisecond)
	}
	if got := reservedOn(st, vltFillCapacity()); got.Cmp(big.NewInt(1_800000)) != 0 {
		t.Fatalf("reserved = %s, want both won orders (1800000)", got)
	}
	close(txm.release)
}
