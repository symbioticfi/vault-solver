package txmanager

import (
	"errors"
	"io"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
)

func TestLaneStateSubscriptionsFanOutWithoutStealingEdges(t *testing.T) {
	m := New(newMockBackend(), mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
	first, unsubscribeFirst := m.SubscribeLaneState()
	second, unsubscribeSecond := m.SubscribeLaneState()
	defer unsubscribeSecond()

	m.markNonceConflict(7, common.HexToHash("0x1234"))
	for name, changes := range map[string]<-chan struct{}{"first": first, "second": second} {
		select {
		case <-changes:
		case <-time.After(time.Second):
			t.Fatalf("%s subscriber did not receive pause edge", name)
		}
	}

	unsubscribeFirst()
	m.clearNonceConflict(7)
	select {
	case <-second:
	case <-time.After(time.Second):
		t.Fatal("remaining subscriber did not receive resume edge")
	}
	select {
	case <-first:
		t.Fatal("unsubscribed consumer received resume edge")
	default:
	}
}

func TestReplacementNonceTooLowReconcilesOwnedInclusionWithoutPausing(t *testing.T) {
	b := &replacementNonceRaceBackend{
		mockBackend:         newMockBackend(),
		publishOwnedReceipt: true,
	}
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{
			Confirmations:       2,
			PollInterval:        time.Millisecond,
			ReplacementInterval: 10 * time.Millisecond,
		},
		logr.Discard(),
	)
	laneStateChanges, unsubscribe := m.SubscribeLaneState()
	defer unsubscribe()
	pending, err := m.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "inclusion race",
	})
	if err != nil {
		t.Fatalf("initial broadcast: %v", err)
	}

	m.tryReplace(t.Context(), pending, false)
	if !m.Available() {
		t.Fatal("owned canonical inclusion paused the nonce lane")
	}
	select {
	case <-laneStateChanges:
		t.Fatal("owned canonical inclusion published a pause edge")
	default:
	}

	b.mu.Lock()
	b.head = 102
	b.mu.Unlock()
	got, done := m.receiptResult(t.Context(), pending)
	if !done || got.Err != nil || got.Receipt == nil {
		t.Fatalf("confirmed receipt outcome = (%+v, %v)", got, done)
	}
}

func TestReplacementNonceTooLowWithoutOwnedReceiptPauses(t *testing.T) {
	b := &replacementNonceRaceBackend{mockBackend: newMockBackend()}
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{ReplacementInterval: 10 * time.Millisecond}, logr.Discard(),
	)
	laneStateChanges, unsubscribe := m.SubscribeLaneState()
	defer unsubscribe()
	pending, err := m.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "unresolved replacement",
	})
	if err != nil {
		t.Fatalf("initial broadcast: %v", err)
	}

	m.tryReplace(t.Context(), pending, false)
	if m.Available() {
		t.Fatal("unexplained nonce consumption left the nonce lane available")
	}
	select {
	case <-laneStateChanges:
	case <-time.After(time.Second):
		t.Fatal("unexplained nonce consumption did not publish a pause edge")
	}
	if len(pending.attempts) != 2 || pending.nonceConflictHash != pending.attempts[1].hash {
		t.Fatalf("pending conflict evidence = %+v", pending)
	}
}

func TestReplacementNonceTooLowDelayedReceiptResumesThenReorgPauses(t *testing.T) {
	b := &replacementNonceRaceBackend{mockBackend: newMockBackend()}
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{Confirmations: 2, PollInterval: time.Millisecond}, logr.Discard(),
	)
	laneStateChanges, unsubscribe := m.SubscribeLaneState()
	defer unsubscribe()
	pending, err := m.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "delayed inclusion race",
	})
	if err != nil {
		t.Fatalf("initial broadcast: %v", err)
	}
	m.tryReplace(t.Context(), pending, false)
	if m.Available() {
		t.Fatal("replacement nonce conflict did not pause the lane")
	}
	select {
	case <-laneStateChanges:
	case <-time.After(time.Second):
		t.Fatal("replacement nonce conflict did not publish a pause edge")
	}

	original := pending.attempts[0]
	b.mu.Lock()
	b.receipts[original.hash] = successfulReceipt(original.tx, b.head)
	b.mu.Unlock()
	type receiptOutcome struct {
		result Result
		done   bool
	}
	result := make(chan receiptOutcome, 1)
	go func() {
		got, done := m.receiptResult(t.Context(), pending)
		result <- receiptOutcome{result: got, done: done}
	}()
	select {
	case <-laneStateChanges:
		if !m.Available() {
			t.Fatal("canonical tracked receipt did not resume the lane")
		}
	case <-time.After(time.Second):
		t.Fatal("delayed canonical receipt did not publish a resume edge")
	}

	b.mu.Lock()
	delete(b.receipts, original.hash)
	b.mu.Unlock()
	select {
	case got := <-result:
		if got.done {
			t.Fatalf("reorged receipt completed the lifecycle: %+v", got.result)
		}
	case <-time.After(time.Second):
		t.Fatal("receipt disappearance did not resume pending reconciliation")
	}
	select {
	case <-laneStateChanges:
		if m.Available() {
			t.Fatal("receipt reorg did not restore the nonce conflict")
		}
	case <-time.After(time.Second):
		t.Fatal("receipt reorg did not publish a pause edge")
	}
}

