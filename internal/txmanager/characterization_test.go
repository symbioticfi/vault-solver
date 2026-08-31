package txmanager

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr/funcr"

	"github.com/symbioticfi/vault-solver/internal/signer"
)

const characterizationChainID = 11155111

// TestReplacementAttemptCharacterizationTrace is the immutable TX-R1 comparison baseline.
// Candidate implementations must preserve this externally observable trace rather than deriving a
// new expectation from their implementation.
func TestReplacementAttemptCharacterizationTrace(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T) []string
		want string
	}{
		{name: "normal initial send", run: traceNormalInitialSend, want: characterizationWantNormalInitialSend},
		{
			name: "ambiguous exact rebroadcast then fee bump",
			run:  traceAmbiguousReplacement,
			want: characterizationWantAmbiguousExactRebroadcastThenFeeBump,
		},
		{name: "already known initial send", run: traceAlreadyKnown, want: characterizationWantAlreadyKnownInitialSend},
		{
			name: "replacement nonce conflict receipt and reorg",
			run:  traceNonceConflictReorg,
			want: characterizationWantReplacementNonceConflictReceiptAndReorg,
		},
		{
			name: "capped cancellation exact rebroadcast",
			run:  traceCappedCancellation,
			want: characterizationWantCappedCancellationExactRebroadcast,
		},
		{
			name: "shutdown cancellation and drain",
			run:  traceShutdownDrain,
			want: characterizationWantShutdownCancellationAndDrain,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := strings.Join(test.run(t), "\n")
			if got != test.want {
				t.Fatalf("characterization trace changed (-want +got):\n-WANT-\n%s\n-GOT-\n%s", test.want, got)
			}
		})
	}
}

type characterizationHarness struct {
	backend *characterizationBackend
	signer  *countingSigner
	manager *Manager
	logs    *characterizationLogs
}

type characterizationBackend struct {
	*mockBackend

	outcomes     []error
	receiptCalls map[int]bool
	sendEvents   chan int
}

func (b *characterizationBackend) SendTransaction(_ context.Context, tx *types.Transaction) error {
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
	if b.receiptCalls[call] {
		b.receipts[tx.Hash()] = successfulReceipt(tx, b.head)
	}
	if isNonceConsumedError(err) {
		b.latestNonce = tx.Nonce() + 1
		b.pendingNonce = tx.Nonce() + 1
	}
	b.mu.Unlock()
	if b.sendEvents != nil {
		b.sendEvents <- call
	}
	return err
}

type countingSigner struct {
	signer.Signer

	calls atomic.Int64
}

func (s *countingSigner) SignTx(
	ctx context.Context,
	tx *types.Transaction,
	chainID *big.Int,
) (*types.Transaction, error) {
	s.calls.Add(1)
	return s.Signer.SignTx(ctx, tx, chainID)
}

type characterizationLogs struct {
	mu      sync.Mutex
	entries []string
}

type characterizationReceiptOutcome struct {
	result Result
	done   bool
}

func (l *characterizationLogs) append(entry string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var normalized map[string]any
	if err := json.Unmarshal([]byte(entry), &normalized); err != nil {
		panic(fmt.Sprintf("normalize characterization log: %v", err))
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		panic(fmt.Sprintf("marshal characterization log: %v", err))
	}
	l.entries = append(l.entries, string(encoded))
}

func (l *characterizationLogs) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.entries...)
}

func newCharacterizationHarness(
	t *testing.T,
	outcomes []error,
	cfg Config,
) *characterizationHarness {
	t.Helper()
	base := newMockBackend()
	backend := &characterizationBackend{
		mockBackend:  base,
		outcomes:     outcomes,
		receiptCalls: make(map[int]bool),
	}
	baseSigner := mustSigner(t)
	counted := &countingSigner{Signer: baseSigner}
	logs := new(characterizationLogs)
	logger := funcr.NewJSON(logs.append, funcr.Options{Verbosity: 1})
	if cfg.MaxFeeGwei == 0 {
		cfg.MaxFeeGwei = 100
	}
	if cfg.TipGwei == 0 {
		cfg.TipGwei = 1
	}
	return &characterizationHarness{
		backend: backend,
		signer:  counted,
		manager: New(backend, counted, big.NewInt(characterizationChainID), cfg, logger),
		logs:    logs,
	}
}

