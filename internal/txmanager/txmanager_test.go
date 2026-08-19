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

	latestNonce   uint64
	pendingNonce  uint64
	history       *ethereum.FeeHistory
	historyErr    error
	historyReq    feeHistoryRequest
	tip           *big.Int
	tipErr        error
	tipCalls      int
	baseFee       *big.Int
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
		history:      constantFeeHistory(big.NewInt(1e9)),
		tip:          big.NewInt(1e9),
		baseFee:      big.NewInt(20e9),
		gasEstimate:  50_000,
		head:         100,
		receipts:     map[common.Hash]*types.Receipt{},
	}
}

func (b *mockBackend) NonceAt(context.Context, common.Address, *big.Int) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.latestNonce, nil
}

func (b *mockBackend) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.pendingNonce, nil
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
	return b.history, b.historyErr
}

func constantFeeHistory(reward *big.Int) *ethereum.FeeHistory {
	history := &ethereum.FeeHistory{Reward: make([][]*big.Int, feeHistoryBlocks)}
	for i := range history.Reward {
		history.Reward[i] = []*big.Int{new(big.Int).Set(reward)}
	}
	return history
}

func (b *mockBackend) SuggestGasTipCap(context.Context) (*big.Int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tipCalls++
	return b.tip, b.tipErr
}

func (b *mockBackend) HeaderByNumber(_ context.Context, number *big.Int) (*types.Header, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if number != nil {
		header := receiptTestHeader(number.Uint64())
		if b.reorgedHeader {
			header = forkedReceiptHeader(number.Uint64(), "reorged")
		}
		return header, nil
	}
	if b.reorgOnHeadRead {
		b.reorgedHeader = true
	}
	head := b.head
	if len(b.latestHeads) > 0 {
		head = b.latestHeads[0]
		b.latestHeads = b.latestHeads[1:]
	}
	header := receiptTestHeader(head)
	if b.reorgedHeader {
		header = forkedReceiptHeader(head, "reorged")
	}
	header.BaseFee = new(big.Int).Set(b.baseFee)
	return header, nil
}

func (b *mockBackend) HeaderByHash(_ context.Context, hash common.Hash) (*types.Header, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.headerHashReads++
	for number := b.head; ; number-- {
		header := receiptTestHeader(number)
		if b.reorgedHeader {
			header = forkedReceiptHeader(number, "reorged")
		}
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

func TestMaxFeePerGasMatchesSendFeePolicy(t *testing.T) {
	b := newMockBackend()
	m := newTestManager(t, b)

	fee, err := m.MaxFeePerGas(context.Background())
	if err != nil {
		t.Fatalf("MaxFeePerGas: %v", err)
	}
	if fee.String() != "46125000000" {
		t.Fatalf("max fee = %s, want one-replacement ceiling 46125000000", fee)
	}
}

func TestTipGweiFloorsNodeSuggestionWithoutBreakingFeeCap(t *testing.T) {
	tests := map[string]struct {
		tip     *big.Int
		tipErr  error
		wantTip int64
	}{
		"low suggestion":         {tip: big.NewInt(1_500), wantTip: 1_000_000_000},
		"higher suggestion":      {tip: big.NewInt(2_000_000_000), wantTip: 2_000_000_000},
		"suggestion above cap":   {tip: big.NewInt(30_000_000_000), wantTip: 20_500_000_000},
		"suggestion unavailable": {tipErr: context.DeadlineExceeded, wantTip: 1_000_000_000},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			b := newMockBackend()
			b.tip, b.tipErr = test.tip, test.tipErr
			m := New(b, mustSigner(t), big.NewInt(11155111), Config{TipGwei: 1}, logr.Discard())
			limit := big.NewInt(40_500_000_000)

			fees, err := m.currentFees(t.Context(), limit)
			if err != nil {
				t.Fatalf("currentFees: %v", err)
			}
			if fees.tip.Cmp(big.NewInt(test.wantTip)) != 0 {
				t.Fatalf("tip = %s, want %d", fees.tip, test.wantTip)
			}
			if fees.maxFee.Cmp(limit) != 0 {
				t.Fatalf("max fee = %s, want hard cap %s", fees.maxFee, limit)
			}
		})
	}
}

