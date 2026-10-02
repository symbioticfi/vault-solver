//go:build integration

package txmanager

import (
	"context"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
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
	client, err := chain.Dial(t.Context(), []string{endpoint}, "", common.Address{}.Hex(), time.Second)
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
		Confirmations: 1, MaxFeeGwei: 100,
		PollInterval: 10 * time.Millisecond, ReplacementInterval: time.Second,
		PendingTimeout: time.Minute, ShutdownTimeout: time.Second,
	}
}

func assertAnvilReconciliationOutcome(t *testing.T, result Result, expectedHash common.Hash) {
	t.Helper()
	if result.Hash != expectedHash || result.NotAdmitted {
		t.Fatalf("reconciliation changed the admitted transaction identity: %+v, expected hash %s", result, expectedHash)
	}
	if result.Outcome == OutcomeConfirmed {
		if result.Err != nil || result.Receipt == nil || result.Receipt.TxHash != expectedHash || !result.Outcome.Included() {
			t.Fatalf("confirmed result lacks owned inclusion: %+v", result)
		}
		return
	}
	if result.Outcome == OutcomeNonceConsumed {
		if result.Receipt != nil || result.Outcome.Included() || !errors.Is(result.Err, ErrNonceConsumed) {
			t.Fatalf("nonce-only result fabricated execution: %+v", result)
		}
		return
	}
	t.Fatalf("unexpected reconciliation outcome: %+v", result)
}

func assertAnvilCanonicalTransaction(t *testing.T, client *ethclient.Client, hash common.Hash, nonce uint64) {
	t.Helper()
	tx, pending, err := client.TransactionByHash(t.Context(), hash)
	if err != nil || pending || tx.Nonce() != nonce {
		t.Fatalf("transaction %s did not consume expected nonce %d: tx=%v pending=%v err=%v", hash, nonce, tx, pending, err)
	}
	receipt, err := client.TransactionReceipt(t.Context(), hash)
	if err != nil || receipt.TxHash != hash || receipt.Status != types.ReceiptStatusSuccessful {
		t.Fatalf("transaction %s lacks successful receipt: receipt=%v err=%v", hash, receipt, err)
	}
	canonical, err := client.HeaderByNumber(t.Context(), receipt.BlockNumber)
	if err != nil || canonical.Hash() != receipt.BlockHash {
		t.Fatalf("transaction %s receipt is not canonical: header=%v receipt=%v err=%v", hash, canonical, receipt, err)
	}
	head, err := client.HeaderByNumber(t.Context(), nil)
	confirmedAt := new(big.Int).Add(receipt.BlockNumber, big.NewInt(1))
	if err != nil || head.Number.Cmp(confirmedAt) < 0 {
		t.Fatalf("transaction %s lacks confirmation depth %d: head=%v receipt=%v err=%v", hash, 1, head, receipt, err)
	}
}

