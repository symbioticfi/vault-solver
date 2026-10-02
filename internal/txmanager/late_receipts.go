package txmanager

import (
	"context"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// receiptMetadata retains only telemetry: passive observation never needs signed bytes,
// obsolescence checks, the sending key, or a solver's mutable business state.
type receiptMetadata struct {
	nonce         uint64
	label         string
	solver        string
	confirmations uint64
	observe       func(context.Context, Result)
	sendSpan      trace.SpanContext
}

type lateReceiptObservation struct {
	hash     common.Hash
	metadata receiptMetadata
	expires  time.Time
}

// Hash-addressed immutable headers let long proofs accumulate across bounded polls. A separate
// capacity and TTL, using the same configured bounds, limits this cache independently of hashes
// awaiting observation and the accounted-receipt ledger. Every walk still checks each exact link.
type lateReceiptHeader struct {
	header  *types.Header
	expires time.Time
}

func (m *Manager) receiptMetadata(req Request, nonce uint64) receiptMetadata {
	return receiptMetadata{
		nonce: nonce, label: req.Label, solver: req.Solver,
		confirmations: m.confirmations(req), observe: req.ObserveReceipt,
	}
}

// recordReceipt is the single receipt-cost and telemetry gate for ordinary and passive results.
// The ledger is bounded independently of the passive queue; eviction or expiry deliberately
// limits duplicate suppression to locally retained hashes, rather than claiming durable recovery.
func (m *Manager) recordReceipt(ctx context.Context, metadata receiptMetadata, result Result, late bool) bool {
	if result.Receipt == nil {
		return false
	}
	now := time.Now()
	m.receiptMu.Lock()
	if late {
		m.expireLateReceiptsLocked(now)
		if ctx.Err() != nil || !slices.ContainsFunc(m.lateReceiptQueue, func(entry lateReceiptObservation) bool { return entry.hash == result.Hash }) {
			m.receiptMu.Unlock()
			return false
		}
	}
	m.expireAccountedReceiptsLocked(now)
	if _, counted := m.accountedReceipts[result.Hash]; counted {
		m.receiptMu.Unlock()
		return false
	}
	if len(m.accountedReceipts) >= m.cfg.LateReceiptMaxHashes {
		var oldestHash common.Hash
		var oldest time.Time
		for hash, expires := range m.accountedReceipts {
			if oldest.IsZero() || expires.Before(oldest) {
				oldestHash, oldest = hash, expires
			}
		}
		delete(m.accountedReceipts, oldestHash)
	}
	m.accountedReceipts[result.Hash] = now.Add(m.cfg.LateReceiptTimeout)
	confirmedWinner := result.Outcome == OutcomeConfirmed ||
		(result.Outcome == OutcomeReverted && ctx.Err() == nil)
	for index := 0; index < len(m.lateReceiptQueue); {
		entry := m.lateReceiptQueue[index]
		// Once the owned winner is canonical at the requested depth, its other signed
		// same-nonce candidates cannot also execute on that chain. A normal unconfirmed
		// receipt retires only its own hash and retains the existing finality semantics.
		if entry.hash == result.Hash || (confirmedWinner && entry.metadata.nonce == metadata.nonce) {
			m.removeLateReceiptLocked(index, "")
			continue
		}
		index++
	}
	m.receiptMu.Unlock()
	m.metrics.observeReceipt(metadata.label, result, late)
	if metadata.observe != nil {
		metadata.observe(ctx, result)
	}
	return true
}

func (m *Manager) retainLateReceipts(ctx context.Context, pending *pendingTransaction) {
	metadata := m.receiptMetadata(pending.req, pending.nonce)
	metadata.sendSpan = trace.SpanFromContext(ctx).SpanContext()
	now := time.Now()
	m.receiptMu.Lock()
	defer m.receiptMu.Unlock()
	m.expireLateReceiptsLocked(now)
	m.expireAccountedReceiptsLocked(now)
	for _, attempt := range pending.attempts {
		if _, counted := m.accountedReceipts[attempt.hash]; counted {
			continue
		}
		if slices.ContainsFunc(m.lateReceiptQueue, func(entry lateReceiptObservation) bool { return entry.hash == attempt.hash }) {
			continue
		}
		if m.lateReceiptStopped || m.admissionsStopped() {
			if m.metrics != nil {
				m.metrics.lateReceiptDropped.WithLabelValues(metadata.label, "shutdown").Inc()
			}
			continue
		}
		if len(m.lateReceiptQueue) >= m.cfg.LateReceiptMaxHashes {
			m.removeLateReceiptLocked(0, "capacity")
		}
		m.lateReceiptQueue = append(m.lateReceiptQueue, lateReceiptObservation{
			hash: attempt.hash, metadata: metadata, expires: now.Add(m.cfg.LateReceiptTimeout),
		})
		m.metrics.retainLateReceipt(metadata.label)
	}
	select {
	case m.lateReceiptWake <- struct{}{}:
	default:
	}
}

func (m *Manager) monitorLateReceipts(ctx context.Context) {
	defer m.dropLateReceiptsOnShutdown()
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-m.lateReceiptWake:
		}
		m.pollLateReceipts(ctx)
	}
}

