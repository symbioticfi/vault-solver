package txmanager

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"reflect"
	"sync"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
)

const trackerTestGuard = time.Second

type blockingReceiptBackend struct {
	*mockBackend

	pollStarted chan time.Time
	pollDone    chan struct{}
	mu          sync.Mutex
	deadlines   []time.Time
}

func newBlockingReceiptBackend() *blockingReceiptBackend {
	return &blockingReceiptBackend{
		mockBackend: newMockBackend(),
		pollStarted: make(chan time.Time, 64),
		pollDone:    make(chan struct{}, 64),
	}
}

func (b *blockingReceiptBackend) TransactionReceipt(ctx context.Context, _ common.Hash) (*types.Receipt, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Time{}
	}
	b.mu.Lock()
	b.deadlines = append(b.deadlines, deadline)
	b.mu.Unlock()
	b.pollStarted <- deadline
	<-ctx.Done()
	b.pollDone <- struct{}{}
	return nil, ctx.Err()
}

func (b *blockingReceiptBackend) uniqueDeadlines() []time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	unique := make([]time.Time, 0, len(b.deadlines))
	for _, deadline := range b.deadlines {
		if deadline.IsZero() || len(unique) > 0 && deadline.Equal(unique[len(unique)-1]) {
			continue
		}
		unique = append(unique, deadline)
	}
	return unique
}

func awaitTx(t *testing.T, ch <-chan *types.Transaction) *types.Transaction {
	t.Helper()
	select {
	case tx := <-ch:
		return tx
	case <-time.After(trackerTestGuard):
		t.Fatal("timed out waiting for transaction")
		return nil
	}
}

func awaitResult(t *testing.T, ch <-chan Result) Result {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(trackerTestGuard):
		t.Fatal("timed out waiting for transaction result")
		return Result{}
	}
}

func awaitReceiptObservation(
	t *testing.T,
	b *mockBackend,
	hash common.Hash,
	match func(receiptObservation) bool,
) {
	t.Helper()
	timer := time.NewTimer(trackerTestGuard)
	defer timer.Stop()
	for {
		select {
		case observation := <-b.receiptCh:
			if observation.hash == hash && match(observation) {
				return
			}
		case <-timer.C:
			t.Fatal("timed out waiting for matching receipt observation")
			return
		}
	}
}

