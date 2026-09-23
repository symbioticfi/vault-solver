package rfq

import (
	"context"
	"math/big"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func vltFillCapacity() liquidlane.CapacityID {
	return liquidlane.RouteCapacityID(testInventory(vlt, tIn, tOut, maxUint256(), maxUint256()).Route)
}

func reservedOn(st *store, capacityID liquidlane.CapacityID) *big.Int {
	if amount := st.pendingReservations("", nil)[capacityID]; amount != nil {
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
			srv.quotes.reservations = func(string, []solverInventory) liquidlane.CapacityReservations { return reservations }

			rr := do(t, srv.handler(), http.MethodPost, "/quote", testSecret, validQuoteBody())
			if rr.Code != tc.want {
				t.Fatalf("quote status = %d, want %d (body %s)", rr.Code, tc.want, rr.Body.String())
			}
		})
	}
}

// The poll loop reserves a won order only while the submitter is busy with another fill. An idle
// submitter is woken instead and reserves through its own plan, so each order is planned once.
func TestPollReservesWonOrderOnlyWhileSubmitterBusy(t *testing.T) {
	for _, busy := range []bool{true, false} {
		t.Run(map[bool]string{true: "busy submitter", false: "idle submitter"}[busy], func(t *testing.T) {
			st, be := fillFixtures(t)
			txm := &fakeTxm{result: confirmedTxResult()}
			e := newExec(t, st, be, txm)
			plans := 0
			e.strategy = fixedFillStrategy{plan: baseFillPlan(), onBuild: func() { plans++ }}
			e.sending.Store(busy)

			e.syncOnce(t.Context())

			if txm.calls != 0 {
				t.Fatalf("poll sent %d transactions, want none", txm.calls)
			}
			want := big.NewInt(0)
			if busy {
				want = big.NewInt(900000)
			}
			if got := reservedOn(st, vltFillCapacity()); got.Cmp(want) != 0 {
				t.Fatalf("reserved = %s, want %s", got, want)
			}
			if (plans == 1) != busy {
				t.Fatalf("plans = %d during poll, want 1 only for a busy submitter", plans)
			}
		})
	}
}

// A reserved order the backend stops reporting cannot hold capacity forever: unsigned work expires at
// the order's own deadline.
func TestReservationExpiresAtOrderDeadlineWhenBackendForgetsOrder(t *testing.T) {
	st, be := fillFixtures(t)
	now := time.Unix(1_000, 0)
	st.now = func() time.Time { return now }
	e := newExec(t, st, be, &fakeTxm{result: confirmedTxResult()})
	e.now = st.now
	e.reader.(*fakeRecoveryReader).chainTime = time.Unix(4_102_444_700, 0)
	e.sending.Store(true) // a busy submitter: the poll loop reserves

	e.syncOnce(t.Context())
	if !st.reserved("o1") {
		t.Fatal("won order was not reserved")
	}
	be.executable, be.order, be.open = nil, nil, nil

	syncCycle(t.Context(), e)
	if !st.reserved("o1") {
		t.Fatal("reservation released before the order deadline")
	}
	now = now.Add(200 * time.Second) // the deadline is 100 s past the observed chain time
	syncCycle(t.Context(), e)

	if rec := st.order("o1"); rec == nil || rec.Status != statusExpired {
		t.Fatalf("order = %+v, want expired at its deadline", rec)
	}
	if st.reserved("o1") {
		t.Fatal("expired order kept its reservation")
	}
}