// pollLateReceipts gives all reads and canonical checks one overall budget. A blocked or stalled
// candidate cannot monopolize observation: the cursor advances before I/O and depth waits are
// retried on a later pass. Neither the queue nor this read-only work participates in lane ownership.
func (m *Manager) pollLateReceipts(ctx context.Context) {
	pollCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	m.receiptMu.Lock()
	m.expireLateReceiptsLocked(time.Now())
	count := len(m.lateReceiptQueue)
	m.receiptMu.Unlock()
	for range count {
		if pollCtx.Err() != nil {
			return
		}
		entry, ok := m.nextLateReceipt()
		if !ok {
			return
		}
		m.observeLateReceipt(pollCtx, entry)
	}
}

func (m *Manager) observeLateReceipt(ctx context.Context, entry lateReceiptObservation) (err error) {
	ctx, cancel := context.WithDeadline(ctx, entry.expires)
	defer cancel()
	metadata := entry.metadata
	ctx = observability.WithLogger(ctx, m.log.WithValues("solver", metadata.solver, "label", metadata.label))
	var links []trace.Link
	if metadata.sendSpan.IsValid() {
		links = []trace.Link{{SpanContext: metadata.sendSpan}}
	}
	ctx, end := tracer.StartLinked(ctx, "txmanager.late_receipt", links,
		observability.AttrSolver.String(metadata.solver),
		observability.AttrTxLabel.String(metadata.label),
		observability.AttrTxHash.String(entry.hash.Hex()),
		observability.AttrTxNonce.Int64(int64(metadata.nonce)),
	)
	defer func() { end(err) }()
	result, err := m.readLateReceipt(ctx, entry)
	if err != nil {
		if ctx.Err() == nil {
			observability.Log(ctx).V(1).Info("late receipt observation deferred", "error", err, "hash", entry.hash.Hex())
		}
		return err
	}
	if result.Receipt == nil {
		return nil
	}
	if m.recordReceipt(ctx, metadata, result, true) {
		RecordResult(ctx, result)
		observability.Log(ctx).Info("late transaction receipt observed",
			"hash", entry.hash.Hex(), "nonce", metadata.nonce, "outcome", result.Outcome,
			"gasUsed", result.Receipt.GasUsed, "effectiveGasPrice", optionalBigString(result.Receipt.EffectiveGasPrice))
	}
	return result.Err
}

func (m *Manager) readLateReceipt(ctx context.Context, entry lateReceiptObservation) (Result, error) {
	receipt, err := m.confirmationReceipt(ctx, entry.hash)
	if errors.Is(err, errReceiptReorged) {
		observability.Decline(ctx, "receipt_unavailable", "owned receipt is not available")
		return Result{}, nil
	}
	if err != nil {
		return Result{}, err
	}
	head, err := m.confirmationHead(ctx)
	if err != nil {
		return Result{}, err
	}
	if !head.Number.IsUint64() || !receipt.BlockNumber.IsUint64() {
		return Result{}, errors.New("late receipt confirmation block number exceeds uint64")
	}
	included, latest := receipt.BlockNumber.Uint64(), head.Number.Uint64()
	if latest < included || latest-included < entry.metadata.confirmations {
		observability.Decline(ctx, "confirmation_depth_pending", "owned receipt awaits configured depth")
		return Result{}, nil
	}
	// Depth and ancestry must describe the same stable head. Reading a separate, newer
	// depth after canonical preflight could incorrectly count a receipt from a displaced fork.
	if err := m.confirmReceiptAncestryUsing(ctx, head, receipt, m.lateReceiptParentHeader); err != nil {
		return Result{}, err
	}
	headAfter, err := m.confirmationHead(ctx)
	if err != nil {
		return Result{}, err
	}
	if head.Hash() != headAfter.Hash() {
		return Result{}, errors.New("late receipt confirmation head changed during ancestry check")
	}
	result := Result{Hash: entry.hash, Receipt: receipt, Outcome: OutcomeConfirmed}
	if receipt.Status == types.ReceiptStatusFailed {
		result.Outcome = OutcomeReverted
		result.Err = errors.Errorf("tx %s reverted on-chain", entry.hash.Hex())
	}
	return result, nil
}

