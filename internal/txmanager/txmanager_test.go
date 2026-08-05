package txmanager

import (
	"context"
	"errors"
	"math/big"
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

type mockBackend struct {
	mu sync.Mutex

	pendingNonce uint64
	tip          *big.Int
	baseFee      *big.Int
	gasEstimate  uint64
	head         uint64

	sendErrs  []error // returned, in order, by successive SendTransaction calls
	sendCalls int
	sent      []*types.Transaction
	receipts  map[common.Hash]*types.Receipt
}

func newMockBackend() *mockBackend {
	return &mockBackend{
		pendingNonce: 7,
		tip:          big.NewInt(1e9),
		baseFee:      big.NewInt(20e9),
		gasEstimate:  50_000,
		head:         100,
		receipts:     map[common.Hash]*types.Receipt{},
	}
}

func (b *mockBackend) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.pendingNonce, nil
}

func (b *mockBackend) SuggestGasTipCap(context.Context) (*big.Int, error) { return b.tip, nil }

func (b *mockBackend) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return &types.Header{Number: new(big.Int).SetUint64(b.head), BaseFee: b.baseFee}, nil
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
	if i < len(b.sendErrs) && b.sendErrs[i] != nil {
		return b.sendErrs[i]
	}
	b.sent = append(b.sent, tx)
	b.receipts[tx.Hash()] = &types.Receipt{
		Status:      types.ReceiptStatusSuccessful,
		TxHash:      tx.Hash(),
		BlockNumber: new(big.Int).SetUint64(b.head),
	}
	return nil
}

func (b *mockBackend) TransactionReceipt(_ context.Context, h common.Hash) (*types.Receipt, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r, ok := b.receipts[h]; ok {
		return r, nil
	}
	return nil, ethereum.NotFound
}

func (b *mockBackend) BlockNumber(context.Context) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.head, nil
}

func (b *mockBackend) lastSent() *types.Transaction {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.sent) == 0 {
		return nil
	}
	return b.sent[len(b.sent)-1]
}

func newTestManager(t *testing.T, b Backend) (*Manager, context.CancelFunc) {
	t.Helper()
	s, err := signer.NewFromHexKey(testKey)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	m := New(b, s, big.NewInt(11155111), Config{Confirmations: 0, PollInterval: time.Millisecond}, logr.Discard())
	ctx, cancel := context.WithCancel(context.Background())
	go m.Start(ctx)
	return m, cancel
}

func TestSend_HappyPath(t *testing.T) {
	b := newMockBackend()
	m, cancel := newTestManager(t, b)
	defer cancel()

	res := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), Data: []byte{0x01}, Label: "test"})
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.Receipt == nil || res.Receipt.Status != types.ReceiptStatusSuccessful {
		t.Fatalf("expected successful receipt, got %+v", res.Receipt)
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

func TestMaxFeePerGasMatchesSendFeePolicy(t *testing.T) {
	b := newMockBackend()
	m, cancel := newTestManager(t, b)
	defer cancel()

	fee, err := m.MaxFeePerGas(context.Background())
	if err != nil {
		t.Fatalf("MaxFeePerGas: %v", err)
	}
	if fee.String() != "41000000000" {
		t.Fatalf("max fee = %s, want 41000000000", fee)
	}
}

func TestMaxFeeGweiCapsDerivedFeeWithoutConsumingReplacementHeadroom(t *testing.T) {
	b := newMockBackend()
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{MaxFeeGwei: 100, PollInterval: time.Millisecond},
		logr.Discard(),
	)
	fee, err := m.MaxFeePerGas(t.Context())
	if err != nil {
		t.Fatalf("MaxFeePerGas: %v", err)
	}
	if fee.String() != "41000000000" {
		t.Fatalf("max fee = %s, want derived 41000000000 below the 100 gwei cap", fee)
	}
}

func TestMaxFeeGweiRejectsCurrentBaseFeeAboveCap(t *testing.T) {
	b := newMockBackend()
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{MaxFeeGwei: 10, PollInterval: time.Millisecond},
		logr.Discard(),
	)
	if _, err := m.MaxFeePerGas(t.Context()); err == nil {
		t.Fatal("expected max fee cap below current base fee to fail")
	}
}

