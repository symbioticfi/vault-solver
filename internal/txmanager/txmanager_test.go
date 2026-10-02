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

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
	"github.com/symbioticfi/vault-solver/internal/signer"
)

// anvil account #0 — a well-known throwaway key, fine for unit tests.
const testKey = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

type feeHistoryRequest struct {
	blocks      uint64
	newest      *big.Int
	percentiles []float64
}

type mockBackend struct {
	mu sync.Mutex

	latestNonce  uint64
	pendingNonce uint64
	historyErr   error
	historyReq   feeHistoryRequest
	// Every block in the fee window has baseFee, gasUsedRatio and reward, and the next block's base
	// fee is baseFee too. The default half-full blocks leave room for any test call.
	baseFee       *big.Int
	gasUsedRatio  float64
	reward        *big.Int
	gasEstimate   uint64
	estimateCalls atomic.Int64
	head          uint64
	reorgedHeader bool

	reorgOnHeadRead bool
	latestHeads     []uint64
	headerHashReads int

	sendErrs  []error // returned, in order, by successive SendTransaction calls
	sendCalls int
	attempted []*types.Transaction
	sent      []*types.Transaction
	receipts  map[common.Hash]*types.Receipt
}

func newMockBackend() *mockBackend {
	return &mockBackend{
		latestNonce:  7,
		pendingNonce: 7,
		baseFee:      big.NewInt(20e9),
		gasUsedRatio: 0.5,
		reward:       big.NewInt(1e9),
		gasEstimate:  50_000,
		head:         100,
		receipts:     map[common.Hash]*types.Receipt{},
	}
}

func (b *mockBackend) NonceAt(_ context.Context, _ common.Address, number *big.Int) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	block := b.head
	if number != nil {
		block = number.Uint64()
	}
	return b.minedNonceAtLocked(block), nil
}

// The seed nonce describes pre-test state. A signed transaction advances account state only at
// its receipt block, so confirmation-anchor reads cannot see a more recent inclusion.
func (b *mockBackend) minedNonceAtLocked(block uint64) uint64 {
	nonce := b.latestNonce
	for _, tx := range append(append([]*types.Transaction(nil), b.sent...), b.attempted...) {
		if receipt := b.receipts[tx.Hash()]; receipt != nil && receipt.BlockNumber != nil && receipt.BlockNumber.Uint64() <= block && tx.Nonce() >= nonce {
			nonce = tx.Nonce() + 1
		}
	}
	return nonce
}

func (b *mockBackend) headerLocked(block uint64) *types.Header {
	// Canonical fake headers have fixed fees so their hashes and ancestry stay coherent when pricing
	// evidence changes. The mutable fee scenario is served independently by FeeHistory.
	header := receiptTestHeader(block)
	if b.reorgedHeader {
		header = forkedReceiptHeader(block, "reorged")
	}
	return header
}

func (b *mockBackend) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return max(b.pendingNonce, b.minedNonceAtLocked(b.head)), nil
}

func (b *mockBackend) FeeHistory(
	_ context.Context,
	blockCount uint64,
	newestBlock *big.Int,
	rewardPercentiles []float64,
) (*ethereum.FeeHistory, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.historyReq = feeHistoryRequest{
		blocks: blockCount, percentiles: append([]float64(nil), rewardPercentiles...),
	}
	if newestBlock != nil {
		b.historyReq.newest = new(big.Int).Set(newestBlock)
	}
	if b.historyErr != nil {
		return nil, b.historyErr
	}
	return uniformFeeHistory(b.head, blockCount, b.baseFee, b.gasUsedRatio, b.reward), nil
}

// uniformFeeHistory is a fee window of count blocks ending at head, each with the same base fee,
// fullness and reward; the next block's base fee is the same.
func uniformFeeHistory(head, count uint64, baseFee *big.Int, gasUsedRatio float64, reward *big.Int) *ethereum.FeeHistory {
	history := &ethereum.FeeHistory{OldestBlock: new(big.Int).SetUint64(head + 1 - count)}
	for range count {
		history.BaseFee = append(history.BaseFee, new(big.Int).Set(baseFee))
		history.GasUsedRatio = append(history.GasUsedRatio, gasUsedRatio)
		history.Reward = append(history.Reward, []*big.Int{new(big.Int).Set(reward)})
	}
	history.BaseFee = append(history.BaseFee, new(big.Int).Set(baseFee))
	return history
}

func (b *mockBackend) HeaderByNumber(_ context.Context, number *big.Int) (*types.Header, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if number != nil {
		return b.headerLocked(number.Uint64()), nil
	}
	if b.reorgOnHeadRead {
		b.reorgedHeader = true
	}
	head := b.head
	if len(b.latestHeads) > 0 {
		head = b.latestHeads[0]
		b.latestHeads = b.latestHeads[1:]
	}
	return b.headerLocked(head), nil
}

// mine advances the head by one block and sets the base fee of the fee window and the next block, so
// a pending call whose fee cap no longer covers it reprices on the next evaluation.
func (b *mockBackend) mine(baseFee *big.Int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.head++
	b.baseFee = new(big.Int).Set(baseFee)
}

func (b *mockBackend) HeaderByHash(_ context.Context, hash common.Hash) (*types.Header, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.headerHashReads++
	for number := b.head; ; number-- {
		header := b.headerLocked(number)
		if header.Hash() == hash {
			return header, nil
		}
		if number == 0 {
			return nil, ethereum.NotFound
		}
	}
}

func (b *mockBackend) EstimateGas(context.Context, ethereum.CallMsg) (uint64, error) {
	b.estimateCalls.Add(1)
	if b.gasEstimate == 0 {
		return 0, errors.New("estimate failed")
	}
	return b.gasEstimate, nil
}

// EstimateGasNextBlock serves the next-block estimate the chain client provides, with the same
// result as a latest-state estimate.
func (b *mockBackend) EstimateGasNextBlock(
	ctx context.Context, call ethereum.CallMsg, _ *types.Header, _ time.Duration,
) (uint64, error) {
	return b.EstimateGas(ctx, call)
}

