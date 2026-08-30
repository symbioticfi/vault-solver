package txmanager

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
)

func TestBroadcastRejectsExpiredRequest(t *testing.T) {
	b := newMockBackend()
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
	_, err := m.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000,
		CancelAt: time.Now().Add(-time.Second), Label: "expired",
	})
	if err == nil || b.sendCalls != 0 {
		t.Fatalf("expired broadcast = %v, send calls = %d", err, b.sendCalls)
	}
}

func TestBroadcastRejectsObsoleteRequestBeforeSigning(t *testing.T) {
	b := newMockBackend()
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
	checks := 0

	_, err := m.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "obsolete",
		Obsolete: func(context.Context) (bool, error) {
			checks++
			return true, nil
		},
	})
	if !errors.Is(err, errRequestObsolete) {
		t.Fatalf("broadcast error = %v, want obsolete request", err)
	}
	if checks != 1 {
		t.Fatalf("obsolescence checks = %d, want 1", checks)
	}
	if attempted := b.attemptedTransactions(); len(attempted) != 0 {
		t.Fatalf("obsolete request broadcast %d transactions", len(attempted))
	}
}

func TestBroadcastContinuesWhenObsolescenceIsUnknown(t *testing.T) {
	b := newMockBackend()
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
	checkErr := errors.New("status RPC unavailable")

	pending, err := m.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "unknown",
		Obsolete: func(context.Context) (bool, error) { return false, checkErr },
	})
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	if pending == nil || len(b.attemptedTransactions()) != 1 {
		t.Fatalf("unknown obsolescence result = pending %v, attempts %d", pending != nil, len(b.attemptedTransactions()))
	}
}

func TestBroadcastTimeout(t *testing.T) {
	for name, test := range map[string]struct {
		configured time.Duration
		want       time.Duration
	}{
		"independent default": {want: defaultBroadcastTimeout},
		"explicit override":   {configured: 7 * time.Second, want: 7 * time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			m := New(nil, nil, nil, Config{
				BroadcastTimeout: test.configured, ReplacementInterval: 2 * time.Millisecond,
			}, logr.Discard())
			if got := m.cfg.BroadcastTimeout; got != test.want {
				t.Fatalf("broadcast timeout = %s, want %s", got, test.want)
			}
		})
	}
}

func TestSend_SequentialNoncesMonotonic(t *testing.T) {
	b := newMockBackend()
	m := newTestManager(t, b)

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

func TestSendAsyncKeepsFutureNonceUnsignedUntilPriorConfirmation(t *testing.T) {
	b := newMockBackend()
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{Confirmations: 2, PollInterval: time.Millisecond}, logr.Discard(),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Start(ctx)

	first, accepted := m.SendAsync(
		context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "first"},
	)
	if !accepted {
		t.Fatal("first SendAsync was not accepted")
	}
	waitForSentTransactions(t, b, 1)

	type submission struct {
		result   <-chan Result
		accepted bool
	}
	secondSubmission := make(chan submission, 1)
	go func() {
		result, secondAccepted := m.SendAsync(
			context.Background(), Request{To: common.HexToAddress("0xabc"), Label: "waiting"},
		)
		secondSubmission <- submission{result: result, accepted: secondAccepted}
	}()
	select {
	case got := <-secondSubmission:
		t.Fatalf("future request was admitted before prior confirmation: %+v", got)
	case <-time.After(20 * time.Millisecond):
	}
	b.mu.Lock()
	if len(b.sent) != 1 || b.sent[0].Nonce() != 7 {
		b.mu.Unlock()
		t.Fatalf("sent transactions = %v, want only nonce 7", b.sent)
	}
	if calls := b.estimateCalls.Load(); calls != 0 {
		b.mu.Unlock()
		t.Fatalf("waiting request was estimated before admission: %d calls", calls)
	}
	b.head = 102
	b.mu.Unlock()
	if got := <-first; got.Err != nil {
		t.Fatalf("first result: %v", got.Err)
	}
	var second submission
	select {
	case second = <-secondSubmission:
		if !second.accepted {
			t.Fatal("second SendAsync was not accepted after prior confirmation")
		}
	case <-time.After(time.Second):
		t.Fatal("second SendAsync remained blocked after prior confirmation")
	}
	waitForSentTransactions(t, b, 2)
	if calls := b.estimateCalls.Load(); calls != 1 {
		t.Fatalf("admitted request gas estimates = %d, want 1", calls)
	}
	b.mu.Lock()
	secondTx := b.sent[1]
	b.head = 104
	b.mu.Unlock()
	if secondTx.Nonce() != 8 {
		t.Fatalf("second nonce = %d, want 8", secondTx.Nonce())
	}
	if got := <-second.result; got.Err != nil {
		t.Fatalf("second result: %v", got.Err)
	}
}

