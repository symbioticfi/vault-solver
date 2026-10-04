package txmanager

import (
	"context"
	"io"
	"math/big"
	"testing"
	"testing/synctest"
	"time"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/signer"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
)

type abandonedPoolBackend struct{ *mockBackend }

func (b *abandonedPoolBackend) SendTransaction(_ context.Context, tx *types.Transaction) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent = append(b.sent, tx)
	b.pendingNonce = tx.Nonce() + 1
	return nil
}

func (b *abandonedPoolBackend) TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error) {
	return nil, ethereum.NotFound
}

func TestPendingTimeoutReusesNonceForFreshBusinessRequest(t *testing.T) {
	b := &abandonedPoolBackend{newMockBackend()}
	m := New(b, mustSigner(t), big.NewInt(1), Config{
		PendingTimeout: 25 * time.Millisecond, PollInterval: time.Millisecond,
		ReplacementInterval: time.Hour, ShutdownTimeout: time.Millisecond,
	}, logr.Discard())
	startManagerForTest(t, m)
	first := m.Send(t.Context(), Request{To: common.HexToAddress("0xaaa"), Data: []byte{1}, GasLimit: 50_000})
	if first.Outcome != Outcome("abandoned") || first.Receipt != nil || first.Hash == (common.Hash{}) {
		t.Fatalf("expired attempt result = %+v, want uncertain abandoned identity", first)
	}
	b.mu.Lock()
	if len(b.sent) != 1 {
		t.Fatalf("abandonment sent %d transactions, want original only", len(b.sent))
	}
	original := b.sent[0]
	b.mu.Unlock()
	fresh := m.Send(t.Context(), Request{To: common.HexToAddress("0xbbb"), Data: []byte{2, 3}, Value: big.NewInt(4), GasLimit: 60_000})
	if fresh.Outcome != Outcome("abandoned") {
		t.Fatalf("fresh result = %+v", fresh)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.sent) != 2 {
		t.Fatalf("sent %d transactions, want two business requests", len(b.sent))
	}
	next := b.sent[1]
	if next.Nonce() != original.Nonce() || next.To() == nil || *next.To() != common.HexToAddress("0xbbb") || next.Value().Cmp(big.NewInt(4)) != 0 || next.Gas() != 60_000 || string(next.Data()) != string([]byte{2, 3}) {
		t.Fatalf("fresh order did not replace old nonce with fresh payload: %v", next)
	}
	if next.GasFeeCap().Cmp(bumpFee(original.GasFeeCap())) < 0 || next.GasTipCap().Cmp(bumpFee(original.GasTipCap())) < 0 {
		t.Fatal("fresh replacement did not honor both replacement fee floors")
	}
}

func TestAbandonmentNeverSendsAdditionalTransaction(t *testing.T) {
	for _, reason := range []string{"deadline", "obsolete", "pending timeout"} {
		t.Run(reason, func(t *testing.T) {
			b := &abandonedPoolBackend{newMockBackend()}
			m := New(b, mustSigner(t), big.NewInt(1), Config{PendingTimeout: 20 * time.Millisecond, PollInterval: time.Millisecond, ReplacementInterval: time.Hour}, logr.Discard())
			req := Request{To: common.HexToAddress("0xabc"), Data: []byte{9}, GasLimit: 50_000}
			if reason == "deadline" {
				req.Deadline = time.Now().Add(20 * time.Millisecond)
				m.cfg.PendingTimeout = time.Hour
			}
			pending, err := m.broadcast(t.Context(), req)
			if err != nil {
				t.Fatal(err)
			}
			if reason == "obsolete" {
				pending.req.Obsolete = func(context.Context) (bool, error) { return true, nil }
			}
			m.trackUnminedTransaction(pending)
			result := m.waitForPendingTransaction(t.Context(), pending)
			if result.Outcome != OutcomeAbandoned || !errors.Is(result.Err, ErrAbandoned) || !result.Outcome.NonceUncertain() || result.Outcome.Included() || result.Receipt != nil {
				t.Fatalf("abandoned result = %+v", result)
			}
			if reason == "obsolete" && !errors.Is(result.Err, ErrRequestObsolete) {
				t.Fatalf("obsolete cause missing: %v", result.Err)
			}
			if len(b.sent) != 1 || m.reusable == nil || m.reusable.nonce != 7 {
				t.Fatalf("sends=%d reusable=%+v", len(b.sent), m.reusable)
			}
		})
	}
}