func (b *mockBackend) SendTransaction(_ context.Context, tx *types.Transaction) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	i := b.sendCalls
	b.sendCalls++
	b.attempted = append(b.attempted, tx)
	if i < len(b.sendErrs) && b.sendErrs[i] != nil {
		return b.sendErrs[i]
	}
	b.sent = append(b.sent, tx)
	b.receipts[tx.Hash()] = successfulReceipt(tx, b.head)
	b.receipts[tx.Hash()].BlockHash = b.headerLocked(b.head).Hash()
	return nil
}

func (b *mockBackend) attemptedTransactions() []*types.Transaction {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*types.Transaction(nil), b.attempted...)
}

func (b *mockBackend) TransactionReceipt(_ context.Context, h common.Hash) (*types.Receipt, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r, ok := b.receipts[h]; ok {
		return r, nil
	}
	return nil, ethereum.NotFound
}

func (b *mockBackend) lastSent() *types.Transaction {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.sent) == 0 {
		return nil
	}
	return b.sent[len(b.sent)-1]
}

// managerCtx carries the manager's logger the way Start does for its worker. Tests that drive a
// lifecycle function directly need it, or the lines those functions log through
// observability.Log(ctx) reach the process default instead of the test's capture logger.
func managerCtx(ctx context.Context, m *Manager) context.Context {
	return observability.WithLogger(ctx, m.log)
}

func startManagerForTest(t *testing.T, m *Manager) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Start(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("transaction manager did not stop")
		}
	})
}

func newTestManager(t *testing.T, b Backend) *Manager {
	t.Helper()
	s, err := signer.NewFromHexKey(testKey)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	m := New(b, s, big.NewInt(11155111), Config{Confirmations: 0, PollInterval: time.Millisecond}, logr.Discard())
	startManagerForTest(t, m)
	return m
}

func TestSend_HappyPath(t *testing.T) {
	b := newMockBackend()
	m := newTestManager(t, b)

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
	// gas = estimate + 5%
	if tx.Gas() != 52_500 {
		t.Fatalf("expected gas 52500 (50000 + 5%%), got %d", tx.Gas())
	}
}

func TestMaxFeePerGasIsOneBumpOverTheHorizonCap(t *testing.T) {
	b := newMockBackend()
	m := newTestManager(t, b)

	fee, err := m.MaxFeePerGas(context.Background())
	if err != nil {
		t.Fatalf("MaxFeePerGas: %v", err)
	}
	// bumpFee(grow(20 gwei, 5 blocks) + the 0.02 gwei tip floor)
	if fee.String() != "40568230590" {
		t.Fatalf("max fee = %s, want one-replacement ceiling 40568230590", fee)
	}
}

func TestFeeSnapshotReadsTheHorizonWindow(t *testing.T) {
	b := newMockBackend()
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())

	snapshot, header, err := m.readFeeSnapshot(t.Context())
	if err != nil {
		t.Fatalf("readFeeSnapshot: %v", err)
	}
	if b.historyReq.blocks != 3 || b.historyReq.newest != nil ||
		len(b.historyReq.percentiles) != 1 || b.historyReq.percentiles[0] != 50 {
		t.Fatalf(
			"fee history request = blocks %d, newest %v, percentiles %v; want 3, latest, [50]",
			b.historyReq.blocks, b.historyReq.newest, b.historyReq.percentiles,
		)
	}
	if snapshot.head != 100 || header.Number.Uint64() != 100 || snapshot.gasLimit != header.GasLimit ||
		snapshot.nextBaseFee.Cmp(b.baseFee) != 0 || len(snapshot.blocks) != 3 {
		t.Fatalf("snapshot = %+v at header %d", snapshot, header.Number)
	}
	b.historyErr = errors.New("fee history unavailable")
	if _, _, err := m.readFeeSnapshot(t.Context()); !errors.Is(err, errFreshFeesUnavailable) {
		t.Fatalf("history error = %v, want fresh-fees error", err)
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

func TestValidateFeeHeadroom(t *testing.T) {
	tests := []struct {
		name    string
		maxFee  float64
		tipCap  float64
		wantErr bool
	}{
		{name: "default congested tip cap", maxFee: 50},
		{name: "cap one wei below reserved cap", maxFee: 50, tipCap: 44.444444443},
		{name: "cap equals reserved cap", maxFee: 50, tipCap: 44.444444444, wantErr: true},
		{name: "cap one wei above reserved cap", maxFee: 50, tipCap: 44.444444445, wantErr: true},
		{name: "cap above the global fee cap", maxFee: 50, tipCap: 60, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := &Manager{
				cfg:     Config{MaxFeeGwei: test.maxFee},
				horizon: newHorizonPolicy(HorizonConfig{CongestedTipCapGwei: test.tipCap}),
			}
			err := m.ValidateFeeHeadroom()
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateFeeHeadroom() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestSend_ReservesReplacementHeadroomInsideRequestCap(t *testing.T) {
	b := newMockBackend()
	m := newTestManager(t, b)

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
	wantInitialCap := reserveFeeBump(big.NewInt(40_000_000_000))
	if tx.GasFeeCap().Cmp(wantInitialCap) != 0 {
		t.Fatalf("gas fee cap = %s, want replacement-reserved cap %s", tx.GasFeeCap(), wantInitialCap)
	}
	if tx.GasTipCap().Cmp(gweiToWei(defaultHorizonTipFloorGwei)) != 0 {
		t.Fatalf("gas tip cap = %s, want the tip floor", tx.GasTipCap())
	}
}

func TestSend_RejectsRequestCapWithoutReplacementHeadroom(t *testing.T) {
	b := newMockBackend()
	m := newTestManager(t, b)

	res := m.Send(context.Background(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "capped",
		MaxFeePerGas: big.NewInt(20_500_000_000),
	})
	if res.Err == nil {
		t.Fatal("expected request cap without replacement headroom to fail")
	}
	if res.NotAdmitted {
		t.Fatal("fee failure was classified as a manager admission failure")
	}
	if tx := b.lastSent(); tx != nil {
		t.Fatalf("underfunded request sent transaction %s", tx.Hash())
	}
}

func TestBroadcastRejectsExpiredRequest(t *testing.T) {
	b := newMockBackend()
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
	_, err := m.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000,
		Deadline: time.Now().Add(-time.Second), Label: "expired",
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
	if !errors.Is(err, ErrRequestObsolete) {
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
			if got := m.broadcastTimeout(); got != test.want {
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
	startManagerForTest(t, m)

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
	if m.LaneReady() {
		t.Fatal("uninitialized manager lane is ready")
	}
	if err := m.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
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
				To: common.HexToAddress("0xabc"), Deadline: time.Now().Add(-time.Second), Label: "expired",
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
			Horizon:             HorizonConfig{BlockTime: 4 * time.Millisecond},
		},
		logr.Discard(),
	)
	feeCap, err := m.MaxFeePerGas(t.Context())
	if err != nil {
		t.Fatalf("MaxFeePerGas: %v", err)
	}
	startManagerForTest(t, m)

	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), Data: []byte{0x01}, GasLimit: 21_000,
		MaxFeePerGas: feeCap, Label: "replace",
	})
	if !accepted {
		t.Fatal("SendAsync was not accepted")
	}
	waitForSentTransactions(t, b.mockBackend, 1)
	b.mine(gweiToWei(33)) // a base fee the initial fee cap no longer covers two blocks ahead
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

	m.tryReplace(t.Context(), pending, replaceIntent{})
	firstBump := bumpFee(original.maxFee)
	if pending.fees.maxFee.Cmp(firstBump) != 0 {
		t.Fatalf("ambiguous replacement max fee = %s, want %s", pending.fees.maxFee, firstBump)
	}
	if len(pending.attempts) != 1 || !pending.attempts[0].exactRebroadcastPending {
		t.Fatalf("ambiguous replacement attempts = %+v", pending.attempts)
	}
	firstHash := pending.attempts[0].hash

	m.tryReplace(t.Context(), pending, replaceIntent{})
	attempted := b.attemptedTransactions()
	if len(attempted) != 2 || attempted[0].Hash() != firstHash || attempted[1].Hash() != firstHash ||
		len(pending.attempts) != 1 || pending.attempts[0].exactRebroadcastPending {
		t.Fatalf("exact replacement retry = %v, pending %+v", transactionHashes(attempted), pending)
	}

	m.tryReplace(t.Context(), pending, replaceIntent{})
	if len(pending.attempts) != 2 {
		t.Fatalf("post-retry replacement attempts = %+v", pending.attempts)
	}
	wantMaxFee := bumpFee(firstBump)
	if pending.fees.maxFee.Cmp(wantMaxFee) != 0 {
		t.Fatalf("post-retry replacement max fee = %s, want %s", pending.fees.maxFee, wantMaxFee)
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

	m.tryReplace(t.Context(), pending, replaceIntent{})
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
		BroadcastTimeout: 5 * time.Second, Horizon: HorizonConfig{BlockTime: 5 * time.Second},
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
			&pendingTransaction{deadline: test.deadline}, now,
		); got != test.want {
			t.Errorf("%s: slack = %v, want %v", name, got, test.want)
		}
	}
}

