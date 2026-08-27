package txmanager

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
)

func TestSend_GasEstimateFailurePropagates(t *testing.T) {
	b := newMockBackend()
	b.gasEstimate = 0 // forces EstimateGas to error
	m := newTestManager(t, b)

	res := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), Label: "noestimate"})
	if res.Err == nil {
		t.Fatal("expected gas-estimate error to propagate")
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

// TestSend_CallerCancelAfterEnqueueStillReturnsResult guards the fund-moving invariant: once a
// request is enqueued the worker broadcasts it on the manager's context, so Send must report that
// real outcome. Cancelling the caller's context after enqueue must NOT make Send return a
// cancellation while the tx still lands on-chain (which would read as "not sent").
func TestSend_CallerCancelAfterEnqueueStillReturnsResult(t *testing.T) {
	bb := &blockingBackend{mockBackend: newMockBackend(), entered: make(chan struct{}), release: make(chan struct{})}
	m := New(bb, mustSigner(t), big.NewInt(11155111), Config{PollInterval: time.Millisecond}, logr.Discard())
	startTestManager(t, m) // manager context lives until test cleanup; the caller's is cancelled below

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

func TestStartCancelInterruptsPreSignRPC(t *testing.T) {
	b := &blockingEstimateBackend{mockBackend: newMockBackend(), entered: make(chan struct{})}
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
	managerCtx, cancelManager := context.WithCancel(t.Context())
	defer cancelManager()
	startDone := make(chan struct{})
	go func() {
		m.Start(managerCtx)
		close(startDone)
	}()

	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), Label: "blocked pre-sign rpc",
	})
	if !accepted {
		t.Fatal("transaction was not accepted")
	}
	select {
	case <-b.entered:
	case <-time.After(time.Second):
		t.Fatal("gas estimation did not start")
	}
	cancelManager()

	select {
	case got := <-result:
		if !errors.Is(got.Err, context.Canceled) {
			t.Fatalf("pre-sign result = %+v, want context cancellation", got)
		}
	case <-time.After(time.Second):
		t.Fatal("pre-sign RPC did not stop after manager cancellation")
	}
	select {
	case <-startDone:
	case <-time.After(time.Second):
		t.Fatal("transaction manager did not stop after pre-sign cancellation")
	}
	if b.sendCalls != 0 {
		t.Fatalf("broadcast calls = %d, want none before signing", b.sendCalls)
	}
}

func TestStartCancelInterruptsInitialSigner(t *testing.T) {
	b := newMockBackend()
	s := &blockingTxSigner{
		Signer: mustSigner(t), entered: make(chan struct{}), release: make(chan struct{}),
	}
	m := New(
		b, s, big.NewInt(11155111),
		Config{ShutdownTimeout: 20 * time.Millisecond}, logr.Discard(),
	)
	managerCtx, cancelManager := context.WithCancel(t.Context())
	startDone := make(chan struct{})
	go func() {
		m.Start(managerCtx)
		close(startDone)
	}()

	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "blocked initial signer",
	})
	if !accepted {
		t.Fatal("transaction was not accepted for initial signing")
	}
	select {
	case <-s.entered:
	case <-time.After(time.Second):
		t.Fatal("initial signing did not start")
	}
	cancelManager()

	select {
	case got := <-result:
		if !errors.Is(got.Err, context.Canceled) || !got.NotAdmitted ||
			got.Hash != (common.Hash{}) || got.Receipt != nil {
			t.Fatalf("initial-sign result = %+v, want not-admitted context cancellation", got)
		}
	case <-time.After(time.Second):
		t.Fatal("initial signer did not stop after manager cancellation")
	}
	select {
	case <-startDone:
	case <-time.After(time.Second):
		t.Fatal("blocked initial signer kept the transaction manager alive")
	}
	if b.sendCalls != 0 {
		t.Fatalf("broadcast calls = %d, want none after cancelled signing", b.sendCalls)
	}
}

