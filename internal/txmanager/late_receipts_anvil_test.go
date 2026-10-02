//go:build integration

package txmanager

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
)

// Only the fresh replacement's send models a private relay. Its acknowledgement forwards no
// bytes; every nonce, pool, receipt and canonical confirmation read still uses the real Anvil chain.
type lateReceiptAnvilRelay struct {
	*sharedAnvilNonceBackend

	silentTarget common.Address
	silentSends  chan *types.Transaction
}

func (b *lateReceiptAnvilRelay) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if tx.To() != nil && *tx.To() == b.silentTarget {
		select {
		case b.silentSends <- tx:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return b.sharedAnvilNonceBackend.SendTransaction(ctx, tx)
}

func TestAnvilAbandonedOriginalExecutesAfterSilentFreshReplacement(t *testing.T) {
	rpcClient, ethClient, endpoint := startAnvilWithoutMining(t)
	sgnr := anvilSigner(t)
	toB := common.HexToAddress("0xbeef")
	backend := &lateReceiptAnvilRelay{
		sharedAnvilNonceBackend: newSharedAnvilBackend(t, endpoint),
		silentTarget:            toB,
		silentSends:             make(chan *types.Transaction, 4),
	}
	cfg := anvilReconciliationConfig()
	cfg.PendingTimeout = 5 * time.Second
	cfg.ReplacementInterval = 100 * time.Millisecond
	cfg.LateReceiptTimeout = 5 * time.Second
	cfg.LateReceiptMaxHashes = 16
	metrics := newTestMetrics(t)
	m := NewWithMetrics(backend, sgnr, big.NewInt(31337), cfg, metrics, logr.Discard())
	if err := m.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	startManagerForTest(t, m)
	successes := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "anvil_business_success_total", Help: "Business successes observed through the receipt callback.",
	}, []string{"business"})
	aReceipts, bReceipts, cReceipts := make(chan Result, 4), make(chan Result, 4), make(chan Result, 4)
	observe := func(business string, receipts chan<- Result) func(context.Context, Result) {
		return func(ctx context.Context, result Result) {
			if result.Receipt != nil && result.Receipt.Status == types.ReceiptStatusSuccessful {
				successes.WithLabelValues(business).Inc()
			}
			select {
			case receipts <- result:
			case <-ctx.Done():
			}
		}
	}
	aResult, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xdead"), Data: []byte{0x11, 0x22}, GasLimit: 50_000,
		Deadline: time.Now().Add(time.Second), Label: "business-A",
		ObserveReceipt: observe("A", aReceipts),
	})
	if !accepted {
		t.Fatal("original business call was not admitted")
	}
	initial := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(poolTransaction) bool { return true })
	originalHash := common.HexToHash(initial.Hash)
	abandoned := waitForTxResult(t, aResult)
	if abandoned.Outcome != OutcomeAbandoned || !errors.Is(abandoned.Err, ErrAbandoned) || abandoned.Hash != originalHash || abandoned.Receipt != nil {
		t.Fatalf("original result = %+v, want abandoned without a receipt", abandoned)
	}
	waitForAdmissionDemand(t, m, 0)
	if !m.LaneReady() {
		t.Fatal("passive receipt observation kept the nonce lane occupied")
	}
	assertMetric(t, metrics.requests.WithLabelValues("business-A", string(OutcomeAbandoned)), 1)
	assertMetric(t, metrics.inflight.WithLabelValues("business-A"), 0)
	assertMetric(t, successes.WithLabelValues("A"), 0)
	bResult, accepted := m.SendAsync(t.Context(), Request{
		To: toB, Data: []byte{0x33, 0x44}, Value: big.NewInt(17), GasLimit: 55_000,
		Label: "business-B", ObserveReceipt: observe("B", bReceipts),
	})
	if !accepted {
		t.Fatal("passive original observation blocked a fresh business decision")
	}
	var txB *types.Transaction
	select {
	case txB = <-backend.silentSends:
	case <-time.After(5 * time.Second):
		t.Fatal("fresh business transaction was not sent to the silent relay")
	}
	if txB.Nonce() != 0 || txB.To() == nil || *txB.To() != toB || txB.Value().Cmp(big.NewInt(17)) != 0 {
		t.Fatalf("fresh replacement lost its own business payload: nonce=%d target=%v value=%s", txB.Nonce(), txB.To(), txB.Value())
	}
	assertAnvilReplacementFeeFloors(t, initial, anvilPoolFees(txB))
	unchanged := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(poolTransaction) bool { return true })
	if unchanged.Hash != initial.Hash {
		t.Fatal("silent relay acknowledgement removed the real pending original")
	}
	assertAnvilPoolNonceAbsent(t, rpcClient, sgnr.Address(), 1)
	mineAnvilBlock(t, rpcClient)
	select {
	case premature := <-aReceipts:
		t.Fatalf("late callback ignored configured confirmation depth: %+v", premature)
	case <-time.After(50 * time.Millisecond):
	}
	mineAnvilBlock(t, rpcClient)
	late := waitForTxResult(t, aReceipts)
	if late.Outcome != OutcomeConfirmed || late.Err != nil || late.Hash != originalHash || late.Receipt == nil || late.Receipt.TxHash != originalHash {
		t.Fatalf("late callback did not report the original owned receipt: %+v", late)
	}
	assertAnvilCanonicalTransaction(t, ethClient, originalHash, 0)
	canonicalReceipt, err := ethClient.TransactionReceipt(t.Context(), originalHash)
	if err != nil {
		t.Fatal(err)
	}
	if late.Receipt.BlockHash != canonicalReceipt.BlockHash || late.Receipt.GasUsed != canonicalReceipt.GasUsed ||
		late.Receipt.EffectiveGasPrice == nil || late.Receipt.EffectiveGasPrice.Cmp(canonicalReceipt.EffectiveGasPrice) != 0 {
		t.Fatalf("late callback changed actual receipt costs: late=%+v canonical=%+v", late.Receipt, canonicalReceipt)
	}
	consumed := waitForTxResult(t, bResult)
	if consumed.Outcome != OutcomeNonceConsumed || !errors.Is(consumed.Err, ErrNonceConsumed) || consumed.Hash != txB.Hash() || consumed.Receipt != nil || consumed.Outcome.Included() {
		t.Fatalf("fresh replacement fabricated its own execution: %+v", consumed)
	}
	waitForAdmissionDemand(t, m, 0)
	cResult, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xcafe"), GasLimit: 21_000, Label: "business-C",
		ObserveReceipt: observe("C", cReceipts),
	})
	if !accepted {
		t.Fatal("late observation blocked the next distinct order")
	}
	next := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 1, func(poolTransaction) bool { return true })
	mineAnvilBlock(t, rpcClient)
	mineAnvilBlock(t, rpcClient)
	assertAnvilReconciliationOutcome(t, waitForTxResult(t, cResult), common.HexToHash(next.Hash))
	assertAnvilCanonicalTransaction(t, ethClient, common.HexToHash(next.Hash), 1)
	normal := waitForTxResult(t, cReceipts)
	if normal.Outcome != OutcomeConfirmed || normal.Err != nil || normal.Hash != common.HexToHash(next.Hash) || normal.Receipt == nil || normal.Receipt.TxHash != normal.Hash {
		t.Fatalf("normal callback did not retain its owned receipt: %+v", normal)
	}
	// Further real blocks and receipt sweeps must not produce another result or double-count A.
	mineAnvilBlock(t, rpcClient)
	select {
	case duplicate := <-aReceipts:
		t.Fatalf("late original success was observed twice: %+v", duplicate)
	case wrong := <-bReceipts:
		t.Fatalf("losing fresh transaction received somebody else's receipt: %+v", wrong)
	case redelivered := <-aResult:
		t.Fatalf("late receipt rewrote the caller's terminal result: %+v", redelivered)
	case <-time.After(100 * time.Millisecond):
	}
	assertMetric(t, successes.WithLabelValues("A"), 1)
	assertMetric(t, successes.WithLabelValues("B"), 0)
	assertMetric(t, successes.WithLabelValues("C"), 1)
	assertMetric(t, metrics.requests.WithLabelValues("business-A", string(OutcomeAbandoned)), 1)
	assertMetric(t, metrics.requests.WithLabelValues("business-A", string(OutcomeConfirmed)), 0)
	assertMetric(t, metrics.inflight.WithLabelValues("business-A"), 0)
	assertMetric(t, metrics.lateReceipts.WithLabelValues("business-A", string(OutcomeConfirmed)), 1)
	assertMetric(t, metrics.lateReceipts.WithLabelValues("business-B", string(OutcomeConfirmed)), 0)
	assertMetric(t, metrics.lateReceipts.WithLabelValues("business-C", string(OutcomeConfirmed)), 0)
	assertMetric(t, metrics.gasUsed.WithLabelValues("business-A", string(OutcomeConfirmed)), float64(canonicalReceipt.GasUsed))
	fee := new(big.Int).Mul(new(big.Int).SetUint64(canonicalReceipt.GasUsed), canonicalReceipt.EffectiveGasPrice)
	feePaid, _ := new(big.Float).SetInt(fee).Float64()
	assertMetric(t, metrics.feePaidWei.WithLabelValues("business-A", string(OutcomeConfirmed)), feePaid)
	assertMetric(t, metrics.gasUsed.WithLabelValues("business-B", string(OutcomeConfirmed)), 0)
	assertMetric(t, metrics.feePaidWei.WithLabelValues("business-B", string(OutcomeConfirmed)), 0)
	backend.mu.Lock()
	actualBroadcasts := len(backend.sent)
	backend.mu.Unlock()
	if actualBroadcasts != 2 || len(backend.silentSends) != 0 {
		t.Fatalf("passive receipt observation sent extra transactions: actual=%d extra silent=%d", actualBroadcasts, len(backend.silentSends))
	}
}