func traceNormalInitialSend(t *testing.T) []string {
	t.Helper()
	h := newCharacterizationHarness(t, []error{nil}, Config{Confirmations: 0})
	trace := []string{managerStateTrace("before", h.manager, nil, h.signer.calls.Load())}
	pending := mustCharacterizationBroadcast(t, h, "normal")
	trace = append(trace, backendSendTraces(t, h)...)
	trace = append(trace, managerStateTrace("after-send", h.manager, pending, h.signer.calls.Load()))
	h.publishReceipt(pending.attempts[0].tx)
	result, done := h.manager.receiptResult(t.Context(), pending)
	trace = append(trace, resultTrace(result, done))
	trace = append(trace, logTraces(h.logs)...)
	return trace
}

func traceAmbiguousReplacement(t *testing.T) []string {
	t.Helper()
	h := newCharacterizationHarness(t, []error{io.ErrUnexpectedEOF, io.ErrUnexpectedEOF, nil}, Config{})
	pending := mustCharacterizationBroadcast(t, h, "ambiguous")
	trace := []string{managerStateTrace("ambiguous-initial", h.manager, pending, h.signer.calls.Load())}

	attemptsBefore := len(pending.attempts)
	signsBefore := h.signer.calls.Load()
	feesBefore := cloneFeeQuote(pending.fees)
	rawBefore := mustMarshalTransaction(t, pending.attempts[0].tx)
	h.manager.tryReplace(t.Context(), pending, false)
	if len(pending.attempts) != attemptsBefore {
		t.Fatalf("exact retry appended an attempt: before=%d after=%d", attemptsBefore, len(pending.attempts))
	}
	if h.signer.calls.Load() != signsBefore {
		t.Fatalf("exact retry signed again: before=%d after=%d", signsBefore, h.signer.calls.Load())
	}
	if !equalFeeQuote(pending.fees, feesBefore) {
		t.Fatalf("exact retry mutated fees: before=%s after=%s", feeQuoteTrace(feesBefore), feeQuoteTrace(pending.fees))
	}
	if rawAfter := mustMarshalTransaction(t, h.backend.attemptedTransactions()[1]); !bytes.Equal(rawBefore, rawAfter) {
		t.Fatalf("exact retry changed raw transaction bytes: before=%x after=%x", rawBefore, rawAfter)
	}
	trace = append(trace, "exact-retry invariant attempts=1 signs=1 fees=unchanged raw=identical")
	trace = append(trace, managerStateTrace("after-exact", h.manager, pending, h.signer.calls.Load()))

	h.manager.tryReplace(t.Context(), pending, false)
	trace = append(trace, managerStateTrace("after-bump", h.manager, pending, h.signer.calls.Load()))
	trace = append(trace, backendSendTraces(t, h)...)
	h.publishReceipt(pending.attempts[1].tx)
	result, done := h.manager.receiptResult(t.Context(), pending)
	trace = append(trace, resultTrace(result, done))
	trace = append(trace, logTraces(h.logs)...)
	return trace
}

func traceAlreadyKnown(t *testing.T) []string {
	t.Helper()
	h := newCharacterizationHarness(t, []error{errors.New("already known")}, Config{})
	pending := mustCharacterizationBroadcast(t, h, "already-known")
	trace := backendSendTraces(t, h)
	trace = append(trace, managerStateTrace("after-known", h.manager, pending, h.signer.calls.Load()))
	h.publishReceipt(pending.attempts[0].tx)
	result, done := h.manager.receiptResult(t.Context(), pending)
	trace = append(trace, resultTrace(result, done))
	trace = append(trace, logTraces(h.logs)...)
	return trace
}

