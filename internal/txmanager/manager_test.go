package txmanager

import (
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/signer"
)

const testPrivateKey = "0000000000000000000000000000000000000000000000000000000000000001"

type testBackend struct {
	mu       sync.Mutex
	latest   uint64
	pending  uint64
	head     *types.Header
	blocks   map[string]*types.Header
	receipts map[common.Hash]*types.Receipt
	sent     []*types.Transaction
	send     func(*types.Transaction) error
	header   func(context.Context, *big.Int) error
}

func newTestBackend() *testBackend {
	head := &types.Header{Number: big.NewInt(1), BaseFee: big.NewInt(1_000_000_000), Time: 1}
	return &testBackend{
		head: head, blocks: map[string]*types.Header{"1": head}, receipts: make(map[common.Hash]*types.Receipt),
	}
}

func (b *testBackend) NonceAt(context.Context, common.Address, *big.Int) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.latest, nil
}

func (b *testBackend) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.pending, nil
}

func (b *testBackend) TransactionSenderBalanceAt(context.Context, common.Address, *big.Int) (*big.Int, error) {
	return new(big.Int), nil
}

func (b *testBackend) FeeHistory(context.Context, uint64, *big.Int, []float64) (*ethereum.FeeHistory, error) {
	rewards := make([][]*big.Int, feeHistoryBlocks)
	for index := range rewards {
		rewards[index] = []*big.Int{big.NewInt(1_000_000_000)}
	}
	return &ethereum.FeeHistory{Reward: rewards}, nil
}

func (b *testBackend) SuggestGasTipCap(context.Context) (*big.Int, error) {
	return big.NewInt(1_000_000_000), nil
}