func TestReusableNonceRetainsHintAcrossPreparationFailures(t *testing.T) {
	for _, failure := range []string{"fee cap", "obsolete", "signer", "rejected", "nonce conflict", "nonce RPC error", "nonce RPC deadline"} {
		t.Run(failure, func(t *testing.T) {
			b := &silentAcceptanceBackend{mockBackend: newMockBackend()}
			m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, ReplacementInterval: 40 * time.Millisecond, PollInterval: time.Millisecond, Horizon: HorizonConfig{BlockTime: time.Millisecond}}, logr.Discard())
			first, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xaaa"), GasLimit: 50_000})
			if err != nil {
				t.Fatal(err)
			}
			m.abandonPending(t.Context(), first, "pending_timeout", nil)
			b.pendingNonce = 8
			request := Request{To: common.HexToAddress("0xbbb"), Data: []byte{4}, GasLimit: 55_000}
			originalSigner := m.signer
			switch failure {
			case "fee cap":
				request.MaxFeePerGas = big.NewInt(22_000_000_000)
			case "obsolete":
				request.Obsolete = func(context.Context) (bool, error) { return true, nil }
			case "signer":
				m.signer = &alwaysFailSigner{Signer: m.signer}
			case "nonce RPC error":
				b.nonceErr = io.ErrUnexpectedEOF
			case "nonce RPC deadline":
				b.blockNonce = true
			case "rejected", "nonce conflict":
				// A definite validation rejection cannot enter the relay; an underpriced candidate
				// reveals contention but cannot establish accepted fees for the next floor.
				b.sendCalls = 1
				b.sendErrs = []error{nil, errors.New("insufficient funds")}
				if failure == "nonce conflict" {
					b.sendErrs[1] = errors.New("replacement transaction underpriced")
				}
				m.backend = b.mockBackend
				delete(b.receipts, first.originalHash)
			}
			pending, fail := m.broadcast(t.Context(), request)
			if failure == "nonce conflict" {
				if fail != nil || pending == nil || pending.broadcastErr == nil {
					t.Fatalf("expected nonce conflict, got %+v %v", pending, fail)
				}
			} else if fail == nil {
				t.Fatal("preparation unexpectedly succeeded")
			}
			if m.reusable == nil || m.reusable.nonce != first.nonce {
				t.Fatal("preparation failure discarded reusable nonce")
			}
			request.MaxFeePerGas, request.Obsolete = nil, nil
			m.signer = originalSigner
			b.nonceErr, b.blockNonce = nil, false
			b.sendErrs = nil
			m.backend = b
			fresh, err := m.broadcast(t.Context(), request)
			if err != nil || fresh == nil || fresh.nonce != first.nonce {
				t.Fatalf("retry failed to reuse original gap: %+v %v", fresh, err)
			}
			if fresh.attempts[0].tx.GasFeeCap().Cmp(bumpFee(first.fees.maxFee)) < 0 || fresh.attempts[0].tx.GasTipCap().Cmp(bumpFee(first.fees.tip)) < 0 {
				t.Fatal("fresh candidate lost original fee floor")
			}
			if failure == "nonce conflict" && fresh.attempts[0].tx.GasFeeCap().Cmp(pending.fees.maxFee) != 0 {
				t.Fatal("rejected candidate ratcheted the accepted fee hint")
			}
		})
	}
}

type alwaysFailSigner struct{ signer.Signer }

func (*alwaysFailSigner) SignTx(context.Context, *types.Transaction, *big.Int) (*types.Transaction, error) {
	return nil, io.ErrUnexpectedEOF
}

func TestConsumedAbandonedNonceUsesFreshMinedNonce(t *testing.T) {
	b := &silentAcceptanceBackend{mockBackend: newMockBackend()}
	m := New(b, mustSigner(t), big.NewInt(1), Config{}, logr.Discard())
	first, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xaaa"), GasLimit: 50_000})
	if err != nil {
		t.Fatal(err)
	}
	m.abandonPending(t.Context(), first, "pending_timeout", nil)
	b.latestNonce, b.pendingNonce = 8, 11
	next, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xbbb"), GasLimit: 60_000})
	if err != nil || next == nil || next.nonce != 8 || m.reusable != nil {
		t.Fatalf("consumed hint was reused: next=%+v err=%v hint=%+v", next, err, m.reusable)
	}
}

