//go:build integration

package txmanager

import (
	"context"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

type sharedAnvilNonceBackend struct {
	*chain.Client

	barrier  chan struct{}
	arrivals atomic.Int64
	mu       sync.Mutex
	sent     []*types.Transaction
}

func newSharedAnvilBackend(t *testing.T, endpoint string) *sharedAnvilNonceBackend {
	t.Helper()
	client, err := chain.Dial(t.Context(), []string{endpoint}, "", common.Address{}.Hex(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return &sharedAnvilNonceBackend{Client: client}
}

func (b *sharedAnvilNonceBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	b.mu.Lock()
	b.sent = append(b.sent, tx)
	b.mu.Unlock()
	if tx.Nonce() == 0 && b.barrier != nil {
		if b.arrivals.Add(1) == 3 {
			close(b.barrier)
		}
		select {
		case <-b.barrier:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return b.Client.SendTransaction(ctx, tx)
}

func (b *sharedAnvilNonceBackend) waitForSend(t *testing.T, index int) *types.Transaction {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		if len(b.sent) > index {
			tx := b.sent[index]
			b.mu.Unlock()
			return tx
		}
		b.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for signed transaction %d", index)
	return nil
}

func anvilReconciliationConfig() Config {
	return Config{
		Confirmations: 1, MaxFeeGwei: 100,
		PollInterval: 10 * time.Millisecond, ReplacementInterval: time.Second,
		PendingTimeout: time.Minute, ShutdownTimeout: time.Second,
		Horizon: HorizonConfig{BlockTime: 200 * time.Millisecond},
	}
}

func assertAnvilReconciliationOutcome(t *testing.T, result Result, expectedHash common.Hash) {
	t.Helper()
	if result.Hash != expectedHash || result.NotAdmitted {
		t.Fatalf("reconciliation changed the admitted transaction identity: %+v, expected hash %s", result, expectedHash)
	}
	if result.Outcome == OutcomeConfirmed {
		if result.Err != nil || result.Receipt == nil || result.Receipt.TxHash != expectedHash || !result.Outcome.Included() {
			t.Fatalf("confirmed result lacks owned inclusion: %+v", result)
		}
		return
	}
	if result.Outcome == OutcomeNonceConsumed {
		if result.Receipt != nil || result.Outcome.Included() || !errors.Is(result.Err, ErrNonceConsumed) {
			t.Fatalf("nonce-only result fabricated execution: %+v", result)
		}
		return
	}
	t.Fatalf("unexpected reconciliation outcome: %+v", result)
}

func assertAnvilCanonicalTransaction(t *testing.T, client *ethclient.Client, hash common.Hash, nonce uint64) {
	t.Helper()
	tx, pending, err := client.TransactionByHash(t.Context(), hash)
	if err != nil || pending || tx.Nonce() != nonce {
		t.Fatalf("transaction %s did not consume expected nonce %d: tx=%v pending=%v err=%v", hash, nonce, tx, pending, err)
	}
	receipt, err := client.TransactionReceipt(t.Context(), hash)
	if err != nil || receipt.TxHash != hash || receipt.Status != types.ReceiptStatusSuccessful {
		t.Fatalf("transaction %s lacks successful receipt: receipt=%v err=%v", hash, receipt, err)
	}
	canonical, err := client.HeaderByNumber(t.Context(), receipt.BlockNumber)
	if err != nil || canonical.Hash() != receipt.BlockHash {
		t.Fatalf("transaction %s receipt is not canonical: header=%v receipt=%v err=%v", hash, canonical, receipt, err)
	}
	head, err := client.HeaderByNumber(t.Context(), nil)
	confirmedAt := new(big.Int).Add(receipt.BlockNumber, big.NewInt(1))
	if err != nil || head.Number.Cmp(confirmedAt) < 0 {
		t.Fatalf("transaction %s lacks confirmation depth %d: head=%v receipt=%v err=%v", hash, 1, head, receipt, err)
	}
}

// Three managers share a key and RPC, with no state exchanged between managers. The test caller
// retries raced orders, modelling the solver's fresh protocol-state check rather than calldata replay.
func TestAnvilThreeSameKeyReplicasExecuteDistinctOrdersAtMinedNonce(t *testing.T) {
	rpcClient, ethClient, endpoint := startAnvilWithoutMining(t)
	backend := newSharedAnvilBackend(t, endpoint)
	backend.barrier = make(chan struct{})
	sgnr := anvilSigner(t)
	managers := make([]*Manager, 3)
	results := make([]<-chan Result, 3)
	requests := make([]Request, 3)
	for i := range managers {
		m := New(backend, sgnr, big.NewInt(31337), anvilReconciliationConfig(), logr.Discard())
		if err := m.Initialize(t.Context()); err != nil {
			t.Fatal(err)
		}
		startManagerForTest(t, m)
		managers[i] = m
		requests[i] = Request{To: common.BigToAddress(big.NewInt(int64(0xdead1000 + i))), GasLimit: 21_000}
		var accepted bool
		results[i], accepted = m.SendAsync(t.Context(), requests[i])
		if !accepted {
			t.Fatal("replica did not admit its order")
		}
	}
	select {
	case <-backend.barrier:
	case <-time.After(5 * time.Second):
		t.Fatal("replicas did not meet the nonce race barrier")
	}
	winner := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(poolTransaction) bool { return true })
	winnerIndex := -1
	losers := make([]int, 0, 2)
	for i, m := range managers {
		if requests[i].To == common.HexToAddress(winner.To) {
			winnerIndex = i
			continue
		}
		raced := waitForTxResult(t, results[i])
		if raced.Outcome != OutcomeNonceConflict || !errors.Is(raced.Err, ErrNonceConflict) || raced.Receipt != nil || raced.Outcome.Included() {
			t.Fatalf("initial losing order: %+v", raced)
		}
		waitForAdmissionDemand(t, m, 0)
		if !m.LaneReady() {
			t.Fatal("nonce race paused fresh local work")
		}
		losers = append(losers, i)
	}
	if winnerIndex < 0 || len(losers) != 2 {
		t.Fatalf("unexpected initial race: winner %d losers %v", winnerIndex, losers)
	}
	// A fresh losing decision before mining keeps the lowest nonce and fresh market fees. A
	// rejected bid cannot establish the winning fee floor, so it yields without evicting the winner.
	replacer := losers[0]
	var admitted bool
	results[replacer], admitted = managers[replacer].SendAsync(t.Context(), requests[replacer])
	if !admitted {
		t.Fatal("fresh raced order was not admitted")
	}
	raced := waitForTxResult(t, results[replacer])
	if raced.Outcome != OutcomeNonceConflict || !errors.Is(raced.Err, ErrNonceConflict) || raced.Receipt != nil {
		t.Fatalf("fresh losing decision did not yield contention: %+v", raced)
	}
	stillPending := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(poolTransaction) bool { return true })
	if stillPending.Hash != winner.Hash {
		t.Fatal("rejected fresh fees evicted the initial winner")
	}
	assertAnvilPoolNonceAbsent(t, rpcClient, sgnr.Address(), 1)
	mineAnvilBlock(t, rpcClient)
	mineAnvilBlock(t, rpcClient)
	assertAnvilReconciliationOutcome(t, waitForTxResult(t, results[winnerIndex]), common.HexToHash(winner.Hash))
	assertAnvilCanonicalTransaction(t, ethClient, common.HexToHash(winner.Hash), 0)
	// Fresh business-state checks omit the completed order and rebuild each remaining order. The
	// canonical nonce advances only after mining, so all three distinct orders execute in order.
	for offset, i := range losers {
		nonce := uint64(offset + 1)
		waitForAdmissionDemand(t, managers[i], 0)
		result, accepted := managers[i].SendAsync(t.Context(), requests[i])
		if !accepted {
			t.Fatal("remaining business order was not admitted")
		}
		tx := waitForPoolTransaction(t, rpcClient, sgnr.Address(), nonce, func(tx poolTransaction) bool {
			return common.HexToAddress(tx.To) == requests[i].To
		})
		assertAnvilPoolNonceAbsent(t, rpcClient, sgnr.Address(), nonce+1)
		mineAnvilBlock(t, rpcClient)
		mineAnvilBlock(t, rpcClient)
		assertAnvilReconciliationOutcome(t, waitForTxResult(t, result), common.HexToHash(tx.Hash))
		assertAnvilCanonicalTransaction(t, ethClient, common.HexToHash(tx.Hash), nonce)
	}
}

func TestAnvilForeignPendingTransactionPreservesIndependentBusinessWork(t *testing.T) {
	rpcClient, ethClient, endpoint := startAnvilWithoutMining(t)
	sgnr := anvilSigner(t)
	to := common.HexToAddress("0xdead")
	foreign, err := sgnr.SignTx(t.Context(), types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(31337), Nonce: 0, To: &to, Gas: 21_000,
		GasFeeCap: big.NewInt(10e9), GasTipCap: big.NewInt(1e9),
	}), big.NewInt(31337))
	if err != nil {
		t.Fatal(err)
	}
	if err := ethClient.SendTransaction(t.Context(), foreign); err != nil {
		t.Fatal(err)
	}
	backend := newSharedAnvilBackend(t, endpoint)
	cfg := anvilReconciliationConfig()
	cfg.PendingTimeout = 200 * time.Millisecond
	m := New(backend, sgnr, big.NewInt(31337), cfg, logr.Discard())
	if err := m.Initialize(t.Context()); err != nil || !m.LaneReady() {
		t.Fatalf("foreign pending startup: %v", err)
	}
	startManagerForTest(t, m)
	time.Sleep(600 * time.Millisecond) // Beyond this replica's local pending timeout.
	unchanged := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(poolTransaction) bool { return true })
	if unchanged.Hash != foreign.Hash().Hex() {
		t.Fatal("replica changed a transaction it did not send")
	}
	// A fresh call tries the current mined nonce, but its profitability cap cannot replace the much
	// more expensive foreign call. The collision releases the lane without skipping to nonce 1.
	req := Request{To: common.HexToAddress("0xbeef"), GasLimit: 21_000, MaxFeePerGas: big.NewInt(2e9)}
	result, admitted := m.SendAsync(t.Context(), req)
	if !admitted {
		t.Fatal("foreign pending transaction blocked fresh work")
	}
	attempted := backend.waitForSend(t, 0)
	if attempted.Nonce() != 0 || attempted.GasFeeCap().Cmp(req.MaxFeePerGas) > 0 {
		t.Fatalf("foreign pending call bypassed mined nonce or profitability cap: nonce=%d fee=%s", attempted.Nonce(), attempted.GasFeeCap())
	}
	raced := waitForTxResult(t, result)
	if raced.Outcome != OutcomeNonceConflict || !errors.Is(raced.Err, ErrNonceConflict) || raced.Receipt != nil || raced.Outcome.Included() {
		t.Fatalf("foreign pending collision = %+v", raced)
	}
	assertAnvilPoolNonceAbsent(t, rpcClient, sgnr.Address(), 1)
	unchanged = waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(poolTransaction) bool { return true })
	if unchanged.Hash != foreign.Hash().Hex() {
		t.Fatal("underpriced business call changed the foreign pending transaction")
	}
	mineAnvilBlock(t, rpcClient)
	mineAnvilBlock(t, rpcClient)
	assertAnvilCanonicalTransaction(t, ethClient, foreign.Hash(), 0)
	waitForAdmissionDemand(t, m, 0)
	result, admitted = m.SendAsync(t.Context(), req)
	if !admitted {
		t.Fatal("foreign inclusion paused fresh work")
	}
	fresh := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 1, func(poolTransaction) bool { return true })
	mineAnvilBlock(t, rpcClient)
	mineAnvilBlock(t, rpcClient)
	assertAnvilReconciliationOutcome(t, waitForTxResult(t, result), common.HexToHash(fresh.Hash))
	assertAnvilCanonicalTransaction(t, ethClient, common.HexToHash(fresh.Hash), 1)
}