func traceNonceConflictReorg(t *testing.T) []string {
	t.Helper()
	h := newCharacterizationHarness(t, []error{nil, errors.New("nonce too low")}, Config{
		Confirmations: 2, PollInterval: time.Millisecond,
	})
	laneEdges, unsubscribe := h.manager.SubscribeLaneState()
	defer unsubscribe()
	pending := mustCharacterizationBroadcast(t, h, "nonce-conflict")
	trace := []string{managerStateTrace("before-replacement", h.manager, pending, h.signer.calls.Load())}
	h.manager.tryReplace(t.Context(), pending, false)
	awaitCharacterizationSignal(t, laneEdges, "nonce-conflict pause")
	trace = append(trace, "lane-edge pause")
	trace = append(trace, managerStateTrace("conflicted", h.manager, pending, h.signer.calls.Load()))

	original := pending.attempts[0]
	h.publishReceipt(original.tx)
	outcome := make(chan characterizationReceiptOutcome, 1)
	go func() {
		result, done := h.manager.receiptResult(t.Context(), pending)
		outcome <- characterizationReceiptOutcome{result: result, done: done}
	}()
	awaitCharacterizationSignal(t, laneEdges, "canonical receipt resume")
	trace = append(trace, "lane-edge resume-canonical")
	trace = append(trace, managerStateTrace("canonical-confirming", h.manager, pending, h.signer.calls.Load()))
	h.removeReceipt(original.hash)
	first := awaitReceiptOutcome(t, outcome)
	if first.done {
		t.Fatalf("reorg phase unexpectedly completed: %+v", first.result)
	}
	awaitCharacterizationSignal(t, laneEdges, "receipt reorg pause")
	trace = append(trace, resultTrace(first.result, first.done), "lane-edge pause-reorg")
	trace = append(trace, managerStateTrace("reorged", h.manager, pending, h.signer.calls.Load()))

	h.backend.mu.Lock()
	h.backend.head = 102
	h.backend.receipts[original.hash] = successfulReceipt(original.tx, 100)
	h.backend.mu.Unlock()
	result, done := h.manager.receiptResult(t.Context(), pending)
	awaitCharacterizationSignal(t, laneEdges, "restored receipt resume")
	trace = append(trace, "lane-edge resume-restored")
	trace = append(trace, managerStateTrace("restored", h.manager, pending, h.signer.calls.Load()))
	trace = append(trace, backendSendTraces(t, h)...)
	trace = append(trace, resultTrace(result, done))
	trace = append(trace, logTraces(h.logs)...)
	return trace
}

func traceCappedCancellation(t *testing.T) []string {
	t.Helper()
	h := newCharacterizationHarness(t, []error{nil, io.ErrUnexpectedEOF, nil}, Config{MaxFeeGwei: 50})
	h.backend.baseFee = big.NewInt(30_000_000_000)
	pending := mustCharacterizationBroadcast(t, h, "capped-cancel")
	h.manager.tryReplace(t.Context(), pending, true)
	trace := []string{managerStateTrace("cancellation-at-cap", h.manager, pending, h.signer.calls.Load())}

	attemptsBefore := len(pending.attempts)
	signsBefore := h.signer.calls.Load()
	feesBefore := cloneFeeQuote(pending.fees)
	cancellationRaw := mustMarshalTransaction(t, pending.attempts[1].tx)
	h.manager.tryReplace(t.Context(), pending, true)
	if len(pending.attempts) != attemptsBefore || h.signer.calls.Load() != signsBefore ||
		!equalFeeQuote(pending.fees, feesBefore) {
		t.Fatalf("capped exact retry mutated state: attempts %d/%d signs %d/%d fees %s/%s",
			attemptsBefore, len(pending.attempts), signsBefore, h.signer.calls.Load(),
			feeQuoteTrace(feesBefore), feeQuoteTrace(pending.fees))
	}
	if rawAfter := mustMarshalTransaction(t, h.backend.attemptedTransactions()[2]); !bytes.Equal(cancellationRaw, rawAfter) {
		t.Fatalf("capped cancellation retry changed bytes: before=%x after=%x", cancellationRaw, rawAfter)
	}
	trace = append(trace, "capped-exact invariant attempts=2 signs=2 fees=unchanged raw=identical")
	trace = append(trace, managerStateTrace("after-capped-exact", h.manager, pending, h.signer.calls.Load()))
	trace = append(trace, backendSendTraces(t, h)...)
	h.publishReceipt(pending.attempts[1].tx)
	result, done := h.manager.receiptResult(t.Context(), pending)
	trace = append(trace, resultTrace(result, done))
	trace = append(trace, logTraces(h.logs)...)
	return trace
}