func TestCanonicalReceiptWinsAtAbandonmentDeadline(t *testing.T) {
	b := newMockBackend()
	m := New(b, mustSigner(t), big.NewInt(1), Config{}, logr.Discard())
	pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xaaa"), GasLimit: 50_000})
	if err != nil {
		t.Fatal(err)
	}
	m.trackUnminedTransaction(pending)
	pending.deadline = time.Now().Add(-time.Second)
	pending.req.Obsolete = func(context.Context) (bool, error) { return true, nil }
	result := m.waitForPendingTransaction(t.Context(), pending)
	if result.Outcome != OutcomeConfirmed || result.Err != nil || m.reusable != nil || len(b.sent) != 1 {
		t.Fatalf("deadline masked canonical receipt: %+v hint=%+v sends=%d", result, m.reusable, len(b.sent))
	}
}

func TestShutdownDrainsWithoutSendingCancellation(t *testing.T) {
	b := &abandonedPoolBackend{newMockBackend()}
	m := New(b, mustSigner(t), big.NewInt(1), Config{PollInterval: time.Millisecond, ReplacementInterval: time.Hour, PendingTimeout: time.Hour, ShutdownTimeout: 20 * time.Millisecond}, logr.Discard())
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() { m.Start(ctx); close(stopped) }()
	result, accepted := m.SendAsync(t.Context(), Request{To: common.HexToAddress("0xabc"), Data: []byte{1}, GasLimit: 50_000})
	if !accepted {
		t.Fatal("not admitted")
	}
	deadline := time.Now().Add(time.Second)
	for b.lastSent() == nil {
		if time.Now().After(deadline) {
			t.Fatal("no initial transaction")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	completed := <-result
	if completed.Outcome != OutcomeTrackingStopped || !errors.Is(completed.Err, errShutdownTimeout) || completed.Hash == (common.Hash{}) {
		t.Fatalf("shutdown result=%+v", completed)
	}
	<-stopped
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.sent) != 1 {
		t.Fatalf("shutdown sent %d transactions, want initial business call only", len(b.sent))
	}
}

func TestMaxFeePerGasIncludesReusableNonceReplacementFloor(t *testing.T) {
	b := &silentAcceptanceBackend{mockBackend: newMockBackend()}
	m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, logr.Discard())
	previous := feeQuote{baseFee: gweiToWei(20), tip: gweiToWei(4), maxFee: gweiToWei(60)}
	m.rememberReusable(7, previous)
	b.pendingNonce = 8
	feeLimit, err := m.MaxFeePerGas(t.Context())
	if err != nil || feeLimit.Cmp(bumpFee(previous.maxFee)) < 0 {
		t.Fatalf("fresh planning feeLimit=%v err=%v, want retained replacement floor >=%v", feeLimit, err, bumpFee(previous.maxFee))
	}
	pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 50_000, MaxFeePerGas: feeLimit})
	if err != nil || pending == nil || pending.nonce != 7 {
		t.Fatalf("quoted fresh request cannot use retained nonce: %+v %v", pending, err)
	}
	if pending.fees.maxFee.Cmp(feeLimit) > 0 || pending.fees.tip.Cmp(bumpFee(previous.tip)) < 0 {
		t.Fatalf("fresh floor exceeded priced feeLimit or lost tip floor: %+v feeLimit=%v", pending.fees, feeLimit)
	}
}

