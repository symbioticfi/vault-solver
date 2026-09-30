//go:build integration

package txmanager

import (
	"bytes"
	"context"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

func TestAnvilTxManagerJournalRecovery(t *testing.T) {
	t.Run("original mined before restart", func(t *testing.T) { testAnvilJournalRecovery(t, true) })
	t.Run("unresolved original cancelled at same nonce", func(t *testing.T) { testAnvilJournalRecovery(t, false) })
}

// The real chain handles inclusion and replacement. The wrapper records immutable
// signed attempts under a mutex so assertions cannot race the lifecycle worker.
type journalAnvilBackend struct {
	*ethclient.Client

	mu   sync.Mutex
	sent []*types.Transaction
}

func (b *journalAnvilBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	b.mu.Lock()
	b.sent = append(b.sent, tx)
	b.mu.Unlock()
	return b.Client.SendTransaction(ctx, tx)
}

func (b *journalAnvilBackend) attempts() []*types.Transaction {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*types.Transaction(nil), b.sent...)
}

func testAnvilJournalRecovery(t *testing.T, originalMined bool) {
	t.Helper()
	rpcClient, client, _ := startAnvilWithoutMining(t)
	backend := &journalAnvilBackend{Client: client}
	sgnr := anvilSigner(t)
	path := filepath.Join(t.TempDir(), "transactions.json")
	cfg := Config{
		StateFile: path, MaxFeeGwei: 100, Confirmations: 0,
		PollInterval: 20 * time.Millisecond, ReplacementInterval: 200 * time.Millisecond,
		PendingTimeout: time.Minute, ShutdownTimeout: time.Second,
		Horizon: HorizonConfig{BlockTime: 200 * time.Millisecond},
	}
	first := New(backend, sgnr, big.NewInt(31337), cfg, logr.Discard())
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := first.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	original, err := first.broadcast(managerCtx(t.Context(), first), Request{
		To: common.HexToAddress("0xdead"), GasLimit: 21_000, Label: "journal recovery", Solver: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	poolOriginal := waitForPoolTransaction(t, rpcClient, sgnr.Address(), original.nonce, func(tx poolTransaction) bool {
		return strings.EqualFold(tx.Hash, original.originalHash.Hex())
	})
	if originalMined {
		mineAnvilBlock(t, rpcClient)
	}
	// No lifecycle worker is started in the first process, so closing its lock
	// models a crash after submission while leaving the write-ahead state intact.
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	latest, err := client.NonceAt(t.Context(), sgnr.Address(), nil)
	if err != nil {
		t.Fatal(err)
	}
	poolNonce, err := client.PendingNonceAt(t.Context(), sgnr.Address())
	if err != nil {
		t.Fatal(err)
	}
	wantLatest := original.nonce
	if originalMined {
		wantLatest++
	}
	if latest != wantLatest || poolNonce != original.nonce+1 {
		t.Fatalf("restart account nonce = latest %d pending %d, want %d/%d", latest, poolNonce, wantLatest, original.nonce+1)
	}
	restarted := New(backend, sgnr, big.NewInt(31337), cfg, logr.Discard())
	if err := restarted.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if restarted.recovered == nil || restarted.Available() || restarted.LaneReady() || restarted.Idle() {
		t.Fatal("recovered nonce did not block fresh admission before Start")
	}
	assertAnvilSignedBytes(t, restarted.recovered.attempts[0].tx, original.attempts[0].tx)
	results := make(chan Result, 1)
	restarted.recovered.result = results
	startAnvilJournalRecovery(t, restarted)
	winnerHash := original.originalHash
	if !originalMined {
		cancellation := waitForPoolTransaction(t, rpcClient, sgnr.Address(), original.nonce, func(tx poolTransaction) bool {
			return strings.EqualFold(tx.To, sgnr.Address().Hex()) && tx.Input == "0x" && tx.Value == "0x0"
		})
		if strings.EqualFold(cancellation.Hash, poolOriginal.Hash) || cancellation.Gas != "0x5208" {
			t.Fatalf("invalid recovered cancellation: %+v", cancellation)
		}
		winnerHash = common.HexToHash(cancellation.Hash)
		mineAnvilBlock(t, rpcClient)
	}
	result := waitForTxResult(t, results)
	wantOutcome := OutcomeConfirmed
	if !originalMined {
		wantOutcome = OutcomeCancelled
	}
	if result.Outcome != wantOutcome || result.Hash != winnerHash || result.Receipt == nil {
		t.Fatalf("recovery result = %+v, want %s hash %s", result, wantOutcome, winnerHash.Hex())
	}
	if originalMined && result.Err != nil {
		t.Fatalf("original recovery failed: %v", result.Err)
	}
	if !originalMined && result.Err == nil {
		t.Fatal("cancellation did not report the original request as cancelled")
	}
	waitJournalReady(t, restarted)
	assertAnvilJournalWinner(t, backend, result, original, originalMined)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canonical zero-confirmation winner did not clear journal: %v", err)
	}
	latest, err = client.NonceAt(t.Context(), sgnr.Address(), nil)
	if err != nil || latest != original.nonce+1 {
		t.Fatalf("resolved nonce = %d, %v; want %d", latest, err, original.nonce+1)
	}
}

func startAnvilJournalRecovery(t *testing.T, m *Manager) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Start(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("journal manager did not finish bounded shutdown")
			return
		}
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	})
}

