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
)

// Every healthy call is slower than zero but faster than its own RPC budget. A long ancestry walk
// must not consume the budgets of subsequent calls. synctest keeps these scenarios deterministic.
type delayedNonceRPCBackend struct {
	*hashNonceTestBackend

	delay       time.Duration
	stuck       string
	parentReads int
}

func (b *delayedNonceRPCBackend) wait(ctx context.Context, operation string) error {
	if b.stuck == operation {
		<-ctx.Done()
		return ctx.Err()
	}
	timer := time.NewTimer(b.delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *delayedNonceRPCBackend) NonceAt(ctx context.Context, account common.Address, number *big.Int) (uint64, error) {
	if err := b.wait(ctx, "latest"); err != nil {
		return 0, err
	}
	return b.hashNonceTestBackend.NonceAt(ctx, account, number)
}

func (b *delayedNonceRPCBackend) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	if err := b.wait(ctx, "pending"); err != nil {
		return 0, err
	}
	return b.hashNonceTestBackend.PendingNonceAt(ctx, account)
}

func (b *delayedNonceRPCBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	operation := "head"
	if number != nil {
		operation = "ancestor"
	}
	if err := b.wait(ctx, operation); err != nil {
		return nil, err
	}
	return b.hashNonceTestBackend.HeaderByNumber(ctx, number)
}

func (b *delayedNonceRPCBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	b.parentReads++
	if err := b.wait(ctx, "parent"); err != nil {
		return nil, err
	}
	return b.hashNonceTestBackend.HeaderByHash(ctx, hash)
}

func (b *delayedNonceRPCBackend) ReadNonceAtHash(ctx context.Context, account common.Address, hash common.Hash) (uint64, error) {
	operation := "state"
	if hash == receiptTestHeader(b.head).Hash() {
		operation = "current"
	}
	if err := b.wait(ctx, operation); err != nil {
		return 0, err
	}
	return b.hashNonceTestBackend.ReadNonceAtHash(ctx, account, hash)
}

func (b *delayedNonceRPCBackend) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	if err := b.wait(ctx, "receipt"); err != nil {
		return nil, err
	}
	return b.hashNonceTestBackend.TransactionReceipt(ctx, hash)
}

func newDelayedNonceRPCManager(t *testing.T) (*Manager, *delayedNonceRPCBackend) {
	t.Helper()
	b := &delayedNonceRPCBackend{
		hashNonceTestBackend: &hashNonceTestBackend{mockBackend: newMockBackend(), confirmedNonce: 7},
		delay:                2 * time.Millisecond,
	}
	m := sharedNonceManager(t, b, 64)
	m.cfg.ReplacementInterval = 10 * time.Millisecond // Each RPC gets 5ms; the healthy proof needs >128ms.
	return m, b
}

func TestNonceReconciliationHealthyCallsHaveIndependentBudgets(t *testing.T) {
	for _, operation := range []string{"fresh admission", "consumed nonce", "tracked receipt", "contested cancellation"} {
		t.Run(operation, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m, b := newDelayedNonceRPCManager(t)
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				started := time.Now()
				tx := types.NewTx(&types.DynamicFeeTx{Nonce: 7, To: ptr(common.HexToAddress("0x123")), Gas: 21_000})
				pending := &pendingTransaction{nonce: 7, originalHash: tx.Hash(), attempts: []txAttempt{{hash: tx.Hash(), tx: tx}}}
				switch operation {
				case "fresh admission":
					if nonce, err := m.freshReconciledNonce(ctx); err != nil || nonce != 7 || !m.Available() {
						t.Fatalf("healthy deeper proof could not initialize admission: nonce=%d err=%v", nonce, err)
					}
				case "consumed nonce":
					b.latestNonce, b.pendingNonce, b.confirmedNonce = 8, 8, 8
					result, consumed := m.confirmConsumedNonce(ctx, pending)
					if !consumed || !errors.Is(result.Err, ErrNonceConsumed) || result.Receipt != nil || !m.Available() {
						t.Fatalf("healthy deeper proof could not reconcile consumption: %+v", result)
					}
				case "tracked receipt":
					b.receipts[tx.Hash()] = successfulReceipt(tx, b.head-64)
					if !m.hasCanonicalTrackedReceipt(ctx, pending) {
						t.Fatal("healthy deeper receipt ancestry did not reconcile")
					}
				case "contested cancellation":
					if err := m.cancelContestedNonce(ctx, pending); err != nil || len(pending.attempts) != 2 {
						t.Fatalf("healthy current-state proof truncated capped cancellation: %v", err)
					}
				}
				if ctx.Err() != nil || time.Since(started) <= m.receiptReadTimeout() {
					t.Fatal("proof did not complete beyond one RPC budget within its caller deadline")
				}
				if operation != "contested cancellation" && b.parentReads < 64 {
					t.Fatalf("deeper canonical ancestry was not exercised: %d parents", b.parentReads)
				}
			})
		})
	}
}

func TestNonceReconciliationStuckCallAndCallerCancellationRemainBounded(t *testing.T) {
	for _, operation := range []string{"latest", "pending", "head", "ancestor", "parent", "state", "current", "caller cancellation"} {
		t.Run(operation, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m, b := newDelayedNonceRPCManager(t)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if operation == "caller cancellation" {
					go func() {
						time.Sleep(10 * time.Millisecond)
						cancel()
					}()
				} else {
					b.stuck = operation
				}
				started := time.Now()
				_, err := m.freshReconciledNonce(ctx)
				if err == nil || m.Available() || len(b.attemptedTransactions()) != 0 {
					t.Fatal("unproved nonce admitted fresh work")
				}
				if operation == "caller cancellation" {
					if !errors.Is(err, context.Canceled) || time.Since(started) != 10*time.Millisecond {
						t.Fatalf("proof did not honor caller cancellation: elapsed=%s err=%v", time.Since(started), err)
					}
				} else if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil || time.Since(started) >= time.Second {
					t.Fatalf("single RPC timeout escaped its own bound: elapsed=%s err=%v", time.Since(started), err)
				}
			})
		})
	}
}