func TestTipGweiZeroUsesEtherscanFastFeeHistoryPolicy(t *testing.T) {
	b := newMockBackend()
	b.history = &ethereum.FeeHistory{Reward: [][]*big.Int{
		{big.NewInt(3_000_000_000)},
		{big.NewInt(500_000_000)},
		{big.NewInt(2_000_000_000)},
		{big.NewInt(1_000_000_000)},
		{big.NewInt(1_500_000_000)},
	}}
	b.tip = big.NewInt(1_500)
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
	limit := big.NewInt(40_500_000_000)

	fees, err := m.currentFees(t.Context(), limit)
	if err != nil {
		t.Fatalf("currentFees: %v", err)
	}
	if want := big.NewInt(500_000_000); fees.tip.Cmp(want) != 0 {
		t.Fatalf("tip = %s, want minimum p25 reward %s", fees.tip, want)
	}
	if b.historyReq.blocks != 5 || b.historyReq.newest != nil ||
		len(b.historyReq.percentiles) != 1 || b.historyReq.percentiles[0] != 25.0 {
		t.Fatalf(
			"fee history request = blocks %d, newest %v, percentiles %v; want 5, latest, [25]",
			b.historyReq.blocks, b.historyReq.newest, b.historyReq.percentiles,
		)
	}
	if fees.maxFee.Cmp(limit) != 0 {
		t.Fatalf("max fee = %s, want hard cap %s", fees.maxFee, limit)
	}
	b.history.Reward[0][0] = new(big.Int)
	fees, err = m.currentFees(t.Context(), limit)
	if err != nil {
		t.Fatalf("currentFees with zero reward: %v", err)
	}
	if fees.tip.Sign() != 0 {
		t.Fatalf("tip = %s, want zero minimum reward", fees.tip)
	}
	b.history = constantFeeHistory(big.NewInt(30_000_000_000))
	fees, err = m.currentFees(t.Context(), limit)
	if err != nil {
		t.Fatalf("currentFees with reward above cap: %v", err)
	}
	if want := big.NewInt(20_500_000_000); fees.tip.Cmp(want) != 0 {
		t.Fatalf("tip = %s, want reward clamped to %s", fees.tip, want)
	}
	b.history.Reward = b.history.Reward[:feeHistoryBlocks-1]
	if _, err := m.currentFees(t.Context(), limit); !errors.Is(err, errFreshFeesUnavailable) {
		t.Fatalf("short fee history error = %v, want fresh-fees error", err)
	}
	b.historyErr = errors.New("fee history unavailable")
	if _, err := m.currentFees(t.Context(), limit); !errors.Is(err, errFreshFeesUnavailable) {
		t.Fatalf("history error = %v, want fresh-fees error", err)
	}
	if b.tipCalls != 0 {
		t.Fatalf("node suggestion called %d times", b.tipCalls)
	}
}

