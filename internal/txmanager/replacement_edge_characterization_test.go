package txmanager

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr/funcr"
)

const (
	edgeOriginalHash = "0x8e97c5ceeed718d973a80568b1be803ecf6741373bd57cfbd5056b818ebc462b"
	edgeOriginalRaw  = "02f87183aa36a707843b9aca0085098bca5a0082b26e940000000000000000000000000000000000000abc1183010203c080a0a5265a3b95152516899a876dce3228569af93d495aae138311ffee706c984fb5a023968a8d258d9e76c76a1f68efedc197b93daa9cc7a25c37525e0d37fec145cf"

	edgeReplacementHash = "0xbed1489e14e161359c9c7048fdf00951658860f07c88db5221b6441e74f5684b"
	edgeReplacementRaw  = "02f87183aa36a70784430e2340850abd43a54082b26e940000000000000000000000000000000000000abc1183010203c080a018196c83abea6e3fb7d203b3af5997ab1190e8c0ffe323058b6e27a9d46364e4a044d9477e6dbef5a8ca664f5e03162dc93565db90d9113d02993aa6beddf0e86d"

	edgeCancellationHash = "0xd364e9d2b240c80ef90453f67f81bef0b09a9475e98c23e3712d2c1f4b57e293"
	edgeCancellationRaw  = "02f86e83aa36a70784430e2340850abd43a54082520894f39fd6e51aad88f6f4ce6ab8827279cfffb922668080c001a00c09b16ba3ab6353e34e4e3e9fec08d7c4a7cf4881d120baf44e62d46eaff0eaa02ef68a773363b3218c38a29616f3cc5006255cce91fcc1563bdff73f947e201c"
)

// TestReplacementEdgeCharacterization is the supplementary immutable TX-R1 edge baseline.
// Expected classifications and logs are literals: this test must not call the production error
// classifiers to manufacture its expectations.
func TestReplacementEdgeCharacterization(t *testing.T) {
	t.Run("pre-existing nonce conflict is a complete no-op", characterizePreExistingConflictNoOp)
	t.Run("cancellation due at entry replaces with cancellation", characterizeCancellationDueAtEntry)
	t.Run("fee replacement already known appends once", characterizeKnownFeeReplacement)
	t.Run("definite replacement rejection is not tracked", characterizeDefiniteReplacementRejection)
	t.Run("normal ambiguous exact retry already known stays in place", characterizeKnownExactRetry)
	t.Run("capped same-mode ambiguous retry stays in place", characterizeCappedAmbiguousRetry)
	t.Run("capped nonce-low retry records conflict", characterizeCappedNonceLowRetry)
	t.Run("capped retry without matching mode returns false", characterizeNoEligibleCappedRetry)
	t.Run("cancellation requested at fee barrier recurses before signing", characterizeCancellationAtFeeBarrier)
	t.Run("replacement fee failures preserve ledger", characterizeReplacementFeeFailures)
}

type replacementEdgeBackend struct {
	*mockBackend

	eventsMu sync.Mutex
	events   []string

	headerCalls          int
	receiptCalls         int
	respectHeaderContext bool
	blockHeaderCall      int
	headerEntered        chan struct{}
	headerRelease        chan struct{}
	headerEnteredOnce    sync.Once

	outcomes []error
}

func newReplacementEdgeBackend() *replacementEdgeBackend {
	return &replacementEdgeBackend{mockBackend: newMockBackend()}
}

func (b *replacementEdgeBackend) recordEvent(event string) {
	b.eventsMu.Lock()
	defer b.eventsMu.Unlock()
	b.events = append(b.events, event)
}

func (b *replacementEdgeBackend) eventSnapshot() []string {
	b.eventsMu.Lock()
	defer b.eventsMu.Unlock()
	return append([]string(nil), b.events...)
}

