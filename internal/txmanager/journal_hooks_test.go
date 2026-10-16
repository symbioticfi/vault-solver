package txmanager

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
)

func TestReservedOwnerBlocksOtherSolversAfterTrackingEnds(t *testing.T) {
	b := &abandonedPoolBackend{newMockBackend()}
	m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, PollInterval: time.Millisecond, PendingTimeout: 20 * time.Millisecond, ShutdownTimeout: time.Millisecond}, logr.Discard())
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := m.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	go m.Start(ctx)
	if err := m.Hold("durable-job"); err != nil {
		t.Fatal(err)
	}
	if m.LaneReady() {
		t.Fatal("reserved lane advertised ready")
	}
	if err := m.Hold("other-job"); err == nil {
		t.Fatal("second owner entered lane")
	}
	to := common.HexToAddress("0x1234")
	got := m.Send(ctx, Request{To: to, Label: "other-solver"})
	if !got.NotAdmitted {
		t.Fatalf("foreign solver admitted: %+v", got)
	}
	got = m.Send(ctx, Request{To: to, Owner: "durable-job", Label: "execute", BeforeBroadcast: func(_, _ *types.Transaction) error { return nil }})
	if !got.Outcome.NonceUncertain() {
		t.Fatalf("expected uncertain work: %+v", got)
	}
	if m.LaneReady() {
		t.Fatal("uncertain job lost signer lane")
	}
	if err := m.Release("wrong-owner"); err == nil {
		t.Fatal("foreign owner released lane")
	}
	if err := m.Release("durable-job"); err != nil {
		t.Fatal(err)
	}
}

func TestJournalFailurePreventsBroadcast(t *testing.T) {
	b := newMockBackend()
	m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, ShutdownTimeout: time.Millisecond}, logr.Discard())
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go m.Start(ctx)
	want := errors.New("disk full")
	got := m.Send(ctx, Request{To: common.HexToAddress("0x1234"), Label: "execute", BeforeBroadcast: func(_, _ *types.Transaction) error { return want }})
	if !errors.Is(got.Err, want) {
		t.Fatalf("missing persistence failure: %+v", got)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sendCalls != 0 {
		t.Fatal("transaction broadcast before durable persistence")
	}
}

func TestPersistenceHookCoversFeeReplacement(t *testing.T) {
	b := &replacementBackend{mockBackend: newMockBackend(), receiptOnSameNonce: 2}
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{PollInterval: time.Millisecond, ReplacementInterval: 2 * time.Millisecond, PendingTimeout: time.Second, Horizon: HorizonConfig{BlockTime: 4 * time.Millisecond}}, logr.Discard())
	startManagerForTest(t, m)
	persisted := make(chan common.Hash, 16)
	result, accepted := m.SendAsync(t.Context(), Request{To: common.HexToAddress("0xabc"), Data: []byte{1}, GasLimit: 21000, Label: "replace", BeforeBroadcast: func(_, signed *types.Transaction) error { persisted <- signed.Hash(); return nil }})
	if !accepted {
		t.Fatal("not admitted")
	}
	waitForSentTransactions(t, b.mockBackend, 1)
	b.mine(gweiToWei(33))
	select {
	case got := <-result:
		if got.Err != nil {
			t.Fatal(got.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("replacement did not finish")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.sent) < 2 {
		t.Fatal("replacement missing")
	}
	for _, tx := range b.sent {
		select {
		case hash := <-persisted:
			if hash != tx.Hash() {
				t.Fatal("broadcast differs from persisted attempt")
			}
		default:
			t.Fatal("broadcast preceded persistence")
		}
	}
}

func TestReservationLinearizesAfterQueuedAdmission(t *testing.T) {
	b := newMockBackend()
	m := New(b, mustSigner(t), big.NewInt(1), Config{}, logr.Discard())
	accepted := make(chan struct{})
	go func() {
		_, _ = m.SendAsync(t.Context(), Request{To: common.HexToAddress("0xabc"), Label: "already-waiting"})
		close(accepted)
	}()
	deadline := time.Now().Add(time.Second)
	for len(m.lifecycleSlot) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("admission did not acquire lane")
		}
		time.Sleep(time.Millisecond)
	}
	held := make(chan error, 1)
	go func() { held <- m.Hold("journal") }()
	select {
	case err := <-held:
		t.Fatalf("reservation returned before actual queue acceptance: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	startManagerForTest(t, m)
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("existing admission did not finish")
	}
	select {
	case err := <-held:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("reservation never completed")
	}
}

func TestDurableReservationPausesForeignQuoteAvailability(t *testing.T) {
	m := newTestManager(t, newMockBackend())
	if err := m.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !m.Available() {
		t.Fatal("initialized sender unavailable before reservation")
	}
	if err := m.Hold("merkle-job"); err != nil {
		t.Fatal(err)
	}
	if m.Available() {
		t.Fatal("RFQ and UniswapX quote availability ignores durable owner")
	}
	if err := m.Release("merkle-job"); err != nil {
		t.Fatal(err)
	}
	if !m.Available() {
		t.Fatal("released sender never resumes quoting")
	}
}