func TestSend_ClampsFeeToRequestCap(t *testing.T) {
	b := newMockBackend()
	m, cancel := newTestManager(t, b)
	defer cancel()

	res := m.Send(context.Background(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "capped",
		MaxFeePerGas: big.NewInt(40_000_000_000),
	})
	if res.Err != nil {
		t.Fatalf("send: %v", res.Err)
	}
	tx := b.lastSent()
	if tx == nil {
		t.Fatal("no transaction sent")
	}
	if tx.GasFeeCap().Cmp(big.NewInt(40_000_000_000)) != 0 {
		t.Fatalf("gas fee cap = %s, want 40000000000", tx.GasFeeCap())
	}
	if tx.GasTipCap().Cmp(big.NewInt(1_000_000_000)) != 0 {
		t.Fatalf("gas tip cap = %s, want 1000000000", tx.GasTipCap())
	}
}

func TestSend_ClampsTipToFitRequestCap(t *testing.T) {
	b := newMockBackend()
	m, cancel := newTestManager(t, b)
	defer cancel()

	res := m.Send(context.Background(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "capped",
		MaxFeePerGas: big.NewInt(20_500_000_000),
	})
	if res.Err != nil {
		t.Fatalf("send: %v", res.Err)
	}
	tx := b.lastSent()
	if tx == nil {
		t.Fatal("no transaction sent")
	}
	if tx.GasFeeCap().Cmp(big.NewInt(20_500_000_000)) != 0 {
		t.Fatalf("gas fee cap = %s, want 20500000000", tx.GasFeeCap())
	}
	if tx.GasTipCap().Cmp(big.NewInt(500_000_000)) != 0 {
		t.Fatalf("gas tip cap = %s, want 500000000", tx.GasTipCap())
	}
}

func TestSend_RejectsRequestCapBelowCurrentBaseFee(t *testing.T) {
	b := newMockBackend()
	m, cancel := newTestManager(t, b)
	defer cancel()

	res := m.Send(context.Background(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "capped",
		MaxFeePerGas: big.NewInt(19_000_000_000),
	})
	if res.Err == nil {
		t.Fatal("expected base-fee rejection")
	}
	if tx := b.lastSent(); tx != nil {
		t.Fatalf("underpriced request sent transaction %s", tx.Hash())
	}
}

func TestSend_SequentialNoncesMonotonic(t *testing.T) {
	b := newMockBackend()
	m, cancel := newTestManager(t, b)
	defer cancel()

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

func TestSendAsyncBroadcastsSequentialNoncesBeforeConfirmations(t *testing.T) {
	b := newMockBackend()
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{Confirmations: 2, PollInterval: time.Millisecond}, logr.Discard(),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Start(ctx)

	results := make([]<-chan Result, 0, 3)
	for range 3 {
		result, accepted := m.SendAsync(
			context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "pipeline"},
		)
		if !accepted {
			t.Fatal("SendAsync was not accepted")
		}
		results = append(results, result)
	}
	waitForSentTransactions(t, b, 3)
	b.mu.Lock()
	for i, tx := range b.sent {
		if want := uint64(7 + i); tx.Nonce() != want {
			b.mu.Unlock()
			t.Fatalf("transaction %d nonce = %d, want %d", i, tx.Nonce(), want)
		}
	}
	b.head = 102
	b.mu.Unlock()
	for i, result := range results {
		select {
		case got := <-result:
			if got.Err != nil {
				t.Fatalf("result %d: %v", i, got.Err)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for result %d", i)
		}
	}
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

func TestSendAsyncRejectsAfterManagerStops(t *testing.T) {
	m := stoppedTestManager(t)
	type sendResult struct {
		result   <-chan Result
		accepted bool
	}
	returned := make(chan sendResult, 1)
	go func() {
		result, accepted := m.SendAsync(t.Context(), Request{Label: "after-stop"})
		returned <- sendResult{result: result, accepted: accepted}
	}()

	select {
	case got := <-returned:
		if got.accepted || got.result != nil {
			t.Fatalf("SendAsync after manager stopped = (%v, %v), want (nil, false)", got.result, got.accepted)
		}
	case <-time.After(time.Second):
		t.Fatal("SendAsync blocked after manager stopped")
	}
}

func TestSendAfterManagerStopsReturnsError(t *testing.T) {
	m := stoppedTestManager(t)
	returned := make(chan Result, 1)
	go func() {
		returned <- m.Send(t.Context(), Request{Label: "after-stop"})
	}()

	select {
	case got := <-returned:
		if !errors.Is(got.Err, errManagerStopped) {
			t.Fatalf("Send error = %v, want %v", got.Err, errManagerStopped)
		}
	case <-time.After(time.Second):
		t.Fatal("Send blocked after manager stopped")
	}
}

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
	go m.Start(t.Context())

	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), Data: []byte{0x01}, GasLimit: 21_000, Label: "replace",
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
}