func awaitHead(t *testing.T, b *mockBackend, want uint64) {
	t.Helper()
	timer := time.NewTimer(trackerTestGuard)
	defer timer.Stop()
	for {
		select {
		case head := <-b.blockCh:
			if head == want {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for observed head %d", want)
		}
	}
}

func sendAsync(m *Manager, req Request) <-chan Result {
	result := make(chan Result, 1)
	go func() {
		result <- m.Send(context.Background(), req)
	}()
	return result
}

func TestTrack_BlockingReceiptDoesNotDelayReplacementBoundary(t *testing.T) {
	const interval = 15 * time.Millisecond
	b := newBlockingReceiptBackend()
	b.heldNonces[7] = true
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: interval,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	resultCh := sendAsync(m, Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	original := awaitTx(t, b.sentCh)
	select {
	case deadline := <-b.pollStarted:
		if deadline.IsZero() {
			t.Fatal("receipt poll has no replacement-boundary deadline")
		}
	case <-time.After(trackerTestGuard):
		t.Fatal("receipt poll did not start")
	}
	select {
	case <-b.pollDone:
	case <-time.After(trackerTestGuard):
		t.Fatal("receipt poll was not cancelled at replacement boundary")
	}
	replacement := awaitTx(t, b.sentCh)
	if replacement.Nonce() != original.Nonce() || replacement.Hash() == original.Hash() {
		t.Fatalf("replacement = %s nonce %d, want distinct same-nonce attempt", replacement.Hash(), replacement.Nonce())
	}

	result := awaitResult(t, resultCh)
	if result.State != StateUnresolved || !errors.Is(result.Err, ErrUnresolved) {
		t.Fatalf("result = %+v, want unresolved at overall boundary", result)
	}
}

func TestTrack_BlockingReceiptCompletesByAbsoluteOverallDeadline(t *testing.T) {
	const interval = 15 * time.Millisecond
	b := newBlockingReceiptBackend()
	b.heldNonces[7] = true
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: interval,
		FeeBumpBps: 1_250, MaxReplacements: 2,
	})
	defer func() { cancel(); <-done }()

	resultCh := sendAsync(m, Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	_ = awaitTx(t, b.sentCh)
	_ = awaitTx(t, b.sentCh)
	_ = awaitTx(t, b.sentCh)
	result := awaitResult(t, resultCh)
	completedAt := time.Now()
	if result.State != StateUnresolved || !errors.Is(result.Err, ErrUnresolved) {
		t.Fatalf("result = %+v, want unresolved at absolute overall deadline", result)
	}

	deadlines := b.uniqueDeadlines()
	if len(deadlines) != 3 {
		t.Fatalf("unique poll deadlines = %v, want three absolute window boundaries", deadlines)
	}
	for i := 1; i < len(deadlines); i++ {
		if got := deadlines[i].Sub(deadlines[i-1]); got != interval {
			t.Fatalf("deadline %d delta = %s, want %s", i, got, interval)
		}
	}
	if lag := completedAt.Sub(deadlines[len(deadlines)-1]); lag < 0 || lag > 100*time.Millisecond {
		t.Fatalf("unresolved completion lag after overall deadline = %s", lag)
	}
}

func TestTrack_TransientReceiptErrorRetries(t *testing.T) {
	temporary := errors.New("temporary receipt failure")
	b := newMockBackend()
	b.heldNonces[7] = true
	b.receiptErrs = []error{temporary}
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: time.Second,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	resultCh := sendAsync(m, Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	original := awaitTx(t, b.sentCh)
	awaitReceiptObservation(t, b, original.Hash(), func(observation receiptObservation) bool {
		return errors.Is(observation.err, temporary)
	})
	b.setReceipt(original.Hash(), b.canonicalReceipt(original, types.ReceiptStatusSuccessful, 100))

	result := awaitResult(t, resultCh)
	if result.State != StateConfirmed || result.Err != nil || result.Hash != original.Hash() {
		t.Fatalf("result = %+v, want confirmed original after transient receipt error", result)
	}
}

func TestTrack_CanonicalAttemptStopsPollingLaterHashes(t *testing.T) {
	b := newMockBackend()
	first := types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(1), Nonce: 7, Gas: 21_000})
	second := types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(1), Nonce: 7, Gas: 21_000, Data: []byte{1}})
	b.setReceipt(first.Hash(), b.canonicalReceipt(first, types.ReceiptStatusSuccessful, 100))
	m := New(b, mustSigner(t), big.NewInt(1), Config{}, logr.Discard())

	result, err := m.pollAttempts(t.Context(), &trackedTx{
		nonce:    7,
		state:    StatePending,
		attempts: []*types.Transaction{first, second},
	})
	if err != nil || result == nil || result.State != StateConfirmed {
		t.Fatalf("poll result/error = %+v/%v, want confirmed first attempt", result, err)
	}
	if got := b.receiptCallCount(second.Hash()); got != 0 {
		t.Fatalf("later attempt receipt calls = %d, want 0 after canonical result", got)
	}
}