// The submitter records the same bound when it plans an order itself.
func TestSubmissionRecordsOrderDeadlineBound(t *testing.T) {
	st, be := fillFixtures(t)
	e := newExec(t, st, be, &fakeTxm{result: confirmedTxResult()})
	e.now = func() time.Time { return time.Unix(1_000, 0) }
	e.reader.(*fakeRecoveryReader).chainTime = time.Unix(4_102_444_700, 0)

	syncCycle(t.Context(), e)

	if want := time.Unix(1_100, 0); !st.order("o1").RetryDeadline.Equal(want) {
		t.Fatalf("recorded deadline = %v, want chain deadline translated to %v", st.order("o1").RetryDeadline, want)
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

// A snapshot read at or after a fill's confirmed inclusion already reflects it, so the order's
// reservation is not subtracted from that snapshot. Earlier or unknown snapshot blocks still are.
func TestPendingReservationsHonourSnapshotBlock(t *testing.T) {
	capacityID := vltFillCapacity()
	item := func(block uint64) solverInventory {
		inv := testInventory(vlt, tIn, tOut, maxUint256(), maxUint256())
		inv.BlockNumber = block
		return inv
	}
	for _, tc := range []struct {
		name       string
		includedAt uint64
		inventory  []solverInventory
		subtracted bool
	}{
		{name: "not yet included, snapshot known", inventory: []solverInventory{item(100)}, subtracted: true},
		{name: "snapshot before inclusion", includedAt: 100, inventory: []solverInventory{item(99)}, subtracted: true},
		{name: "snapshot at inclusion", includedAt: 100, inventory: []solverInventory{item(100)}},
		{name: "snapshot after inclusion", includedAt: 100, inventory: []solverInventory{item(150)}},
		{name: "snapshot block unknown", includedAt: 100, inventory: []solverInventory{item(0)}, subtracted: true},
		{name: "one item of the capacity unknown", includedAt: 100, inventory: []solverInventory{item(150), item(0)}, subtracted: true},
		{name: "oldest item of the capacity decides", includedAt: 100, inventory: []solverInventory{item(150), item(99)}, subtracted: true},
		{name: "no inventory given", includedAt: 100, subtracted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newStore(func() time.Time { return time.Unix(0, 0) })
			st.upsertQueued(queuedOrder{OrderID: "o1", QuoteID: "q1"})
			held := liquidlane.CapacityReservations{}
			held.Add(capacityID, big.NewInt(7))
			if !st.reserve("o1", held) {
				t.Fatal("reserve")
			}
			if tc.includedAt != 0 {
				st.markIncluded("o1", tc.includedAt)
			}
			got := st.pendingReservations("", tc.inventory)[capacityID]
			if (got != nil) != tc.subtracted {
				t.Fatalf("reserved = %v, want subtracted %v", got, tc.subtracted)
			}
		})
	}
}

func TestConfirmedFillRecordsInclusionBlock(t *testing.T) {
	st, be := fillFixtures(t)
	be.order.OrderStatus = "open" // keep the order active so its record is inspectable
	result := confirmedTxResult()
	result.Receipt = &ethtypes.Receipt{TxHash: result.Hash, Status: ethtypes.ReceiptStatusSuccessful, BlockNumber: big.NewInt(4242)}
	e := newExec(t, st, be, &fakeTxm{result: result})

	syncCycle(t.Context(), e)

	if rec := st.order("o1"); rec == nil || rec.IncludedAt != 4242 {
		t.Fatalf("order = %+v, want inclusion block 4242", rec)
	}
}

// End to end: the backend reports the block its maxAssets was read at; once that block is at or past
// the confirmed fill's inclusion, the quote no longer subtracts the still-open order's reservation.
func TestQuoteUsesBackendSnapshotBlock(t *testing.T) {
	body := validQuoteBody()
	parsed, err := body.toStrategy(1)
	if err != nil || parsed == nil {
		t.Fatalf("parse quote body: %v", err)
	}
	capacityID := liquidlane.RouteCapacityID(parsed.inv[0].Route)
	for _, tc := range []struct {
		name  string
		block *string
		want  int
	}{
		{name: "no block reported", want: http.StatusNoContent},
		{name: "block before inclusion", block: strPtr("99"), want: http.StatusNoContent},
		{name: "block at inclusion", block: strPtr("100"), want: http.StatusOK},
		{name: "malformed block", block: strPtr("0x64"), want: http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newStore(func() time.Time { return time.Unix(0, 0) })
			st.upsertQueued(queuedOrder{OrderID: "o1", QuoteID: "q1"})
			held := liquidlane.CapacityReservations{}
			held.Add(capacityID, big.NewInt(10_000_000)) // the whole advertised capacity
			if !st.reserve("o1", held) {
				t.Fatal("reserve")
			}
			st.markIncluded("o1", 100)
			srv := testServer()
			srv.quotes.reservations = st.pendingReservations
			req := validQuoteBody()
			req.Adapters[0].BlockNumber = tc.block

			rr := do(t, srv.handler(), http.MethodPost, "/quote", testSecret, req)
			if rr.Code != tc.want {
				t.Fatalf("quote status = %d, want %d (body %s)", rr.Code, tc.want, rr.Body.String())
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
