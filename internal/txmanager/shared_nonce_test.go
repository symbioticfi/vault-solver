package txmanager

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

type hashNonceTestBackend struct {
	*mockBackend

	confirmedNonce uint64
	hashNonceErr   error
	queriedHash    common.Hash
	orphanAncestor bool
	blockHashRead  bool
	enteredHash    chan struct{}
}

func (b *hashNonceTestBackend) ReadNonceAtHash(ctx context.Context, _ common.Address, hash common.Hash) (uint64, error) {
	b.mu.Lock()
	b.queriedHash = hash
	nonce, err, blocked := b.confirmedNonce, b.hashNonceErr, b.blockHashRead
	entered := b.enteredHash
	b.mu.Unlock()
	if entered != nil {
		select {
		case entered <- struct{}{}:
		default:
		}
	}
	if blocked {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	return nonce, err
}

func (b *hashNonceTestBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	header, err := b.mockBackend.HeaderByNumber(ctx, number)
	b.mu.Lock()
	orphan := b.orphanAncestor
	b.mu.Unlock()
	if number != nil && orphan {
		return forkedReceiptHeader(number.Uint64(), "orphan"), nil
	}
	return header, err
}

func sharedNonceManager(t *testing.T, b Backend, confirmations uint64) *Manager {
	t.Helper()
	return New(b, mustSigner(t), big.NewInt(1), Config{
		ReconcileNonces: true, Confirmations: confirmations, MaxFeeGwei: 100,
		PollInterval: time.Millisecond, ReplacementInterval: time.Second,
	}, logr.Discard())
}

func TestReconcileStartupWaitsForPendingAndUnconfirmedNonce(t *testing.T) {
	for _, state := range []string{"pending", "mined unconfirmed"} {
		t.Run(state, func(t *testing.T) {
			b := &hashNonceTestBackend{mockBackend: newMockBackend(), confirmedNonce: 7}
			if state == "pending" {
				b.pendingNonce = 8
			} else {
				b.latestNonce, b.pendingNonce = 8, 8
			}
			m := sharedNonceManager(t, b, 2)
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer cancel()
			if err := m.Initialize(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("initialization must wait for confirmed empty lane, got %v", err)
			}
			if m.Available() || len(b.attemptedTransactions()) != 0 {
				t.Fatal("unresolved startup exposed readiness or signed work")
			}
			b.mu.Lock()
			b.latestNonce, b.pendingNonce, b.confirmedNonce = 8, 8, 8
			b.mu.Unlock()
			if err := m.Initialize(t.Context()); err != nil || !m.Available() {
				t.Fatalf("confirmed nonce did not restore initialization: %v", err)
			}
		})
	}
}

func TestReconcileAdmissionUsesFreshNonceAndDeclinesUnknownPending(t *testing.T) {
	b := &hashNonceTestBackend{mockBackend: newMockBackend(), confirmedNonce: 7}
	m := sharedNonceManager(t, b, 1)
	if err := m.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	b.latestNonce, b.pendingNonce, b.confirmedNonce = 8, 8, 8
	fresh, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0x123")})
	if err != nil || fresh.nonce != 8 {
		t.Fatalf("fresh admission reused cached nonce: %+v, %v", fresh, err)
	}
	b.latestNonce, b.pendingNonce, b.confirmedNonce = 9, 10, 9
	if pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0x123")}); pending != nil || !errors.Is(err, errNonceLanePaused) {
		t.Fatalf("unknown pending admission = %+v, %v", pending, err)
	}
	if len(b.attemptedTransactions()) != 1 {
		t.Fatal("unknown pending transaction was replaced")
	}
	b.pendingNonce = 9
	if _, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0x123")}); err == nil {
		t.Fatal("dropped pool entry was mistaken for proof old signed bytes cannot land")
	}
	b.latestNonce, b.pendingNonce, b.confirmedNonce = 10, 10, 10
	pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0x123")})
	if err != nil || pending.nonce != 10 {
		t.Fatalf("confirmed consumption did not admit fresh work: %+v, %v", pending, err)
	}
}