func (b *replacementEdgeBackend) HeaderByNumber(
	ctx context.Context,
	number *big.Int,
) (*types.Header, error) {
	b.mu.Lock()
	b.headerCalls++
	call := b.headerCalls
	b.mu.Unlock()
	b.recordEvent(fmt.Sprintf("header[%d]:enter", call))
	if call == b.blockHeaderCall {
		b.headerEnteredOnce.Do(func() { close(b.headerEntered) })
		select {
		case <-b.headerRelease:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if b.respectHeaderContext {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	header, headerErr := b.mockBackend.HeaderByNumber(ctx, number)
	b.recordEvent(fmt.Sprintf("header[%d]:return", call))
	return header, headerErr
}

func (b *replacementEdgeBackend) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	b.mu.Lock()
	call := b.tipCalls + 1
	b.mu.Unlock()
	b.recordEvent(fmt.Sprintf("tip[%d]", call))
	return b.mockBackend.SuggestGasTipCap(ctx)
}

func (b *replacementEdgeBackend) SendTransaction(_ context.Context, tx *types.Transaction) error {
	b.mu.Lock()
	call := b.sendCalls
	b.sendCalls++
	b.attempted = append(b.attempted, tx)
	var err error
	if call < len(b.outcomes) {
		err = b.outcomes[call]
	}
	if err == nil {
		b.sent = append(b.sent, tx)
	}
	b.mu.Unlock()
	b.recordEvent(fmt.Sprintf("send[%d]", call+1))
	return err
}

func (b *replacementEdgeBackend) TransactionReceipt(
	ctx context.Context,
	hash common.Hash,
) (*types.Receipt, error) {
	b.mu.Lock()
	b.receiptCalls++
	b.mu.Unlock()
	return b.mockBackend.TransactionReceipt(ctx, hash)
}

type replacementEdgeCounts struct {
	headers  int
	tips     int
	sends    int
	receipts int
}

func (b *replacementEdgeBackend) counts() replacementEdgeCounts {
	b.mu.Lock()
	defer b.mu.Unlock()
	return replacementEdgeCounts{
		headers: b.headerCalls, tips: b.tipCalls, sends: b.sendCalls, receipts: b.receiptCalls,
	}
}

type replacementEdgeHarness struct {
	backend *replacementEdgeBackend
	signer  *countingSigner
	manager *Manager
	logs    *characterizationLogs
}

func newReplacementEdgeHarness(
	t *testing.T,
	cfg Config,
) *replacementEdgeHarness {
	t.Helper()
	backend := newReplacementEdgeBackend()
	counted := &countingSigner{Signer: mustSigner(t)}
	logs := new(characterizationLogs)
	logger := funcr.NewJSON(logs.append, funcr.Options{Verbosity: 1})
	if cfg.MaxFeeGwei == 0 {
		cfg.MaxFeeGwei = 100
	}
	if cfg.TipGwei == 0 {
		cfg.TipGwei = 1
	}
	return &replacementEdgeHarness{
		backend: backend,
		signer:  counted,
		manager: New(backend, counted, big.NewInt(characterizationChainID), cfg, logger),
		logs:    logs,
	}
}

func edgePending(t *testing.T, label string) *pendingTransaction {
	t.Helper()
	original := mustEdgeTransaction(t, edgeOriginalRaw)
	return &pendingTransaction{
		req: Request{
			To:    common.HexToAddress("0x0000000000000000000000000000000000000abc"),
			Data:  []byte{0x01, 0x02, 0x03},
			Value: big.NewInt(17),
			Label: label,
		},
		nonce: 7,
		gas:   45_678,
		value: big.NewInt(17),
		fees: feeQuote{
			baseFee: big.NewInt(20_000_000_000),
			tip:     big.NewInt(1_000_000_000),
			maxFee:  big.NewInt(41_000_000_000),
		},
		attempts:        []txAttempt{{hash: original.Hash(), tx: original}},
		originalHash:    original.Hash(),
		cancelRequested: make(chan struct{}),
	}
}

func mustEdgeTransaction(t *testing.T, rawHex string) *types.Transaction {
	t.Helper()
	raw, err := hex.DecodeString(rawHex)
	if err != nil {
		t.Fatalf("decode transaction fixture: %v", err)
	}
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(raw); err != nil {
		t.Fatalf("unmarshal transaction fixture: %v", err)
	}
	return tx
}

func edgeLedgerSnapshot(t *testing.T, pending *pendingTransaction) string {
	t.Helper()
	attempts := ""
	for i, attempt := range pending.attempts {
		raw := "nil"
		if attempt.tx != nil {
			raw = hex.EncodeToString(mustMarshalTransaction(t, attempt.tx))
		}
		attempts += fmt.Sprintf("%d:%s:%t:%t:%s;", i, attempt.hash.Hex(), attempt.cancellation,
			attempt.exactRebroadcastPending, raw)
	}
	return fmt.Sprintf("fees=%s/%s/%s conflict=%s attempts=%s",
		pending.fees.baseFee, pending.fees.tip, pending.fees.maxFee,
		pending.nonceConflictHash.Hex(), attempts)
}

func requireEdgeLogs(t *testing.T, logs *characterizationLogs, want ...string) {
	t.Helper()
	got := logs.snapshot()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("logs changed:\n got: %q\nwant: %q", got, want)
	}
}

func requireEdgeCounts(
	t *testing.T,
	h *replacementEdgeHarness,
	wantHeaders, wantTips, wantSends, wantReceipts int,
) {
	t.Helper()
	got := h.backend.counts()
	if got.headers != wantHeaders || got.tips != wantTips || got.sends != wantSends || got.receipts != wantReceipts {
		t.Fatalf("backend calls header/tip/send/receipt = %d/%d/%d/%d, want %d/%d/%d/%d",
			got.headers, got.tips, got.sends, got.receipts,
			wantHeaders, wantTips, wantSends, wantReceipts)
	}
}

func characterizePreExistingConflictNoOp(t *testing.T) {
	h := newReplacementEdgeHarness(t, Config{})
	pending := edgePending(t, "edge-conflict-noop")
	conflictHash := common.HexToHash("0x111122223333444455556666777788889999aaaabbbbccccddddeeeeffff0000")
	h.manager.conflict = &nonceConflict{nonce: pending.nonce, hash: conflictHash}
	before := edgeLedgerSnapshot(t, pending)

	h.manager.tryReplace(t.Context(), pending, false)

	if got := edgeLedgerSnapshot(t, pending); got != before {
		t.Fatalf("pre-existing conflict mutated ledger:\n before: %s\n after: %s", before, got)
	}
	if h.signer.calls.Load() != 0 {
		t.Fatalf("sign calls = %d, want 0", h.signer.calls.Load())
	}
	requireEdgeCounts(t, h, 0, 0, 0, 0)
	h.manager.mu.Lock()
	conflict := h.manager.conflict
	h.manager.mu.Unlock()
	if conflict == nil || conflict.nonce != 7 || conflict.hash != conflictHash {
		t.Fatalf("conflict changed: %+v", conflict)
	}
	requireEdgeLogs(t, h.logs)
}

func characterizeCancellationDueAtEntry(t *testing.T) {
	h := newReplacementEdgeHarness(t, Config{})
	pending := edgePending(t, "edge-due-at-entry")
	requestCancellation(pending)

	h.manager.tryReplace(t.Context(), pending, false)

	if h.signer.calls.Load() != 1 || len(pending.attempts) != 2 {
		t.Fatalf("signs/attempts = %d/%d, want one cancellation append", h.signer.calls.Load(), len(pending.attempts))
	}
	cancellation := pending.attempts[1]
	if !cancellation.cancellation || cancellation.exactRebroadcastPending ||
		cancellation.hash.Hex() != edgeCancellationHash {
		t.Fatalf("cancellation attempt = %+v", cancellation)
	}
	if got := hex.EncodeToString(mustMarshalTransaction(t, cancellation.tx)); got != edgeCancellationRaw {
		t.Fatalf("cancellation raw = %s, want %s", got, edgeCancellationRaw)
	}
	if got := feeQuoteTrace(pending.fees); got != "base=20000000000/tip=1125000000/max=46125000000" {
		t.Fatalf("cached fees = %s", got)
	}
	if events := h.backend.eventSnapshot(); fmt.Sprint(events) !=
		"[header[1]:enter header[1]:return tip[1] send[1]]" {
		t.Fatalf("entry cancellation call order = %v", events)
	}
	requireEdgeCounts(t, h, 1, 1, 1, 0)
	requireEdgeLogs(t, h.logs,
		`{"cancellation":true,"hash":"0xd364e9d2b240c80ef90453f67f81bef0b09a9475e98c23e3712d2c1f4b57e293","label":"edge-due-at-entry","level":0,"logger":"txmanager","maxFeePerGas":"46125000000","maxPriorityFeePerGas":"1125000000","msg":"pending transaction replaced","nonce":7}`,
	)
}

func characterizeKnownFeeReplacement(t *testing.T) {
	h := newReplacementEdgeHarness(t, Config{})
	h.backend.outcomes = []error{errors.New("already known")}
	pending := edgePending(t, "edge-known")

	h.manager.tryReplace(t.Context(), pending, false)

	if h.signer.calls.Load() != 1 || len(pending.attempts) != 2 {
		t.Fatalf("signs/attempts = %d/%d, want 1/2", h.signer.calls.Load(), len(pending.attempts))
	}
	replacement := pending.attempts[1]
	if replacement.hash.Hex() != edgeReplacementHash || replacement.cancellation ||
		replacement.exactRebroadcastPending {
		t.Fatalf("replacement attempt = %+v", replacement)
	}
	if pending.nonceConflictHash != (common.Hash{}) || h.manager.conflict != nil {
		t.Fatalf("already-known replacement marked conflict: pending=%s manager=%+v",
			pending.nonceConflictHash, h.manager.conflict)
	}
	if got := hex.EncodeToString(mustMarshalTransaction(t, replacement.tx)); got != edgeReplacementRaw {
		t.Fatalf("replacement raw = %s, want %s", got, edgeReplacementRaw)
	}
	if replacement.tx.Nonce() != 7 || replacement.tx.To() == nil ||
		*replacement.tx.To() != pending.req.To || !bytes.Equal(replacement.tx.Data(), []byte{1, 2, 3}) ||
		replacement.tx.Value().Cmp(big.NewInt(17)) != 0 || replacement.tx.Gas() != 45_678 {
		t.Fatalf("replacement changed transaction shape: %s", transactionTrace(t, replacement.tx, h.signer.Address()))
	}
	if got := feeQuoteTrace(pending.fees); got != "base=20000000000/tip=1125000000/max=46125000000" {
		t.Fatalf("cached fees = %s", got)
	}
	requireEdgeCounts(t, h, 1, 1, 1, 0)
	requireEdgeLogs(t, h.logs,
		`{"cancellation":false,"hash":"0xbed1489e14e161359c9c7048fdf00951658860f07c88db5221b6441e74f5684b","label":"edge-known","level":0,"logger":"txmanager","msg":"replacement already known by write RPC","nonce":7,"rpcResult":"already known"}`,
	)
}

func characterizeDefiniteReplacementRejection(t *testing.T) {
	h := newReplacementEdgeHarness(t, Config{})
	h.backend.outcomes = []error{errors.New("insufficient funds for gas * price + value")}
	pending := edgePending(t, "edge-rejected")
	before := edgeLedgerSnapshot(t, pending)

	h.manager.tryReplace(t.Context(), pending, false)

	if got := edgeLedgerSnapshot(t, pending); got != before {
		t.Fatalf("definite rejection reached ledger:\n before: %s\n after: %s", before, got)
	}
	if h.signer.calls.Load() != 1 {
		t.Fatalf("sign calls = %d, want 1", h.signer.calls.Load())
	}
	requireEdgeCounts(t, h, 1, 1, 1, 0)
	attempted := h.backend.attemptedTransactions()
	if len(attempted) != 1 || hex.EncodeToString(mustMarshalTransaction(t, attempted[0])) != edgeReplacementRaw {
		t.Fatalf("attempted rejected transaction = %v", transactionHashes(attempted))
	}
	requireEdgeLogs(t, h.logs,
		`{"cancellation":false,"error":"broadcast rejected before acceptance: insufficient funds for gas * price + value","label":"edge-rejected","logger":"txmanager","msg":"pending transaction replacement rejected","nonce":7}`,
	)
}

func characterizeKnownExactRetry(t *testing.T) {
	h := newReplacementEdgeHarness(t, Config{})
	h.backend.outcomes = []error{errors.New("already known")}
	pending := edgePending(t, "edge-exact-known")
	pending.attempts[0].exactRebroadcastPending = true
	beforeFees := feeQuoteTrace(pending.fees)
	beforeRaw := mustMarshalTransaction(t, pending.attempts[0].tx)

	h.manager.tryReplace(t.Context(), pending, false)

	if h.signer.calls.Load() != 0 || len(pending.attempts) != 1 ||
		pending.attempts[0].exactRebroadcastPending || feeQuoteTrace(pending.fees) != beforeFees {
		t.Fatalf("exact retry state = signs:%d ledger:%s", h.signer.calls.Load(), edgeLedgerSnapshot(t, pending))
	}
	attempted := h.backend.attemptedTransactions()
	if len(attempted) != 1 || !bytes.Equal(mustMarshalTransaction(t, attempted[0]), beforeRaw) {
		t.Fatal("exact retry did not resend identical raw bytes")
	}
	requireEdgeCounts(t, h, 0, 0, 1, 0)
	requireEdgeLogs(t, h.logs,
		`{"hash":"0x8e97c5ceeed718d973a80568b1be803ecf6741373bd57cfbd5056b818ebc462b","label":"edge-exact-known","level":0,"logger":"txmanager","msg":"uncertain transaction already known by write RPC","nonce":7,"reason":"ambiguous-broadcast","rpcResult":"already known"}`,
	)
}

func characterizeCappedAmbiguousRetry(t *testing.T) {
	h := newReplacementEdgeHarness(t, Config{MaxFeeGwei: 50})
	h.backend.outcomes = []error{io.ErrUnexpectedEOF}
	pending := edgePending(t, "edge-capped-ambiguous")
	before := edgeLedgerSnapshot(t, pending)

	h.manager.tryReplace(t.Context(), pending, false)

	if got := edgeLedgerSnapshot(t, pending); got != before {
		t.Fatalf("capped ambiguous retry mutated ledger:\n before: %s\n after: %s", before, got)
	}
	if h.signer.calls.Load() != 0 {
		t.Fatalf("sign calls = %d, want 0", h.signer.calls.Load())
	}
	requireEdgeCounts(t, h, 1, 1, 1, 0)
	requireEdgeLogs(t, h.logs,
		`{"cancellation":false,"error":"unexpected EOF","hash":"0x8e97c5ceeed718d973a80568b1be803ecf6741373bd57cfbd5056b818ebc462b","label":"edge-capped-ambiguous","logger":"txmanager","msg":"capped transaction rebroadcast failed","nonce":7}`,
	)
}

func characterizeCappedNonceLowRetry(t *testing.T) {
	h := newReplacementEdgeHarness(t, Config{MaxFeeGwei: 50})
	h.backend.outcomes = []error{errors.New("nonce too low")}
	pending := edgePending(t, "edge-capped-nonce-low")
	before := edgeLedgerSnapshot(t, pending)
	laneEdges, unsubscribe := h.manager.SubscribeLaneState()
	defer unsubscribe()

	h.manager.tryReplace(t.Context(), pending, false)
	awaitCharacterizationSignal(t, laneEdges, "capped nonce-low lane pause")

	if h.signer.calls.Load() != 0 || len(pending.attempts) != 1 ||
		pending.attempts[0].cancellation || pending.attempts[0].exactRebroadcastPending ||
		hex.EncodeToString(mustMarshalTransaction(t, pending.attempts[0].tx)) != edgeOriginalRaw ||
		feeQuoteTrace(pending.fees) != "base=20000000000/tip=1000000000/max=41000000000" {
		t.Fatalf("capped nonce-low changed signed ledger: %s", edgeLedgerSnapshot(t, pending))
	}
	if pending.nonceConflictHash.Hex() != edgeOriginalHash {
		t.Fatalf("pending conflict hash = %s, want %s", pending.nonceConflictHash, edgeOriginalHash)
	}
	h.manager.mu.Lock()
	conflict := h.manager.conflict
	h.manager.mu.Unlock()
	if conflict == nil || conflict.nonce != 7 || conflict.hash.Hex() != edgeOriginalHash || h.manager.Available() {
		t.Fatalf("manager conflict = %+v available=%t", conflict, h.manager.Available())
	}
	if before == edgeLedgerSnapshot(t, pending) {
		t.Fatal("nonce-low retry did not record its exact conflict hash")
	}
	requireEdgeCounts(t, h, 1, 1, 1, 1)
	requireEdgeLogs(t, h.logs,
		`{"error":"nonce ownership is uncertain","hash":"0x8e97c5ceeed718d973a80568b1be803ecf6741373bd57cfbd5056b818ebc462b","logger":"txmanager","msg":"transaction manager paused pending nonce reconciliation","nonce":7}`,
		`{"cancellation":false,"error":"nonce too low","hash":"0x8e97c5ceeed718d973a80568b1be803ecf6741373bd57cfbd5056b818ebc462b","label":"edge-capped-nonce-low","logger":"txmanager","msg":"capped transaction rebroadcast failed","nonce":7}`,
	)
}

func characterizeNoEligibleCappedRetry(t *testing.T) {
	h := newReplacementEdgeHarness(t, Config{MaxFeeGwei: 50})
	pending := edgePending(t, "edge-no-capped-mode")
	pending.attempts[0].cancellation = true
	before := edgeLedgerSnapshot(t, pending)

	if h.manager.rebroadcastLatestAttempt(t.Context(), pending, false) {
		t.Fatal("mismatched capped attempt was reported eligible")
	}
	if got := edgeLedgerSnapshot(t, pending); got != before {
		t.Fatalf("ineligible capped retry mutated ledger:\n before: %s\n after: %s", before, got)
	}
	if h.signer.calls.Load() != 0 {
		t.Fatalf("sign calls = %d, want 0", h.signer.calls.Load())
	}
	requireEdgeCounts(t, h, 0, 0, 0, 0)
	requireEdgeLogs(t, h.logs)
}

func characterizeCancellationAtFeeBarrier(t *testing.T) {
	h := newReplacementEdgeHarness(t, Config{})
	h.backend.blockHeaderCall = 1
	h.backend.headerEntered = make(chan struct{})
	h.backend.headerRelease = make(chan struct{})
	pending := edgePending(t, "edge-fee-barrier-cancel")

	done := make(chan struct{})
	go func() {
		h.manager.tryReplace(t.Context(), pending, false)
		close(done)
	}()
	awaitCharacterizationSignal(t, h.backend.headerEntered, "replacement fee-read barrier")
	requestCancellation(pending)
	close(h.backend.headerRelease)
	awaitCharacterizationSignal(t, done, "recursive cancellation replacement")

	if h.signer.calls.Load() != 1 || len(pending.attempts) != 2 {
		t.Fatalf("signs/attempts = %d/%d, want one cancellation append", h.signer.calls.Load(), len(pending.attempts))
	}
	cancellation := pending.attempts[1]
	if !cancellation.cancellation || cancellation.exactRebroadcastPending ||
		cancellation.hash.Hex() != edgeCancellationHash {
		t.Fatalf("cancellation attempt = %+v", cancellation)
	}
	if got := hex.EncodeToString(mustMarshalTransaction(t, cancellation.tx)); got != edgeCancellationRaw {
		t.Fatalf("cancellation raw = %s, want %s", got, edgeCancellationRaw)
	}
	if got := feeQuoteTrace(pending.fees); got != "base=20000000000/tip=1125000000/max=46125000000" {
		t.Fatalf("cached fees = %s", got)
	}
	if events := h.backend.eventSnapshot(); fmt.Sprint(events) !=
		"[header[1]:enter header[1]:return tip[1] header[2]:enter header[2]:return tip[2] send[1]]" {
		t.Fatalf("fee/cancellation call order = %v", events)
	}
	requireEdgeCounts(t, h, 2, 2, 1, 0)
	requireEdgeLogs(t, h.logs,
		`{"cancellation":true,"hash":"0xd364e9d2b240c80ef90453f67f81bef0b09a9475e98c23e3712d2c1f4b57e293","label":"edge-fee-barrier-cancel","level":0,"logger":"txmanager","maxFeePerGas":"46125000000","maxPriorityFeePerGas":"1125000000","msg":"pending transaction replaced","nonce":7}`,
	)
}

func characterizeReplacementFeeFailures(t *testing.T) {
	t.Run("base fee exceeds normal limit", func(t *testing.T) {
		h := newReplacementEdgeHarness(t, Config{MaxFeeGwei: 50})
		h.backend.baseFee = big.NewInt(50_000_000_000)
		pending := edgePending(t, "edge-base-over-limit")
		before := edgeLedgerSnapshot(t, pending)

		h.manager.tryReplace(t.Context(), pending, false)

		if got := edgeLedgerSnapshot(t, pending); got != before {
			t.Fatalf("base-fee failure mutated ledger:\n before: %s\n after: %s", before, got)
		}
		if h.signer.calls.Load() != 0 {
			t.Fatalf("sign calls = %d, want 0", h.signer.calls.Load())
		}
		requireEdgeCounts(t, h, 1, 1, 0, 0)
		requireEdgeLogs(t, h.logs,
			`{"cancellation":false,"error":"replacement base fee 50000000000 exceeds fee limit 44444444444","label":"edge-base-over-limit","logger":"txmanager","msg":"cannot replace pending transaction","nonce":7}`,
		)
	})

	t.Run("cancelled fee read fails at signing boundary", func(t *testing.T) {
		h := newReplacementEdgeHarness(t, Config{})
		h.backend.respectHeaderContext = true
		pending := edgePending(t, "edge-fee-read-cancelled")
		before := edgeLedgerSnapshot(t, pending)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		h.manager.tryReplace(ctx, pending, false)

		if got := edgeLedgerSnapshot(t, pending); got != before {
			t.Fatalf("cancelled fee-read path mutated ledger:\n before: %s\n after: %s", before, got)
		}
		if h.signer.calls.Load() != 1 {
			t.Fatalf("sign calls = %d, want current failure timing at signing", h.signer.calls.Load())
		}
		requireEdgeCounts(t, h, 1, 0, 0, 0)
		requireEdgeLogs(t, h.logs,
			`{"error":"fresh fees unavailable: header by number: context canceled","level":1,"logger":"txmanager","msg":"fresh replacement fees unavailable; using cached bump"}`,
			`{"cancellation":false,"error":"sign transaction: context canceled","label":"edge-fee-read-cancelled","logger":"txmanager","msg":"pending transaction replacement rejected","nonce":7}`,
		)
	})
}

var _ Backend = (*replacementEdgeBackend)(nil)
