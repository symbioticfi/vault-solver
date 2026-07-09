package txmanager

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/signer"
)

// anvil account #0 — a well-known throwaway key, fine for unit tests.
const testKey = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

type mockBackend struct {
	mu sync.Mutex

	pendingNonce uint64
	tip          *big.Int
	baseFee      *big.Int
	gasEstimate  uint64
	head         uint64

	sendErrs  []error // returned, in order, by successive SendTransaction calls
	sendCalls int
	sent      []*types.Transaction
	receipts  map[common.Hash]*types.Receipt
}

func newMockBackend() *mockBackend {
	return &mockBackend{
		pendingNonce: 7,
		tip:          big.NewInt(1e9),
		baseFee:      big.NewInt(20e9),
		gasEstimate:  50_000,
		head:         100,
		receipts:     map[common.Hash]*types.Receipt{},
	}
}

func (b *mockBackend) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.pendingNonce, nil
}

func (b *mockBackend) SuggestGasTipCap(context.Context) (*big.Int, error) { return b.tip, nil }

func (b *mockBackend) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return &types.Header{Number: new(big.Int).SetUint64(b.head), BaseFee: b.baseFee}, nil
}

func (b *mockBackend) EstimateGas(context.Context, ethereum.CallMsg) (uint64, error) {
	if b.gasEstimate == 0 {
		return 0, errors.New("estimate failed")
	}
	return b.gasEstimate, nil
}

func (b *mockBackend) SendTransaction(_ context.Context, tx *types.Transaction) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	i := b.sendCalls
	b.sendCalls++
	if i < len(b.sendErrs) && b.sendErrs[i] != nil {
		return b.sendErrs[i]
	}
	b.sent = append(b.sent, tx)
	b.receipts[tx.Hash()] = &types.Receipt{
		Status:      types.ReceiptStatusSuccessful,
		TxHash:      tx.Hash(),
		BlockNumber: new(big.Int).SetUint64(b.head),
	}
	return nil
}

func (b *mockBackend) TransactionReceipt(_ context.Context, h common.Hash) (*types.Receipt, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r, ok := b.receipts[h]; ok {
		return r, nil
	}
	return nil, ethereum.NotFound
}

func (b *mockBackend) BlockNumber(context.Context) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.head, nil
}

func (b *mockBackend) lastSent() *types.Transaction {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.sent) == 0 {
		return nil
	}
	return b.sent[len(b.sent)-1]
}

func newTestManager(t *testing.T, b Backend) (*Manager, context.CancelFunc) {
	t.Helper()
	s, err := signer.NewFromHexKey(testKey)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	m := New(b, s, big.NewInt(11155111), Config{Confirmations: 0, PollInterval: time.Millisecond}, logr.Discard())
	ctx, cancel := context.WithCancel(context.Background())
	go m.Start(ctx)
	return m, cancel
}

func TestSend_HappyPath(t *testing.T) {
	b := newMockBackend()
	m, cancel := newTestManager(t, b)
	defer cancel()

	res := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), Data: []byte{0x01}, Label: "test"})
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.Receipt == nil || res.Receipt.Status != types.ReceiptStatusSuccessful {
		t.Fatalf("expected successful receipt, got %+v", res.Receipt)
	}

	tx := b.lastSent()
	if tx == nil {
		t.Fatal("no transaction sent")
	}
	if tx.Nonce() != 7 {
		t.Fatalf("expected nonce 7 (seeded from pending), got %d", tx.Nonce())
	}
	if tx.Type() != types.DynamicFeeTxType {
		t.Fatalf("expected EIP-1559 tx, got type %d", tx.Type())
	}
	// gas = estimate + 20%
	if tx.Gas() != 60_000 {
		t.Fatalf("expected gas 60000 (50000 + 20%%), got %d", tx.Gas())
	}
}

func TestSend_SequentialNoncesMonotonic(t *testing.T) {
	b := newMockBackend()
	m, cancel := newTestManager(t, b)
	defer cancel()

	for i, wantNonce := range []uint64{7, 8, 9} {
		res := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21000})
		if res.Err != nil {
			t.Fatalf("send %d: %v", i, res.Err)
		}
		if got := b.lastSent().Nonce(); got != wantNonce {
			t.Fatalf("send %d: expected nonce %d, got %d", i, wantNonce, got)
		}
	}
}

func TestSend_NonceTooLowResyncsAndRetries(t *testing.T) {
	b := newMockBackend()
	b.sendErrs = []error{errors.New("nonce too low")} // first send fails, second succeeds
	m, cancel := newTestManager(t, b)
	defer cancel()

	// Simulate the chain having advanced past our seeded nonce.
	b.pendingNonce = 9

	res := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21000, Label: "retry"})
	if res.Err != nil {
		t.Fatalf("expected success after resync, got %v", res.Err)
	}
	if got := b.lastSent().Nonce(); got != 9 {
		t.Fatalf("expected resynced nonce 9, got %d", got)
	}
}

func TestSend_GasEstimateFailurePropagates(t *testing.T) {
	b := newMockBackend()
	b.gasEstimate = 0 // forces EstimateGas to error
	m, cancel := newTestManager(t, b)
	defer cancel()

	res := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), Label: "noestimate"})
	if res.Err == nil {
		t.Fatal("expected gas-estimate error to propagate")
	}
}