func TestCappedNormalRebroadcastStopsAtDeadline(t *testing.T) {
	b := &cappedRebroadcastDeadlineBackend{mockBackend: newMockBackend()}
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{MaxFeeGwei: 50}, logr.Discard())
	pending, err := m.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "capped deadline",
	})
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	pending.deadline = time.Now().Add(time.Second)
	if !m.rebroadcastLatestAttempt(t.Context(), pending) ||
		!b.deadlineOK || !b.deadline.Equal(pending.deadline) {
		t.Fatalf("capped rebroadcast deadline = (%s, %v), want %s", b.deadline, b.deadlineOK, pending.deadline)
	}
}

func TestReplacementFeesRespectCapAndFullBump(t *testing.T) {
	quote := func(baseFee, tip, maxFee float64) feeQuote {
		return feeQuote{baseFee: gweiToWei(baseFee), tip: gweiToWei(tip), maxFee: gweiToWei(maxFee)}
	}
	// congestedReward, when set, makes every recent block full at that market reward, so the fresh
	// tip follows it (up to a raised congested cap); otherwise blocks have room and the fresh tip is
	// the floor.
	tests := map[string]struct {
		previous        feeQuote
		baseFee         float64
		congestedReward float64
		want            feeQuote
		wantErr         bool
	}{
		"fresh tip is bounded by the cap": {
			previous: quote(20, 1, 44), baseFee: 20, congestedReward: 40, want: quote(20, 30, 50),
		},
		"raw tip bump may exceed effective headroom": {
			previous: quote(20, 10, 44), baseFee: 39.5, want: quote(39.5, 11.25, 50),
		},
		"max fee bump does not fit": {
			previous: quote(20, 1, 45), baseFee: 20, wantErr: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			b := newMockBackend()
			b.baseFee = gweiToWei(test.baseFee)
			if test.congestedReward > 0 {
				b.gasUsedRatio, b.reward = 1, gweiToWei(test.congestedReward)
			}
			m := New(b, mustSigner(t), big.NewInt(11155111), Config{
				Horizon: HorizonConfig{CongestedTipCapGwei: 45},
			}, logr.Discard())

			got, err := m.nextReplacementFees(t.Context(), test.previous, gweiToWei(50), 21_000)
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

func TestReceiptReorgKeepsLifecyclePending(t *testing.T) {
	tests := map[string]func(*mockBackend) Backend{
		"receipt disappears": func(b *mockBackend) Backend {
			return &disappearingReceiptBackend{mockBackend: b}
		},
		"receipt reorgs during head read": func(b *mockBackend) Backend {
			b.reorgOnHeadRead = true
			return b
		},
		"receipt block is no longer canonical": func(b *mockBackend) Backend {
			b.reorgedHeader = true
			return b
		},
	}
	for name, backend := range tests {
		t.Run(name, func(t *testing.T) {
			b := newMockBackend()
			m := New(
				backend(b), mustSigner(t), big.NewInt(11155111),
				Config{Confirmations: 2, PollInterval: time.Millisecond}, logr.Discard(),
			)
			tx := types.NewTx(&types.DynamicFeeTx{
				ChainID: big.NewInt(11155111), Nonce: 7, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
				Gas: 21_000, To: ptr(common.HexToAddress("0xabc")),
			})
			b.receipts[tx.Hash()] = successfulReceipt(tx, b.head-2)
			pending := &pendingTransaction{
				req: Request{Label: "reorged"}, nonce: 7,
				attempts: []txAttempt{{hash: tx.Hash(), tx: tx}},
			}
			m.trackUnminedTransaction(pending)

			if result, done := m.receiptResult(t.Context(), pending); done {
				t.Fatalf("reorged receipt completed lifecycle: %+v", result)
			}
			m.unminedMu.Lock()
			tracked := m.unmined == pending
			m.unminedMu.Unlock()
			if !tracked {
				t.Fatal("reorged lifecycle lost active ownership")
			}
		})
	}
}

func TestConfirmationsRequireStableHead(t *testing.T) {
	b := newMockBackend()
	b.head = 102
	b.latestHeads = []uint64{102, 100, 102, 102}
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(11155111), Nonce: 7, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
		Gas: 21_000, To: ptr(common.HexToAddress("0xabc")),
	})
	receipt := successfulReceipt(tx, 100)
	b.receipts[tx.Hash()] = receipt
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{Confirmations: 2, PollInterval: time.Millisecond}, logr.Discard(),
	)

	got, err := m.waitForConfirmations(t.Context(), tx.Hash(), receipt, 2)
	if err != nil || got != receipt {
		t.Fatalf("waitForConfirmations = (%+v, %v), want stable confirmed receipt", got, err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.latestHeads) != 0 {
		t.Fatalf("confirmation returned before stable head snapshot; unread heads = %v", b.latestHeads)
	}
	if b.headerHashReads != 4 {
		t.Fatalf("ancestry reads = %d, want 4 before both final head checks", b.headerHashReads)
	}
}

func TestConfirmationsRejectReceiptFromDifferentFork(t *testing.T) {
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(11155111), Nonce: 7, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
		Gas: 21_000, To: ptr(common.HexToAddress("0xabc")),
	})
	receipt := successfulReceipt(tx, 100)
	receipt.BlockHash = forkedReceiptHeader(100, "fallback").Hash()
	backend := &mixedForkBackend{mockBackend: newMockBackend()}
	backend.receipts[tx.Hash()] = receipt
	m := New(
		backend, mustSigner(t), big.NewInt(11155111),
		Config{Confirmations: 2, PollInterval: time.Millisecond}, logr.Discard(),
	)

	got, err := m.waitForConfirmations(t.Context(), tx.Hash(), receipt, 2)
	if got != receipt || !errors.Is(err, errReceiptReorged) {
		t.Fatalf("waitForConfirmations = (%+v, %v), want reorg error", got, err)
	}
}

