package txmanager

import (
	"context"
	"errors"
	"io"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
)

func TestSendAsyncReplacesPendingTransactionWithHigherFees(t *testing.T) {
	b := &replacementBackend{
		mockBackend:        newMockBackend(),
		receiptOnSameNonce: 2,
	}
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{
			PollInterval:        time.Millisecond,
			ReplacementInterval: 2 * time.Millisecond,
			PendingTimeout:      time.Second,
		},
		logr.Discard(),
	)
	feeCap, err := m.MaxFeePerGas(t.Context())
	if err != nil {
		t.Fatalf("MaxFeePerGas: %v", err)
	}
	startTestManager(t, m)

	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), Data: []byte{0x01}, GasLimit: 21_000,
		MaxFeePerGas: feeCap, Label: "replace",
	})
	if !accepted {
		t.Fatal("SendAsync was not accepted")
	}
	select {
	case got := <-result:
		if got.Err != nil {
			t.Fatalf("replacement result: %v", got.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("replacement did not complete")
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.sent) < 2 {
		t.Fatalf("sent transactions = %d, want at least 2", len(b.sent))
	}
	first, replacement := b.sent[0], b.sent[1]
	if replacement.Nonce() != first.Nonce() || string(replacement.Data()) != string(first.Data()) {
		t.Fatalf("replacement changed transaction: first=%+v replacement=%+v", first, replacement)
	}
	if replacement.GasFeeCapCmp(first) <= 0 || replacement.GasTipCapCmp(first) <= 0 {
		t.Fatalf(
			"replacement fees did not increase: first=%s/%s replacement=%s/%s",
			first.GasFeeCap(), first.GasTipCap(), replacement.GasFeeCap(), replacement.GasTipCap(),
		)
	}
	if replacement.GasFeeCap().Cmp(feeCap) > 0 {
		t.Fatalf("replacement fee %s exceeds request cap %s", replacement.GasFeeCap(), feeCap)
	}
}

func TestAmbiguousReplacementGetsOneExactRebroadcast(t *testing.T) {
	b := newMockBackend()
	b.sendErrs = []error{
		errors.New("temporary broadcast failure"),
		errors.New("temporary exact rebroadcast failure"),
	}
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{MaxFeeGwei: 100, PollInterval: time.Millisecond},
		logr.Discard(),
	)
	original := feeQuote{
		baseFee: big.NewInt(20_000_000_000),
		tip:     big.NewInt(1_000_000_000),
		maxFee:  big.NewInt(41_000_000_000),
	}
	pending := &pendingTransaction{
		req:   Request{To: common.HexToAddress("0xabc"), Label: "replace"},
		nonce: 7,
		gas:   21_000,
		value: new(big.Int),
		fees:  cloneFeeQuote(original),
	}

	m.tryReplace(t.Context(), pending, false)
	firstBump := bumpFee(original.maxFee)
	if pending.fees.maxFee.Cmp(firstBump) != 0 {
		t.Fatalf("ambiguous replacement max fee = %s, want %s", pending.fees.maxFee, firstBump)
	}
	if len(pending.attempts) != 1 || !pending.attempts[0].exactRebroadcastPending {
		t.Fatalf("ambiguous replacement attempts = %+v", pending.attempts)
	}
	firstHash := pending.attempts[0].hash

	m.tryReplace(t.Context(), pending, false)
	attempted := b.attemptedTransactions()
	if len(attempted) != 2 || attempted[0].Hash() != firstHash || attempted[1].Hash() != firstHash ||
		len(pending.attempts) != 1 || pending.attempts[0].exactRebroadcastPending {
		t.Fatalf("exact replacement retry = %v, pending %+v", transactionHashes(attempted), pending)
	}

	m.tryReplace(t.Context(), pending, false)
	if len(pending.attempts) != 2 {
		t.Fatalf("post-retry replacement attempts = %+v", pending.attempts)
	}
	wantMaxFee := bumpFee(firstBump)
	if pending.fees.maxFee.Cmp(wantMaxFee) != 0 {
		t.Fatalf("post-retry replacement max fee = %s, want %s", pending.fees.maxFee, wantMaxFee)
	}
}