func traceShutdownDrain(t *testing.T) []string {
	t.Helper()
	h := newCharacterizationHarness(t, []error{nil, nil}, Config{
		PollInterval:        time.Hour,
		ReplacementInterval: time.Hour,
		PendingTimeout:      time.Hour,
		ShutdownTimeout:     time.Second,
	})
	h.backend.receiptCalls[1] = true
	h.backend.sendEvents = make(chan int, 2)
	managerCtx, cancelManager := context.WithCancel(t.Context())
	startDone := make(chan struct{})
	go func() {
		h.manager.Start(managerCtx)
		close(startDone)
	}()
	result, accepted := h.manager.SendAsync(t.Context(), characterizationRequest("shutdown"))
	if !accepted {
		t.Fatal("shutdown characterization request was not accepted")
	}
	awaitCharacterizationSend(t, h.backend.sendEvents, 0)
	trace := []string{managerStateTrace("before-shutdown", h.manager, nil, h.signer.calls.Load())}
	cancelManager()
	awaitCharacterizationSend(t, h.backend.sendEvents, 1)
	terminal := awaitCharacterizationResult(t, result)
	awaitCharacterizationSignal(t, startDone, "manager drain")
	trace = append(trace, backendSendTraces(t, h)...)
	trace = append(trace, managerStateTrace("after-drain", h.manager, nil, h.signer.calls.Load()))
	trace = append(trace, resultTrace(terminal, true))
	trace = append(trace, logTraces(h.logs)...)
	return trace
}

func mustCharacterizationBroadcast(
	t *testing.T,
	h *characterizationHarness,
	label string,
) *pendingTransaction {
	t.Helper()
	pending, err := h.manager.broadcast(t.Context(), characterizationRequest(label))
	if err != nil {
		t.Fatalf("broadcast %q: %v", label, err)
	}
	return pending
}

func characterizationRequest(label string) Request {
	return Request{
		To:       common.HexToAddress("0x0000000000000000000000000000000000000abc"),
		Data:     []byte{0x01, 0x02, 0x03},
		Value:    big.NewInt(17),
		GasLimit: 45_678,
		Label:    label,
	}
}

func (h *characterizationHarness) publishReceipt(tx *types.Transaction) {
	h.backend.mu.Lock()
	defer h.backend.mu.Unlock()
	h.backend.receipts[tx.Hash()] = successfulReceipt(tx, h.backend.head)
}

func (h *characterizationHarness) removeReceipt(hash common.Hash) {
	h.backend.mu.Lock()
	defer h.backend.mu.Unlock()
	delete(h.backend.receipts, hash)
}

func backendSendTraces(t *testing.T, h *characterizationHarness) []string {
	t.Helper()
	transactions := h.backend.attemptedTransactions()
	traces := make([]string, 0, len(transactions)*2)
	transactionIDs := make(map[common.Hash]int)
	unique := make([]*types.Transaction, 0, len(transactions))
	for i, tx := range transactions {
		id, ok := transactionIDs[tx.Hash()]
		if !ok {
			id = len(unique)
			transactionIDs[tx.Hash()] = id
			unique = append(unique, tx)
		}
		var outcome error
		if i < len(h.backend.outcomes) {
			outcome = h.backend.outcomes[i]
		}
		traces = append(traces, fmt.Sprintf("send[%d] class=%s tx=%d", i, broadcastClass(outcome), id))
	}
	for id, tx := range unique {
		traces = append(traces, fmt.Sprintf("tx[%d] %s", id, transactionTrace(t, tx, h.signer.Address())))
	}
	return traces
}

func broadcastClass(err error) string {
	switch {
	case err == nil:
		return "success"
	case isKnownTransactionError(err):
		return "already-known"
	case isNonceConsumedError(err):
		return "nonce-low"
	default:
		return "ambiguous"
	}
}