func TestTrack_ReceiptDisappearsBeforeConfirmation(t *testing.T) {
	b := newMockBackend()
	b.heldNonces[7] = true
	m, cancel, done := newTestManager(t, b, Config{
		Confirmations: 2, PollInterval: time.Millisecond, PendingInterval: time.Second,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	resultCh := sendAsync(m, Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	original := awaitTx(t, b.sentCh)
	b.setReceipt(original.Hash(), b.canonicalReceipt(original, types.ReceiptStatusSuccessful, 100))
	awaitReceiptObservation(t, b, original.Hash(), func(observation receiptObservation) bool {
		return observation.found
	})
	awaitHead(t, b, 100)

	b.deleteReceipt(original.Hash())
	b.setHead(103)
	awaitReceiptObservation(t, b, original.Hash(), func(observation receiptObservation) bool {
		return errors.Is(observation.err, ethereum.NotFound)
	})
	select {
	case result := <-resultCh:
		t.Fatalf("disappeared receipt finalized: %+v", result)
	default:
	}

	b.setReceipt(original.Hash(), b.canonicalReceipt(original, types.ReceiptStatusSuccessful, 101))
	result := awaitResult(t, resultCh)
	if result.State != StateConfirmed || result.Hash != original.Hash() || result.Receipt.BlockNumber.Uint64() != 101 {
		t.Fatalf("result = %+v, want newly included canonical receipt", result)
	}
}

func TestTrack_BlockHashMismatchIsNotCanonical(t *testing.T) {
	b := newMockBackend()
	b.heldNonces[7] = true
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: time.Second,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	resultCh := sendAsync(m, Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	original := awaitTx(t, b.sentCh)
	mismatched := b.canonicalReceipt(original, types.ReceiptStatusSuccessful, 100)
	mismatched.BlockHash = common.HexToHash("0xdead")
	b.setReceipt(original.Hash(), mismatched)
	awaitReceiptObservation(t, b, original.Hash(), func(observation receiptObservation) bool {
		return observation.found
	})

	guard := time.NewTimer(trackerTestGuard)
	defer guard.Stop()
	observedAgain := false
	for !observedAgain {
		select {
		case result := <-resultCh:
			t.Fatalf("block-hash-mismatched receipt finalized: %+v", result)
		case observation := <-b.receiptCh:
			observedAgain = observation.hash == original.Hash() && observation.found
		case <-guard.C:
			t.Fatal("timed out waiting for block-hash-mismatched receipt to be polled again")
		}
	}

	b.setReceipt(original.Hash(), b.canonicalReceipt(original, types.ReceiptStatusSuccessful, 100))
	result := awaitResult(t, resultCh)
	if result.State != StateConfirmed || result.Hash != original.Hash() {
		t.Fatalf("result = %+v, want confirmed after canonical block hash appears", result)
	}
}

func TestTrack_HeaderNumberMismatchIsNotCanonical(t *testing.T) {
	b := newMockBackend()
	b.heldNonces[7] = true
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: time.Second,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	resultCh := sendAsync(m, Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	original := awaitTx(t, b.sentCh)
	b.mu.Lock()
	mismatchedHeader := b.headerFor(101)
	b.headers[100] = mismatchedHeader
	b.receipts[original.Hash()] = &types.Receipt{
		Status:      types.ReceiptStatusSuccessful,
		TxHash:      original.Hash(),
		BlockNumber: big.NewInt(100),
		BlockHash:   mismatchedHeader.Hash(),
	}
	b.mu.Unlock()
	awaitReceiptObservation(t, b, original.Hash(), func(observation receiptObservation) bool {
		return observation.found
	})

	guard := time.NewTimer(trackerTestGuard)
	defer guard.Stop()
	observedAgain := false
	for !observedAgain {
		select {
		case result := <-resultCh:
			t.Fatalf("header-number-mismatched receipt finalized: %+v", result)
		case observation := <-b.receiptCh:
			observedAgain = observation.hash == original.Hash() && observation.found
		case <-guard.C:
			t.Fatal("timed out waiting for header-number-mismatched receipt to be polled again")
		}
	}

	b.mu.Lock()
	delete(b.headers, 100)
	b.mu.Unlock()
	b.setReceipt(original.Hash(), b.canonicalReceipt(original, types.ReceiptStatusSuccessful, 100))
	result := awaitResult(t, resultCh)
	if result.State != StateConfirmed || result.Hash != original.Hash() {
		t.Fatalf("result = %+v, want confirmed after matching header number appears", result)
	}
}

func TestTrack_ReceiptTransactionHashMismatchIsTransient(t *testing.T) {
	b := newMockBackend()
	b.heldNonces[7] = true
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: time.Second,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	resultCh := sendAsync(m, Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	original := awaitTx(t, b.sentCh)
	mismatched := b.canonicalReceipt(original, types.ReceiptStatusSuccessful, 100)
	mismatched.TxHash = common.HexToHash("0xbad")
	b.setReceipt(original.Hash(), mismatched)
	awaitReceiptObservation(t, b, original.Hash(), func(observation receiptObservation) bool {
		return observation.found
	})

	guard := time.NewTimer(trackerTestGuard)
	defer guard.Stop()
	observedAgain := false
	for !observedAgain {
		select {
		case result := <-resultCh:
			t.Fatalf("transaction-hash-mismatched receipt finalized: %+v", result)
		case observation := <-b.receiptCh:
			observedAgain = observation.hash == original.Hash() && observation.found
		case <-guard.C:
			t.Fatal("timed out waiting for transaction-hash-mismatched receipt to be polled again")
		}
	}

	b.setReceipt(original.Hash(), b.canonicalReceipt(original, types.ReceiptStatusSuccessful, 100))
	result := awaitResult(t, resultCh)
	if result.State != StateConfirmed || result.Hash != original.Hash() {
		t.Fatalf("result = %+v, want confirmed after matching transaction hash appears", result)
	}
}

func TestTrack_RevertWaitsForCanonicalConfirmations(t *testing.T) {
	b := newMockBackend()
	b.heldNonces[7] = true
	m, cancel, done := newTestManager(t, b, Config{
		Confirmations: 2, PollInterval: time.Millisecond, PendingInterval: time.Second,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	resultCh := sendAsync(m, Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	original := awaitTx(t, b.sentCh)
	reverted := b.canonicalReceipt(original, types.ReceiptStatusFailed, 100)
	b.setReceipt(original.Hash(), reverted)
	awaitReceiptObservation(t, b, original.Hash(), func(observation receiptObservation) bool {
		return observation.found
	})
	select {
	case result := <-resultCh:
		t.Fatalf("revert finalized before confirmations: %+v", result)
	default:
	}

	b.setHead(102)
	result := awaitResult(t, resultCh)
	if result.State != StateReverted || result.Err == nil || result.Receipt != reverted || result.Hash != original.Hash() {
		t.Fatalf("result = %+v, want canonically confirmed revert", result)
	}
}

func TestTrack_ReplacementPreservesPayloadAndBumpsFees(t *testing.T) {
	b := newMockBackend()
	b.heldNonces[7] = true
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: 5 * time.Millisecond,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	resultCh := sendAsync(m, Request{
		To:       common.HexToAddress("0xabc"),
		Value:    big.NewInt(1234),
		Data:     []byte{0xde, 0xad, 0xbe, 0xef},
		GasLimit: 54_321,
	})
	original := awaitTx(t, b.sentCh)
	replacement := awaitTx(t, b.sentCh)

	if replacement.Nonce() != original.Nonce() ||
		replacement.To() == nil || original.To() == nil || *replacement.To() != *original.To() ||
		replacement.Value().Cmp(original.Value()) != 0 ||
		!bytes.Equal(replacement.Data(), original.Data()) ||
		replacement.Gas() != original.Gas() ||
		replacement.ChainId().Cmp(original.ChainId()) != 0 ||
		!reflect.DeepEqual(replacement.AccessList(), original.AccessList()) {
		t.Fatal("replacement changed logical transaction payload")
	}
	if replacement.GasTipCapCmp(original) <= 0 || replacement.GasFeeCapCmp(original) <= 0 {
		t.Fatal("replacement did not monotonically increase both EIP-1559 fee fields")
	}

	b.releaseNonce(7)
	result := awaitResult(t, resultCh)
	if result.State != StateConfirmed || result.Hash != replacement.Hash() || len(result.Hashes) != 2 ||
		result.Hashes[0] != original.Hash() || result.Hashes[1] != replacement.Hash() {
		t.Fatalf("result = %+v, want ordered attempts and canonical replacement hash", result)
	}
}

func TestTrack_RejectedReplacementKeepsOlderHashEligible(t *testing.T) {
	b := newMockBackend()
	b.heldNonces[7] = true
	b.sendErrs = []error{nil, errors.New("insufficient funds")}
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: 5 * time.Millisecond,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	resultCh := sendAsync(m, Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	original := awaitTx(t, b.sentCh)
	if attempted := awaitTx(t, b.sendCallCh); attempted.Hash() != original.Hash() {
		t.Fatalf("initial send-call hash = %s, want %s", attempted.Hash(), original.Hash())
	}
	rejectedReplacement := awaitTx(t, b.sendCallCh)
	if rejectedReplacement.Hash() == original.Hash() {
		t.Fatal("fee-bumped replacement reused the original hash")
	}

	b.setReceipt(original.Hash(), b.canonicalReceipt(original, types.ReceiptStatusSuccessful, 100))
	result := awaitResult(t, resultCh)
	if result.State != StateConfirmed || result.Hash != original.Hash() || len(result.Hashes) != 2 ||
		result.Hashes[0] != original.Hash() || result.Hashes[1] != rejectedReplacement.Hash() {
		t.Fatalf("result = %+v, want older canonical hash retained after rejected replacement", result)
	}
	if calls, admitted := b.sendCount(), b.sentCount(); calls != 2 || admitted != 1 {
		t.Fatalf("send calls/admitted = %d/%d, want 2/1", calls, admitted)
	}
}

func TestTrack_AmbiguousBroadcastRebroadcastsIdenticalHashBeforeReplacement(t *testing.T) {
	b := newMockBackend()
	b.heldNonces[7] = true
	b.sendErrs = []error{context.DeadlineExceeded, nil, nil}
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: 5 * time.Millisecond,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	resultCh := sendAsync(m, Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	original := awaitTx(t, b.sendCallCh)
	identical := awaitTx(t, b.sendCallCh)
	replacement := awaitTx(t, b.sendCallCh)
	if identical.Hash() != original.Hash() {
		t.Fatalf("identical re-broadcast hash = %s, want %s", identical.Hash(), original.Hash())
	}
	if replacement.Hash() == original.Hash() {
		t.Fatal("fee-changing replacement did not create a distinct hash")
	}

	b.releaseNonce(7)
	result := awaitResult(t, resultCh)
	if result.State != StateConfirmed || result.Hash != replacement.Hash() || len(result.Hashes) != 2 ||
		result.Hashes[0] != original.Hash() || result.Hashes[1] != replacement.Hash() {
		t.Fatalf("result = %+v, want distinct signed attempts oldest to newest", result)
	}
	if got := b.sendCount(); got != 3 {
		t.Fatalf("SendTransaction calls = %d, want initial + identical re-broadcast + replacement", got)
	}
}

func TestTrack_ExplicitFeeCapPreventsBumpAndReturnsUnresolved(t *testing.T) {
	b := newMockBackend()
	b.heldNonces[7] = true
	m, cancel, done := newTestManager(t, b, Config{
		MaxFeeGwei: 41, TipGwei: 1,
		PollInterval: time.Millisecond, PendingInterval: 5 * time.Millisecond,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	resultCh := sendAsync(m, Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	original := awaitTx(t, b.sentCh)
	result := awaitResult(t, resultCh)
	if result.State != StateUnresolved || !errors.Is(result.Err, ErrUnresolved) {
		t.Fatalf("result = %+v, want unresolved fee-cap outcome", result)
	}
	if got := b.sendCount(); got != 1 {
		t.Fatalf("SendTransaction calls = %d, want original only", got)
	}
	if result.Hash != original.Hash() || len(result.Hashes) != 1 || result.Hashes[0] != original.Hash() {
		t.Fatalf("result hashes = %v/%v, want only original %s", result.Hash, result.Hashes, original.Hash())
	}
}

func TestTrack_TransientHeaderAndHeadErrorsRetry(t *testing.T) {
	headerFailure := errors.New("temporary header failure")
	headFailure := errors.New("temporary head failure")
	b := newMockBackend()
	b.heldNonces[7] = true
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: time.Second,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	resultCh := sendAsync(m, Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	original := awaitTx(t, b.sentCh)
	b.mu.Lock()
	b.headerErrs = []error{headerFailure}
	b.blockErrs = []error{headFailure}
	b.receipts[original.Hash()] = &types.Receipt{
		Status:      types.ReceiptStatusSuccessful,
		TxHash:      original.Hash(),
		BlockNumber: big.NewInt(100),
		BlockHash:   b.headerFor(100).Hash(),
	}
	b.mu.Unlock()

	result := awaitResult(t, resultCh)
	if result.State != StateConfirmed || result.Err != nil {
		t.Fatalf("result = %+v, want confirmed after transient canonicality errors", result)
	}
	if headers, blocks := b.remainingCanonicalityErrors(); headers != 0 || blocks != 0 {
		t.Fatalf("unconsumed canonicality errors: headers=%d blocks=%d", headers, blocks)
	}
}

func TestTrack_NilCanonicalHeaderIsTransient(t *testing.T) {
	b := newMockBackend()
	b.heldNonces[7] = true
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: time.Second,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})
	defer func() { cancel(); <-done }()

	resultCh := sendAsync(m, Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	original := awaitTx(t, b.sentCh)
	b.mu.Lock()
	b.nilHeaders = 1
	b.receipts[original.Hash()] = &types.Receipt{
		Status:      types.ReceiptStatusSuccessful,
		TxHash:      original.Hash(),
		BlockNumber: big.NewInt(100),
		BlockHash:   b.headerFor(100).Hash(),
	}
	b.mu.Unlock()

	result := awaitResult(t, resultCh)
	if result.State != StateConfirmed || result.Err != nil {
		t.Fatalf("result = %+v, want confirmed after nil canonical header", result)
	}
	if remaining := b.remainingNilHeaders(); remaining != 0 {
		t.Fatalf("nil header responses remaining = %d, want 0", remaining)
	}
}

func TestStart_CancellationReturnsUnresolvedAndJoinsTrackers(t *testing.T) {
	b := newMockBackend()
	b.heldNonces[7] = true
	m, cancel, done := newTestManager(t, b, Config{
		PollInterval: time.Millisecond, PendingInterval: time.Hour,
		FeeBumpBps: 1_250, MaxReplacements: 1,
	})

	resultCh := sendAsync(m, Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
	original := awaitTx(t, b.sentCh)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start returned %v", err)
		}
	case <-time.After(trackerTestGuard):
		t.Fatal("Start did not join the admitted tracker")
	}

	result := awaitResult(t, resultCh)
	if result.State != StateUnresolved || !errors.Is(result.Err, ErrUnresolved) ||
		!errors.Is(result.Err, context.Canceled) || result.Hash != original.Hash() {
		t.Fatalf("result = %+v, want cancellation-qualified unresolved outcome", result)
	}
	if got := b.sendCount(); got != 1 {
		t.Fatalf("SendTransaction calls after cancellation = %d, want 1", got)
	}
}

func TestSend_AfterManagerStopsReturnsNotBroadcast(t *testing.T) {
	m := New(newMockBackend(), mustSigner(t), big.NewInt(1), Config{}, logr.Discard())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	result := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc")})
	if result.State != StateNotBroadcast || !result.SafeToRetry() || !errors.Is(result.Err, ErrManagerStopped) ||
		result.Hash != (common.Hash{}) || len(result.Hashes) != 0 {
		t.Fatalf("result = %+v, want safe not_broadcast after manager stop", result)
	}
}