func TestReconcileInitialCollisionTracksThenReportsConsumedNonce(t *testing.T) {
	for _, collision := range []string{"nonce too low", "replacement transaction underpriced"} {
		t.Run(collision, func(t *testing.T) {
			b := &hashNonceTestBackend{mockBackend: newMockBackend(), confirmedNonce: 7}
			b.sendErrs = []error{errors.New(collision)}
			m := sharedNonceManager(t, b, 2)
			pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0x123")})
			if err != nil || pending == nil {
				t.Fatalf("collision discarded signed candidate: %+v %v", pending, err)
			}
			m.trackUnminedTransaction(pending)
			if _, err := m.tryReplace(t.Context(), pending, replaceIntent{cancellation: true}); err != nil {
				t.Fatal(err)
			}
			b.mine(big.NewInt(20e9))
			m.evaluateHorizon(t.Context(), pending, false, func(intent replaceIntent) bool {
				cancelling, _ := m.tryReplace(t.Context(), pending, intent)
				return cancelling
			})
			b.latestNonce, b.pendingNonce, b.confirmedNonce = 8, 8, 8
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			result := m.waitForPendingTransaction(ctx, pending)
			if result.Outcome != OutcomeNonceConsumed || !errors.Is(result.Err, ErrNonceConsumed) || result.Receipt != nil || result.Outcome.Included() || result.Hash != pending.originalHash {
				t.Fatalf("nonce consumption fabricated request success: %+v", result)
			}
			if len(b.attemptedTransactions()) != 1 || !m.Available() {
				t.Fatal("collision replayed business/cancellation bytes or remained paused")
			}
		})
	}
}

func TestReconcileCanonicalProofRejectsUnstableOrUnavailableState(t *testing.T) {
	for _, scenario := range []string{"stable", "changing head", "orphan ancestor", "lookup error", "lookup deadline", "insufficient history"} {
		t.Run(scenario, func(t *testing.T) {
			b := &hashNonceTestBackend{mockBackend: newMockBackend(), confirmedNonce: 8}
			m := sharedNonceManager(t, b, 2)
			switch scenario {
			case "changing head":
				b.latestHeads = []uint64{100, 101}
			case "orphan ancestor":
				b.orphanAncestor = true
			case "lookup error":
				b.hashNonceErr = errors.New("historical account state unavailable")
			case "lookup deadline":
				b.blockHashRead = true
				m.cfg.ReplacementInterval = 10 * time.Millisecond
			case "insufficient history":
				b.head = 1
			}
			nonce, err := m.canonicalAccountNonce(t.Context(), 2)
			if scenario == "stable" {
				if err != nil || nonce != 8 || b.queriedHash != receiptTestHeader(98).Hash() {
					t.Fatalf("canonical ancestor proof: nonce=%d hash=%s err=%v", nonce, b.queriedHash, err)
				}
			} else if err == nil {
				t.Fatal("invalid canonical proof was accepted")
			}
		})
	}
}

func TestReconcileMissingCapabilityFailsClosed(t *testing.T) {
	b := newMockBackend()
	m := sharedNonceManager(t, b, 0)
	if err := m.Initialize(t.Context()); err == nil || m.Available() {
		t.Fatal("unsupported backend admitted reconciliation mode")
	}
	if _, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0x123")}); err == nil || len(b.attemptedTransactions()) != 0 {
		t.Fatal("unsupported backend signed work")
	}
}