func TestTransientReceiptErrorKeepsTrackingPendingTransaction(t *testing.T) {
	b := &receiptErrorBackend{mockBackend: newMockBackend(), receiptFailures: 1}
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
	startManagerForTest(t, m)

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

func TestReceiptLookupTimeoutDoesNotStarveOlderAttempt(t *testing.T) {
	b := newMockBackend()
	older := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(11155111), Nonce: 7, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
		Gas: 21_000, To: ptr(common.HexToAddress("0xabc")),
	})
	newest := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(11155111), Nonce: 7, GasTipCap: big.NewInt(2), GasFeeCap: big.NewInt(3),
		Gas: 21_000, To: ptr(common.HexToAddress("0xabc")),
	})
	b.receipts[older.Hash()] = successfulReceipt(older, b.head)
	backend := &blockedReceiptHashBackend{mockBackend: b, hash: newest.Hash()}
	m := New(
		backend, mustSigner(t), big.NewInt(11155111),
		Config{ReplacementInterval: 2 * time.Millisecond}, logr.Discard(),
	)
	pending := &pendingTransaction{
		req: Request{Label: "fair receipt lookup"}, nonce: 7,
		attempts: []txAttempt{{hash: older.Hash()}, {hash: newest.Hash()}},
		// Exercise the slow newest hash first; the older attempt still gets its own budget.
		receiptCursor: 1,
	}
	result, done := m.receiptResult(t.Context(), pending)
	if !done || result.Err != nil || result.Hash != older.Hash() || result.Receipt != b.receipts[older.Hash()] {
		t.Fatalf("older mined attempt result = (%+v, %v)", result, done)
	}
}

func TestMalformedReceiptDoesNotCompleteLifecycle(t *testing.T) {
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(11155111), Nonce: 7, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
		Gas: 21_000, To: ptr(common.HexToAddress("0xabc")),
	})
	tests := map[string]func(*types.Receipt){
		"mismatched transaction hash": func(receipt *types.Receipt) {
			receipt.TxHash = common.HexToHash("0x1234")
		},
		"missing block hash": func(receipt *types.Receipt) {
			receipt.BlockHash = common.Hash{}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			b := newMockBackend()
			receipt := successfulReceipt(tx, b.head)
			mutate(receipt)
			b.receipts[tx.Hash()] = receipt
			m := New(b, mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
			pending := &pendingTransaction{
				req: Request{Label: "malformed receipt"}, nonce: 7,
				attempts: []txAttempt{{hash: tx.Hash()}},
			}
			m.trackUnminedTransaction(pending)

			if result, done := m.receiptResult(t.Context(), pending); done {
				t.Fatalf("malformed receipt completed lifecycle: %+v", result)
			}
			m.unminedMu.Lock()
			tracked := m.unmined == pending
			m.unminedMu.Unlock()
			if !tracked {
				t.Fatal("malformed receipt released the pending nonce")
			}
		})
	}
}

