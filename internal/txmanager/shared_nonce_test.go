package txmanager

import (
	"context"
	"io"
	"math/big"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

type minedNonceErrorBackend struct {
	*mockBackend

	err   error
	block bool
}

func (b *minedNonceErrorBackend) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	if b.block {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	if b.err != nil {
		return 0, b.err
	}
	return b.mockBackend.NonceAt(ctx, account, blockNumber)
}

func TestFreshMinedNonceIgnoresOtherSendersPendingWork(t *testing.T) {
	b := newMockBackend()
	b.pendingNonce = 10 // Three foreign pending transactions must not skip the mined nonce.
	m := New(b, mustSigner(t), big.NewInt(1), Config{Confirmations: 2}, logr.Discard())
	if err := m.Initialize(t.Context()); err != nil || !m.LaneReady() {
		t.Fatalf("foreign pending work blocked initialization: %v", err)
	}
	for _, nonce := range []uint64{7, 11} {
		b.latestNonce, b.pendingNonce = nonce, nonce+3
		pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000})
		if err != nil || pending.nonce != nonce {
			t.Fatalf("fresh request nonce: pending=%+v err=%v, want %d", pending, err, nonce)
		}
	}
	if len(b.attemptedTransactions()) != 2 {
		t.Fatal("initialization sent a transaction for another sender's nonce")
	}
}

func TestInitialNonceRaceReleasesLaneForFreshOrder(t *testing.T) {
	for _, rpcError := range []string{"nonce too low", "nonce is too low", "nonce has already been used", "replacement transaction underpriced"} {
		t.Run(rpcError, func(t *testing.T) {
			b := newMockBackend()
			b.sendErrs = []error{errors.New(rpcError)}
			m := newTestManager(t, b)
			request := Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "raced"}
			result := m.Send(t.Context(), request)
			if result.Outcome != OutcomeNonceConflict || !errors.Is(result.Err, ErrNonceConflict) || result.Hash == (common.Hash{}) || result.Receipt != nil || result.NotAdmitted || result.Outcome.Included() {
				t.Fatalf("nonce race result: %+v", result)
			}
			waitForAdmissionDemand(t, m, 0)
			if !m.LaneReady() || b.sendCalls != 1 {
				t.Fatal("nonce race held the lane or attempted an obsolete replay")
			}
			b.mu.Lock()
			b.latestNonce, b.pendingNonce = 9, 11
			b.mu.Unlock()
			fresh := m.Send(t.Context(), Request{To: common.HexToAddress("0xdef"), GasLimit: 21_000, Label: "fresh"})
			if fresh.Outcome != OutcomeConfirmed || fresh.Err != nil || b.lastSent().Nonce() != 9 {
				t.Fatalf("next order did not use a fresh mined nonce: %+v", fresh)
			}
		})
	}
}

func TestMinedNonceReadFailureIsBoundedAndRetryable(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "timeout"}[blocked], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				b := &minedNonceErrorBackend{mockBackend: newMockBackend(), err: io.ErrUnexpectedEOF, block: blocked}
				m := New(b, mustSigner(t), big.NewInt(1), Config{}, logr.Discard())
				want := io.ErrUnexpectedEOF
				if blocked {
					want = context.DeadlineExceeded
				}
				started := time.Now()
				if err := m.Initialize(t.Context()); !errors.Is(err, want) || m.Available() {
					t.Fatalf("failed initialization: %v", err)
				}
				if elapsed := time.Since(started); elapsed > m.receiptReadTimeout() {
					t.Fatalf("nonce read exceeded budget: %s", elapsed)
				}
				b.err, b.block = nil, false
				if err := m.Initialize(t.Context()); err != nil || !m.Available() {
					t.Fatalf("nonce RPC recovery failed: %v", err)
				}
				b.err = io.ErrUnexpectedEOF
				if pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000}); pending != nil || !errors.Is(err, b.err) || len(b.attemptedTransactions()) != 0 {
					t.Fatalf("failed read signed bytes: pending=%+v err=%v", pending, err)
				}
				b.err = nil
				if pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xdef"), GasLimit: 21_000}); err != nil || pending.nonce != 7 {
					t.Fatalf("fresh send after RPC recovery: pending=%+v err=%v", pending, err)
				}
			})
		})
	}
}

func TestNonceUncertainNeverMeansIncluded(t *testing.T) {
	for _, outcome := range []Outcome{OutcomeConfirmed, OutcomeIncludedUnconfirmed, OutcomeReverted, OutcomeAbandoned, OutcomeSubmissionError, OutcomeNonceConflict, OutcomeNonceConsumed} {
		want := outcome == OutcomeNonceConflict || outcome == OutcomeNonceConsumed || outcome == OutcomeAbandoned
		if outcome.NonceUncertain() != want || (want && outcome.Included()) {
			t.Fatalf("outcome %s: uncertain=%v included=%v", outcome, outcome.NonceUncertain(), outcome.Included())
		}
	}
}