func TestCancellationRequestBypassesAmbiguousExactRebroadcast(t *testing.T) {
	b := newMockBackend()
	b.sendErrs = []error{io.ErrUnexpectedEOF}
	s := mustSigner(t)
	m := New(b, s, big.NewInt(11155111), Config{MaxFeeGwei: 100}, logr.Discard())
	pending, err := m.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xabc"), Data: []byte{0x01}, GasLimit: 21_000, Label: "shutdown",
	})
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	pending.cancelDeadline = time.Now().Add(time.Hour)
	pending.cancelRequested = make(chan struct{})
	requestCancellation(pending)

	m.tryReplace(t.Context(), pending, false)
	attempted := b.attemptedTransactions()
	if len(attempted) != 2 || attempted[1].Hash() == attempted[0].Hash() {
		t.Fatalf("cancellation attempts = %v, want a new same-nonce transaction", transactionHashes(attempted))
	}
	cancellation := attempted[1]
	if cancellation.To() == nil || *cancellation.To() != s.Address() || len(cancellation.Data()) != 0 ||
		cancellation.Gas() != cancellationGasLimit || !pending.attempts[1].cancellation {
		t.Fatalf("shutdown retry was not a self-cancellation: tx=%+v attempt=%+v", cancellation, pending.attempts[1])
	}
}

func TestCancellationSignalDoesNotSendBackToBackReplacements(t *testing.T) {
	b := &replacementBackend{mockBackend: newMockBackend()}
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{
		MaxFeeGwei: 100, PollInterval: time.Second, ReplacementInterval: time.Second,
	}, logr.Discard())
	pending, err := m.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "simultaneous cancellation",
	})
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	pending.cancelDeadline = time.Now().Add(-time.Second)
	pending.cancelRequested = make(chan struct{})
	requestCancellation(pending)

	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan Result, 1)
	go func() { result <- m.waitForPendingTransaction(ctx, pending) }()
	waitForSentTransactions(t, b.mockBackend, 2)
	cancel()
	select {
	case <-result:
	case <-time.After(time.Second):
		t.Fatal("pending lifecycle did not stop")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.sent) != 2 {
		t.Fatalf("sent transactions = %d, want initial plus one cancellation", len(b.sent))
	}
}

func TestExactRebroadcastNonceTooLowReconcilesOriginalReceipt(t *testing.T) {
	b := &replacementNonceRaceBackend{
		mockBackend: newMockBackend(), publishOwnedReceipt: true, firstSendErr: io.ErrUnexpectedEOF,
	}
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{MaxFeeGwei: 100}, logr.Discard())
	pending, err := m.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "exact retry inclusion",
	})
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}

	m.tryReplace(t.Context(), pending, false)
	attempted := b.attemptedTransactions()
	if len(attempted) != 2 || attempted[0].Hash() != attempted[1].Hash() || len(pending.attempts) != 1 {
		t.Fatalf("nonce-low exact retry = %v, tracked = %+v", transactionHashes(attempted), pending.attempts)
	}
	if !m.Available() {
		t.Fatal("owned original receipt paused the nonce lane")
	}
	result, done := m.receiptResult(t.Context(), pending)
	if !done || result.Err != nil || result.Receipt == nil || result.Hash != pending.originalHash {
		t.Fatalf("reconciled receipt = (%+v, %v)", result, done)
	}
}

func TestExactRebroadcastSlack(t *testing.T) {
	now := time.Unix(1_000, 0)
	m := New(nil, nil, nil, Config{
		BroadcastTimeout: 5 * time.Second, ReplacementInterval: 5 * time.Second,
	}, logr.Discard())
	for name, test := range map[string]struct {
		deadline time.Time
		want     bool
	}{
		"no deadline":        {want: true},
		"at safety bound":    {deadline: now.Add(10 * time.Second)},
		"after safety bound": {deadline: now.Add(11 * time.Second), want: true},
	} {
		if got := m.hasExactRebroadcastSlack(
			&pendingTransaction{cancelDeadline: test.deadline}, now,
		); got != test.want {
			t.Errorf("%s: slack = %v, want %v", name, got, test.want)
		}
	}
}

func TestCappedNormalRebroadcastStopsAtCancellationDeadline(t *testing.T) {
	b := &cappedRebroadcastDeadlineBackend{mockBackend: newMockBackend()}
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{MaxFeeGwei: 50}, logr.Discard())
	pending, err := m.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "capped deadline",
	})
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	pending.cancelDeadline = time.Now().Add(time.Second)
	if !m.rebroadcastLatestAttempt(t.Context(), pending, false) ||
		!b.deadlineOK || !b.deadline.Equal(pending.cancelDeadline) {
		t.Fatalf("capped rebroadcast deadline = (%s, %v), want %s", b.deadline, b.deadlineOK, pending.cancelDeadline)
	}
}