func TestTransientConfirmationHeadErrorKeepsTrackingPendingTransaction(t *testing.T) {
	b := &transientHeadErrorBackend{mockBackend: newMockBackend()}
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{Confirmations: 2, PollInterval: time.Millisecond},
		logr.Discard(),
	)
	startManagerForTest(t, m)

	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "confirmation retry",
	})
	if !accepted {
		t.Fatal("transaction was not accepted")
	}
	waitForSentTransactions(t, b.mockBackend, 1)
	b.errorMu.Lock()
	b.blockFailures = 1
	b.errorMu.Unlock()
	b.mu.Lock()
	b.head = 102
	b.mu.Unlock()

	if got := <-result; got.Err != nil || got.Outcome != OutcomeConfirmed {
		t.Fatalf("confirmation retry result: %+v", got)
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

func waitForAdmissionDemand(t *testing.T, m *Manager, want int64) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := m.admissionDemand.Load(); got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("admission demand = %d, want %d", m.admissionDemand.Load(), want)
}

type receiptErrorBackend struct {
	*mockBackend

	errorMu         sync.Mutex
	receiptFailures int
}

type transientHeadErrorBackend struct {
	*mockBackend

	errorMu       sync.Mutex
	blockFailures int
}

type blockedReceiptHashBackend struct {
	*mockBackend

	hash common.Hash
}

type disappearingReceiptBackend struct {
	*mockBackend

	receiptReads atomic.Int64
}

type mixedForkBackend struct{ *mockBackend }

func (b *mixedForkBackend) HeaderByNumber(_ context.Context, number *big.Int) (*types.Header, error) {
	height := uint64(102)
	if number != nil {
		height = number.Uint64()
	}
	return forkedReceiptHeader(height, "primary"), nil
}

func (b *mixedForkBackend) HeaderByHash(_ context.Context, hash common.Hash) (*types.Header, error) {
	for number := uint64(102); ; number-- {
		header := forkedReceiptHeader(number, "primary")
		if header.Hash() == hash {
			return header, nil
		}
		if number == 0 {
			return nil, ethereum.NotFound
		}
	}
}

var receiptHeaderCache sync.Map

func forkedReceiptHeader(number uint64, fork string) *types.Header {
	key := struct {
		number uint64
		fork   string
	}{number: number, fork: fork}
	if cached, ok := receiptHeaderCache.Load(key); ok {
		return types.CopyHeader(cached.(*types.Header))
	}
	header := &types.Header{
		Number: new(big.Int).SetUint64(number), BaseFee: big.NewInt(20e9), GasLimit: 60_000_000, Time: number * 12,
	}
	if number > 0 {
		header.ParentHash = forkedReceiptHeader(number-1, fork).Hash()
	}
	if fork != "" {
		header.Extra = []byte(fork)
	}
	actual, _ := receiptHeaderCache.LoadOrStore(key, header)
	return types.CopyHeader(actual.(*types.Header))
}

func (b *disappearingReceiptBackend) TransactionReceipt(
	ctx context.Context,
	hash common.Hash,
) (*types.Receipt, error) {
	if b.receiptReads.Add(1) > 1 {
		return nil, ethereum.NotFound
	}
	return b.mockBackend.TransactionReceipt(ctx, hash)
}

type replacementNonceRaceBackend struct {
	*mockBackend

	publishOwnedReceipt bool
	firstSendErr        error
}

type cappedRebroadcastDeadlineBackend struct {
	*mockBackend

	deadline   time.Time
	deadlineOK bool
}

func (b *cappedRebroadcastDeadlineBackend) SendTransaction(
	ctx context.Context,
	tx *types.Transaction,
) error {
	b.deadline, b.deadlineOK = ctx.Deadline()
	return b.mockBackend.SendTransaction(ctx, tx)
}

func (b *replacementNonceRaceBackend) SendTransaction(
	_ context.Context,
	tx *types.Transaction,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sendCalls++
	b.sent = append(b.sent, tx)
	b.attempted = append(b.attempted, tx)
	if b.sendCalls == 1 {
		return b.firstSendErr
	}
	if b.publishOwnedReceipt {
		original := b.sent[0]
		b.receipts[original.Hash()] = successfulReceipt(original, b.head)
	}
	b.latestNonce = tx.Nonce() + 1
	b.pendingNonce = tx.Nonce() + 1
	return errors.New("nonce too low")
}

type blockingTxSigner struct {
	signer.Signer

	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockingTxSigner) SignTx(
	ctx context.Context,
	tx *types.Transaction,
	chainID *big.Int,
) (*types.Transaction, error) {
	s.once.Do(func() { close(s.entered) })
	select {
	case <-s.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return s.Signer.SignTx(ctx, tx, chainID)
}

func (b *blockedReceiptHashBackend) TransactionReceipt(
	ctx context.Context,
	hash common.Hash,
) (*types.Receipt, error) {
	if hash == b.hash {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return b.mockBackend.TransactionReceipt(ctx, hash)
}

func (b *receiptErrorBackend) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	b.errorMu.Lock()
	if b.receiptFailures > 0 {
		b.receiptFailures--
		b.errorMu.Unlock()
		return nil, errors.New("temporary receipt failure")
	}
	b.errorMu.Unlock()
	return b.mockBackend.TransactionReceipt(ctx, hash)
}

func (b *transientHeadErrorBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	b.errorMu.Lock()
	if b.blockFailures > 0 {
		b.blockFailures--
		b.errorMu.Unlock()
		return nil, errors.New("temporary head failure")
	}
	b.errorMu.Unlock()
	return b.mockBackend.HeaderByNumber(ctx, number)
}

type replacementBackend struct {
	*mockBackend

	receiptOnSameNonce int
	sameNonceSends     int
}

func (b *replacementBackend) SendTransaction(_ context.Context, tx *types.Transaction) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sendCalls++
	b.sent = append(b.sent, tx)

	if tx.Nonce() == b.pendingNonce {
		b.sameNonceSends++
		if b.receiptOnSameNonce > 0 && b.sameNonceSends >= b.receiptOnSameNonce {
			b.receipts[tx.Hash()] = successfulReceipt(tx, b.head)
		}
		return nil
	}
	b.receipts[tx.Hash()] = successfulReceipt(tx, b.head)
	return nil
}