func TestStartCancelKeepsAcceptedLifecycleOwned(t *testing.T) {
	sgnr := mustSigner(t)
	b := &replacementBackend{mockBackend: newMockBackend(), cancellationTo: sgnr.Address()}
	m := New(
		b, sgnr, big.NewInt(11155111),
		Config{
			PollInterval:        time.Millisecond,
			ReplacementInterval: time.Hour,
			PendingTimeout:      time.Hour,
			ShutdownTimeout:     time.Second,
		},
		logr.Discard(),
	)
	managerCtx, cancelManager := context.WithCancel(t.Context())
	startDone := make(chan struct{})
	go func() {
		m.Start(managerCtx)
		close(startDone)
	}()

	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "shutdown drain",
	})
	if !accepted {
		t.Fatal("transaction was not accepted")
	}
	waitForSentTransactions(t, b.mockBackend, 1)
	type submission struct {
		result   <-chan Result
		accepted bool
	}
	waiterReady := make(chan struct{})
	waiter := make(chan submission, 1)
	go func() {
		close(waiterReady)
		waitingResult, waiterAccepted := m.SendAsync(context.Background(), Request{
			To: common.HexToAddress("0xdef"), GasLimit: 21_000, Label: "shutdown waiter",
		})
		waiter <- submission{result: waitingResult, accepted: waiterAccepted}
	}()
	<-waiterReady
	cancelManager()

	select {
	case got := <-result:
		if got.Receipt == nil || got.Err == nil ||
			!strings.Contains(got.Err.Error(), "cancelled at nonce 7") {
			t.Fatalf("drained result = %+v", got)
		}
		if errors.Is(got.Err, context.Canceled) {
			t.Fatalf("accepted lifecycle was abandoned: %v", got.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("accepted lifecycle was not cancelled and drained")
	}
	select {
	case waiting := <-waiter:
		if !waiting.accepted {
			t.Fatal("shutdown waiter returned without a terminal result")
		}
		if got := <-waiting.result; !errors.Is(got.Err, errManagerStopped) || !got.NotAdmitted {
			t.Fatalf("shutdown waiter result = %+v, want not-admitted manager stop", got)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown waiter remained blocked after manager cancellation")
	}
	select {
	case <-startDone:
	case <-time.After(time.Second):
		t.Fatal("transaction manager did not finish draining")
	}
}

func TestStartCancelBoundsUnresolvedNonceConflict(t *testing.T) {
	receiptGate := make(chan struct{})
	b := &acceptedThenNonceLowBackend{mockBackend: newMockBackend(), receiptGate: receiptGate}
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{
			PollInterval:        time.Millisecond,
			ReplacementInterval: time.Hour,
			PendingTimeout:      time.Hour,
			ShutdownTimeout:     20 * time.Millisecond,
		},
		logr.Discard(),
	)
	laneStateChanges, unsubscribe := m.SubscribeLaneState()
	defer unsubscribe()
	managerCtx, cancelManager := context.WithCancel(t.Context())
	startDone := make(chan struct{})
	go func() {
		m.Start(managerCtx)
		close(startDone)
	}()

	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "shutdown conflict",
	})
	if !accepted {
		t.Fatal("transaction was not accepted")
	}
	timeout := time.After(time.Second)
	for m.Available() {
		select {
		case <-laneStateChanges:
		case <-timeout:
			t.Fatal("initial nonce conflict did not pause the lane")
		}
	}
	cancelManager()

	select {
	case got := <-result:
		if !errors.Is(got.Err, context.DeadlineExceeded) || got.Hash == (common.Hash{}) || got.NotAdmitted {
			t.Fatalf("bounded conflict result = %+v, want shutdown deadline with tracked hash", got)
		}
	case <-time.After(time.Second):
		t.Fatal("unresolved nonce conflict exceeded the shutdown bound")
	}
	select {
	case <-startDone:
	case <-time.After(time.Second):
		t.Fatal("manager did not return after the conflict drain deadline")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sendCalls != 1 {
		t.Fatalf("broadcast calls = %d, want no unsafe cancellation during conflict", b.sendCalls)
	}
}