func TestFees_RejectsSuggestedTipAboveConfiguredMaxFee(t *testing.T) {
	b := newMockBackend()
	b.tip = big.NewInt(3_000_000_000)
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{MaxFeeGwei: 2}, logr.Discard())

	_, _, err := m.fees(t.Context())
	if err == nil {
		t.Fatal("expected suggested tip above configured max fee to be rejected")
	}
	if !strings.Contains(err.Error(), "exceeds max fee") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFees_RejectsDerivedMaxBelowTip(t *testing.T) {
	b := newMockBackend()
	b.tip = big.NewInt(3_000_000_000)
	b.baseFee = big.NewInt(-1_000_000_000)
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())

	_, _, err := m.fees(t.Context())
	if err == nil {
		t.Fatal("expected derived max fee below the selected tip to be rejected")
	}
	if !strings.Contains(err.Error(), "exceeds max fee") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFees_AllowsTipAtOrBelowConfiguredMaxFee(t *testing.T) {
	tests := []struct {
		name       string
		cfg        Config
		suggested  *big.Int
		wantTipWei *big.Int
		wantMaxWei *big.Int
	}{
		{
			name:       "explicit decimal tip below cap",
			cfg:        Config{MaxFeeGwei: 2.5, TipGwei: 1.25},
			suggested:  big.NewInt(3_000_000_000),
			wantTipWei: big.NewInt(1_250_000_000),
			wantMaxWei: big.NewInt(2_500_000_000),
		},
		{
			name:       "suggested tip equal to cap",
			cfg:        Config{MaxFeeGwei: 2},
			suggested:  big.NewInt(2_000_000_000),
			wantTipWei: big.NewInt(2_000_000_000),
			wantMaxWei: big.NewInt(2_000_000_000),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newMockBackend()
			b.tip = tt.suggested
			m := New(b, mustSigner(t), big.NewInt(11155111), tt.cfg, logr.Discard())

			tip, maxFee, err := m.fees(t.Context())
			if err != nil {
				t.Fatalf("fees: %v", err)
			}
			if tip.Cmp(tt.wantTipWei) != 0 {
				t.Fatalf("tip = %s wei, want %s", tip, tt.wantTipWei)
			}
			if maxFee.Cmp(tt.wantMaxWei) != 0 {
				t.Fatalf("max fee = %s wei, want %s", maxFee, tt.wantMaxWei)
			}
		})
	}
}

func TestSend_RevertedReceiptIsError(t *testing.T) {
	rb := &revertingBackend{mockBackend: newMockBackend()}
	m := New(rb, mustSigner(t), big.NewInt(11155111), Config{PollInterval: time.Millisecond}, logr.Discard())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Start(ctx)

	res := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21000, Label: "revert"})
	if res.Err == nil {
		t.Fatal("expected reverted receipt to surface as an error")
	}
	if res.Receipt == nil || res.Receipt.Status != types.ReceiptStatusFailed {
		t.Fatalf("expected failed receipt attached, got %+v", res.Receipt)
	}
}

// revertingBackend records a failed receipt instead of a successful one.
type revertingBackend struct{ *mockBackend }

func (b *revertingBackend) SendTransaction(_ context.Context, tx *types.Transaction) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent = append(b.sent, tx)
	b.receipts[tx.Hash()] = &types.Receipt{
		Status:      types.ReceiptStatusFailed,
		TxHash:      tx.Hash(),
		BlockNumber: new(big.Int).SetUint64(b.head),
	}
	return nil
}

// blockingBackend parks inside SendTransaction until released, so a test can cancel the caller's
// context while a transaction is mid-broadcast on the worker.
type blockingBackend struct {
	*mockBackend

	entered chan struct{}
	release chan struct{}
}

func (b *blockingBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	close(b.entered)
	<-b.release
	return b.mockBackend.SendTransaction(ctx, tx)
}

// TestSend_CallerCancelAfterEnqueueStillReturnsResult guards the fund-moving invariant: once a
// request is enqueued the worker broadcasts it on the manager's context, so Send must report that
// real outcome. Cancelling the caller's context after enqueue must NOT make Send return a
// cancellation while the tx still lands on-chain (which would read as "not sent").
func TestSend_CallerCancelAfterEnqueueStillReturnsResult(t *testing.T) {
	bb := &blockingBackend{mockBackend: newMockBackend(), entered: make(chan struct{}), release: make(chan struct{})}
	m := New(bb, mustSigner(t), big.NewInt(11155111), Config{PollInterval: time.Millisecond}, logr.Discard())
	go m.Start(t.Context()) // manager context lives until test cleanup; the caller's is cancelled below

	callerCtx, cancelCaller := context.WithCancel(context.Background())
	resCh := make(chan Result, 1)
	go func() {
		resCh <- m.Send(callerCtx, Request{To: common.HexToAddress("0xabc"), GasLimit: 21000, Label: "fill"})
	}()

	<-bb.entered   // worker has dequeued the job and is broadcasting
	cancelCaller() // caller gives up now, mid-broadcast
	close(bb.release)

	res := <-resCh
	if res.Err != nil {
		t.Fatalf("tx was broadcast but Send reported %v; caller cancellation must not mask a sent tx", res.Err)
	}
	if bb.lastSent() == nil {
		t.Fatal("expected the transaction to be broadcast")
	}
}

func mustSigner(t *testing.T) signer.Signer {
	t.Helper()
	s, err := signer.NewFromHexKey(testKey)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	return s
}
