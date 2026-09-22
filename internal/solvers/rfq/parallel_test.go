package rfq

import (
	"context"
	"math/big"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type parallelOrderBackend struct {
	*fakeBackend

	orders map[string]*backendOrder
}

func (b *parallelOrderBackend) getExecutableOrder(_ context.Context, id, _ string) (*backendOrder, error) {
	return b.orders[id], nil
}

type parallelTxSender struct {
	started chan txmanager.Request
	results chan txmanager.Result
}

func (s *parallelTxSender) Send(_ context.Context, req txmanager.Request) txmanager.Result {
	s.started <- req
	return <-s.results
}

func TestParallelOrdersBoundedDeduplicatedAndDrained(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		st, base := fillFixtures(t)
		base.open = nil
		base.order = nil
		backend := &parallelOrderBackend{fakeBackend: base, orders: make(map[string]*backendOrder)}
		for i, id := range []string{"o1", "o2", "o3"} {
			o := *base.executable
			o.OrderID = id
			order := sampleOrder()
			order.Request.Nonce = big.NewInt(int64(i + 1))
			encoded, err := orderTupleArgs.Pack(order)
			if err != nil {
				t.Fatal(err)
			}
			o.EncodedOrder = strPtr(hexutil.Encode(encoded))
			backend.orders[id] = &o
			base.open = append(base.open, o)
		}
		txm := &parallelTxSender{started: make(chan txmanager.Request, 10), results: make(chan txmanager.Result, 10)}
		e := newExec(t, st, backend, txm)
		e.discountsEnabled = false
		e.maxConcurrentOrders = 2
		e.vaults = []recoveryVault{{Adapter: vlt}}
		e.reader = &fakeRecoveryReader{
			permInv:  []solverInventory{testInventory(vlt, tIn, tOut, big.NewInt(9_000_000), big.NewInt(1e18))},
			quoteOut: map[common.Address]*big.Int{tOut: big.NewInt(900_000)},
		}
		e.pollInterval = time.Second
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan struct{})
		go func() { e.run(ctx); close(done) }()
		defer func() { cancel(); close(txm.results); <-done }()
		synctest.Wait()
		if len(txm.started) != 2 {
			t.Fatalf("started %d fills, want two before either finishes", len(txm.started))
		}
		time.Sleep(3 * time.Second)
		synctest.Wait()
		if len(txm.started) != 2 {
			t.Fatal("polling exceeded capacity or submitted a duplicate")
		}
		txm.results <- confirmedTxResult()
		synctest.Wait()
		time.Sleep(time.Second)
		synctest.Wait()
		if len(txm.started) != 3 {
			t.Fatalf("free worker did not pick up remaining order: %d", len(txm.started))
		}
		cancel()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("shutdown abandoned accepted fills")
		default:
		}
		txm.results <- confirmedTxResult()
		txm.results <- confirmedTxResult()
		synctest.Wait()
		<-done
		for _, id := range []string{"o1", "o2", "o3"} {
			if o := st.order(id); o == nil || o.Status != statusSubmitted {
				t.Fatalf("%s: %+v", id, o)
			}
		}
	})
}

func TestOrderWorkerRefreshesStateAfterAcquiringOwnership(t *testing.T) {
	st, backend := fillFixtures(t)
	txm := &fakeTxm{result: confirmedTxResult()}
	e := newExec(t, st, backend, txm)
	st.upsertQueued(queuedOrder{OrderID: "o1", QuoteID: "q1"})
	stale := st.order("o1")
	// An earlier worker finishes after the poll snapshot, before dispatch acquires
	// the per-order slot. The new worker must observe the terminal state.
	st.markStatus("o1", statusFilled, confirmedTxResult().Hash, "")
	e.handleOrder(t.Context(), stale)
	if txm.calls != 0 {
		t.Fatal("stale poll snapshot submitted an already completed order")
	}
}