func TestStartCancelBoundsCancellationWriteOutage(t *testing.T) {
	b := &shutdownWriteOutageBackend{
		mockBackend:         newMockBackend(),
		cancellationStarted: make(chan struct{}),
	}
	sgnr := mustSigner(t)
	m := New(
		b, sgnr, big.NewInt(11155111),
		Config{
			PollInterval:        time.Millisecond,
			ReplacementInterval: time.Hour,
			PendingTimeout:      time.Hour,
			ShutdownTimeout:     20 * time.Millisecond,
		},
		logr.Discard(),
	)
	managerCtx, cancelManager := context.WithCancel(t.Context())
	startDone := make(chan struct{})
	go func() {
		m.Start(managerCtx)
		close(startDone)
	}()

	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "shutdown write outage",
	})
	if !accepted {
		t.Fatal("transaction was not accepted")
	}
	waitForSentTransactions(t, b.mockBackend, 1)
	cancelManager()
	select {
	case <-b.cancellationStarted:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not attempt same-nonce cancellation")
	}

	select {
	case got := <-result:
		if !errors.Is(got.Err, context.DeadlineExceeded) || got.Hash == (common.Hash{}) || got.NotAdmitted {
			t.Fatalf("bounded write-outage result = %+v, want shutdown deadline with tracked hash", got)
		}
	case <-time.After(time.Second):
		t.Fatal("write outage exceeded the shutdown bound")
	}
	select {
	case <-startDone:
	case <-time.After(time.Second):
		t.Fatal("manager did not return after cancelling the blocked write RPC")
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sendCalls != 2 {
		t.Fatalf("broadcast calls = %d, want initial fill plus one cancellation", b.sendCalls)
	}
	cancellation := b.sent[1]
	if cancellation.To() == nil || *cancellation.To() != sgnr.Address() ||
		len(cancellation.Data()) != 0 || cancellation.Value().Sign() != 0 {
		t.Fatalf("shutdown replacement is not a self-cancellation: %+v", cancellation)
	}
}

func TestStartCancelReturnsWhenCancellationSignerBlocks(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseSigner := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseSigner)

	baseSigner := mustSigner(t)
	s := &shutdownBlockingSigner{
		Signer:             baseSigner,
		replacementStarted: make(chan struct{}),
		release:            release,
	}
	b := &replacementBackend{mockBackend: newMockBackend(), cancellationTo: baseSigner.Address()}
	m := New(
		b, s, big.NewInt(11155111),
		Config{
			PollInterval:        time.Millisecond,
			ReplacementInterval: time.Hour,
			PendingTimeout:      time.Hour,
			ShutdownTimeout:     20 * time.Millisecond,
		},
		logr.Discard(),
	)
	managerCtx, cancelManager := context.WithCancel(t.Context())
	startDone := make(chan struct{})
	go func() {
		m.Start(managerCtx)
		close(startDone)
	}()

	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "blocked shutdown signer",
	})
	if !accepted {
		t.Fatal("transaction was not accepted")
	}
	waitForSentTransactions(t, b.mockBackend, 1)
	cancelManager()
	select {
	case <-s.replacementStarted:
	case <-time.After(time.Second):
		t.Fatal("shutdown cancellation did not reach the signer")
	}

	select {
	case got := <-result:
		if !errors.Is(got.Err, errShutdownTimeout) || got.Hash == (common.Hash{}) || got.NotAdmitted {
			t.Fatalf("blocked-signer result = %+v, want tracked shutdown timeout", got)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked signer prevented the accepted caller from completing")
	}
	select {
	case <-startDone:
	case <-time.After(time.Second):
		t.Fatal("blocked signer kept the transaction manager alive past its shutdown bound")
	}

	releaseSigner()
	lifecycleDone := make(chan struct{})
	go func() {
		m.lifecycleWG.Wait()
		close(lifecycleDone)
	}()
	select {
	case <-lifecycleDone:
	case <-time.After(time.Second):
		t.Fatal("released signer did not let the detached lifecycle finish")
	}
	select {
	case extra := <-result:
		t.Fatalf("accepted caller received a second terminal result: %+v", extra)
	default:
	}
}