func (b *testBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	if b.header != nil {
		if err := b.header(ctx, number); err != nil {
			return nil, err
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if number == nil {
		return b.head, nil
	}
	return b.blocks[number.String()], nil
}

func (b *testBackend) EstimateGas(context.Context, ethereum.CallMsg) (uint64, error) {
	return 100_000, nil
}

func (b *testBackend) SendTransaction(_ context.Context, tx *types.Transaction) error {
	b.mu.Lock()
	b.sent = append(b.sent, tx)
	send := b.send
	b.mu.Unlock()
	if send != nil {
		return send(tx)
	}
	return nil
}

func (b *testBackend) TransactionReceipt(_ context.Context, hash common.Hash) (*types.Receipt, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	receipt := b.receipts[hash]
	if receipt == nil {
		return nil, ethereum.NotFound
	}
	receiptCopy := *receipt
	return &receiptCopy, nil
}

func (b *testBackend) mine(tx *types.Transaction) {
	b.mu.Lock()
	defer b.mu.Unlock()
	blockNumber := new(big.Int).SetUint64(tx.Nonce() + 10)
	included := &types.Header{Number: blockNumber, BaseFee: big.NewInt(1_000_000_000), Time: blockNumber.Uint64()}
	head := &types.Header{
		Number: new(big.Int).Add(blockNumber, big.NewInt(1)), BaseFee: big.NewInt(1_000_000_000),
		ParentHash: included.Hash(), Time: blockNumber.Uint64() + 1,
	}
	b.blocks[blockNumber.String()] = included
	b.blocks[head.Number.String()] = head
	b.head = head
	b.latest, b.pending = tx.Nonce()+1, tx.Nonce()+1
	b.receipts[tx.Hash()] = &types.Receipt{
		TxHash: tx.Hash(), BlockHash: included.Hash(), BlockNumber: blockNumber,
		Status: types.ReceiptStatusSuccessful, GasUsed: tx.Gas(),
	}
}

func (b *testBackend) transactions() []*types.Transaction {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*types.Transaction(nil), b.sent...)
}

func testManager(t *testing.T, backend *testBackend, mutate func(*Config)) *Manager {
	t.Helper()
	sgnr, err := signer.NewFromHexKey(testPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Confirmations: 1, MaxFeeGwei: 50, TipGwei: 1,
		PollInterval: time.Millisecond, BroadcastTimeout: 20 * time.Millisecond,
		ReplacementInterval: 5 * time.Millisecond, PendingTimeout: 100 * time.Millisecond,
		ShutdownTimeout: 50 * time.Millisecond,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	return New(backend, sgnr, big.NewInt(1), cfg, logr.Discard())
}

func runManager(t *testing.T, manager *Manager) (context.CancelFunc, <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		manager.Start(ctx)
		close(done)
	}()
	return cancel, done
}

func stopManager(t *testing.T, cancel context.CancelFunc, done <-chan struct{}) {
	t.Helper()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("manager did not stop")
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSendSerializesNonceLane(t *testing.T) {
	backend := newTestBackend()
	backend.send = func(tx *types.Transaction) error {
		if tx.Nonce() == 1 {
			backend.mine(tx)
		}
		return nil
	}
	manager := testManager(t, backend, func(cfg *Config) {
		cfg.ReplacementInterval = time.Second
		cfg.PendingTimeout = 2 * time.Second
	})
	cancel, done := runManager(t, manager)
	defer stopManager(t, cancel, done)

	first := make(chan Result, 1)
	go func() { first <- manager.Send(t.Context(), Request{To: common.HexToAddress("0x1"), Label: "first"}) }()
	waitFor(t, func() bool { return len(backend.transactions()) == 1 })
	secondResult := make(chan Result, 1)
	go func() {
		secondResult <- manager.Send(t.Context(), Request{To: common.HexToAddress("0x2"), Label: "second"})
	}()
	time.Sleep(10 * time.Millisecond)
	if len(backend.transactions()) != 1 || manager.LaneReady() {
		t.Fatal("second request bypassed the active nonce lifecycle")
	}
	backend.mine(backend.transactions()[0])
	if result := <-first; result.Err != nil {
		t.Fatalf("first result: %v", result.Err)
	}
	if result := <-secondResult; result.Err != nil {
		t.Fatalf("second result: %v", result.Err)
	}
	if txs := backend.transactions(); len(txs) != 2 || txs[0].Nonce() != 0 || txs[1].Nonce() != 1 {
		t.Fatalf("transactions = %+v", txs)
	}
}

func TestAmbiguousBroadcastRetriesExactBytes(t *testing.T) {
	backend := newTestBackend()
	var sends int
	backend.send = func(tx *types.Transaction) error {
		sends++
		if sends == 1 {
			return errors.New("write response lost")
		}
		backend.mine(tx)
		return nil
	}
	manager := testManager(t, backend, func(cfg *Config) { cfg.ReplacementInterval = 20 * time.Millisecond })
	cancel, done := runManager(t, manager)
	defer stopManager(t, cancel, done)
	result := manager.Send(t.Context(), Request{To: common.HexToAddress("0x1"), Label: "ambiguous"})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	txs := backend.transactions()
	if len(txs) != 2 || txs[0].Hash() != txs[1].Hash() {
		t.Fatalf("exact rebroadcast hashes = %v", txs)
	}
}

func TestDeadlineCancelsAtSameNonce(t *testing.T) {
	backend := newTestBackend()
	var sends int
	backend.send = func(tx *types.Transaction) error {
		sends++
		if sends == 2 {
			backend.mine(tx)
		}
		return nil
	}
	manager := testManager(t, backend, func(cfg *Config) { cfg.ReplacementInterval = 20 * time.Millisecond })
	cancel, done := runManager(t, manager)
	defer stopManager(t, cancel, done)
	result := manager.Send(t.Context(), Request{
		To: common.HexToAddress("0x1"), CancelAt: time.Now().Add(10 * time.Millisecond), Label: "expiring",
	})
	if result.Err == nil {
		t.Fatal("cancellation returned success")
	}
	txs := backend.transactions()
	if len(txs) != 2 || txs[0].Nonce() != txs[1].Nonce() || txs[1].To() == nil ||
		*txs[1].To() != manager.signer.Address() || len(txs[1].Data()) != 0 {
		t.Fatalf("cancellation transactions = %+v", txs)
	}
}

func TestReplacementCrossingDeadlineStartsOneCancellation(t *testing.T) {
	backend := newTestBackend()
	entered, release := make(chan struct{}), make(chan struct{})
	var headerCalls int
	backend.header = func(ctx context.Context, _ *big.Int) error {
		headerCalls++
		if headerCalls != 2 {
			return nil
		}
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	manager := testManager(t, backend, func(cfg *Config) {
		cfg.ReplacementInterval = 100 * time.Millisecond
		cfg.PendingTimeout = time.Second
	})
	cancel, done := runManager(t, manager)
	defer stopManager(t, cancel, done)
	cancelAt := time.Now().Add(120 * time.Millisecond)
	result := make(chan Result, 1)
	go func() {
		result <- manager.Send(t.Context(), Request{
			To: common.HexToAddress("0x1"), CancelAt: cancelAt, Label: "deadline-crossing",
		})
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("replacement fee read did not start")
	}
	time.Sleep(max(time.Until(cancelAt)+5*time.Millisecond, 0))
	close(release)
	waitFor(t, func() bool { return len(backend.transactions()) == 2 })
	time.Sleep(10 * time.Millisecond)
	transactions := backend.transactions()
	if len(transactions) != 2 {
		t.Fatalf("cancellation broadcasts = %d, want 1", len(transactions)-1)
	}
	backend.mine(transactions[1])
	if got := <-result; got.Err == nil {
		t.Fatal("cancellation returned success")
	}
}

func TestCanonicalConfirmationAndReorg(t *testing.T) {
	backend := newTestBackend()
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(1), To: new(common.Address), Gas: 21_000,
		GasFeeCap: big.NewInt(2), GasTipCap: big.NewInt(1),
	})
	backend.mine(tx)
	manager := testManager(t, backend, nil)
	receipt, err := backend.TransactionReceipt(t.Context(), tx.Hash())
	if err != nil || manager.confirmCanonicalReceipt(t.Context(), receipt) != nil {
		t.Fatalf("canonical receipt = %v, %v", receipt, err)
	}
	backend.mu.Lock()
	backend.blocks[receipt.BlockNumber.String()] = &types.Header{
		Number: receipt.BlockNumber, BaseFee: big.NewInt(1), Extra: []byte("reorg"),
	}
	backend.mu.Unlock()
	if err := manager.confirmCanonicalReceipt(t.Context(), receipt); !errors.Is(err, errReceiptReorged) {
		t.Fatalf("reorg error = %v", err)
	}
}

func TestNonceCollisionPausesLane(t *testing.T) {
	backend := newTestBackend()
	backend.send = func(*types.Transaction) error { return errors.New("replacement transaction underpriced") }
	manager := testManager(t, backend, nil)
	cancel, done := runManager(t, manager)
	result := manager.Send(t.Context(), Request{To: common.HexToAddress("0x1"), Label: "collision"})
	if result.Err == nil || manager.Available() {
		t.Fatalf("collision result/availability = %v/%v", result.Err, manager.Available())
	}
	stopManager(t, cancel, done)
}

func TestShutdownDrainsThroughCancellation(t *testing.T) {
	backend := newTestBackend()
	var sends int
	backend.send = func(tx *types.Transaction) error {
		sends++
		if sends == 2 {
			backend.mine(tx)
		}
		return nil
	}
	manager := testManager(t, backend, nil)
	cancel, done := runManager(t, manager)
	result := make(chan Result, 1)
	go func() {
		result <- manager.Send(t.Context(), Request{To: common.HexToAddress("0x1"), Label: "shutdown"})
	}()
	waitFor(t, func() bool { return len(backend.transactions()) == 1 })
	cancel()
	if got := <-result; got.Err == nil || got.NotAdmitted {
		t.Fatalf("shutdown result = %+v", got)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("manager did not drain")
	}
}

func TestFeePolicy(t *testing.T) {
	base, tip, limit := big.NewInt(10), big.NewInt(3), big.NewInt(20)
	quote, err := boundedFeeQuote(base, tip, new(big.Int), limit)
	if err != nil || quote.maxFee.Cmp(limit) != 0 || quote.tip.Cmp(tip) != 0 {
		t.Fatalf("bounded quote = %+v, %v", quote, err)
	}
	if _, err := boundedFeeQuote(big.NewInt(21), tip, new(big.Int), limit); err == nil {
		t.Fatal("base fee above cap was accepted")
	}
	if bumpFee(big.NewInt(8)).Cmp(big.NewInt(9)) != 0 {
		t.Fatal("replacement bump is not 12.5% rounded up")
	}
}

func TestExpiredRequestIsNotAdmitted(t *testing.T) {
	manager := testManager(t, newTestBackend(), nil)
	result := manager.Send(t.Context(), Request{
		To: common.HexToAddress("0x1"), CancelAt: time.Now().Add(-time.Second), Label: "expired",
	})
	if !result.NotAdmitted || !errors.Is(result.Err, context.DeadlineExceeded) {
		t.Fatalf("expired result = %+v", result)
	}
}
