package txmanager

import (
	"context"
	"errors"
	"io"
	"math"
	"math/big"
	"runtime"
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

type receiptObservation struct {
	hash  common.Hash
	found bool
	err   error
}

type mockBackend struct {
	mu sync.Mutex

	pendingNonce  uint64
	pendingNonces []uint64
	tip           *big.Int
	baseFee       *big.Int
	gasEstimate   uint64
	head          uint64

	sendErrs     []error // returned, in order, by successive SendTransaction calls
	sendCalls    int
	sent         []*types.Transaction
	sentCh       chan *types.Transaction
	sendCallCh   chan *types.Transaction
	heldNonces   map[uint64]bool
	receipts     map[common.Hash]*types.Receipt
	receiptCalls map[common.Hash]int
	receiptErrs  []error
	receiptCh    chan receiptObservation
	headers      map[uint64]*types.Header
	nilHeaders   int
	headerErrs   []error
	blockErrs    []error
	blockCh      chan uint64
}

func newMockBackend() *mockBackend {
	return &mockBackend{
		pendingNonce: 7,
		tip:          big.NewInt(1e9),
		baseFee:      big.NewInt(20e9),
		gasEstimate:  50_000,
		head:         100,
		sentCh:       make(chan *types.Transaction, 32),
		sendCallCh:   make(chan *types.Transaction, 64),
		heldNonces:   map[uint64]bool{},
		receipts:     map[common.Hash]*types.Receipt{},
		receiptCalls: map[common.Hash]int{},
		receiptCh:    make(chan receiptObservation, 256),
		headers:      map[uint64]*types.Header{},
		blockCh:      make(chan uint64, 64),
	}
}

func (b *mockBackend) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.pendingNonces) > 0 {
		nonce := b.pendingNonces[0]
		b.pendingNonces = b.pendingNonces[1:]
		return nonce, nil
	}
	return b.pendingNonce, nil
}

func (b *mockBackend) SuggestGasTipCap(context.Context) (*big.Int, error) { return b.tip, nil }

func (b *mockBackend) HeaderByNumber(_ context.Context, number *big.Int) (*types.Header, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.nilHeaders > 0 {
		b.nilHeaders--
		return nil, nil
	}
	if len(b.headerErrs) > 0 {
		err := b.headerErrs[0]
		b.headerErrs = b.headerErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	if number == nil {
		return b.headerFor(b.head), nil
	}
	return b.headerFor(number.Uint64()), nil
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
	select {
	case b.sendCallCh <- tx:
	default:
	}
	if i < len(b.sendErrs) && b.sendErrs[i] != nil {
		return b.sendErrs[i]
	}
	b.sent = append(b.sent, tx)
	b.sentCh <- tx
	if !b.heldNonces[tx.Nonce()] {
		header := b.headerFor(b.head)
		b.receipts[tx.Hash()] = &types.Receipt{
			Status:      types.ReceiptStatusSuccessful,
			TxHash:      tx.Hash(),
			BlockNumber: new(big.Int).SetUint64(b.head),
			BlockHash:   header.Hash(),
		}
	}
	return nil
}

func (b *mockBackend) TransactionReceipt(_ context.Context, h common.Hash) (*types.Receipt, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.receiptCalls[h]++
	if len(b.receiptErrs) > 0 {
		err := b.receiptErrs[0]
		b.receiptErrs = b.receiptErrs[1:]
		if err != nil {
			b.observeReceipt(receiptObservation{hash: h, err: err})
			return nil, err
		}
	}
	if r, ok := b.receipts[h]; ok {
		b.observeReceipt(receiptObservation{hash: h, found: true})
		return r, nil
	}
	b.observeReceipt(receiptObservation{hash: h, err: ethereum.NotFound})
	return nil, ethereum.NotFound
}

func (b *mockBackend) BlockNumber(context.Context) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.blockErrs) > 0 {
		err := b.blockErrs[0]
		b.blockErrs = b.blockErrs[1:]
		if err != nil {
			return 0, err
		}
	}
	select {
	case b.blockCh <- b.head:
	default:
	}
	return b.head, nil
}

func (b *mockBackend) observeReceipt(observation receiptObservation) {
	select {
	case b.receiptCh <- observation:
	default:
	}
}