func TestFailedReplacementDoesNotAdvanceFeeState(t *testing.T) {
	b := newMockBackend()
	b.sendErrs = []error{errors.New("temporary broadcast failure")}
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
	if pending.fees.maxFee.Cmp(original.maxFee) != 0 || pending.fees.tip.Cmp(original.tip) != 0 {
		t.Fatalf("failed replacement advanced fees to %+v", pending.fees)
	}
	if len(pending.attempts) != 0 {
		t.Fatalf("failed replacement attempts = %+v", pending.attempts)
	}

	m.tryReplace(t.Context(), pending, false)
	if len(pending.attempts) != 1 {
		t.Fatalf("successful retry attempts = %+v", pending.attempts)
	}
	wantMaxFee := bumpFee(original.maxFee)
	if pending.fees.maxFee.Cmp(wantMaxFee) != 0 {
		t.Fatalf("successful retry max fee = %s, want first bump %s", pending.fees.maxFee, wantMaxFee)
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

	fees, err := m.currentFees(t.Context())
	if err != nil {
		t.Fatalf("fees: %v", err)
	}
	normalLimit := reserveCancellationBump(gweiToWei(50))
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

func TestPendingTimeoutCancelsBlockedNonceAndUnblocksLaterTransaction(t *testing.T) {
	sgnr := mustSigner(t)
	b := &replacementBackend{mockBackend: newMockBackend(), cancellationTo: sgnr.Address()}
	m := New(
		b, sgnr, big.NewInt(11155111),
		Config{
			PollInterval:        time.Millisecond,
			ReplacementInterval: 2 * time.Millisecond,
			PendingTimeout:      8 * time.Millisecond,
		},
		logr.Discard(),
	)
	go m.Start(t.Context())

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
}

func TestIncludedNonceDoesNotBlockLaterCancellationWhileConfirming(t *testing.T) {
	b := newMockBackend()
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{
			Confirmations:       2,
			MaxFeeGwei:          100,
			PollInterval:        time.Millisecond,
			ReplacementInterval: time.Second,
			PendingTimeout:      time.Second,
		},
		logr.Discard(),
	)
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(11155111), Nonce: 7, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
		Gas: 21_000, To: ptr(common.HexToAddress("0xabc")),
	})
	b.receipts[tx.Hash()] = successfulReceipt(tx, b.head)
	pending := &pendingTransaction{
		req:      Request{Label: "confirming"},
		nonce:    7,
		fees:     feeQuote{baseFee: big.NewInt(1), tip: big.NewInt(1), maxFee: big.NewInt(2)},
		attempts: []txAttempt{{hash: tx.Hash()}},
	}
	m.addUnminedNonce(7)
	m.addUnminedNonce(8)

	result := make(chan Result, 1)
	go func() { result <- m.waitForPendingTransaction(t.Context(), pending) }()
	eventually(t, func() bool { return m.isLowestUnminedNonce(8) })
	select {
	case got := <-result:
		t.Fatalf("transaction completed before confirmations: %+v", got)
	default:
	}

	b.mu.Lock()
	b.head = 102
	b.mu.Unlock()
	if got := <-result; got.Err != nil {
		t.Fatalf("confirmed result: %v", got.Err)
	}
}

func TestTransientReceiptErrorKeepsTrackingPendingTransaction(t *testing.T) {
	b := &receiptErrorBackend{mockBackend: newMockBackend(), failures: 1}
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{
			MaxFeeGwei:          100,
			PollInterval:        time.Millisecond,
			ReplacementInterval: time.Second,
			PendingTimeout:      time.Second,
		},
		logr.Discard(),
	)
	go m.Start(t.Context())

	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "receipt retry",
	})
	if !accepted {
		t.Fatal("transaction was not accepted")
	}
	if got := <-result; got.Err != nil {
		t.Fatalf("receipt retry result: %v", got.Err)
	}
}

func waitForSentTransactions(t *testing.T, b *mockBackend, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		sent := len(b.sent)
		b.mu.Unlock()
		if sent >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d broadcasts", count)
}

type receiptErrorBackend struct {
	*mockBackend

	receiptMu sync.Mutex
	failures  int
}

func (b *receiptErrorBackend) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	b.receiptMu.Lock()
	if b.failures > 0 {
		b.failures--
		b.receiptMu.Unlock()
		return nil, errors.New("temporary receipt failure")
	}
	b.receiptMu.Unlock()
	return b.mockBackend.TransactionReceipt(ctx, hash)
}

type replacementBackend struct {
	*mockBackend

	receiptOnSameNonce int
	cancellationTo     common.Address
	sameNonceSends     int
	cancelled          bool
}