func transactionTrace(t *testing.T, tx *types.Transaction, signerAddress common.Address) string {
	t.Helper()
	raw := mustMarshalTransaction(t, tx)
	sender, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	if err != nil {
		t.Fatalf("recover transaction sender: %v", err)
	}
	to := "<contract-creation>"
	if tx.To() != nil {
		to = tx.To().Hex()
	}
	cancellation := tx.To() != nil && *tx.To() == signerAddress && len(tx.Data()) == 0 &&
		tx.Value().Sign() == 0 && tx.Gas() == cancellationGasLimit
	return fmt.Sprintf(
		"hash=%s raw=0x%s sender=%s chain=%s nonce=%d to=%s data=0x%s value=%s gas=%d tip=%s fee=%s cancellation=%t",
		tx.Hash().Hex(), hex.EncodeToString(raw), sender.Hex(), tx.ChainId(), tx.Nonce(), to,
		hex.EncodeToString(tx.Data()), tx.Value(), tx.Gas(), tx.GasTipCap(), tx.GasFeeCap(), cancellation,
	)
}

func mustMarshalTransaction(t *testing.T, tx *types.Transaction) []byte {
	t.Helper()
	raw, err := tx.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal signed transaction: %v", err)
	}
	return raw
}

func managerStateTrace(label string, m *Manager, pending *pendingTransaction, signCalls int64) string {
	m.mu.Lock()
	conflict := "none"
	if m.conflict != nil {
		conflict = fmt.Sprintf("nonce=%d/hash=%s", m.conflict.nonce, m.conflict.hash.Hex())
	}
	m.mu.Unlock()
	pendingState := "none"
	if pending != nil {
		attempts := make([]string, len(pending.attempts))
		for i, attempt := range pending.attempts {
			attempts[i] = fmt.Sprintf("%d:%s/cancel=%t/exact=%t", i, attempt.hash.Hex(), attempt.cancellation, attempt.exactRebroadcastPending)
		}
		pendingState = fmt.Sprintf(
			"nonce=%d fees=%s conflictHash=%s attempts=[%s]",
			pending.nonce, feeQuoteTrace(pending.fees), pending.nonceConflictHash.Hex(), strings.Join(attempts, ","),
		)
	}
	idle := m.admissionDemand.Load() == 0
	return fmt.Sprintf("state[%s] signs=%d available=%t idle=%t ready=%t conflict=%s pending={%s}",
		label, signCalls, m.Available(), idle, m.LaneReady(), conflict, pendingState)
}

func feeQuoteTrace(fees feeQuote) string {
	return fmt.Sprintf("base=%s/tip=%s/max=%s", fees.baseFee, fees.tip, fees.maxFee)
}

func equalFeeQuote(left, right feeQuote) bool {
	return left.baseFee.Cmp(right.baseFee) == 0 && left.tip.Cmp(right.tip) == 0 && left.maxFee.Cmp(right.maxFee) == 0
}

func resultTrace(result Result, done bool) string {
	receipt := "nil"
	if result.Receipt != nil {
		receipt = fmt.Sprintf("hash=%s/status=%d/blockHash=%s/block=%s",
			result.Receipt.TxHash.Hex(), result.Receipt.Status, result.Receipt.BlockHash.Hex(), result.Receipt.BlockNumber)
	}
	errText := "nil"
	if result.Err != nil {
		errText = result.Err.Error()
	}
	return fmt.Sprintf("result done=%t hash=%s receipt={%s} err=%q notAdmitted=%t",
		done, result.Hash.Hex(), receipt, errText, result.NotAdmitted)
}

func logTraces(logs *characterizationLogs) []string {
	entries := logs.snapshot()
	traces := make([]string, len(entries))
	for i, entry := range entries {
		traces[i] = fmt.Sprintf("log[%d] %s", i, entry)
	}
	return traces
}

func awaitCharacterizationSignal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}

func awaitCharacterizationSend(t *testing.T, sends <-chan int, want int) {
	t.Helper()
	select {
	case got := <-sends:
		if got != want {
			t.Fatalf("send barrier = %d, want %d", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for send %d", want)
	}
}

func awaitReceiptOutcome(
	t *testing.T,
	outcome <-chan characterizationReceiptOutcome,
) characterizationReceiptOutcome {
	t.Helper()
	select {
	case got := <-outcome:
		return got
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for receipt outcome")
		return characterizationReceiptOutcome{}
	}
}

func awaitCharacterizationResult(t *testing.T, result <-chan Result) Result {
	t.Helper()
	select {
	case got := <-result:
		return got
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for terminal result")
		return Result{}
	}
}

var _ Backend = (*characterizationBackend)(nil)
var _ signer.Signer = (*countingSigner)(nil)