func TestInitialReplacementUnderpricedPausesTransactionLane(t *testing.T) {
	b := newMockBackend()
	b.sendErrs = []error{errors.New("replacement transaction underpriced")}
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
	laneStateChanges, unsubscribe := m.SubscribeLaneState()
	defer unsubscribe()

	pending, err := m.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "pending collision",
	})
	if pending != nil || err == nil || !strings.Contains(err.Error(), "replacement transaction underpriced") {
		t.Fatalf("pending collision result = (%+v, %v)", pending, err)
	}
	if m.Available() {
		t.Fatal("manager remained available after a pending nonce collision")
	}
	select {
	case <-laneStateChanges:
	case <-time.After(time.Second):
		t.Fatal("pending nonce collision did not publish an availability change")
	}

	second, err := m.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xdef"), GasLimit: 21_000, Label: "must not advance",
	})
	if second != nil || !errors.Is(err, errNonceLanePaused) {
		t.Fatalf("second broadcast = (%+v, %v), want paused error", second, err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sendCalls != 1 {
		t.Fatalf("broadcast calls = %d, want 1", b.sendCalls)
	}
}

func TestNonceTooLowWithExactReceiptReconcilesAndResumes(t *testing.T) {
	receiptGate := make(chan struct{})
	b := &acceptedThenNonceLowBackend{mockBackend: newMockBackend(), receiptGate: receiptGate}
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{PollInterval: time.Millisecond}, logr.Discard(),
	)
	laneStateChanges, unsubscribe := m.SubscribeLaneState()
	defer unsubscribe()
	startTestManager(t, m)
	firstResult, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "accepted before nonce error",
	})
	if !accepted {
		t.Fatal("first transaction was not accepted")
	}
	waitForSentTransactions(t, b.mockBackend, 1)
	timeout := time.After(time.Second)
	paused := false
	for !paused {
		select {
		case <-laneStateChanges:
			paused = !m.Available()
		case <-timeout:
			t.Fatal("nonce conflict did not publish the pause edge")
		}
	}
	type submission struct {
		result   <-chan Result
		accepted bool
	}
	secondSubmission := make(chan submission, 1)
	go func() {
		result, secondAccepted := m.SendAsync(t.Context(), Request{
			To: common.HexToAddress("0xdef"), GasLimit: 21_000, Label: "blocked during reconciliation",
		})
		secondSubmission <- submission{result: result, accepted: secondAccepted}
	}()
	waitForAdmissionDemand(t, m, 2)
	select {
	case got := <-secondSubmission:
		t.Fatalf("second request completed admission during reconciliation: %+v", got)
	case <-time.After(20 * time.Millisecond):
	}
	close(receiptGate)
	first := <-firstResult
	if first.Err != nil || first.Receipt == nil {
		t.Fatalf("reconciled first result = %+v", first)
	}
	select {
	case <-laneStateChanges:
	case <-time.After(time.Second):
		t.Fatal("exact receipt did not publish the resume edge")
	}
	if !m.Available() {
		t.Fatal("manager did not resume after exact receipt reconciliation")
	}
	var second submission
	select {
	case second = <-secondSubmission:
		if !second.accepted {
			t.Fatal("second request was not accepted after reconciliation")
		}
	case <-time.After(time.Second):
		t.Fatal("second request remained blocked after reconciliation")
	}
	if result := <-second.result; result.Err != nil {
		t.Fatalf("second result after reconciliation: %v", result.Err)
	}
	if tx := b.lastSent(); tx == nil || tx.Nonce() != 8 {
		t.Fatalf("second transaction = %v, want nonce 8", tx)
	}
}

