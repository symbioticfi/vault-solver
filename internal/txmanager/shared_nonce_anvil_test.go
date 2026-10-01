//go:build integration

package txmanager

import (
	"context"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

type sharedAnvilNonceBackend struct {
	*chain.Client

	barrier  chan struct{}
	arrivals atomic.Int64
	mu       sync.Mutex
	initial  []*types.Transaction
}

func newSharedAnvilBackend(t *testing.T, endpoint string) *sharedAnvilNonceBackend {
	t.Helper()
	client, err := chain.Dial(t.Context(), []string{endpoint}, "", "", common.Address{}.Hex(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return &sharedAnvilNonceBackend{Client: client}
}

func (b *sharedAnvilNonceBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if tx.Nonce() == 0 && b.barrier != nil {
		b.mu.Lock()
		b.initial = append(b.initial, tx)
		b.mu.Unlock()
		if b.arrivals.Add(1) == 3 {
			close(b.barrier)
		}
		select {
		case <-b.barrier:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return b.Client.SendTransaction(ctx, tx)
}

func anvilReconciliationConfig() Config {
	return Config{
		ReconcileNonces: true, Confirmations: 1, MaxFeeGwei: 100,
		PollInterval: 10 * time.Millisecond, ReplacementInterval: time.Second,
		PendingTimeout: time.Minute, ShutdownTimeout: time.Second,
	}
}

func waitForReconciledAvailability(t *testing.T, m *Manager) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for !m.Available() {
		select {
		case <-ctx.Done():
			t.Fatal("canonical progress never reopened replica admission")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestAnvilReconcileThreeIndependentSameKeyManagers(t *testing.T) {
	rpcClient, ethClient, endpoint := startAnvilWithoutMining(t)
	mineAnvilBlock(t, rpcClient)
	backend := newSharedAnvilBackend(t, endpoint)
	backend.barrier = make(chan struct{})
	sgnr := anvilSigner(t)
	managers := make([]*Manager, 3)
	results := make([]<-chan Result, 3)
	for i := range managers {
		m := New(backend, sgnr, big.NewInt(31337), anvilReconciliationConfig(), logr.Discard())
		if err := m.Initialize(t.Context()); err != nil {
			t.Fatal(err)
		}
		startManagerForTest(t, m)
		managers[i] = m
	}
	for i, m := range managers {
		result, accepted := m.SendAsync(t.Context(), Request{To: common.BigToAddress(big.NewInt(int64(0xdead1000 + i))), GasLimit: 21_000})
		if !accepted {
			t.Fatal("replica did not admit its independent request")
		}
		results[i] = result
	}
	select {
	case <-backend.barrier:
	case <-time.After(5 * time.Second):
		t.Fatal("independent signers did not meet the same-nonce race barrier")
	}
	waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(poolTransaction) bool { return true })
	mineAnvilBlock(t, rpcClient)
	mineAnvilBlock(t, rpcClient)
	confirmed, consumed := 0, 0
	for _, result := range results {
		got := waitForTxResult(t, result)
		switch got.Outcome {
		case OutcomeConfirmed:
			confirmed++
		case OutcomeNonceConsumed:
			consumed++
			if got.Receipt != nil || got.Outcome.Included() || !errors.Is(got.Err, ErrNonceConsumed) {
				t.Fatalf("losing replica fabricated execution: %+v", got)
			}
		case OutcomeIncludedUnconfirmed, OutcomeReverted, OutcomeCancelled, OutcomeCancelledUnconfirmed, OutcomeSubmissionError, OutcomeTrackingStopped:
			t.Fatalf("unexpected race outcome: %+v", got)
		}
	}
	if confirmed != 1 || consumed != 2 {
		t.Fatalf("race outcomes confirmed=%d consumed=%d", confirmed, consumed)
	}
	backend.mu.Lock()
	initial := append([]*types.Transaction(nil), backend.initial...)
	backend.mu.Unlock()
	canonical := 0
	for _, tx := range initial {
		if _, err := ethClient.TransactionReceipt(t.Context(), tx.Hash()); err == nil {
			canonical++
		} else if !errors.Is(err, ethereum.NotFound) {
			t.Fatal(err)
		}
	}
	if canonical != 1 {
		t.Fatalf("canonical initial winners = %d", canonical)
	}
	for i, m := range managers {
		waitForReconciledAvailability(t, m)
		result, accepted := m.SendAsync(t.Context(), Request{To: common.BigToAddress(big.NewInt(int64(0xbeef2000 + i))), GasLimit: 21_000})
		if !accepted {
			t.Fatal("healed replica did not admit fresh work")
		}
		fresh := waitForPoolTransaction(t, rpcClient, sgnr.Address(), uint64(i+1), func(poolTransaction) bool { return true })
		mineAnvilBlock(t, rpcClient)
		mineAnvilBlock(t, rpcClient)
		got := waitForTxResult(t, result)
		if got.Outcome != OutcomeConfirmed || got.Hash.Hex() != fresh.Hash {
			t.Fatalf("replica %d fresh nonce did not confirm: %+v", i, got)
		}
	}
}

func TestAnvilReconcileStaleUnknownNonceWithCappedCancellation(t *testing.T) {
	for _, dropped := range []bool{false, true} {
		t.Run(map[bool]string{false: "pending predecessor", true: "dropped predecessor"}[dropped], func(t *testing.T) {
			rpcClient, ethClient, endpoint := startAnvilWithoutMining(t)
			mineAnvilBlock(t, rpcClient)
			sgnr := anvilSigner(t)
			to := common.HexToAddress("0xdead")
			old, err := sgnr.SignTx(t.Context(), types.NewTx(&types.DynamicFeeTx{
				ChainID: big.NewInt(31337), Nonce: 0, To: &to, Gas: 21_000,
				GasFeeCap: big.NewInt(10e9), GasTipCap: big.NewInt(1e9),
			}), big.NewInt(31337))
			if err != nil {
				t.Fatal(err)
			}
			if err := ethClient.SendTransaction(t.Context(), old); err != nil {
				t.Fatal(err)
			}
			backend := newSharedAnvilBackend(t, endpoint)
			cfg := anvilReconciliationConfig()
			cfg.PendingTimeout = 100 * time.Millisecond
			m := New(backend, sgnr, big.NewInt(31337), cfg, logr.Discard())
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			initialized := make(chan error, 1)
			go func() { initialized <- m.Initialize(ctx) }()
			if dropped {
				for {
					m.reconcileMu.Lock()
					observed := m.unknownNonce != nil
					m.reconcileMu.Unlock()
					if observed {
						break
					}
					select {
					case <-ctx.Done():
						t.Fatal("unknown predecessor was not observed")
					case <-time.After(time.Millisecond):
					}
				}
				dropAnvilTransaction(t, rpcClient, old.Hash().Hex())
			}
			cancellation := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(tx poolTransaction) bool {
				return tx.Hash != old.Hash().Hex()
			})
			mineAnvilBlock(t, rpcClient)
			mineAnvilBlock(t, rpcClient)
			if err := <-initialized; err != nil || !m.Available() {
				t.Fatalf("startup recovery did not confirm: %v", err)
			}
			tx, _, err := ethClient.TransactionByHash(t.Context(), common.HexToHash(cancellation.Hash))
			if err != nil || tx.To() == nil || *tx.To() != sgnr.Address() || tx.Nonce() != 0 || tx.GasFeeCap().Cmp(m.globalFeeLimit()) != 0 || tx.GasTipCap().Cmp(m.globalFeeLimit()) != 0 || len(tx.Data()) != 0 || tx.Value().Sign() != 0 {
				t.Fatalf("recovery cancellation payload: tx=%v err=%v", tx, err)
			}
		})
	}
}