func TestReconcileRuntimeRetainsGateUntilEffectiveConfirmationProof(t *testing.T) {
	for _, scenario := range []string{"unconfirmed", "orphan", "lookup error"} {
		t.Run(scenario, func(t *testing.T) {
			b := &hashNonceTestBackend{mockBackend: newMockBackend(), confirmedNonce: 7}
			b.sendErrs = []error{errors.New("replacement transaction underpriced")}
			m := sharedNonceManager(t, b, 2)
			confirmations := uint64(3)
			pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0x123"), Confirmations: &confirmations})
			if err != nil {
				t.Fatal(err)
			}
			b.latestNonce, b.pendingNonce = 8, 8
			if scenario != "unconfirmed" {
				b.confirmedNonce = 8
			}
			if scenario == "orphan" {
				b.orphanAncestor = true
			}
			if scenario == "lookup error" {
				b.hashNonceErr = errors.New("missing account state")
			}
			if result, consumed := m.confirmConsumedNonce(t.Context(), pending); consumed || m.Available() {
				t.Fatalf("unproved consumption resumed lane: %+v", result)
			}
			b.confirmedNonce, b.orphanAncestor, b.hashNonceErr = 8, false, nil
			result, consumed := m.confirmConsumedNonce(t.Context(), pending)
			if !consumed || result.Outcome != OutcomeNonceConsumed || result.Receipt != nil || !m.Available() {
				t.Fatalf("valid consumption did not restore lane: %+v", result)
			}
			// The subsequent global admission guard reads depth two, but the lifecycle proof used three.
			b.confirmedNonce = 7
			if _, againConsumed := m.confirmConsumedNonce(t.Context(), pending); againConsumed || b.queriedHash != receiptTestHeader(97).Hash() {
				t.Fatal("request confirmation override was lost")
			}
		})
	}
}

func TestReconcileIdleProbeReservesAdmissionUntilRPCCompletes(t *testing.T) {
	b := &hashNonceTestBackend{mockBackend: newMockBackend(), confirmedNonce: 7}
	m := sharedNonceManager(t, b, 0)
	if err := m.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	b.blockHashRead, b.enteredHash = true, make(chan struct{}, 1)
	startManagerForTest(t, m)
	select {
	case <-b.enteredHash:
	case <-time.After(time.Second):
		t.Fatal("idle proof did not enter the blocked RPC")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, accepted := m.SendAsync(ctx, Request{To: common.HexToAddress("0x123")}); accepted {
		t.Fatal("idle proof raced admission into classifying its own pending bytes as unknown")
	}
	if len(b.attemptedTransactions()) != 0 {
		t.Fatal("initial work was signed while idle proof was in flight")
	}
}

func TestReconcileSweepCannotSkipIntermediateOwnedReceipt(t *testing.T) {
	b := &hashNonceTestBackend{mockBackend: newMockBackend(), confirmedNonce: 7}
	b.sendErrs = []error{errors.New("transport timeout")}
	m := sharedNonceManager(t, b, 0)
	pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0x123")})
	if err != nil {
		t.Fatal(err)
	}
	sweep := newReceiptSweep(pending, 1)
	sweep.dispatched(pending, 0)
	m.observeReceiptRead(t.Context(), pending, sweep, receiptRead{attempt: pending.attempts[0], err: ethereum.NotFound})
	for _, fee := range []int64{40e9, 50e9} {
		to := common.HexToAddress("0x123")
		tx, signErr := m.signer.SignTx(t.Context(), types.NewTx(&types.DynamicFeeTx{
			ChainID: m.chainID, Nonce: pending.nonce, To: &to, Gas: 21_000,
			GasFeeCap: big.NewInt(fee), GasTipCap: big.NewInt(2e9),
		}), m.chainID)
		if signErr != nil {
			t.Fatal(signErr)
		}
		pending.attempts = append(pending.attempts, txAttempt{hash: tx.Hash(), tx: tx})
	}
	intermediate := pending.attempts[1]
	b.receipts[intermediate.hash] = successfulReceipt(intermediate.tx, b.head)
	latest := sweep.nextIndex(pending)
	if latest != 2 {
		t.Fatalf("priority read index = %d", latest)
	}
	sweep.dispatched(pending, latest)
	m.observeReceiptRead(t.Context(), pending, sweep, receiptRead{attempt: pending.attempts[latest], err: ethereum.NotFound})
	if sweep.nextIndex(pending) >= 0 || sweep.allAttemptsMissing(pending) {
		t.Fatal("priority sweep inferred absence without reading the intermediate signed hash")
	}
	result, done := m.receiptResult(t.Context(), pending)
	if !done || result.Outcome != OutcomeConfirmed || result.Hash != intermediate.hash {
		t.Fatalf("next complete sweep lost owned intermediate receipt: %+v", result)
	}
}

func TestReconcileOwnedReceiptPrecedesNonceOnlyResult(t *testing.T) {
	b := &hashNonceTestBackend{mockBackend: newMockBackend(), confirmedNonce: 7}
	m := sharedNonceManager(t, b, 1)
	pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0x123")})
	if err != nil {
		t.Fatal(err)
	}
	m.trackUnminedTransaction(pending)
	b.latestNonce, b.pendingNonce, b.confirmedNonce, b.head = 8, 8, 8, 101
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result := m.waitForPendingTransaction(ctx, pending)
	if result.Outcome != OutcomeConfirmed || result.Receipt == nil || result.Hash != pending.originalHash {
		t.Fatalf("owned receipt lost priority: %+v", result)
	}
}

// Publishing a receipt during the account-state proof models receipt and nonce RPCs straddling
// inclusion. The prior NotFound sweep cannot establish which transaction consumed the nonce.
type receiptDuringNonceProofBackend struct {
	*hashNonceTestBackend

	publishReceipt *types.Transaction
	missingReads   int
}

func (b *receiptDuringNonceProofBackend) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	receipt, err := b.mockBackend.TransactionReceipt(ctx, hash)
	if errors.Is(err, ethereum.NotFound) {
		b.mu.Lock()
		b.missingReads++
		b.mu.Unlock()
	}
	return receipt, err
}