func (b *replacementBackend) SendTransaction(_ context.Context, tx *types.Transaction) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sendCalls++
	b.sent = append(b.sent, tx)

	if b.isCancellation(tx) {
		b.cancelled = true
		b.receipts[tx.Hash()] = successfulReceipt(tx, b.head)
		for _, sent := range b.sent {
			if sent.Nonce() > tx.Nonce() {
				b.receipts[sent.Hash()] = successfulReceipt(sent, b.head)
			}
		}
		return nil
	}
	if tx.Nonce() == b.pendingNonce {
		b.sameNonceSends++
		if b.receiptOnSameNonce > 0 && b.sameNonceSends >= b.receiptOnSameNonce {
			b.receipts[tx.Hash()] = successfulReceipt(tx, b.head)
		}
		return nil
	}
	if b.cancelled {
		b.receipts[tx.Hash()] = successfulReceipt(tx, b.head)
	}
	return nil
}

func (b *replacementBackend) cancellationTransaction() *types.Transaction {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, tx := range b.sent {
		if b.isCancellation(tx) {
			return tx
		}
	}
	return nil
}

func (b *replacementBackend) isCancellation(tx *types.Transaction) bool {
	return b.cancellationTo != (common.Address{}) &&
		tx.To() != nil &&
		*tx.To() == b.cancellationTo &&
		len(tx.Data()) == 0 &&
		tx.Value().Sign() == 0 &&
		tx.Gas() == cancellationGasLimit
}

func successfulReceipt(tx *types.Transaction, block uint64) *types.Receipt {
	return &types.Receipt{
		Status:      types.ReceiptStatusSuccessful,
		TxHash:      tx.Hash(),
		BlockNumber: new(big.Int).SetUint64(block),
	}
}

func TestSend_NonceTooLowResyncsAndRetries(t *testing.T) {
	b := newMockBackend()
	b.sendErrs = []error{errors.New("nonce too low")} // first send fails, second succeeds
	m, cancel := newTestManager(t, b)
	defer cancel()

	// Simulate the chain having advanced past our seeded nonce.
	b.pendingNonce = 9

	res := m.Send(context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21000, Label: "retry"})
	if res.Err != nil {
		t.Fatalf("expected success after resync, got %v", res.Err)
	}
	if got := b.lastSent().Nonce(); got != 9 {
		t.Fatalf("expected resynced nonce 9, got %d", got)
	}
}

func TestSend_GasEstimateFailurePropagates(t *testing.T) {
	b := newMockBackend()
	b.gasEstimate = 0 // forces EstimateGas to error
	m, cancel := newTestManager(t, b)
	defer cancel()

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

// revertingBackend records a failed receipt instead of a successful one.
type revertingBackend struct{ *mockBackend }

func (b *revertingBackend) SendTransaction(_ context.Context, tx *types.Transaction) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent = append(b.sent, tx)
	b.receipts[tx.Hash()] = &types.Receipt{
		Status:      types.ReceiptStatusFailed,
		TxHash:      tx.Hash(),
		BlockNumber: new(big.Int).SetUint64(b.head),
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
	go m.Start(t.Context()) // manager context lives until test cleanup; the caller's is cancelled below

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

func TestTrySendRejectsWhileTransactionIsActive(t *testing.T) {
	bb := &blockingBackend{mockBackend: newMockBackend(), entered: make(chan struct{}), release: make(chan struct{})}
	m := New(bb, mustSigner(t), big.NewInt(11155111), Config{PollInterval: time.Millisecond}, logr.Discard())
	go m.Start(t.Context())

	type tryResult struct {
		result   Result
		accepted bool
	}
	first := make(chan tryResult, 1)
	go func() {
		result, accepted := m.TrySend(
			context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "first"},
		)
		first <- tryResult{result: result, accepted: accepted}
	}()

	<-bb.entered
	if result, accepted := m.TrySend(
		context.Background(), Request{To: common.HexToAddress("0xdef"), GasLimit: 21_000, Label: "second"},
	); accepted || result.Err != nil {
		t.Fatalf("busy TrySend = (%+v, %v), want not accepted", result, accepted)
	}
	close(bb.release)
	got := <-first
	if !got.accepted || got.result.Err != nil {
		t.Fatalf("first TrySend = (%+v, %v)", got.result, got.accepted)
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

func stoppedTestManager(t *testing.T) *Manager {
	t.Helper()
	m := New(
		newMockBackend(), mustSigner(t), big.NewInt(11155111),
		Config{PollInterval: time.Millisecond}, logr.Discard(),
	)
	ctx, cancel := context.WithCancel(t.Context())
	go m.Start(ctx)
	cancel()

	select {
	case <-m.done:
	case <-time.After(time.Second):
		t.Fatal("manager did not stop")
	}
	return m
}

func ptr[T any](value T) *T {
	return &value
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met")
}