func TestCappedAmbiguousCancellationRebroadcastsExactSignedTransaction(t *testing.T) {
	b := newMockBackend()
	s := mustSigner(t)
	m := New(
		b, s, big.NewInt(11155111), Config{MaxFeeGwei: 50}, logr.Discard(),
	)
	unsigned := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(11155111), Nonce: 7,
		GasTipCap: big.NewInt(1_000_000_000), GasFeeCap: gweiToWei(50),
		Gas: cancellationGasLimit, To: ptr(s.Address()), Value: new(big.Int),
	})
	signed, err := s.SignTx(t.Context(), unsigned, big.NewInt(11155111))
	if err != nil {
		t.Fatalf("sign cancellation: %v", err)
	}
	pending := &pendingTransaction{
		req:   Request{To: common.HexToAddress("0xabc"), Label: "cancel"},
		nonce: 7,
		fees: feeQuote{
			baseFee: big.NewInt(20_000_000_000),
			tip:     big.NewInt(1_000_000_000),
			maxFee:  gweiToWei(50),
		},
		attempts: []txAttempt{{hash: signed.Hash(), tx: signed, cancellation: true}},
	}

	m.tryReplace(t.Context(), pending, true)
	if b.sendCalls != 1 || len(b.sent) != 1 {
		t.Fatalf("exact rebroadcast calls/sent = %d/%d, want 1/1", b.sendCalls, len(b.sent))
	}
	if b.sent[0].Hash() != signed.Hash() || len(pending.attempts) != 1 {
		t.Fatalf("rebroadcast changed signed attempt: sent=%s attempts=%+v", b.sent[0].Hash(), pending.attempts)
	}
}

func TestNormalFeeLimitReservesOneCancellationBump(t *testing.T) {
	b := newMockBackend()
	b.baseFee = big.NewInt(30_000_000_000)
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{MaxFeeGwei: 50, PollInterval: time.Millisecond},
		logr.Discard(),
	)

	fees, err := m.currentFees(t.Context(), m.normalFeeLimit(Request{}))
	if err != nil {
		t.Fatalf("fees: %v", err)
	}
	normalLimit := reserveFeeBump(gweiToWei(50))
	if fees.maxFee.Cmp(normalLimit) != 0 {
		t.Fatalf("normal max fee = %s, want reserved limit %s", fees.maxFee, normalLimit)
	}
	cancellationFees, err := m.nextReplacementFees(
		t.Context(),
		feeQuote{baseFee: fees.baseFee, tip: fees.tip, maxFee: normalLimit},
		m.globalFeeLimit(),
	)
	if err != nil {
		t.Fatalf("cancellation fees: %v", err)
	}
	if cancellationFees.maxFee.Cmp(gweiToWei(50)) != 0 {
		t.Fatalf("cancellation max fee = %s, want global cap %s", cancellationFees.maxFee, gweiToWei(50))
	}
}

func TestReplacementFeesRespectCapAndFullBump(t *testing.T) {
	quote := func(baseFee, tip, maxFee float64) feeQuote {
		return feeQuote{baseFee: gweiToWei(baseFee), tip: gweiToWei(tip), maxFee: gweiToWei(maxFee)}
	}
	tests := map[string]struct {
		previous feeQuote
		current  feeQuote
		want     feeQuote
		wantErr  bool
	}{
		"fresh tip is bounded by the cap": {
			previous: quote(20, 1, 44), current: quote(20, 40, 0), want: quote(20, 30, 50),
		},
		"raw tip bump may exceed effective headroom": {
			previous: quote(20, 10, 44), current: quote(39.5, 1, 0), want: quote(39.5, 11.25, 50),
		},
		"max fee bump does not fit": {
			previous: quote(20, 1, 45), current: quote(20, 1, 0), wantErr: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			b := newMockBackend()
			b.baseFee = test.current.baseFee
			b.history = constantFeeHistory(test.current.tip)
			m := New(b, mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())

			got, err := m.nextReplacementFees(t.Context(), test.previous, gweiToWei(50))
			if test.wantErr {
				if !errors.Is(err, errReplacementLimitReached) {
					t.Fatalf("nextReplacementFees error = %v, want replacement limit", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("nextReplacementFees: %v", err)
			}
			if got.baseFee.Cmp(test.want.baseFee) != 0 || got.tip.Cmp(test.want.tip) != 0 ||
				got.maxFee.Cmp(test.want.maxFee) != 0 {
				t.Fatalf("fees = %s/%s/%s, want %s/%s/%s",
					got.baseFee, got.tip, got.maxFee, test.want.baseFee, test.want.tip, test.want.maxFee)
			}
		})
	}
}

func TestInitializeRejectsUnknownPendingNonceGap(t *testing.T) {
	b := newMockBackend()
	b.pendingNonce = b.latestNonce + 1
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())

	err := m.Initialize(t.Context())
	if err == nil || !strings.Contains(err.Error(), "unmanaged pending nonce gap") {
		t.Fatalf("Initialize error = %v, want unknown-gap failure", err)
	}
	if m.nonceInit {
		t.Fatal("manager initialized despite an unknown pending transaction")
	}
}