func (b *mockBackend) lastSent() *types.Transaction {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.sent) == 0 {
		return nil
	}
	return b.sent[len(b.sent)-1]
}

func (b *mockBackend) sendCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sendCalls
}

func (b *mockBackend) sentCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.sent)
}

func (b *mockBackend) receiptCallCount(hash common.Hash) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.receiptCalls[hash]
}

func (b *mockBackend) pendingSeedCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pendingNonces)
}

func (b *mockBackend) setReceipt(hash common.Hash, receipt *types.Receipt) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.receipts[hash] = receipt
}

func (b *mockBackend) deleteReceipt(hash common.Hash) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.receipts, hash)
}

func (b *mockBackend) setHead(head uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.head = head
}

func (b *mockBackend) canonicalReceipt(tx *types.Transaction, status uint64, block uint64) *types.Receipt {
	b.mu.Lock()
	defer b.mu.Unlock()
	return &types.Receipt{
		Status:      status,
		TxHash:      tx.Hash(),
		BlockNumber: new(big.Int).SetUint64(block),
		BlockHash:   b.headerFor(block).Hash(),
	}
}

func (b *mockBackend) remainingCanonicalityErrors() (header, block int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.headerErrs), len(b.blockErrs)
}

func (b *mockBackend) remainingNilHeaders() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.nilHeaders
}

func (b *mockBackend) headerFor(number uint64) *types.Header {
	if header := b.headers[number]; header != nil {
		return types.CopyHeader(header)
	}
	return &types.Header{
		Number:  new(big.Int).SetUint64(number),
		BaseFee: new(big.Int).Set(b.baseFee),
		Extra:   []byte{byte(number), byte(number >> 8)},
	}
}

func (b *mockBackend) releaseNonce(nonce uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.heldNonces, nonce)
	for i := len(b.sent) - 1; i >= 0; i-- {
		tx := b.sent[i]
		if tx.Nonce() == nonce {
			header := b.headerFor(b.head)
			b.receipts[tx.Hash()] = &types.Receipt{
				Status:      types.ReceiptStatusSuccessful,
				TxHash:      tx.Hash(),
				BlockNumber: new(big.Int).SetUint64(b.head),
				BlockHash:   header.Hash(),
			}
			return
		}
	}
}

func newTestManager(t *testing.T, b Backend, overrides ...Config) (*Manager, context.CancelFunc, <-chan error) {
	t.Helper()
	cfg := Config{PollInterval: time.Millisecond}
	if len(overrides) == 1 {
		cfg = overrides[0]
		if cfg.PollInterval == 0 {
			cfg.PollInterval = time.Millisecond
		}
	}
	if len(overrides) > 1 {
		t.Fatal("newTestManager accepts at most one Config override")
	}
	m := New(b, mustSigner(t), big.NewInt(11155111), cfg, logr.Discard())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(ctx) }()
	return m, cancel, done
}

func TestResultSafeToRetry(t *testing.T) {
	cases := map[State]bool{
		StateNotBroadcast:     true,
		StateRejected:         true,
		StateBroadcastUnknown: false,
		StatePending:          false,
		StateConfirmed:        false,
		StateReverted:         false,
		StateUnresolved:       false,
	}
	for state, want := range cases {
		if got := (Result{State: state}).SafeToRetry(); got != want {
			t.Errorf("state %q SafeToRetry = %v, want %v", state, got, want)
		}
	}
}