func TestConcurrentNoncePauseStopsSignedBytesBeforeBroadcast(t *testing.T) {
	b := newMockBackend()
	s := &blockingTxSigner{
		Signer: mustSigner(t), entered: make(chan struct{}), release: make(chan struct{}),
	}
	m := New(b, s, big.NewInt(11155111), Config{}, logr.Discard())
	result := make(chan error, 1)
	go func() {
		_, err := m.broadcast(t.Context(), Request{
			To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "pause race",
		})
		result <- err
	}()
	<-s.entered
	m.markNonceConflict(7, common.HexToHash("0x1234"))
	close(s.release)

	if err := <-result; !errors.Is(err, errNonceLanePaused) {
		t.Fatalf("broadcast error = %v, want nonce-lane pause", err)
	}
	if b.sendCalls != 0 || m.nonce != 7 {
		t.Fatalf("pause race broadcast calls/nonce = %d/%d, want 0/7", b.sendCalls, m.nonce)
	}
}

func TestAmbiguousBroadcastErrorsTrackExactSignedHash(t *testing.T) {
	b := newMockBackend()
	b.sendErrs = []error{io.ErrUnexpectedEOF}
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())

	pending, err := m.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "ambiguous",
	})
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	if pending.nonce != 7 || len(pending.attempts) != 1 ||
		pending.attempts[0].tx == nil || pending.attempts[0].hash != pending.attempts[0].tx.Hash() ||
		!pending.attempts[0].exactRebroadcastPending {
		t.Fatalf("pending = nonce %d, attempts %+v", pending.nonce, pending.attempts)
	}
	if m.nonce != 8 {
		t.Fatalf("next nonce = %d, want 8 while exact hash remains tracked", m.nonce)
	}
	originalHash, originalFees := pending.originalHash, cloneFeeQuote(pending.fees)
	m.tryReplace(t.Context(), pending, false)
	attempted := b.attemptedTransactions()
	if len(attempted) != 2 || attempted[0].Hash() != originalHash || attempted[1].Hash() != originalHash ||
		len(pending.attempts) != 1 || pending.attempts[0].exactRebroadcastPending {
		t.Fatalf("exact retry = %v, attempts %+v", transactionHashes(attempted), pending.attempts)
	}
	if pending.fees.maxFee.Cmp(originalFees.maxFee) != 0 || pending.fees.tip.Cmp(originalFees.tip) != 0 {
		t.Fatalf("exact retry changed fees: got %+v want %+v", pending.fees, originalFees)
	}
	m.tryReplace(t.Context(), pending, false)
	attempted = b.attemptedTransactions()
	if len(attempted) != 3 || attempted[2].Hash() == originalHash || len(pending.attempts) != 2 ||
		pending.fees.maxFee.Cmp(bumpFee(originalFees.maxFee)) != 0 {
		t.Fatalf("post-retry replacement = %v, pending %+v", transactionHashes(attempted), pending)
	}
}

func TestDefiniteBroadcastRejectionDoesNotConsumeNonce(t *testing.T) {
	b := newMockBackend()
	b.sendErrs = []error{errors.New("insufficient funds for gas * price + value")}
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
	req := Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "rejected"}

	if pending, err := m.broadcast(t.Context(), req); err == nil || pending != nil {
		t.Fatalf("definite rejection = (%+v, %v), want error without lifecycle", pending, err)
	}
	if m.nonce != 7 {
		t.Fatalf("nonce after definite rejection = %d, want 7", m.nonce)
	}
	pending, err := m.broadcast(t.Context(), req)
	if err != nil {
		t.Fatalf("retry broadcast: %v", err)
	}
	if pending.nonce != 7 {
		t.Fatalf("retry nonce = %d, want original 7", pending.nonce)
	}
}

func TestKnownTransactionErrorClassificationIsNarrow(t *testing.T) {
	if !isKnownTransactionError(errors.New("already known")) {
		t.Fatal("already-known transaction was not recognized")
	}
	for _, message := range []string{"unknown transaction", "unknown transaction type", "nonce too low"} {
		if isKnownTransactionError(errors.New(message)) {
			t.Fatalf("%q was incorrectly classified as already known", message)
		}
	}
}