func TestPendingTimeoutCancelsBlockedNonceAndUnblocksLaterTransaction(t *testing.T) {
	sgnr := mustSigner(t)
	b := &replacementBackend{
		mockBackend: newMockBackend(), cancellationTo: sgnr.Address(),
	}
	m := New(
		b, sgnr, big.NewInt(11155111),
		Config{
			MaxFeeGwei:          50,
			PollInterval:        time.Millisecond,
			ReplacementInterval: 2 * time.Millisecond,
			PendingTimeout:      20 * time.Millisecond,
		},
		logr.Discard(),
	)
	startTestManager(t, m)

	first, accepted := m.SendAsync(t.Context(), Request{
		To:           common.HexToAddress("0xabc"),
		Data:         []byte{0x01},
		GasLimit:     21_000,
		MaxFeePerGas: big.NewInt(42_000_000_000),
		Label:        "blocked",
	})
	if !accepted {
		t.Fatal("first SendAsync was not accepted")
	}
	second, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xdef"), Data: []byte{0x02}, GasLimit: 21_000, Label: "later",
	})
	if !accepted {
		t.Fatal("second SendAsync was not accepted")
	}

	select {
	case got := <-first:
		if got.Err == nil || !strings.Contains(got.Err.Error(), "cancelled at nonce 7") {
			t.Fatalf("first result = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked transaction was not cancelled")
	}
	select {
	case got := <-second:
		if got.Err != nil {
			t.Fatalf("later transaction result: %v", got.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("later nonce remained wedged")
	}
	cancellation := b.cancellationTransaction()
	if cancellation == nil {
		t.Fatal("same-nonce cancellation was not sent")
	}
	if cancellation.GasFeeCap().Cmp(big.NewInt(42_000_000_000)) <= 0 {
		t.Fatalf("cancellation fee %s did not escape the fill profitability cap", cancellation.GasFeeCap())
	}
	later := b.lastSent()
	if later == nil || later.Nonce() != 8 || b.isCancellation(later) {
		t.Fatalf("later transaction = %v, want non-cancellation nonce 8", later)
	}
}

func TestPendingObsolescenceCancelsNonceAndUnblocksLaterTransaction(t *testing.T) {
	sgnr := mustSigner(t)
	b := &replacementBackend{
		mockBackend: newMockBackend(), cancellationTo: sgnr.Address(),
	}
	m := New(
		b, sgnr, big.NewInt(11155111),
		Config{
			MaxFeeGwei:          50,
			PollInterval:        time.Millisecond,
			ReplacementInterval: time.Hour,
			PendingTimeout:      time.Hour,
		},
		logr.Discard(),
	)
	startTestManager(t, m)

	var mode atomic.Int32
	unknownChecked := make(chan struct{})
	var unknownOnce sync.Once
	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), Data: []byte{0x01}, GasLimit: 21_000, Label: "fillable-order",
		Obsolete: func(context.Context) (bool, error) {
			switch mode.Load() {
			case 1:
				unknownOnce.Do(func() { close(unknownChecked) })
				return false, errors.New("status RPC unavailable")
			case 2:
				return true, nil
			default:
				return false, nil
			}
		},
	})
	if !accepted {
		t.Fatal("SendAsync was not accepted")
	}
	waitForSentTransactions(t, b.mockBackend, 1)

	mode.Store(1)
	select {
	case <-unknownChecked:
	case <-time.After(time.Second):
		t.Fatal("pending obsolescence error was not observed")
	}
	if cancellation := b.cancellationTransaction(); cancellation != nil {
		t.Fatalf("unknown order status cancelled transaction %s", cancellation.Hash())
	}

	mode.Store(2)
	select {
	case got := <-result:
		if got.Err == nil || !strings.Contains(got.Err.Error(), "cancelled at nonce 7") {
			t.Fatalf("obsolete request result = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("obsolete request did not cancel promptly")
	}
	cancellation := b.cancellationTransaction()
	if cancellation == nil || cancellation.Nonce() != 7 {
		t.Fatalf("same-nonce cancellation = %v", cancellation)
	}

	later := m.Send(t.Context(), Request{
		To: common.HexToAddress("0xdef"), Data: []byte{0x02}, GasLimit: 21_000, Label: "later",
	})
	if later.Err != nil {
		t.Fatalf("later transaction: %v", later.Err)
	}
	last := b.lastSent()
	if last == nil || last.Nonce() != 8 || b.isCancellation(last) {
		t.Fatalf("later transaction = %v, want non-cancellation nonce 8", last)
	}
}

func TestValidRequestReceiptWinsBeforePendingObsolescenceCheck(t *testing.T) {
	b := newMockBackend()
	m := newTestManager(t, b)
	var checks atomic.Int64

	result := m.Send(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "valid",
		Obsolete: func(context.Context) (bool, error) {
			return checks.Add(1) > 1, nil
		},
	})
	if result.Err != nil {
		t.Fatalf("valid request result: %v", result.Err)
	}
	if got := checks.Load(); got != 1 {
		t.Fatalf("obsolescence checks = %d, want only pre-sign check before owned receipt", got)
	}
	if attempted := b.attemptedTransactions(); len(attempted) != 1 {
		t.Fatalf("valid request attempts = %d, want 1", len(attempted))
	}
}