func TestIdleTracksActiveAndWaitingRequests(t *testing.T) {
	b := newMockBackend()
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{Confirmations: 1, PollInterval: time.Millisecond}, logr.Discard(),
	)
	if !m.Idle() {
		t.Fatal("new manager is not idle")
	}
	startTestManager(t, m)

	first, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "first",
	})
	if !accepted {
		t.Fatal("first request was not accepted")
	}
	waitForSentTransactions(t, b, 1)
	waitForAdmissionDemand(t, m, 1)
	if m.Idle() {
		t.Fatal("manager is idle while a lifecycle is active")
	}

	type submission struct {
		result   <-chan Result
		accepted bool
	}
	secondSubmission := make(chan submission, 1)
	go func() {
		result, secondAccepted := m.SendAsync(t.Context(), Request{
			To: common.HexToAddress("0xdef"), GasLimit: 21_000, Label: "second",
		})
		secondSubmission <- submission{result: result, accepted: secondAccepted}
	}()
	waitForAdmissionDemand(t, m, 2)
	if m.Idle() {
		t.Fatal("manager is idle with an active lifecycle and a waiter")
	}

	b.mu.Lock()
	b.head = 101
	b.mu.Unlock()
	if got := <-first; got.Err != nil {
		t.Fatalf("first result: %v", got.Err)
	}

	var second submission
	select {
	case second = <-secondSubmission:
		if !second.accepted {
			t.Fatal("second request was not accepted after the handoff")
		}
	case <-time.After(time.Second):
		t.Fatal("second request remained blocked after the first completed")
	}
	waitForSentTransactions(t, b, 2)
	waitForAdmissionDemand(t, m, 1)
	if m.Idle() {
		t.Fatal("manager became idle during the lifecycle handoff")
	}

	b.mu.Lock()
	b.head = 102
	b.mu.Unlock()
	if got := <-second.result; got.Err != nil {
		t.Fatalf("second result: %v", got.Err)
	}
	waitForAdmissionDemand(t, m, 0)
	if !m.Idle() {
		t.Fatal("manager did not become idle after the terminal result")
	}
}

func TestLaneStateSignalsBusyAndIdleEdges(t *testing.T) {
	m := New(newMockBackend(), mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
	changes, unsubscribe := m.SubscribeLaneState()
	defer unsubscribe()
	if !m.LaneReady() {
		t.Fatal("new manager lane is not ready")
	}

	m.addAdmissionDemand()
	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive busy edge")
	}
	if m.LaneReady() || m.Idle() || !m.Available() {
		t.Fatal("busy manager reported an inconsistent lane state")
	}

	m.addAdmissionDemand()
	m.releaseAdmissionDemand()
	select {
	case <-changes:
		t.Fatal("non-terminal demand changes published a lane edge")
	default:
	}
	m.releaseAdmissionDemand()
	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive idle edge")
	}
	if !m.LaneReady() {
		t.Fatal("idle available manager lane is not ready")
	}
}

func TestResultMarksManagerAdmissionFailures(t *testing.T) {
	tests := []struct {
		name    string
		manager func(*testing.T) *Manager
		request Request
		wantErr error
	}{
		{
			name: "manager stopped",
			manager: func(t *testing.T) *Manager {
				t.Helper()
				m := New(newMockBackend(), mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				done := make(chan struct{})
				go func() {
					m.Start(ctx)
					close(done)
				}()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("manager did not stop")
				}
				return m
			},
			request: Request{To: common.HexToAddress("0xabc"), Label: "stopped"},
			wantErr: errManagerStopped,
		},
		{
			name: "expired before admission",
			manager: func(t *testing.T) *Manager {
				t.Helper()
				return New(newMockBackend(), mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
			},
			request: Request{
				To: common.HexToAddress("0xabc"), CancelAt: time.Now().Add(-time.Second), Label: "expired",
			},
			wantErr: context.DeadlineExceeded,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := test.manager(t)
			result, accepted := m.SendAsync(t.Context(), test.request)
			if !accepted {
				t.Fatal("manager-level admission failure did not return a terminal result")
			}
			got := <-result
			if !errors.Is(got.Err, test.wantErr) {
				t.Fatalf("result error = %v, want %v", got.Err, test.wantErr)
			}
			if !got.NotAdmitted {
				t.Fatalf("result = %+v, want NotAdmitted", got)
			}
			if got.Hash != (common.Hash{}) || got.Receipt != nil {
				t.Fatalf("not-admitted result has an on-chain outcome: %+v", got)
			}
			if !m.Idle() {
				t.Fatal("terminal admission failure left demand on the lane")
			}
		})
	}
}