func (m *Manager) nextLateReceipt() (lateReceiptObservation, bool) {
	m.receiptMu.Lock()
	defer m.receiptMu.Unlock()
	m.expireLateReceiptsLocked(time.Now())
	if len(m.lateReceiptQueue) == 0 {
		return lateReceiptObservation{}, false
	}
	index := m.lateReceiptCursor % len(m.lateReceiptQueue)
	entry := m.lateReceiptQueue[index]
	m.lateReceiptCursor = (index + 1) % len(m.lateReceiptQueue)
	return entry, true
}

func (m *Manager) removeLateReceiptLocked(index int, reason string) {
	entry := m.lateReceiptQueue[index]
	m.metrics.removeLateReceipt(entry.metadata.label, reason)
	m.lateReceiptQueue = slices.Delete(m.lateReceiptQueue, index, index+1)
	if index < m.lateReceiptCursor {
		m.lateReceiptCursor--
	}
	if m.lateReceiptCursor >= len(m.lateReceiptQueue) {
		m.lateReceiptCursor = 0
	}
}

func (m *Manager) expireLateReceiptsLocked(now time.Time) {
	for index := 0; index < len(m.lateReceiptQueue); {
		if !now.Before(m.lateReceiptQueue[index].expires) {
			m.removeLateReceiptLocked(index, "expired")
			continue
		}
		index++
	}
}

func (m *Manager) expireAccountedReceiptsLocked(now time.Time) {
	for hash, expires := range m.accountedReceipts {
		if !now.Before(expires) {
			delete(m.accountedReceipts, hash)
		}
	}
}

func (m *Manager) dropLateReceiptsOnShutdown() {
	m.receiptMu.Lock()
	defer m.receiptMu.Unlock()
	m.lateReceiptStopped = true
	for len(m.lateReceiptQueue) > 0 {
		m.removeLateReceiptLocked(0, "shutdown")
	}
	clear(m.lateReceiptHeaders)
}

func (m *Manager) admissionsStopped() bool {
	select {
	case <-m.stopping:
		return true
	default:
		return false
	}
}

func (m *Manager) lateReceiptParentHeader(ctx context.Context, hash common.Hash) (*types.Header, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.receiptMu.Lock()
	if cached, ok := m.lateReceiptHeaders[hash]; ok && time.Now().Before(cached.expires) {
		m.receiptMu.Unlock()
		return cached.header, nil
	}
	m.receiptMu.Unlock()
	header, err := m.backend.HeaderByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	if header == nil || header.Number == nil || !header.Number.IsUint64() || header.Hash() != hash {
		return nil, errors.Errorf("passive ancestry header %s is invalid", hash.Hex())
	}
	header = types.CopyHeader(header)
	now := time.Now()
	m.receiptMu.Lock()
	defer m.receiptMu.Unlock()
	if m.lateReceiptStopped || ctx.Err() != nil {
		return header, nil
	}
	for cachedHash, cached := range m.lateReceiptHeaders {
		if !now.Before(cached.expires) {
			delete(m.lateReceiptHeaders, cachedHash)
		}
	}
	if len(m.lateReceiptHeaders) >= m.cfg.LateReceiptMaxHashes {
		var oldestHash common.Hash
		var oldest time.Time
		for cachedHash, cached := range m.lateReceiptHeaders {
			if oldest.IsZero() || cached.expires.Before(oldest) {
				oldestHash, oldest = cachedHash, cached.expires
			}
		}
		delete(m.lateReceiptHeaders, oldestHash)
	}
	m.lateReceiptHeaders[hash] = lateReceiptHeader{header: header, expires: now.Add(m.cfg.LateReceiptTimeout)}
	return header, nil
}