func TestMaxFeePerGasChecksWhetherRememberedNonceIsUnused(t *testing.T) {
	for _, state := range []string{"consumed", "RPC error", "RPC deadline", "global feeLimit"} {
		t.Run(state, func(t *testing.T) {
			b := &silentAcceptanceBackend{mockBackend: newMockBackend()}
			m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, ReplacementInterval: 40 * time.Millisecond, PollInterval: time.Millisecond, Horizon: HorizonConfig{BlockTime: time.Millisecond}}, logr.Discard())
			m.rememberReusable(7, feeQuote{baseFee: gweiToWei(20), tip: gweiToWei(4), maxFee: gweiToWei(60)})
			var want error
			switch state {
			case "consumed":
				b.latestNonce = 8
			case "RPC error":
				b.nonceErr = io.ErrUnexpectedEOF
				want = io.ErrUnexpectedEOF
			case "RPC deadline":
				b.blockNonce = true
				want = context.DeadlineExceeded
			case "global feeLimit":
				m.cfg.MaxFeeGwei = 65
			}
			feeLimit, err := m.MaxFeePerGas(t.Context())
			if state == "consumed" || state == "global feeLimit" {
				if err != nil || m.reusable != nil || feeLimit.Cmp(gweiToWei(60)) >= 0 {
					t.Fatalf("inapplicable hint blocked fresh market pricing: feeLimit=%v err=%v hint=%+v", feeLimit, err, m.reusable)
				}
				return
			}
			if !errors.Is(err, want) || feeLimit != nil || m.reusable == nil {
				t.Fatalf("uncertain quote feeLimit=%v err=%v hint=%+v want=%v", feeLimit, err, m.reusable, want)
			}
		})
	}
}

func TestReplacementFloorRespectsBothFeeFieldsAndCaps(t *testing.T) {
	for _, tc := range []struct {
		name      string
		feeLimit  int64
		marketFee int64
		marketTip int64
		wantFee   int64
		wantTip   int64
		reject    bool
	}{
		{name: "minimum bump", feeLimit: 100, marketFee: 30, marketTip: 2, wantFee: 45, wantTip: 9},
		{name: "fresh market is higher", feeLimit: 100, marketFee: 70, marketTip: 20, wantFee: 70, wantTip: 20},
		{name: "exact feeLimit", feeLimit: 45, marketFee: 30, marketTip: 2, wantFee: 45, wantTip: 9},
		{name: "feeLimit cannot replace", feeLimit: 44, marketFee: 30, marketTip: 2, reject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := feeQuote{baseFee: big.NewInt(20), tip: big.NewInt(tc.marketTip), maxFee: big.NewInt(tc.marketFee)}
			previous := feeQuote{baseFee: big.NewInt(20), tip: big.NewInt(8), maxFee: big.NewInt(40)}
			got, err := replacementFloor(current, previous, big.NewInt(tc.feeLimit))
			if tc.reject {
				if !errors.Is(err, errReplacementLimitReached) {
					t.Fatalf("feeLimit violation err=%v", err)
				}
				return
			}
			if err != nil || got.tip.Cmp(big.NewInt(tc.wantTip)) != 0 || got.maxFee.Cmp(big.NewInt(tc.wantFee)) != 0 {
				t.Fatalf("floor=%+v err=%v", got, err)
			}
		})
	}
}

type blockedAbandonmentReceipts struct{ *mockBackend }

func (*blockedAbandonmentReceipts) TransactionReceipt(ctx context.Context, _ common.Hash) (*types.Receipt, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestAbandonmentGraceDoesNotGrowWithReceiptAttemptCount(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := &blockedAbandonmentReceipts{newMockBackend()}
		m := New(b, mustSigner(t), big.NewInt(1), Config{PendingTimeout: time.Second, PollInterval: time.Second, ReplacementInterval: time.Hour}, logr.Discard())
		pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 50_000})
		if err != nil {
			t.Fatal(err)
		}
		for i := 1; i < 52; i++ {
			pending.attempts = append(pending.attempts, txAttempt{hash: common.BigToHash(big.NewInt(int64(i)))})
		}
		m.trackUnminedTransaction(pending)
		started := time.Now()
		result := m.waitForPendingTransaction(t.Context(), pending)
		if result.Outcome != OutcomeAbandoned || time.Since(started) > m.cfg.PendingTimeout+m.receiptReadTimeout() {
			t.Fatalf("abandonment elapsed=%s result=%+v, want bounded final receipt grace", time.Since(started), result)
		}
		if len(b.sent) != 1 || m.reusable == nil {
			t.Fatalf("abandonment sends=%d hint=%+v", len(b.sent), m.reusable)
		}
	})
}