func successfulReceipt(tx *types.Transaction, block uint64) *types.Receipt {
	return &types.Receipt{
		Status:      types.ReceiptStatusSuccessful,
		TxHash:      tx.Hash(),
		BlockHash:   receiptTestHeader(block).Hash(),
		BlockNumber: new(big.Int).SetUint64(block),
		GasUsed:     tx.Gas(),
	}
}

func receiptTestHeader(block uint64) *types.Header {
	return forkedReceiptHeader(block, "")
}

func TestLaneStateSubscriptionsFanOutWithoutStealingEdges(t *testing.T) {
	m := New(newMockBackend(), mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
	first, unsubscribeFirst := m.SubscribeLaneState()
	second, unsubscribeSecond := m.SubscribeLaneState()
	defer unsubscribeSecond()

	m.notifyLaneStateChange()
	for name, changes := range map[string]<-chan struct{}{"first": first, "second": second} {
		select {
		case <-changes:
		case <-time.After(time.Second):
			t.Fatalf("%s subscriber did not receive pause edge", name)
		}
	}

	unsubscribeFirst()
	m.notifyLaneStateChange()
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
	<-laneStateChanges // Successful first initialization publishes readiness, not a conflict pause.

	m.tryReplace(t.Context(), pending, replaceIntent{})
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
	originalHash, originalFees := pending.originalHash, cloneFeeQuote(pending.fees)
	m.tryReplace(t.Context(), pending, replaceIntent{})
	attempted := b.attemptedTransactions()
	if len(attempted) != 2 || attempted[0].Hash() != originalHash || attempted[1].Hash() != originalHash ||
		len(pending.attempts) != 1 || pending.attempts[0].exactRebroadcastPending {
		t.Fatalf("exact retry = %v, attempts %+v", transactionHashes(attempted), pending.attempts)
	}
	if pending.fees.maxFee.Cmp(originalFees.maxFee) != 0 || pending.fees.tip.Cmp(originalFees.tip) != 0 {
		t.Fatalf("exact retry changed fees: got %+v want %+v", pending.fees, originalFees)
	}
	delete(b.receipts, originalHash) // Exact acceptance still awaits inclusion in this replacement fixture.
	m.tryReplace(t.Context(), pending, replaceIntent{})
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

type cancelOnConfirmationHeadBackend struct {
	*mockBackend

	armed  bool
	cancel func()
}

func (b *cancelOnConfirmationHeadBackend) HeaderByNumber(
	ctx context.Context,
	number *big.Int,
) (*types.Header, error) {
	if b.armed && b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	return b.mockBackend.HeaderByNumber(ctx, number)
}

func TestReceiptResultFailedReceiptWinsOverInterruptedConfirmation(t *testing.T) {
	for _, test := range []struct {
		name string
	}{
		{name: "normal transaction"},
	} {
		t.Run(test.name, func(t *testing.T) {
			logs, logger := newLogCapture(1)
			backend := newMockBackend()
			to := common.HexToAddress("0xabc")
			tx := types.NewTx(&types.DynamicFeeTx{
				ChainID: big.NewInt(11155111), Nonce: 7, Gas: 21_000, To: &to,
				GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
			})
			receipt := successfulReceipt(tx, backend.head)
			receipt.Status = types.ReceiptStatusFailed
			backend.receipts[tx.Hash()] = receipt

			confirmationCtx, cancelConfirmation := context.WithCancelCause(t.Context())
			interruptingBackend := &cancelOnConfirmationHeadBackend{
				mockBackend: backend,
				armed:       true,
				cancel:      func() { cancelConfirmation(context.Canceled) },
			}
			manager := New(
				interruptingBackend,
				mustSigner(t),
				big.NewInt(11155111),
				Config{Confirmations: 1, PollInterval: time.Millisecond},
				logger,
			)
			pending := &pendingTransaction{
				req:   Request{To: to, Data: []byte("request-authorization"), Label: "failed receipt"},
				nonce: 7,
				attempts: []txAttempt{{
					hash: tx.Hash(), tx: tx,
				}},
			}

			result, done := manager.receiptResult(managerCtx(confirmationCtx, manager), pending)
			if !done {
				t.Fatal("failed receipt did not complete the lifecycle")
			}
			if result.Outcome != OutcomeReverted {
				t.Fatalf("outcome = %q, want %q", result.Outcome, OutcomeReverted)
			}
			if result.Receipt != receipt {
				t.Fatalf("receipt = %+v, want failed receipt %+v", result.Receipt, receipt)
			}
			if !errors.Is(result.Err, context.Canceled) {
				t.Fatalf("error = %v, want interrupted confirmation cause", result.Err)
			}
			if result.Outcome.Included() {
				t.Fatal("reverted receipt was classified as a successful inclusion")
			}
			joined := strings.Join(*logs, "\n")
			if !strings.Contains(joined, "transaction reverted") || !strings.Contains(joined, tx.Hash().Hex()) {
				t.Fatalf("missing revert diagnostics: %s", joined)
			}
			if strings.Contains(joined, "tenderly") || strings.Contains(joined, "request-authorization") {
				t.Fatalf("revert log contains calldata: %s", joined)
			}
		})
	}
}

func TestOutcomeIncluded(t *testing.T) {
	tests := []struct {
		outcome Outcome
		want    bool
	}{
		{outcome: OutcomeConfirmed, want: true},
		{outcome: OutcomeIncludedUnconfirmed, want: true},
		{outcome: OutcomeReverted},
		{outcome: OutcomeAbandoned},
		{outcome: OutcomeSubmissionError},
		{outcome: OutcomeTrackingStopped},
		{outcome: ""},
	}
	for _, test := range tests {
		if got := test.outcome.Included(); got != test.want {
			t.Fatalf("%q.Included() = %t, want %t", test.outcome, got, test.want)
		}
	}
}

// revertingBackend records a failed receipt instead of a successful one.
type revertingBackend struct{ *mockBackend }

func (b *revertingBackend) SendTransaction(_ context.Context, tx *types.Transaction) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent = append(b.sent, tx)
	receipt := successfulReceipt(tx, b.head)
	receipt.Status = types.ReceiptStatusFailed
	b.receipts[tx.Hash()] = receipt
	return nil
}

// blockingBackend parks inside SendTransaction until released, so a test can cancel the caller's
// context while a transaction is mid-broadcast on the worker.
type blockingBackend struct {
	*mockBackend

	entered chan struct{}
	release chan struct{}
}

type blockingEstimateBackend struct {
	*mockBackend

	entered chan struct{}
}

func (b *blockingBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	close(b.entered)
	<-b.release
	return b.mockBackend.SendTransaction(ctx, tx)
}

func (b *blockingEstimateBackend) EstimateGas(ctx context.Context, _ ethereum.CallMsg) (uint64, error) {
	close(b.entered)
	<-ctx.Done()
	return 0, ctx.Err()
}

func (b *blockingEstimateBackend) EstimateGasNextBlock(
	ctx context.Context, call ethereum.CallMsg, _ *types.Header, _ time.Duration,
) (uint64, error) {
	return b.EstimateGas(ctx, call)
}

// TestSend_CallerCancelAfterEnqueueStillReturnsResult guards the fund-moving invariant: once a
// request is enqueued the worker broadcasts it on the manager's context, so Send must report that
// real outcome. Cancelling the caller's context after enqueue must NOT make Send return a
// cancellation while the tx still lands on-chain (which would read as "not sent").
func TestSend_CallerCancelAfterEnqueueStillReturnsResult(t *testing.T) {
	bb := &blockingBackend{mockBackend: newMockBackend(), entered: make(chan struct{}), release: make(chan struct{})}
	m := New(bb, mustSigner(t), big.NewInt(11155111), Config{PollInterval: time.Millisecond}, logr.Discard())
	startManagerForTest(t, m) // manager context lives until test cleanup; the caller's is cancelled below

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

func TestTrySendRejectsWhileTransactionIsActive(t *testing.T) {
	bb := &blockingBackend{mockBackend: newMockBackend(), entered: make(chan struct{}), release: make(chan struct{})}
	m := New(bb, mustSigner(t), big.NewInt(11155111), Config{PollInterval: time.Millisecond}, logr.Discard())
	startManagerForTest(t, m)

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

func transactionHashes(transactions []*types.Transaction) []common.Hash {
	hashes := make([]common.Hash, len(transactions))
	for i, transaction := range transactions {
		hashes[i] = transaction.Hash()
	}
	return hashes
}

func ptr[T any](value T) *T {
	return &value
}

func TestSendSpansNestUnderCaller(t *testing.T) {
	rec := tracetest.Install(t)
	b := newMockBackend()
	m := newTestManager(t, b)

	ctx, parent := otel.Tracer("caller").Start(t.Context(), "rfq.order.submit")
	res := m.Send(ctx, Request{
		To: common.HexToAddress("0xabc"), Label: "test-fill", Solver: "rfq", GasLimit: 21_000,
	})
	parent.End()
	if res.Outcome != OutcomeConfirmed {
		t.Fatalf("outcome %v err %v", res.Outcome, res.Err)
	}

	send := tracetest.Ended(t, rec, "txmanager.send test-fill")
	broadcast := tracetest.Ended(t, rec, "txmanager.broadcast")
	if send.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatal("send span is not a child of the caller span")
	}
	if broadcast.Parent().SpanID() != send.SpanContext().SpanID() {
		t.Fatal("broadcast span is not a child of the send span")
	}
	if tracetest.Attr(send, "tx.outcome") != "confirmed" || tracetest.Attr(send, "solver") != "rfq" ||
		tracetest.Attr(send, "tx.label") != "test-fill" || tracetest.Attr(send, "tx.hash") != res.Hash.Hex() ||
		tracetest.Attr(send, "tx.nonce") != "7" {
		t.Fatalf("send attributes: %v", send.Attributes())
	}
	if send.Status().Code != codes.Unset {
		t.Fatalf("send status = %v, want unset for a confirmed transaction", send.Status())
	}
	if tracetest.Attr(broadcast, "tx.hash") != res.Hash.Hex() || tracetest.Attr(broadcast, "tx.nonce") != "7" {
		t.Fatalf("broadcast attributes: %v", broadcast.Attributes())
	}
}

func TestSendSpanRecordsBroadcastFailure(t *testing.T) {
	rec := tracetest.Install(t)
	b := newMockBackend()
	b.sendErrs = []error{errors.New("insufficient funds for gas * price + value")}
	m := newTestManager(t, b)

	res := m.Send(t.Context(), Request{
		To: common.HexToAddress("0xabc"), Label: "rejected", Solver: "rfq", GasLimit: 21_000,
	})
	if res.Outcome != OutcomeSubmissionError || res.Err == nil {
		t.Fatalf("outcome %v err %v, want a submission error", res.Outcome, res.Err)
	}

	send := tracetest.Ended(t, rec, "txmanager.send rejected")
	if tracetest.Attr(send, "tx.outcome") != string(OutcomeSubmissionError) {
		t.Fatalf("send attributes: %v", send.Attributes())
	}
	if send.Status().Code != codes.Error {
		t.Fatalf("send status = %v, want Error", send.Status())
	}
	if broadcast := tracetest.Ended(t, rec, "txmanager.broadcast"); broadcast.Status().Code != codes.Error {
		t.Fatalf("broadcast status = %v, want Error", broadcast.Status())
	}
}

func TestTrySendBusyLaneDeclinesWithoutErrorStatus(t *testing.T) {
	rec := tracetest.Install(t)
	bb := &blockingBackend{mockBackend: newMockBackend(), entered: make(chan struct{}), release: make(chan struct{})}
	m := New(bb, mustSigner(t), big.NewInt(11155111), Config{PollInterval: time.Millisecond}, logr.Discard())
	startManagerForTest(t, m)

	first := make(chan Result, 1)
	go func() {
		result, _ := m.TrySend(
			context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "first"},
		)
		first <- result
	}()

	<-bb.entered
	if _, accepted := m.TrySend(
		context.Background(), Request{To: common.HexToAddress("0xdef"), GasLimit: 21_000, Label: "second"},
	); accepted {
		t.Fatal("busy lane accepted a TrySend")
	}
	close(bb.release)
	if got := <-first; got.Err != nil {
		t.Fatalf("first TrySend: %v", got.Err)
	}

	declined := tracetest.Ended(t, rec, "txmanager.send second")
	if declined.Status().Code != codes.Unset {
		t.Fatalf("busy-lane status = %v, want unset", declined.Status())
	}
	events := declined.Events()
	if len(events) != 1 || events[0].Name != "declined" {
		t.Fatalf("busy-lane events = %v, want one declined event", events)
	}
}

// The lifecycle logs are what an operator joins to a trace, so each line must carry the caller's
// trace id exactly once: the request logger is derived from the base logger at the send span, never
// from a logger that already carries trace ids.
func TestSendLifecycleLogsCarryTraceIDOnce(t *testing.T) {
	tracetest.Install(t)
	log, capture := tracetest.CaptureLogs(t, 1)
	m := New(newMockBackend(), mustSigner(t), big.NewInt(11155111),
		Config{Confirmations: 0, PollInterval: time.Millisecond}, log)
	startManagerForTest(t, m)

	ctx, parent := otel.Tracer("caller").Start(t.Context(), "rfq.order.submit")
	res := m.Send(ctx, Request{
		To: common.HexToAddress("0xabc"), Label: "test-fill", Solver: "rfq", GasLimit: 21_000,
	})
	parent.End()
	if res.Outcome != OutcomeConfirmed {
		t.Fatalf("outcome %v err %v", res.Outcome, res.Err)
	}

	lines := capture()
	tracetest.RequireTraceIDsOnce(t, lines)
	want := `"trace_id":"` + parent.SpanContext().TraceID().String() + `"`
	// "sent" comes from the worker's broadcast, "transaction confirmed" from the detached lifecycle
	// goroutine: both must reach the request's logger and carry the caller's trace.
	seen := map[string]bool{"sent": false, "transaction confirmed": false}
	for _, line := range lines {
		for msg := range seen {
			if !strings.Contains(line, `"msg":"`+msg+`"`) {
				continue
			}
			seen[msg] = true
			if !strings.Contains(line, want) {
				t.Fatalf("%q line does not carry the caller's trace id: %s", msg, line)
			}
			if !strings.Contains(line, `"solver":"rfq"`) {
				t.Fatalf("%q line lost the request's solver: %s", msg, line)
			}
		}
	}
	for msg, ok := range seen {
		if !ok {
			t.Fatalf("no %q line captured: %v", msg, lines)
		}
	}
}

// A manager that has stopped withdraws the request rather than rejecting it. Every other span
// records that as a cancelled event, so the send span must not colour the caller's trace red.
func TestSendSpanRecordsManagerStopAsCancellation(t *testing.T) {
	rec := tracetest.Install(t)
	s, err := signer.NewFromHexKey(testKey)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	m := New(newMockBackend(), s, big.NewInt(11155111),
		Config{Confirmations: 0, PollInterval: time.Millisecond, ShutdownTimeout: time.Second}, logr.Discard())
	managerCtx, cancelManager := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() {
		m.Start(managerCtx)
		close(stopped)
	}()
	cancelManager()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("transaction manager did not stop")
	}

	res := m.Send(t.Context(), Request{
		To: common.HexToAddress("0xabc"), Label: "after stop", Solver: "rfq", GasLimit: 21_000,
	})
	if !errors.Is(res.Err, errManagerStopped) {
		t.Fatalf("send after stop = %+v, want the manager stop", res)
	}

	send := tracetest.Ended(t, rec, "txmanager.send after stop")
	if send.Status().Code != codes.Unset {
		t.Fatalf("send status = %v, want unset for a withdrawn request", send.Status())
	}
	if !tracetest.HasEvent(send, "cancelled") {
		t.Fatalf("send span has no cancelled event: %v", send.Events())
	}
	if got := tracetest.Attr(send, "tx.outcome"); got != string(OutcomeSubmissionError) {
		t.Fatalf("send tx.outcome = %q, want %q", got, OutcomeSubmissionError)
	}
}