// Three managers share a key and RPC, with no state exchanged between managers. The test caller
// retries raced orders, modelling the solver's fresh protocol-state check rather than calldata replay.
func TestAnvilThreeSameKeyReplicasExecuteDistinctPendingOrders(t *testing.T) {
	rpcClient, ethClient, endpoint := startAnvilWithoutMining(t)
	backend := newSharedAnvilBackend(t, endpoint)
	backend.barrier = make(chan struct{})
	sgnr := anvilSigner(t)
	managers := make([]*Manager, 3)
	results := make([]<-chan Result, 3)
	requests := make([]Request, 3)
	for i := range managers {
		m := New(backend, sgnr, big.NewInt(31337), anvilReconciliationConfig(), logr.Discard())
		if err := m.Initialize(t.Context()); err != nil {
			t.Fatal(err)
		}
		startManagerForTest(t, m)
		managers[i] = m
		requests[i] = Request{To: common.BigToAddress(big.NewInt(int64(0xdead1000 + i))), GasLimit: 21_000}
		var accepted bool
		results[i], accepted = m.SendAsync(t.Context(), requests[i])
		if !accepted {
			t.Fatal("replica did not admit its order")
		}
	}
	select {
	case <-backend.barrier:
	case <-time.After(5 * time.Second):
		t.Fatal("replicas did not meet the nonce race barrier")
	}
	winner := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(poolTransaction) bool { return true })
	hashes := make([]common.Hash, 3)
	nonces := make([]uint64, 3)
	nextNonce := uint64(1)
	for i, m := range managers {
		if requests[i].To == common.HexToAddress(winner.To) {
			hashes[i] = common.HexToHash(winner.Hash)
			continue
		}
		raced := waitForTxResult(t, results[i])
		if raced.Outcome != OutcomeNonceConflict || !errors.Is(raced.Err, ErrNonceConflict) || raced.Receipt != nil || raced.Outcome.Included() {
			t.Fatalf("initial losing order: %+v", raced)
		}
		waitForAdmissionDemand(t, m, 0)
		if !m.LaneReady() {
			t.Fatal("nonce race paused fresh local work")
		}
		var admitted bool
		results[i], admitted = m.SendAsync(t.Context(), requests[i])
		if !admitted {
			t.Fatal("fresh order was not admitted")
		}
		fresh := waitForPoolTransaction(t, rpcClient, sgnr.Address(), nextNonce, func(tx poolTransaction) bool {
			return common.HexToAddress(tx.To) == requests[i].To
		})
		hashes[i], nonces[i] = common.HexToHash(fresh.Hash), nextNonce
		nextNonce++
	}
	// All three distinct business transactions coexist before the first inclusion.
	for nonce := uint64(0); nonce < 3; nonce++ {
		tx := waitForPoolTransaction(t, rpcClient, sgnr.Address(), nonce, func(poolTransaction) bool { return true })
		matched := false
		for _, req := range requests {
			if common.HexToAddress(tx.To) == req.To {
				matched = true
			}
		}
		if !matched {
			t.Fatalf("nonce %d does not contain a submitted business order: %+v", nonce, tx)
		}
	}
	mineAnvilBlock(t, rpcClient)
	mineAnvilBlock(t, rpcClient)
	for i, result := range results {
		got := waitForTxResult(t, result)
		assertAnvilReconciliationOutcome(t, got, hashes[i])
		assertAnvilCanonicalTransaction(t, ethClient, hashes[i], nonces[i])
	}
}

func TestAnvilForeignPendingTransactionPreservesIndependentBusinessWork(t *testing.T) {
	rpcClient, ethClient, endpoint := startAnvilWithoutMining(t)
	sgnr := anvilSigner(t)
	to := common.HexToAddress("0xdead")
	foreign, err := sgnr.SignTx(t.Context(), types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(31337), Nonce: 0, To: &to, Gas: 21_000,
		GasFeeCap: big.NewInt(10e9), GasTipCap: big.NewInt(1e9),
	}), big.NewInt(31337))
	if err != nil {
		t.Fatal(err)
	}
	if err := ethClient.SendTransaction(t.Context(), foreign); err != nil {
		t.Fatal(err)
	}
	backend := newSharedAnvilBackend(t, endpoint)
	cfg := anvilReconciliationConfig()
	cfg.PendingTimeout = 200 * time.Millisecond
	m := New(backend, sgnr, big.NewInt(31337), cfg, logr.Discard())
	if err := m.Initialize(t.Context()); err != nil || !m.LaneReady() {
		t.Fatalf("foreign pending startup: %v", err)
	}
	startManagerForTest(t, m)
	time.Sleep(600 * time.Millisecond) // Beyond this replica's local pending timeout.
	unchanged := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(poolTransaction) bool { return true })
	if unchanged.Hash != foreign.Hash().Hex() {
		t.Fatal("replica changed a transaction it did not send")
	}
	// Admit this replica's own new request behind the foreign transaction.
	result, admitted := m.SendAsync(t.Context(), Request{To: common.HexToAddress("0xbeef"), GasLimit: 21_000})
	if !admitted {
		t.Fatal("foreign pending transaction blocked fresh work")
	}
	fresh := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 1, func(poolTransaction) bool { return true })
	mineAnvilBlock(t, rpcClient)
	mineAnvilBlock(t, rpcClient)
	assertAnvilReconciliationOutcome(t, waitForTxResult(t, result), common.HexToHash(fresh.Hash))
	assertAnvilCanonicalTransaction(t, ethClient, foreign.Hash(), 0)
	assertAnvilCanonicalTransaction(t, ethClient, common.HexToHash(fresh.Hash), 1)
}