func TestPersistentOrphanedReceiptCannotPreventAbandonment(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newMockBackend()
		m := New(b, mustSigner(t), big.NewInt(1), Config{PendingTimeout: time.Second, PollInterval: 100 * time.Millisecond, ReplacementInterval: time.Hour}, logr.Discard())
		pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 50_000})
		if err != nil {
			t.Fatal(err)
		}
		b.reorgedHeader = true
		m.trackUnminedTransaction(pending)
		started := time.Now()
		ctx, cancel := context.WithTimeout(t.Context(), m.cfg.PendingTimeout+m.receiptReadTimeout()+100*time.Millisecond)
		defer cancel()
		result := m.waitForPendingTransaction(ctx, pending)
		if result.Outcome != OutcomeAbandoned || time.Since(started) > m.cfg.PendingTimeout+m.receiptReadTimeout() {
			t.Fatalf("orphaned receipt prevented bounded abandonment: elapsed=%s result=%+v", time.Since(started), result)
		}
		if len(b.sent) != 1 || m.reusable == nil {
			t.Fatalf("abandonment sends=%d hint=%+v", len(b.sent), m.reusable)
		}
	})
}

func TestCanonicalReceiptArrivingWithinFinalGraceWins(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := &slowReceiptBackend{mockBackend: newMockBackend(), delay: 1500 * time.Millisecond}
		m := New(b, mustSigner(t), big.NewInt(1), Config{PendingTimeout: time.Second, PollInterval: time.Second, ReplacementInterval: time.Hour}, logr.Discard())
		pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 50_000})
		if err != nil {
			t.Fatal(err)
		}
		m.trackUnminedTransaction(pending)
		result := m.waitForPendingTransaction(t.Context(), pending)
		if result.Outcome != OutcomeConfirmed || result.Err != nil || m.reusable != nil || len(b.sent) != 1 {
			t.Fatalf("available canonical receipt lost to deadline: %+v hint=%+v sends=%d", result, m.reusable, len(b.sent))
		}
	})
}

func TestPositiveDepthOrphanedReceiptCannotHoldExpiredLane(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newMockBackend()
		m := New(b, mustSigner(t), big.NewInt(1), Config{Confirmations: 2, PendingTimeout: time.Second, PollInterval: 100 * time.Millisecond, ReplacementInterval: time.Hour}, logr.Discard())
		pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 50_000})
		if err != nil {
			t.Fatal(err)
		}
		b.reorgedHeader = true // receipt has the right shape but belongs to the displaced fork.
		m.trackUnminedTransaction(pending)
		ctx, cancel := context.WithTimeout(t.Context(), m.cfg.PendingTimeout+m.receiptReadTimeout()+100*time.Millisecond)
		defer cancel()
		started := time.Now()
		result := m.waitForPendingTransaction(ctx, pending)
		if result.Outcome != OutcomeAbandoned || time.Since(started) > m.cfg.PendingTimeout+m.receiptReadTimeout() {
			t.Fatalf("unproven positive-depth receipt held lane: elapsed=%s result=%+v", time.Since(started), result)
		}
		if m.reusable == nil || len(b.sent) != 1 {
			t.Fatalf("abandonment hint=%+v sends=%d", m.reusable, len(b.sent))
		}
	})
}

func TestCanonicalPositiveDepthReceiptRetainsConfirmationWaitPastDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newMockBackend()
		m := New(b, mustSigner(t), big.NewInt(1), Config{Confirmations: 2, PendingTimeout: time.Second, PollInterval: 100 * time.Millisecond, ReplacementInterval: time.Hour}, logr.Discard())
		pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 50_000})
		if err != nil {
			t.Fatal(err)
		}
		m.trackUnminedTransaction(pending)
		result := make(chan Result, 1)
		go func() { result <- m.waitForPendingTransaction(t.Context(), pending) }()
		synctest.Wait()
		time.Sleep(4 * time.Second) // beyond both the pending deadline and final receipt grace.
		synctest.Wait()
		select {
		case got := <-result:
			t.Fatalf("canonical inclusion abandoned before confirmations: %+v", got)
		default:
		}
		b.mu.Lock()
		b.head += 2
		b.mu.Unlock()
		got := <-result
		if got.Outcome != OutcomeConfirmed || got.Err != nil || m.reusable != nil || len(b.sent) != 1 {
			t.Fatalf("canonical wait result=%+v hint=%+v sends=%d", got, m.reusable, len(b.sent))
		}
	})
}