func TestWaitingRequestKeepsAbsoluteCancelAtBeforeBroadcast(t *testing.T) {
	sgnr := mustSigner(t)
	b := &replacementBackend{
		mockBackend: newMockBackend(), cancellationTo: sgnr.Address(),
	}
	m := New(
		b, sgnr, big.NewInt(11155111),
		Config{
			PollInterval:        time.Millisecond,
			ReplacementInterval: time.Second,
			PendingTimeout:      30 * time.Millisecond,
		},
		logr.Discard(),
	)
	startTestManager(t, m)

	first, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "lower nonce",
	})
	if !accepted {
		t.Fatal("lower nonce was not accepted")
	}
	cancelAt := time.Now().Add(10 * time.Millisecond)
	second, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xdef"), GasLimit: 21_000,
		CancelAt: cancelAt, Label: "expired while waiting",
	})
	if !accepted {
		t.Fatal("deadline failure did not return a result")
	}

	if got := <-first; got.Err == nil || !strings.Contains(got.Err.Error(), "cancelled at nonce 7") {
		t.Fatalf("lower nonce result = %+v", got)
	}
	select {
	case got := <-second:
		if got.Err == nil || !strings.Contains(got.Err.Error(), "context deadline exceeded") || !got.NotAdmitted {
			t.Fatalf("expired waiting result = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expired waiting request did not fail")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, tx := range b.sent {
		if tx.Nonce() > 7 {
			t.Fatalf("expired waiting request was signed at nonce %d", tx.Nonce())
		}
	}
}

func TestCancelAtUsesCachedFeesWhenFeeRPCBlocks(t *testing.T) {
	sgnr := mustSigner(t)
	b := &blockedFeeBackend{replacementBackend: &replacementBackend{
		mockBackend: newMockBackend(), cancellationTo: sgnr.Address(),
	}}
	b.tip = big.NewInt(1_500)
	m := New(
		b, sgnr, big.NewInt(11155111),
		Config{
			MaxFeeGwei:          50,
			TipGwei:             1,
			PollInterval:        time.Millisecond,
			ReplacementInterval: 10 * time.Millisecond,
			PendingTimeout:      time.Second,
		},
		logr.Discard(),
	)
	startTestManager(t, m)

	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000,
		CancelAt: time.Now().Add(30 * time.Millisecond), Label: "expiring",
	})
	if !accepted {
		t.Fatal("transaction was not accepted")
	}
	waitForSentTransactions(t, b.mockBackend, 1)
	b.block.Store(true)

	select {
	case got := <-result:
		if got.Err == nil || !strings.Contains(got.Err.Error(), "cancelled at nonce 7") {
			t.Fatalf("cancellation result = %+v", got)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("CancelAt did not promptly cancel the nonce")
	}
	cancellation := b.cancellationTransaction()
	if cancellation == nil || cancellation.Nonce() != 7 {
		t.Fatalf("same-nonce cancellation = %v", cancellation)
	}
	if cancellation.GasFeeCap().Cmp(gweiToWei(50)) > 0 {
		t.Fatalf("cancellation fee %s exceeded global cap", cancellation.GasFeeCap())
	}
}