func TestClassifyBroadcastError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want broadcastClass
	}{
		{name: "nil", want: broadcastAdmitted},
		{name: "already known", err: errors.New("ALREADY KNOWN"), want: broadcastAdmitted},
		{name: "insufficient funds", err: errors.New("insufficient funds for gas * price + value"), want: broadcastRejected},
		{name: "intrinsic gas too low", err: errors.New("intrinsic gas too low"), want: broadcastRejected},
		{name: "invalid sender", err: errors.New("invalid sender"), want: broadcastRejected},
		{name: "max fee below base fee", err: errors.New("max fee per gas less than block base fee"), want: broadcastRejected},
		{name: "priority fee above max fee", err: errors.New("max priority fee per gas higher than max fee per gas"), want: broadcastRejected},
		{name: "unsupported type", err: errors.New("transaction type not supported"), want: broadcastRejected},
		{name: "timeout", err: context.DeadlineExceeded, want: broadcastAmbiguous},
		{name: "eof", err: io.EOF, want: broadcastAmbiguous},
		{name: "nonce too low", err: errors.New("nonce too low"), want: broadcastAmbiguous},
		{name: "replacement underpriced", err: errors.New("replacement transaction underpriced"), want: broadcastAmbiguous},
		{name: "unrecognized", err: errors.New("rpc unavailable"), want: broadcastAmbiguous},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyBroadcastError(tt.err); got != tt.want {
				t.Fatalf("classifyBroadcastError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestSend_CancelBeforeEnqueueIsNotBroadcast(t *testing.T) {
	b := newMockBackend()
	m, cancelManager, done := newTestManager(t, b)
	defer func() { cancelManager(); <-done }()

	for i := 0; i < 1_000; i++ {
		runtime.Gosched()
		ctx, cancelCaller := context.WithCancel(context.Background())
		cancelCaller()
		result := m.Send(ctx, Request{To: common.HexToAddress("0x1"), GasLimit: 21_000})
		if result.State != StateNotBroadcast || !result.SafeToRetry() || result.Hash != (common.Hash{}) {
			t.Fatalf("iteration %d result = %+v, want safe not_broadcast", i, result)
		}
	}
	if got := b.sendCount(); got != 0 {
		t.Fatalf("SendTransaction calls = %d, want 0 for already-canceled callers", got)
	}
}

func TestSend_AmbiguousBroadcastReturnsSignedHashAndCommitsNonce(t *testing.T) {
	b := newMockBackend()
	b.sendErrs = []error{
		context.DeadlineExceeded, // initial broadcast
		context.DeadlineExceeded, // identical re-broadcast
		context.DeadlineExceeded, // first and only fee-bumped replacement
	}
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: 5 * time.Millisecond,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	res := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	if res.State != StateUnresolved || res.Hash == (common.Hash{}) || len(res.Hashes) == 0 {
		t.Fatalf("ambiguous result = %+v", res)
	}
	if res.SafeToRetry() {
		t.Fatal("ambiguous broadcast must never be safe to retry")
	}

	next := m.Send(context.Background(), Request{To: common.HexToAddress("0xdef"), GasLimit: 21_000})
	if next.Nonce != res.Nonce+1 {
		t.Fatalf("next nonce = %d, want %d", next.Nonce, res.Nonce+1)
	}
}

func TestSend_SecondNonceBroadcastsWhileFirstIsPending(t *testing.T) {
	b := newMockBackend()
	b.heldNonces[7] = true
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: time.Second,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	first := make(chan Result, 1)
	go func() {
		first <- m.Send(context.Background(), Request{To: common.HexToAddress("0xa"), GasLimit: 21_000})
	}()
	if tx := <-b.sentCh; tx.Nonce() != 7 {
		t.Fatalf("first broadcast nonce = %d, want 7", tx.Nonce())
	}

	second := make(chan Result, 1)
	go func() {
		second <- m.Send(context.Background(), Request{To: common.HexToAddress("0xb"), GasLimit: 21_000})
	}()
	select {
	case tx := <-b.sentCh:
		if tx.Nonce() != 8 {
			t.Fatalf("second broadcast nonce = %d, want 8", tx.Nonce())
		}
	case <-time.After(time.Second):
		t.Fatal("nonce 8 was head-of-line blocked by nonce 7 receipt tracking")
	}

	b.releaseNonce(7)
	<-first
	<-second
}

func TestSend_AmbiguousNonceFloorSurvivesRegressedPendingNonce(t *testing.T) {
	// First seed is 7. The nonce-7 broadcast returns ambiguous "nonce too low" and is committed.
	// The next seed deliberately regresses to 6, as a stale fallback can do.
	b := newMockBackend()
	b.pendingNonces = []uint64{7, 6}
	b.sendErrs = []error{
		errors.New("nonce too low"), // initial broadcast: ambiguous and invalidates the seed
		context.DeadlineExceeded,    // identical re-broadcast remains ambiguous
		context.DeadlineExceeded,    // sole fee-bumped replacement remains ambiguous
		// The next logical transaction gets the default nil result and a canonical receipt.
	}
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: 5 * time.Millisecond,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	first := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	if first.State != StateUnresolved || first.Nonce != 7 {
		t.Fatalf("first result = %+v, want unresolved nonce 7", first)
	}
	second := m.Send(context.Background(), Request{To: common.HexToAddress("0xdef"), GasLimit: 21_000})
	if second.State != StateConfirmed || second.Nonce != 8 {
		t.Fatalf("second result = %+v, want confirmed at committed floor 8", second)
	}
}

func TestSend_DeterministicRejectionReusesNonceWithoutReseeding(t *testing.T) {
	b := newMockBackend()
	b.pendingNonces = []uint64{7, 9}
	b.sendErrs = []error{errors.New("insufficient funds")}
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: 5 * time.Millisecond,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	rejected := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	if rejected.State != StateRejected || rejected.Nonce != 7 || !rejected.SafeToRetry() {
		t.Fatalf("first result = %+v, want retryable rejected nonce 7", rejected)
	}
	if rejected.Hash == (common.Hash{}) || len(rejected.Hashes) != 1 {
		t.Fatalf("rejected signed attempt did not retain its hash: %+v", rejected)
	}

	confirmed := m.Send(context.Background(), Request{To: common.HexToAddress("0xdef"), GasLimit: 21_000})
	if confirmed.State != StateConfirmed || confirmed.Nonce != 7 {
		t.Fatalf("second result = %+v, want confirmed reuse of nonce 7", confirmed)
	}
	if got := b.pendingSeedCount(); got != 1 {
		t.Fatalf("remaining pending nonce seeds = %d, want 1 (no reseed after rejection)", got)
	}
}

func TestSend_MaxUint64NonceExhaustionDoesNotWrap(t *testing.T) {
	b := newMockBackend()
	b.pendingNonce = math.MaxUint64
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: 5 * time.Millisecond,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	final := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	if final.State != StateConfirmed || final.Nonce != math.MaxUint64 {
		t.Fatalf("first result = %+v, want confirmed MaxUint64 nonce", final)
	}

	exhausted := m.Send(context.Background(), Request{To: common.HexToAddress("0xdef"), GasLimit: 21_000})
	if exhausted.State != StateRejected || exhausted.Err == nil || !strings.Contains(exhausted.Err.Error(), "nonce space exhausted") {
		t.Fatalf("second result = %+v, want nonce-space rejection", exhausted)
	}
	if got := b.sendCount(); got != 1 {
		t.Fatalf("SendTransaction calls = %d, want 1", got)
	}
}

func TestStart_SecondCallRejectedWithoutStoppingFirst(t *testing.T) {
	b := newMockBackend()
	m := New(b, mustSigner(t), big.NewInt(1), Config{PollInterval: time.Millisecond}, logr.Discard())
	ctx, cancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() { firstDone <- m.Start(ctx) }()

	guard := time.NewTimer(trackerTestGuard)
	defer guard.Stop()
	for !m.started.Load() {
		select {
		case <-guard.C:
			t.Fatal("first Start did not acquire manager ownership")
		default:
			runtime.Gosched()
		}
	}

	secondDone := make(chan error, 1)
	go func() { secondDone <- m.Start(context.Background()) }()
	select {
	case err := <-secondDone:
		if !errors.Is(err, ErrManagerAlreadyStarted) {
			t.Fatalf("second Start error = %v, want ErrManagerAlreadyStarted", err)
		}
	case <-time.After(trackerTestGuard):
		t.Fatal("second Start did not return immediately")
	}
	select {
	case <-m.done:
		t.Fatal("rejected second Start closed manager done")
	default:
	}

	result := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	if result.State != StateConfirmed {
		t.Fatalf("first dispatcher result = %+v, want confirmed", result)
	}

	cancel()
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first Start returned %v", err)
		}
	case <-time.After(trackerTestGuard):
		t.Fatal("first Start did not stop cleanly")
	}
	select {
	case <-m.done:
	default:
		t.Fatal("first Start returned without closing done")
	}
}