func TestCurrentFeesRejectConfiguredFloorAboveFeeHeadroom(t *testing.T) {
	b := newMockBackend()
	b.tip = big.NewInt(30_000_000_000)
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{TipGwei: 21}, logr.Discard())

	_, err := m.currentFees(t.Context(), big.NewInt(40_500_000_000))
	if err == nil || !strings.Contains(err.Error(), "priority fee floor") {
		t.Fatalf("currentFees error = %v, want configured-floor error", err)
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
		tip     float64
		wantErr bool
	}{
		{name: "automatic tip", maxFee: 50},
		{name: "floor one wei below reserved cap", maxFee: 50, tip: 39.506172838},
		{name: "floor equals reserved cap", maxFee: 50, tip: 39.506172839, wantErr: true},
		{name: "floor one wei above reserved cap", maxFee: 50, tip: 39.506172840, wantErr: true},
		{name: "reported invalid configuration", maxFee: 50, tip: 40, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := &Manager{cfg: Config{MaxFeeGwei: test.maxFee, TipGwei: test.tip}}
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
	if tx.GasTipCap().Cmp(big.NewInt(1_000_000_000)) != 0 {
		t.Fatalf("gas tip cap = %s, want 1000000000", tx.GasTipCap())
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
		CancelAt: time.Now().Add(-time.Second), Label: "expired",
	})
	if err == nil || b.sendCalls != 0 {
		t.Fatalf("expired broadcast = %v, send calls = %d", err, b.sendCalls)
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
	startManagerForTest(t, m)

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
	if result, accepted := m.TrySend(t.Context(), Request{
		To: common.HexToAddress("0xdef"), GasLimit: 21_000, Label: "try while paused",
	}); accepted || result.Err != nil {
		t.Fatalf("paused TrySend = (%+v, %v), want not accepted", result, accepted)
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
	startManagerForTest(t, m)

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
	startManagerForTest(t, m)

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
	startManagerForTest(t, m)

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
	startManagerForTest(t, m)

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
		// Exercise the slow newest hash first; the next poll must resume at the older attempt.
		receiptCursor: 1,
	}
	if result, done := m.receiptResult(t.Context(), pending); done {
		t.Fatalf("slow newest lookup completed lifecycle: %+v", result)
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

type blockedFeeBackend struct {
	*replacementBackend

	block atomic.Bool
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
	header := &types.Header{Number: new(big.Int).SetUint64(number), BaseFee: big.NewInt(20e9)}
	if number > 0 {
		header.ParentHash = forkedReceiptHeader(number-1, fork).Hash()
	}
	if fork != "" {
		header.Extra = []byte(fork)
	}
	actual, _ := receiptHeaderCache.LoadOrStore(key, header)
	return types.CopyHeader(actual.(*types.Header))
}

func (b *blockedFeeBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	if b.block.Load() {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return b.mockBackend.HeaderByNumber(ctx, number)
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

type acceptedThenNonceLowBackend struct {
	*mockBackend

	first       bool
	receiptGate <-chan struct{}
}

type replacementNonceRaceBackend struct {
	*mockBackend

	publishOwnedReceipt bool
	firstSendErr        error
}

type shutdownWriteOutageBackend struct {
	*mockBackend

	cancellationStarted chan struct{}
	cancellationOnce    sync.Once
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

func (b *acceptedThenNonceLowBackend) SendTransaction(
	_ context.Context,
	tx *types.Transaction,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sendCalls++
	b.sent = append(b.sent, tx)
	b.receipts[tx.Hash()] = successfulReceipt(tx, b.head)
	if !b.first {
		b.first = true
		b.latestNonce = tx.Nonce() + 1
		b.pendingNonce = tx.Nonce() + 1
		return errors.New("nonce too low")
	}
	b.latestNonce = tx.Nonce() + 1
	b.pendingNonce = tx.Nonce() + 1
	return nil
}

func (b *acceptedThenNonceLowBackend) TransactionReceipt(
	ctx context.Context,
	hash common.Hash,
) (*types.Receipt, error) {
	if b.receiptGate != nil {
		select {
		case <-b.receiptGate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return b.mockBackend.TransactionReceipt(ctx, hash)
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

func (b *shutdownWriteOutageBackend) SendTransaction(
	ctx context.Context,
	tx *types.Transaction,
) error {
	b.mu.Lock()
	b.sendCalls++
	call := b.sendCalls
	b.sent = append(b.sent, tx)
	b.mu.Unlock()
	if call == 1 {
		return nil
	}
	b.cancellationOnce.Do(func() { close(b.cancellationStarted) })
	<-ctx.Done()
	return ctx.Err()
}

type blockingTxSigner struct {
	signer.Signer

	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

type shutdownBlockingSigner struct {
	signer.Signer

	mu                 sync.Mutex
	calls              int
	replacementStarted chan struct{}
	release            <-chan struct{}
	once               sync.Once
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

func (s *shutdownBlockingSigner) SignTx(
	ctx context.Context,
	tx *types.Transaction,
	chainID *big.Int,
) (*types.Transaction, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()
	if call > 1 {
		s.once.Do(func() { close(s.replacementStarted) })
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
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
	cancellationTo     common.Address
	sameNonceSends     int
}

func (b *replacementBackend) SendTransaction(_ context.Context, tx *types.Transaction) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sendCalls++
	b.sent = append(b.sent, tx)

	if b.isCancellation(tx) {
		b.receipts[tx.Hash()] = successfulReceipt(tx, b.head)
		return nil
	}
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
	startManagerForTest(t, m)
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
		name         string
		cancellation bool
	}{
		{name: "normal transaction"},
		{name: "cancellation transaction", cancellation: true},
	} {
		t.Run(test.name, func(t *testing.T) {
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
				logr.Discard(),
			)
			pending := &pendingTransaction{
				req:   Request{To: to, Label: "failed receipt"},
				nonce: 7,
				attempts: []txAttempt{{
					hash: tx.Hash(), tx: tx, cancellation: test.cancellation,
				}},
			}

			result, done := manager.receiptResult(confirmationCtx, pending)
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
		{outcome: OutcomeCancelled},
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