func TestRestartStartsAtFirstUnminedNonceDespiteOldPendingWork(t *testing.T) {
	b := &silentAcceptanceBackend{mockBackend: newMockBackend()}
	b.pendingNonce = 12 // stale, expired or foreign pending work must not skip unused nonce 7.
	m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, logr.Discard())
	pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xbeef"), Data: []byte{9}, GasLimit: 60_000})
	if err != nil || pending == nil || pending.nonce != 7 {
		t.Fatalf("restarted manager queued behind unused nonce: %+v err=%v", pending, err)
	}
}

func TestInitialUnderpricedCandidatesKeepFreshMarketFees(t *testing.T) {
	b := newMockBackend()
	b.pendingNonce = 9
	b.sendErrs = []error{errors.New("replacement transaction underpriced"), errors.New("replacement transaction underpriced")}
	m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, PollInterval: time.Millisecond, Horizon: HorizonConfig{BlockTime: time.Millisecond}}, logr.Discard())
	first, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xaaa"), Data: []byte{1}, GasLimit: 50_000})
	if err != nil || first == nil || first.broadcastErr == nil {
		t.Fatalf("first collision=%+v err=%v", first, err)
	}
	if m.reusable != nil {
		t.Fatalf("initial underpriced candidate created a fee hint: %+v", m.reusable)
	}
	pricedLimit, err := m.MaxFeePerGas(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xbbb"), Data: []byte{2}, GasLimit: 55_000, MaxFeePerGas: pricedLimit})
	if err != nil || second == nil || second.broadcastErr == nil || second.nonce != 7 {
		t.Fatalf("second collision=%+v err=%v", second, err)
	}
	if second.fees.maxFee.Cmp(first.fees.maxFee) != 0 || second.fees.tip.Cmp(first.fees.tip) != 0 {
		t.Fatal("rejected first send raised fresh request fees")
	}
	third, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xccc"), Data: []byte{3}, GasLimit: 60_000})
	if err != nil || third == nil || third.nonce != 7 || third.broadcastErr != nil {
		t.Fatalf("third fresh request=%+v err=%v", third, err)
	}
	if third.fees.maxFee.Cmp(second.fees.maxFee) != 0 || third.fees.tip.Cmp(second.fees.tip) != 0 {
		t.Fatal("repeated rejected send raised fresh request fees")
	}
	if third.req.To == first.req.To || string(third.req.Data) == string(first.req.Data) {
		t.Fatal("fresh collision retry replayed old business calldata")
	}
}

func TestLowerMinedNonceDropsHigherFeeHintAfterReorg(t *testing.T) {
	b := &silentAcceptanceBackend{mockBackend: newMockBackend()}
	b.latestNonce, b.pendingNonce = 6, 9
	m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, logr.Discard())
	m.rememberReusable(7, feeQuote{baseFee: gweiToWei(20), tip: gweiToWei(4), maxFee: gweiToWei(60)})
	pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 50_000})
	if err != nil || pending == nil || pending.nonce != 6 || m.reusable != nil {
		t.Fatalf("higher hint skipped rolled-back nonce: pending=%+v err=%v hint=%+v", pending, err, m.reusable)
	}
	if pending.fees.maxFee.Cmp(gweiToWei(60)) >= 0 {
		t.Fatal("hint from different nonce contaminated fresh fee quote")
	}
}

type unexpectedPendingNonceBackend struct {
	*mockBackend

	reads int
}

func (b *unexpectedPendingNonceBackend) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	b.reads++
	return 0, io.ErrUnexpectedEOF
}

func TestAdmissionDoesNotDependOnPendingNonceRPC(t *testing.T) {
	b := &unexpectedPendingNonceBackend{mockBackend: newMockBackend()}
	m := New(b, mustSigner(t), big.NewInt(1), Config{}, logr.Discard())
	if err := m.Initialize(t.Context()); err != nil || !m.Available() {
		t.Fatalf("mined initialization failed: %v", err)
	}
	pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 50_000})
	if err != nil || pending == nil || pending.nonce != 7 || b.reads != 0 {
		t.Fatalf("pending RPC blocked sending: pending=%+v err=%v reads=%d", pending, err, b.reads)
	}
}

func TestInitialUnderpricedResponsesDoNotClimbTowardConfiguredCap(t *testing.T) {
	for _, ceiling := range []string{"request", "global"} {
		t.Run(ceiling, func(t *testing.T) {
			b := newMockBackend()
			for range 20 {
				b.sendErrs = append(b.sendErrs, errors.New("replacement transaction underpriced"))
			}
			m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 60, PollInterval: time.Millisecond, Horizon: HorizonConfig{BlockTime: time.Millisecond}}, logr.Discard())
			request := Request{To: common.HexToAddress("0xabc"), Data: []byte{1}, GasLimit: 50_000}
			limit := gweiToWei(60)
			if ceiling == "request" {
				request.MaxFeePerGas = gweiToWei(48)
				limit = request.MaxFeePerGas
			}
			for range 20 {
				pending, err := m.broadcast(t.Context(), request)
				if err != nil || pending == nil || pending.broadcastErr == nil || pending.nonce != 7 {
					t.Fatalf("collision candidate=%+v err=%v", pending, err)
				}
				if pending.fees.maxFee.Cmp(limit) > 0 || pending.fees.tip.Cmp(limit) > 0 {
					t.Fatal("candidate exceeded configured fee ceiling")
				}
				request.Data = append(request.Data, byte(len(request.Data)+1)) // each attempt is a freshly prepared business call.
			}
			if m.reusable != nil || len(b.attemptedTransactions()) != 20 {
				t.Fatalf("rejected fees locked out fresh sending: hint=%+v sends=%d", m.reusable, len(b.attemptedTransactions()))
			}
		})
	}
}