func assertAnvilSignedBytes(t *testing.T, got, want *types.Transaction) {
	t.Helper()
	gotRaw, err := got.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	wantRaw, err := want.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotRaw, wantRaw) {
		t.Fatal("recovered signed bytes differ from the original transaction")
	}
}

func assertAnvilJournalWinner(t *testing.T, backend *journalAnvilBackend, result Result, original *pendingTransaction, originalMined bool) {
	t.Helper()
	mined, pending, err := backend.TransactionByHash(t.Context(), result.Hash)
	if err != nil || pending {
		t.Fatalf("winning transaction = %v, pending %v, error %v", mined, pending, err)
	}
	if mined.Nonce() != original.nonce || mined.Hash() != result.Receipt.TxHash || result.Receipt.Status != types.ReceiptStatusSuccessful {
		t.Fatalf("wrong mined nonce/hash/status: tx=%s nonce=%d receipt=%+v", mined.Hash().Hex(), mined.Nonce(), result.Receipt)
	}
	header, err := backend.HeaderByNumber(t.Context(), result.Receipt.BlockNumber)
	if err != nil || header.Hash() != result.Receipt.BlockHash {
		t.Fatalf("winning receipt is not canonical: %v", err)
	}
	attempts := backend.attempts()
	if originalMined {
		if len(attempts) != 1 {
			t.Fatalf("mined original was replayed: %d broadcasts", len(attempts))
		}
		assertAnvilSignedBytes(t, mined, original.attempts[0].tx)
		return
	}
	if len(attempts) < 2 {
		t.Fatal("recovery did not broadcast a cancellation")
	}
	signingRule := types.LatestSignerForChainID(big.NewInt(31337))
	wantSender, err := types.Sender(signingRule, original.attempts[0].tx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, attempt := range attempts[1:] {
		sender, err := types.Sender(signingRule, attempt)
		if err != nil || sender != wantSender || attempt.To() == nil || *attempt.To() != sender || attempt.Nonce() != original.nonce ||
			attempt.Value().Sign() != 0 || len(attempt.Data()) != 0 || attempt.Gas() != 21_000 {
			t.Fatalf("recovery replayed business work or changed nonce: tx=%s, err=%v", attempt.Hash().Hex(), err)
		}
		if attempt.Hash() == mined.Hash() {
			found = true
			assertAnvilSignedBytes(t, mined, attempt)
		}
	}
	if !found {
		t.Fatal("mined cancellation was not one of the manager's signed attempts")
	}
}
