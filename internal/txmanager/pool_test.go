package txmanager

import (
	"context"
	"math/big"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/signer"
)

// Pending receipts are controlled independently of submission, so these tests
// detect accidental serialization, lane reuse, and migrating an owned request.
type poolBackend struct {
	*mockBackend

	sendErr error
}

func (b *poolBackend) SendTransaction(_ context.Context, tx *types.Transaction) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent = append(b.sent, tx)
	return b.sendErr
}

func poolFixture(t *testing.T) (*Pool, []*poolBackend) {
	t.Helper()
	var lanes []*Manager
	var backends []*poolBackend
	for _, key := range []string{testKey, "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"} {
		s, err := signer.NewFromHexKey(key)
		if err != nil {
			t.Fatal(err)
		}
		backend := &poolBackend{mockBackend: newMockBackend()}
		backends = append(backends, backend)
		lanes = append(lanes, New(backend, s, big.NewInt(1), Config{
			PollInterval: time.Second, ReplacementInterval: time.Hour,
			PendingTimeout: time.Hour, ShutdownTimeout: time.Millisecond,
		}, logr.Discard()))
	}
	pool, err := NewPool(lanes...)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return pool, backends
}

func TestPoolParallelAdmissionAndIndependentCompletion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool, backends := poolFixture(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go pool.Start(ctx)
		changes, unsubscribe := pool.SubscribeLaneState()
		defer unsubscribe()
		first, ok := pool.SendAsync(ctx, Request{To: common.HexToAddress("0x1234"), Label: "first"})
		if !ok {
			t.Fatal("first was not admitted")
		}
		synctest.Wait()
		if !pool.LaneReady() || pool.Idle() {
			t.Fatal("a pending primary must leave auxiliary capacity ready")
		}
		second, ok := pool.SendAsync(ctx, Request{To: common.HexToAddress("0x5678"), Label: "second"})
		if !ok {
			t.Fatal("second was not admitted")
		}
		synctest.Wait()
		if pool.LaneReady() {
			t.Fatal("all lanes occupied but pool is ready")
		}
		if _, accepted := pool.TrySend(ctx, Request{Label: "busy"}); accepted {
			t.Fatal("busy pool admitted a third request")
		}
		for i, b := range backends {
			b.mu.Lock()
			count := len(b.sent)
			b.mu.Unlock()
			if count != 1 {
				t.Fatalf("lane %d sent %d transactions", i, count)
			}
		}
		// The auxiliary can finish first, making room while the primary remains pending.
		backends[1].mu.Lock()
		backends[1].receipts[backends[1].sent[0].Hash()] = successfulReceipt(backends[1].sent[0], 100)
		backends[1].mu.Unlock()
		time.Sleep(time.Second)
		synctest.Wait()
		if res := <-second; res.Outcome != OutcomeConfirmed {
			t.Fatalf("second = %+v", res)
		}
		select {
		case <-first:
			t.Fatal("primary unexpectedly finished")
		default:
		}
		if !pool.LaneReady() {
			t.Fatal("completed auxiliary did not release capacity")
		}
		select {
		case <-changes:
		default:
			t.Fatal("missing readiness notification")
		}
		backends[0].mu.Lock()
		backends[0].receipts[backends[0].sent[0].Hash()] = successfulReceipt(backends[0].sent[0], 100)
		backends[0].mu.Unlock()
		time.Sleep(time.Second)
		synctest.Wait()
		if res := <-first; res.Outcome != OutcomeConfirmed {
			t.Fatalf("first = %+v", res)
		}
		if !pool.Idle() {
			t.Fatal("pool not idle after completion")
		}
	})
}

func TestPoolRejectsDuplicateSenders(t *testing.T) {
	m := New(newMockBackend(), mustSigner(t), big.NewInt(1), Config{}, logr.Discard())
	if _, err := NewPool(m, m); err == nil {
		t.Fatal("duplicate signer would race on its nonce")
	}
}

func TestPoolAdmissionDeadlineAndShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool, backends := poolFixture(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan struct{})
		go func() { pool.Start(ctx); close(done) }()
		for range pool.Capacity() {
			if _, accepted := pool.SendAsync(ctx, Request{To: common.HexToAddress("0x1234")}); !accepted {
				t.Fatal("not admitted")
			}
		}
		synctest.Wait()
		res := pool.Send(ctx, Request{CancelAt: time.Now().Add(100 * time.Millisecond)})
		if !res.NotAdmitted || !errors.Is(res.Err, context.DeadlineExceeded) {
			t.Fatalf("expired admission = %+v", res)
		}
		for _, backend := range backends {
			backend.mu.Lock()
			count := len(backend.sent)
			backend.mu.Unlock()
			if count != 1 {
				t.Fatalf("expired request broadcast: %d", count)
			}
		}
		waiting := make(chan Result, 1)
		go func() { waiting <- pool.Send(t.Context(), Request{}) }()
		synctest.Wait()
		cancel()
		synctest.Wait()
		if stopped := <-waiting; !stopped.NotAdmitted {
			t.Fatalf("shutdown admission = %+v", stopped)
		}
		<-done
		if pool.LaneReady() || pool.Available() {
			t.Fatal("stopped pool advertises capacity")
		}
	})
}

func TestPoolDoesNotMigrateAmbiguousBroadcast(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool, backends := poolFixture(t)
		backends[0].sendErr = context.DeadlineExceeded
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan struct{})
		go func() { pool.Start(ctx); close(done) }()
		result, _ := pool.SendAsync(ctx, Request{To: common.HexToAddress("0x1234")})
		synctest.Wait()
		select {
		case res := <-result:
			t.Fatalf("ambiguous send relinquished ownership: %+v", res)
		default:
		}
		backends[1].mu.Lock()
		count := len(backends[1].sent)
		backends[1].mu.Unlock()
		if count != 0 {
			t.Fatal("ambiguous send replayed on auxiliary account")
		}
		pool.lanes[0].markNonceConflict(7, common.HexToHash("0x1234"))
		if !pool.LaneReady() {
			t.Fatal("conflicted primary blocked unused auxiliary")
		}
		cancel()
		<-done
	})
}