type oldNonceReadBackend struct {
	*mockBackend

	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *oldNonceReadBackend) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	first := false
	b.once.Do(func() { first = true })
	if first {
		close(b.entered)
		select {
		case <-b.release:
			return 8, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	return b.mockBackend.NonceAt(ctx, account, blockNumber)
}

func TestOlderNonceReadCannotClearNewerUncertainFeeHint(t *testing.T) {
	for _, operation := range []string{"profitability quote", "nonce selection"} {
		t.Run(operation, func(t *testing.T) {
			b := &oldNonceReadBackend{mockBackend: newMockBackend(), entered: make(chan struct{}), release: make(chan struct{})}
			b.sendErrs = []error{io.ErrUnexpectedEOF}
			m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 150}, logr.Discard())
			oldFees := feeQuote{baseFee: gweiToWei(20), tip: gweiToWei(1), maxFee: gweiToWei(40)}
			m.rememberReusable(7, oldFees)
			result := make(chan error, 1)
			go func() {
				if operation == "profitability quote" {
					_, err := m.MaxFeePerGas(t.Context())
					result <- err
					return
				}
				_, _, err := m.selectNonce(t.Context())
				result <- err
			}()
			<-b.entered // old snapshot is now parked in a nonce read from an earlier chain view.
			pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0xbeef"), Data: []byte{2}, GasLimit: 50_000})
			if err != nil || pending == nil {
				t.Fatalf("new uncertain candidate=%+v err=%v", pending, err)
			}
			close(b.release)
			if err := <-result; err != nil {
				t.Fatal(err)
			}
			hint := m.reusableSnapshot()
			if hint == nil || hint.nonce != 7 || hint.fees.maxFee.Cmp(pending.fees.maxFee) != 0 || hint.fees.tip.Cmp(pending.fees.tip) != 0 {
				t.Fatalf("stale read erased newer uncertain fee floor: hint=%+v candidate=%+v", hint, pending.fees)
			}
		})
	}
}

func TestRepeatedInitialConflictsExecuteFreshWorkThenAdvanceMinedNonce(t *testing.T) {
	b := newMockBackend()
	b.pendingNonce = 14
	for range 6 {
		b.sendErrs = append(b.sendErrs, errors.New("replacement transaction underpriced"))
	}
	m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 150, PollInterval: time.Millisecond, Horizon: HorizonConfig{BlockTime: time.Millisecond}}, logr.Discard())
	startManagerForTest(t, m)
	var previousFee, previousTip *big.Int
	for i := range 7 {
		pricedLimit, err := m.MaxFeePerGas(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		result := m.Send(t.Context(), Request{To: common.HexToAddress("0xbeef"), Data: []byte{byte(i + 1)}, GasLimit: 50_000, MaxFeePerGas: pricedLimit})
		attempted := b.attemptedTransactions()
		tx := attempted[len(attempted)-1]
		if tx.Nonce() != 7 || len(tx.Data()) != 1 || tx.Data()[0] != byte(i+1) || tx.GasFeeCap().Cmp(pricedLimit) > 0 {
			t.Fatalf("fresh request %d sent stale/beyond-cap work: %v", i, tx)
		}
		if previousFee != nil && (tx.GasFeeCap().Cmp(previousFee) != 0 || tx.GasTipCap().Cmp(previousTip) != 0) {
			t.Fatalf("request %d ratcheted rejected replacement fees", i)
		}
		previousFee, previousTip = tx.GasFeeCap(), tx.GasTipCap()
		if i < 6 {
			if result.Outcome != OutcomeNonceConflict || !errors.Is(result.Err, ErrNonceConflict) || result.Receipt != nil {
				t.Fatalf("initial conflict %d result=%+v", i, result)
			}
		} else if result.Outcome != OutcomeConfirmed || result.Err != nil || result.Receipt == nil {
			t.Fatalf("fresh seventh request did not execute: %+v", result)
		}
	}
	next := m.Send(t.Context(), Request{To: common.HexToAddress("0xcafe"), Data: []byte{8}, GasLimit: 50_000})
	if next.Outcome != OutcomeConfirmed || next.Err != nil || b.lastSent().Nonce() != 8 || m.reusableSnapshot() != nil {
		t.Fatalf("mined advancement reused stale hint: result=%+v tx=%v hint=%+v", next, b.lastSent(), m.reusableSnapshot())
	}
	if len(b.attemptedTransactions()) != 8 {
		t.Fatalf("sent %d candidates, want six conflicts and two business fills", len(b.attemptedTransactions()))
	}
}