func TestSend_HappyPath(t *testing.T) {
	b := newMockBackend()
	m, cancel, done := newTestManager(t, b)
	defer func() { cancel(); <-done }()

	res := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), Data: []byte{0x01}, Label: "test"})
	if res.State != StateConfirmed || res.Err != nil {
		t.Fatalf("result = %+v, want confirmed", res)
	}
	if res.Receipt == nil || res.Receipt.Status != types.ReceiptStatusSuccessful {
		t.Fatalf("expected successful receipt, got %+v", res.Receipt)
	}
	if res.Receipt.BlockHash != b.headerFor(res.Receipt.BlockNumber.Uint64()).Hash() {
		t.Fatalf("receipt block hash %s is not canonical", res.Receipt.BlockHash)
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
	m, cancel, done := newTestManager(t, b)
	defer func() { cancel(); <-done }()

	for i, wantNonce := range []uint64{7, 8, 9} {
		res := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21000})
		if res.State != StateConfirmed || res.Err != nil {
			t.Fatalf("send %d result = %+v, want confirmed", i, res)
		}
		if got := b.lastSent().Nonce(); got != wantNonce {
			t.Fatalf("send %d: expected nonce %d, got %d", i, wantNonce, got)
		}
	}
}

func TestSend_NonceTooLowIsAmbiguousAndNeverReplaysAtNewNonce(t *testing.T) {
	b := newMockBackend()
	b.pendingNonces = []uint64{7, 9}
	b.sendErrs = []error{
		errors.New("nonce too low"),
		context.DeadlineExceeded,
		context.DeadlineExceeded,
	}
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: 5 * time.Millisecond,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	first := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21000, Label: "ambiguous"})
	if first.State != StateUnresolved || first.SafeToRetry() || first.Nonce != 7 {
		t.Fatalf("first result = %+v, want non-retryable unresolved nonce 7", first)
	}
	for i := 0; i < 3; i++ {
		if tx := awaitTx(t, b.sendCallCh); tx.Nonce() != 7 {
			t.Fatalf("attempt %d replayed logical request at nonce %d, want 7", i, tx.Nonce())
		}
	}

	second := m.Send(context.Background(), Request{To: common.HexToAddress("0xdef"), GasLimit: 21000})
	if second.State != StateConfirmed || second.Nonce != 9 {
		t.Fatalf("second result = %+v, want separate request confirmed at reseeded nonce 9", second)
	}
}