func (b *receiptDuringNonceProofBackend) ReadNonceAtHash(ctx context.Context, account common.Address, hash common.Hash) (uint64, error) {
	nonce, err := b.hashNonceTestBackend.ReadNonceAtHash(ctx, account, hash)
	b.mu.Lock()
	if b.publishReceipt != nil {
		b.receipts[b.publishReceipt.Hash()] = successfulReceipt(b.publishReceipt, b.head-1)
	}
	b.mu.Unlock()
	return nonce, err
}

func TestReconcileOwnedReceiptAppearingDuringNonceProofRemainsUnknown(t *testing.T) {
	b := &receiptDuringNonceProofBackend{hashNonceTestBackend: &hashNonceTestBackend{mockBackend: newMockBackend(), confirmedNonce: 7}}
	m := sharedNonceManager(t, b, 1)
	pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0x123")})
	if err != nil {
		t.Fatal(err)
	}
	m.trackUnminedTransaction(pending)
	delete(b.receipts, pending.originalHash)
	b.latestNonce, b.pendingNonce, b.confirmedNonce, b.head = 8, 8, 8, 101
	b.publishReceipt = pending.attempts[0].tx
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result := m.waitForPendingTransaction(ctx, pending)
	if result.Outcome != OutcomeNonceConsumed || result.Receipt != nil || result.Outcome.Included() || !errors.Is(result.Err, ErrNonceConsumed) || result.Hash != pending.originalHash {
		t.Fatalf("proof-time inclusion fabricated execution: %+v", result)
	}
	if b.missingReads != 1 || b.receipts[pending.originalHash] == nil || !m.Available() {
		t.Fatalf("receipt/proof boundary was not exercised: missing reads=%d, receipt=%v, available=%v", b.missingReads, b.receipts[pending.originalHash], m.Available())
	}
}

func TestReconcileIdleMonitorHealsUnsignedAdmissionPause(t *testing.T) {
	b := &hashNonceTestBackend{mockBackend: newMockBackend(), confirmedNonce: 7}
	m := sharedNonceManager(t, b, 0)
	if err := m.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	b.pendingNonce = 8
	if _, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0x123")}); err == nil {
		t.Fatal("busy admission did not pause")
	}
	startManagerForTest(t, m)
	b.mu.Lock()
	b.latestNonce, b.pendingNonce, b.confirmedNonce = 8, 8, 8
	b.mu.Unlock()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for !m.Available() {
		select {
		case <-deadline.C:
			t.Fatal("idle monitor never healed unsigned pause")
		case <-time.After(time.Millisecond):
		}
	}
	if len(b.attemptedTransactions()) != 0 {
		t.Fatal("idle healing signed or cancelled unknown work")
	}
}