// A Deadline deadline reached while the request waits for the nonce lane is the caller withdrawing
// it, not the manager failing it.
func TestSendSpanRecordsRequestDeadlineAsCancellation(t *testing.T) {
	rec := tracetest.Install(t)
	bb := &blockingBackend{mockBackend: newMockBackend(), entered: make(chan struct{}), release: make(chan struct{})}
	m := New(bb, mustSigner(t), big.NewInt(11155111), Config{PollInterval: time.Millisecond}, logr.Discard())
	startManagerForTest(t, m)

	first := make(chan Result, 1)
	go func() {
		first <- m.Send(
			context.Background(), Request{To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "holder"},
		)
	}()
	<-bb.entered

	res := m.Send(t.Context(), Request{
		To: common.HexToAddress("0xdef"), GasLimit: 21_000, Label: "expiring",
		Deadline: time.Now().Add(50 * time.Millisecond),
	})
	if !errors.Is(res.Err, context.DeadlineExceeded) {
		t.Fatalf("expiring send = %+v, want its Deadline deadline", res)
	}
	close(bb.release)
	if got := <-first; got.Err != nil {
		t.Fatalf("holder send: %v", got.Err)
	}

	send := tracetest.Ended(t, rec, "txmanager.send expiring")
	if send.Status().Code != codes.Unset {
		t.Fatalf("send status = %v, want unset for a withdrawn request", send.Status())
	}
	if !tracetest.HasEvent(send, "cancelled") {
		t.Fatalf("send span has no cancelled event: %v", send.Events())
	}
}