func TestSend_GasEstimateFailurePropagates(t *testing.T) {
	b := newMockBackend()
	b.gasEstimate = 0 // forces EstimateGas to error
	m, cancel, done := newTestManager(t, b)
	defer func() { cancel(); <-done }()

	res := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), Label: "noestimate"})
	if res.State != StateRejected || !res.SafeToRetry() || res.Err == nil || res.Hash != (common.Hash{}) {
		t.Fatalf("result = %+v, want retryable pre-broadcast rejection", res)
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
	m, cancel, done := newTestManager(t, rb)
	defer func() { cancel(); <-done }()

	res := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21000, Label: "revert"})
	if res.State != StateReverted || res.Err == nil || res.SafeToRetry() {
		t.Fatalf("result = %+v, want non-retryable reverted", res)
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
	b.sendCalls++
	b.sendCallCh <- tx
	b.sent = append(b.sent, tx)
	b.sentCh <- tx
	header := b.headerFor(b.head)
	b.receipts[tx.Hash()] = &types.Receipt{
		Status:      types.ReceiptStatusFailed,
		TxHash:      tx.Hash(),
		BlockNumber: new(big.Int).SetUint64(b.head),
		BlockHash:   header.Hash(),
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
	managerCtx, cancelManager := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(managerCtx) }()
	defer func() { cancelManager(); <-done }()

	callerCtx, cancelCaller := context.WithCancel(context.Background())
	resCh := make(chan Result, 1)
	go func() {
		resCh <- m.Send(callerCtx, Request{To: common.HexToAddress("0xabc"), GasLimit: 21000, Label: "fill"})
	}()

	<-bb.entered   // worker has dequeued the job and is broadcasting
	cancelCaller() // caller gives up now, mid-broadcast
	close(bb.release)

	res := <-resCh
	if res.State != StateConfirmed || res.Err != nil {
		t.Fatalf("tx was broadcast but Send reported %+v; caller cancellation must not mask a sent tx", res)
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