func TestSendAsyncWaitsForNonceConflictToClear(t *testing.T) {
	b := newMockBackend()
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{PollInterval: time.Millisecond}, logr.Discard())
	m.markNonceConflict(7, common.HexToHash("0x1234"))
	startTestManager(t, m)

	type submission struct {
		result   <-chan Result
		accepted bool
	}
	submitted := make(chan submission, 1)
	go func() {
		result, accepted := m.SendAsync(t.Context(), Request{
			To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "wait for reconciliation",
		})
		submitted <- submission{result: result, accepted: accepted}
	}()
	waitForAdmissionDemand(t, m, 1)
	select {
	case got := <-submitted:
		t.Fatalf("request completed admission while nonce lane was paused: %+v", got)
	case <-time.After(20 * time.Millisecond):
	}
	m.clearNonceConflict(7)
	var got submission
	select {
	case got = <-submitted:
		if !got.accepted {
			t.Fatal("waiting request was not accepted after reconciliation")
		}
	case <-time.After(time.Second):
		t.Fatal("waiting request did not resume after reconciliation")
	}
	if result := <-got.result; result.Err != nil || result.Receipt == nil {
		t.Fatalf("resumed request result = %+v", result)
	}
	waitForAdmissionDemand(t, m, 0)
	if !m.LaneReady() {
		t.Fatal("lane did not become ready after the resumed lifecycle completed")
	}
}

func TestSendAsyncNonceConflictWaitHonorsCancellation(t *testing.T) {
	t.Run("request deadline", func(t *testing.T) {
		m := New(newMockBackend(), mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
		m.markNonceConflict(7, common.HexToHash("0x1234"))
		result, accepted := m.SendAsync(t.Context(), Request{
			To:       common.HexToAddress("0xabc"),
			CancelAt: time.Now().Add(20 * time.Millisecond),
			Label:    "expires while paused",
		})
		if !accepted {
			t.Fatal("request deadline did not return a terminal admission result")
		}
		got := <-result
		if !errors.Is(got.Err, context.DeadlineExceeded) || !got.NotAdmitted {
			t.Fatalf("deadline result = %+v", got)
		}
		if !m.Idle() {
			t.Fatal("deadline left admission demand on the lane")
		}
	})

	t.Run("caller context", func(t *testing.T) {
		m := New(newMockBackend(), mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
		m.markNonceConflict(7, common.HexToHash("0x1234"))
		ctx, cancel := context.WithCancel(t.Context())
		type submission struct {
			result   <-chan Result
			accepted bool
		}
		submitted := make(chan submission, 1)
		go func() {
			result, accepted := m.SendAsync(ctx, Request{
				To: common.HexToAddress("0xabc"), Label: "caller cancels while paused",
			})
			submitted <- submission{result: result, accepted: accepted}
		}()
		waitForAdmissionDemand(t, m, 1)
		cancel()
		select {
		case got := <-submitted:
			if got.accepted || got.result != nil {
				t.Fatalf("caller cancellation submission = %+v, want not accepted", got)
			}
		case <-time.After(time.Second):
			t.Fatal("caller cancellation did not stop nonce-conflict admission wait")
		}
		waitForAdmissionDemand(t, m, 0)
	})

	t.Run("manager stop", func(t *testing.T) {
		m := New(newMockBackend(), mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
		m.markNonceConflict(7, common.HexToHash("0x1234"))
		managerCtx, cancelManager := context.WithCancel(t.Context())
		managerDone := make(chan struct{})
		go func() {
			m.Start(managerCtx)
			close(managerDone)
		}()
		type submission struct {
			result   <-chan Result
			accepted bool
		}
		submitted := make(chan submission, 1)
		go func() {
			result, accepted := m.SendAsync(t.Context(), Request{
				To: common.HexToAddress("0xabc"), Label: "manager stops while paused",
			})
			submitted <- submission{result: result, accepted: accepted}
		}()
		waitForAdmissionDemand(t, m, 1)
		cancelManager()
		select {
		case <-managerDone:
		case <-time.After(time.Second):
			t.Fatal("manager did not stop")
		}
		select {
		case got := <-submitted:
			if !got.accepted {
				t.Fatal("manager stop did not return a terminal admission result")
			}
			result := <-got.result
			if !errors.Is(result.Err, errManagerStopped) || !result.NotAdmitted {
				t.Fatalf("manager stop result = %+v", result)
			}
		case <-time.After(time.Second):
			t.Fatal("manager stop did not stop nonce-conflict admission wait")
		}
		waitForAdmissionDemand(t, m, 0)
	})
}

func TestSendAsyncCanCompleteAtInclusion(t *testing.T) {
	b := newMockBackend()
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{Confirmations: 2, PollInterval: time.Millisecond}, logr.Discard(),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Start(ctx)
	confirmations := uint64(0)
	result, accepted := m.SendAsync(context.Background(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Confirmations: &confirmations, Label: "inclusion",
	})
	if !accepted {
		t.Fatal("SendAsync was not accepted")
	}
	select {
	case got := <-result:
		if got.Err != nil || got.Receipt == nil || got.Receipt.BlockNumber.Uint64() != 100 {
			t.Fatalf("result = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not complete at inclusion")
	}
}