func TestAnvilRestartRetriesWithoutFeeRatchetAfterUnknownPendingCallDrops(t *testing.T) {
	rpcClient, ethClient, endpoint := startAnvilWithoutMining(t)
	sgnr := anvilSigner(t)
	oldConfig := anvilReconciliationConfig()
	oldConfig.Horizon.MaxBlocks = 12
	oldConfig.ShutdownTimeout = 50 * time.Millisecond
	oldManager := New(newSharedAnvilBackend(t, endpoint), sgnr, big.NewInt(31337), oldConfig, logr.Discard())
	if err := oldManager.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	oldContext, stopOld := context.WithCancel(t.Context())
	oldStopped := make(chan struct{})
	go func() {
		defer close(oldStopped)
		oldManager.Start(oldContext)
	}()
	t.Cleanup(stopOld)
	oldResult, accepted := oldManager.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xdead"), Data: []byte{0x11, 0x22}, GasLimit: 50_000,
	})
	if !accepted {
		t.Fatal("old business call was not admitted")
	}
	initial := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(poolTransaction) bool { return true })
	stopOld()
	select {
	case <-oldStopped:
	case <-time.After(5 * time.Second):
		t.Fatal("old manager did not stop")
	}
	stopped := waitForTxResult(t, oldResult)
	if stopped.Outcome != OutcomeTrackingStopped || stopped.Receipt != nil || stopped.Outcome.Included() {
		t.Fatalf("stopping old manager proved unexpected inclusion: %+v", stopped)
	}
	latest, err := ethClient.NonceAt(t.Context(), sgnr.Address(), nil)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := ethClient.PendingNonceAt(t.Context(), sgnr.Address())
	if err != nil || latest != 0 || pending != 1 {
		t.Fatalf("restart nonce state = latest %d pending %d err %v, want 0/1", latest, pending, err)
	}
	// A separate manager and RPC client receive no transaction or fee history from the old manager.
	backend := newSharedAnvilBackend(t, endpoint)
	newConfig := anvilReconciliationConfig()
	newConfig.ShutdownTimeout = 50 * time.Millisecond
	m := New(backend, sgnr, big.NewInt(31337), newConfig, logr.Discard())
	if err := m.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	startManagerForTest(t, m)
	var result <-chan Result
	var replacement poolTransaction
	var previous *types.Transaction
	conflicts := 0
	for attempt := range 4 {
		ceiling, err := m.MaxFeePerGas(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		req := Request{
			To: common.HexToAddress("0xbeef"), Data: []byte{0x33, byte(attempt)}, Value: big.NewInt(17),
			GasLimit: 55_000, MaxFeePerGas: ceiling, Label: "fresh business decision",
		}
		result, accepted = m.SendAsync(t.Context(), req)
		if !accepted {
			t.Fatal("restart paused fresh business work")
		}
		tx := backend.waitForSend(t, attempt)
		if tx.Nonce() != 0 || tx.To() == nil || *tx.To() != req.To || tx.GasFeeCap().Cmp(ceiling) > 0 || tx.GasFeeCap().Cmp(big.NewInt(100e9)) > 0 {
			t.Fatalf("fresh call bypassed lowest unused nonce, payload, or fee cap: nonce=%d target=%v fee=%s ceiling=%s", tx.Nonce(), tx.To(), tx.GasFeeCap(), ceiling)
		}
		if previous != nil {
			if tx.GasFeeCap().Cmp(previous.GasFeeCap()) != 0 || tx.GasTipCap().Cmp(previous.GasTipCap()) != 0 {
				t.Fatal("rejected fresh attempt ratcheted fees after restart")
			}
		}
		previous = tx
		// Accepted calls appear in the real pool; rejected calls return a nonce conflict immediately.
		pooled, raced := waitForAnvilSubmission(t, rpcClient, sgnr.Address(), tx, result)
		if raced != nil {
			if raced.Outcome != OutcomeNonceConflict || !errors.Is(raced.Err, ErrNonceConflict) || raced.Receipt != nil || raced.Outcome.Included() {
				t.Fatalf("unknown pending fee collision = %+v", raced)
			}
			conflicts++
			waitForAdmissionDemand(t, m, 0)
			if conflicts == 3 {
				// A private relay can discard an obsolete call without consuming the nonce.
				if err := rpcClient.CallContext(t.Context(), nil, "anvil_dropTransaction", initial.Hash); err != nil {
					t.Fatalf("drop unknown pending call: %v", err)
				}
				assertAnvilPoolNonceAbsent(t, rpcClient, sgnr.Address(), 0)
			}
			continue
		}
		replacement = pooled
		if replacement.Input != hexutil.Encode(req.Data) || replacement.Value != "0x11" || replacement.Gas != "0xd6d8" {
			t.Fatalf("replacement lost the fresh business decision: %+v", replacement)
		}
		break
	}
	if replacement.Hash == "" || conflicts != 3 {
		t.Fatalf("restart did not recover when unknown work dropped: replacement %+v conflicts %d", replacement, conflicts)
	}
	assertAnvilPoolNonceAbsent(t, rpcClient, sgnr.Address(), 1)
	mineAnvilBlock(t, rpcClient)
	mineAnvilBlock(t, rpcClient)
	assertAnvilReconciliationOutcome(t, waitForTxResult(t, result), common.HexToHash(replacement.Hash))
	assertAnvilCanonicalTransaction(t, ethClient, common.HexToHash(replacement.Hash), 0)
	// After recovery, another distinct order can use the next canonical nonce.
	waitForAdmissionDemand(t, m, 0)
	next, accepted := m.SendAsync(t.Context(), Request{To: common.HexToAddress("0xcafe"), GasLimit: 21_000})
	if !accepted {
		t.Fatal("restart recovery blocked the next order")
	}
	nextTx := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 1, func(poolTransaction) bool { return true })
	mineAnvilBlock(t, rpcClient)
	mineAnvilBlock(t, rpcClient)
	assertAnvilReconciliationOutcome(t, waitForTxResult(t, next), common.HexToHash(nextTx.Hash))
	assertAnvilCanonicalTransaction(t, ethClient, common.HexToHash(nextTx.Hash), 1)
}

func waitForAnvilSubmission(
	t *testing.T, client *rpc.Client, sender common.Address, tx *types.Transaction, result <-chan Result,
) (poolTransaction, *Result) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case got := <-result:
			return poolTransaction{}, &got
		default:
		}
		pooled, exists, err := poolTransactionAt(t.Context(), client, sender, tx.Nonce())
		if err != nil {
			t.Fatal(err)
		}
		if exists && pooled.Hash == tx.Hash().Hex() {
			return pooled, nil
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("transaction %s neither entered the pool nor returned a result", tx.Hash())
	return poolTransaction{}, nil
}

func assertAnvilPoolNonceAbsent(t *testing.T, client *rpc.Client, sender common.Address, nonce uint64) {
	t.Helper()
	if queued, exists, err := poolTransactionAt(t.Context(), client, sender, nonce); err != nil || exists {
		t.Fatalf("queued a later nonce %d: tx=%+v exists=%v err=%v", nonce, queued, exists, err)
	}
}
