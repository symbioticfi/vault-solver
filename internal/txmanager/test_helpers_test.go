package txmanager

import (
	"context"
	"errors"
	"math/big"
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

func startTestManager(t *testing.T, m *Manager) {
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
	startTestManager(t, m)
	return m
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

	receiptMu sync.Mutex
	failures  int
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
	}
}

func receiptTestHeader(block uint64) *types.Header {
	return forkedReceiptHeader(block, "")
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
