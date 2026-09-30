package txmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// The journal is a write-ahead record of one owned nonce. Signed bytes are the source of truth;
// request metadata only supplies confirmation, fee-ceiling and cancellation policy. Obsolete is
// deliberately absent: a restarted process cannot reconstruct a solver's closure safely.
type journalState struct {
	Version      int              `json:"version"`
	ChainID      string           `json:"chainId"`
	Sender       common.Address   `json:"sender"`
	Request      journalRequest   `json:"request"`
	Deadline     time.Time        `json:"deadline"`
	BaseFee      string           `json:"baseFee"`
	CancelReason string           `json:"cancelReason"`
	Obsolete     bool             `json:"obsolete"`
	Attempts     []journalAttempt `json:"attempts"`
}

type journalRequest struct {
	To            common.Address `json:"to"`
	Data          []byte         `json:"data"`
	Value         string         `json:"value"`
	MaxFeePerGas  string         `json:"maxFeePerGas"`
	CancelAt      time.Time      `json:"cancelAt"`
	Confirmations uint64         `json:"confirmations"`
	Label         string         `json:"label"`
	Solver        string         `json:"solver"`
}

type journalAttempt struct {
	Raw          []byte `json:"raw"`
	Cancellation bool   `json:"cancellation"`
}

// initializeJournalLocked runs before Start while mu is held. Recovery owns the lifecycle slot
// before solvers can observe readiness. Unknown work outside the single owned nonce fails closed.
func (m *Manager) initializeJournalLocked(ctx context.Context) error {
	if m.journal != nil {
		return m.journalErr
	}
	store, err := openJournal(m.cfg.StateFile)
	if err != nil {
		return errors.Errorf("open transaction journal: %w", err)
	}
	m.journal = store
	fail := func(err error) error {
		closeErr := store.close()
		m.journal = nil
		return errors.Join(err, closeErr)
	}
	data, err := store.load()
	if err != nil {
		return fail(errors.Errorf("load transaction journal: %w", err))
	}
	if data == nil {
		if err := m.initializeNonceLocked(ctx); err != nil {
			return fail(err)
		}
		return nil
	}
	pending, err := m.decodeJournal(data)
	if err != nil {
		return fail(errors.Errorf("invalid transaction journal: %w", err))
	}
	latest, err := m.backend.NonceAt(ctx, m.signer.Address(), nil)
	if err != nil {
		return fail(errors.Errorf("journal recovery latest nonce: %w", err))
	}
	poolNonce, err := m.backend.PendingNonceAt(ctx, m.signer.Address())
	if err != nil {
		return fail(errors.Errorf("journal recovery pending nonce: %w", err))
	}
	if latest < pending.nonce || latest > pending.nonce+1 || poolNonce < latest || poolNonce > pending.nonce+1 {
		return fail(errors.Errorf("journal nonce %d cannot own account latest %d and pending %d", pending.nonce, latest, poolNonce))
	}
	m.nonce, m.nonceInit = pending.nonce+1, true
	m.recovered = pending
	m.recovering.Store(true)
	m.lifecycleSlot <- struct{}{}
	m.addAdmissionDemand()
	return nil
}

// Close releases the journal's process lock after Start has drained. It also supports initialization
// followed by an early startup error. A hard shutdown with an uncooperative worker keeps the lock:
// process exit, rather than a second manager, must end that worker's ownership.
func (m *Manager) Close() error {
	m.journalMu.Lock()
	defer m.journalMu.Unlock()
	m.unminedMu.Lock()
	active := m.unmined != nil
	m.unminedMu.Unlock()
	if active {
		return errors.New("cannot close transaction journal while a lifecycle is running")
	}
	if m.journal == nil {
		return nil
	}
	return m.journal.close()
}

func (m *Manager) startRecoveredLifecycle(ctx context.Context) {
	pending := m.recovered
	if pending == nil {
		return
	}
	m.recovered = nil
	spanCtx, span := startSendSpan(ctx, pending.req)
	pending.span = span
	spanCtx = observability.WithLogger(spanCtx, m.requestLog(pending.req))
	pending.lifecycle = m.metrics.beginLifecycle(pending.req.Label)
	pending.lifecycle.transitionPhase(lifecyclePhasePending)
	m.trackUnminedTransaction(pending)
	observability.Log(spanCtx).Info("recovering transaction lifecycle", "label", pending.req.Label,
		"nonce", pending.nonce, "hashes", attemptHashStrings(pending.attempts))
	m.lifecycleWG.Go(func() {
		defer m.releaseLifecycleSlot()
		m.complete(spanCtx, pending)
	})
}

func (m *Manager) persistSignedAttempt(pending *pendingTransaction, signed *types.Transaction, fees feeQuote, cancellation bool) error {
	if m.cfg.StateFile == "" {
		return nil
	}
	m.journalMu.Lock()
	defer m.journalMu.Unlock()
	if m.journal == nil {
		return m.pauseJournal(errors.New("transaction journal must be initialized before signing"))
	}
	state := journalState{
		Version: 1, ChainID: m.chainID.String(), Sender: m.signer.Address(),
		Request: journalRequest{
			To: pending.req.To, Data: pending.req.Data, Value: pending.value.String(),
			MaxFeePerGas: optionalBigString(pending.req.MaxFeePerGas), CancelAt: pending.req.CancelAt,
			Confirmations: m.confirmations(pending.req), Label: pending.req.Label, Solver: pending.req.Solver,
		},
		Deadline: pending.cancelDeadline, BaseFee: fees.baseFee.String(), CancelReason: pending.cancelReason,
		Obsolete: pending.obsolete, Attempts: make([]journalAttempt, 0, len(pending.attempts)+1),
	}
	for _, attempt := range pending.attempts {
		raw, err := attempt.tx.MarshalBinary()
		if err != nil {
			return m.pauseJournal(errors.Errorf("encode tracked signed attempt: %w", err))
		}
		state.Attempts = append(state.Attempts, journalAttempt{Raw: raw, Cancellation: attempt.cancellation})
	}
	raw, err := signed.MarshalBinary()
	if err != nil {
		return m.pauseJournal(errors.Errorf("encode new signed attempt: %w", err))
	}
	state.Attempts = append(state.Attempts, journalAttempt{Raw: raw, Cancellation: cancellation})
	data, err := json.Marshal(state)
	if err != nil {
		return m.pauseJournal(errors.Errorf("encode transaction journal: %w", err))
	}
	if err := m.journal.save(data); err != nil {
		return m.pauseJournal(errors.Errorf("persist signed transaction before broadcast: %w", err))
	}
	return nil
}

func (m *Manager) clearRejectedJournal() error {
	if m.cfg.StateFile == "" {
		return nil
	}
	m.journalMu.Lock()
	defer m.journalMu.Unlock()
	if err := m.journal.clear(); err != nil {
		return m.pauseJournal(errors.Errorf("clear rejected transaction journal: %w", err))
	}
	return nil
}

func (m *Manager) finishJournal(ctx context.Context, pending *pendingTransaction, result Result) Result {
	if m.cfg.StateFile == "" {
		return result
	}
	// An observed receipt alone is not terminal: an interrupted confirmation wait or a reorg
	// must leave enough state for the next process to recover the same signed lifecycle.
	var err error
	if !pending.finalized || result.Receipt == nil {
		err = errors.New("transaction journal retained for unresolved lifecycle")
	}
	if err == nil {
		m.journalMu.Lock()
		err = m.journal.clear()
		m.journalMu.Unlock()
	}
	if err != nil {
		err = m.pauseJournal(errors.Errorf("finish transaction journal: %w", err))
		if pending.finalized {
			observability.Log(ctx).Error(err, "final transaction journal could not be cleared", "label", pending.req.Label, "nonce", pending.nonce)
		} else {
			observability.Log(ctx).V(1).Info("transaction journal retained for restart recovery", "label", pending.req.Label, "nonce", pending.nonce)
		}
		result.Err = errors.Join(result.Err, err)
		return result
	}
	m.recovering.Store(false)
	m.notifyLaneStateChange()
	return result
}

func (m *Manager) pauseJournal(err error) error {
	m.mu.Lock()
	first := m.journalErr == nil
	if first {
		m.journalErr = err
	}
	m.mu.Unlock()
	if first {
		m.notifyLaneStateChange()
	}
	return err
}

func (m *Manager) journalFailure() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.journalErr
}

func (m *Manager) decodeJournal(data []byte) (*pendingTransaction, error) {
	var state journalState
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return nil, errors.Errorf("decode state: %w", err)
	}
	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return nil, errors.New("journal must contain exactly one state")
	}
	if state.Version != 1 || state.ChainID != m.chainID.String() || state.Sender != m.signer.Address() {
		return nil, errors.New("journal version, chain or sender mismatch")
	}
	switch state.CancelReason {
	case "", "pending_timeout", "request_deadline", "shutdown", "obsolete", "recovery":
	default:
		return nil, errors.New("invalid journal cancellation reason")
	}
	if len(state.Attempts) == 0 || state.Deadline.IsZero() || state.Attempts[0].Cancellation {
		return nil, errors.New("journal requires an original signed call and cancellation deadline")
	}
	if !state.Request.CancelAt.IsZero() && state.Deadline.After(state.Request.CancelAt) {
		return nil, errors.New("journal cancellation deadline exceeds request deadline")
	}
	value, err := journalInteger(state.Request.Value)
	if err != nil {
		return nil, err
	}
	baseFee, err := journalInteger(state.BaseFee)
	if err != nil {
		return nil, err
	}
	limit, err := journalInteger(state.Request.MaxFeePerGas)
	if err != nil {
		return nil, err
	}
	if limit.Sign() == 0 {
		limit = nil
	}
	req := Request{
		To: state.Request.To, Data: state.Request.Data, Value: value, MaxFeePerGas: limit,
		CancelAt: state.Request.CancelAt, Confirmations: &state.Request.Confirmations,
		Label: state.Request.Label, Solver: state.Request.Solver,
	}
	pending := &pendingTransaction{
		req: req, value: value, cancelDeadline: state.Deadline, cancelReason: state.CancelReason,
		obsolete: state.Obsolete, recovered: true,
	}
	seen := make(map[common.Hash]bool, len(state.Attempts))
	cancelling := false
	for i, saved := range state.Attempts {
		tx := new(types.Transaction)
		if err := tx.UnmarshalBinary(saved.Raw); err != nil {
			return nil, errors.Errorf("decode signed attempt %d: %w", i, err)
		}
		if i == 0 {
			pending.nonce, pending.originalHash = tx.Nonce(), tx.Hash()
		}
		if err := m.validateJournalAttempt(pending, tx, saved.Cancellation); err != nil {
			return nil, errors.Errorf("signed attempt %d: %w", i, err)
		}
		if seen[tx.Hash()] || (cancelling && !saved.Cancellation) {
			return nil, errors.New("duplicate attempt or normal call after cancellation")
		}
		seen[tx.Hash()] = true
		cancelling = saved.Cancellation
		pending.attempts = append(pending.attempts, txAttempt{hash: tx.Hash(), tx: tx, cancellation: saved.Cancellation})
		if !saved.Cancellation {
			pending.gas = tx.Gas()
		}
	}
	last := pending.latestAttempt().tx
	pending.fees = feeQuote{baseFee: baseFee, tip: last.GasTipCap(), maxFee: last.GasFeeCap()}
	return pending, nil
}

func (m *Manager) validateJournalAttempt(pending *pendingTransaction, tx *types.Transaction, cancellation bool) error {
	if tx.Type() != types.DynamicFeeTxType || tx.ChainId().Cmp(m.chainID) != 0 || tx.Nonce() != pending.nonce || tx.Nonce() == math.MaxUint64 {
		return errors.New("signed transaction type, chain or nonce mismatch")
	}
	sender, err := types.Sender(types.LatestSignerForChainID(m.chainID), tx)
	if err != nil || sender != m.signer.Address() {
		return errors.New("signed transaction sender mismatch")
	}
	if tx.To() == nil || tx.Gas() == 0 || tx.GasFeeCap().Sign() <= 0 || tx.GasTipCap().Cmp(tx.GasFeeCap()) > 0 {
		return errors.New("invalid signed call or fees")
	}
	if cancellation {
		if *tx.To() != m.signer.Address() || tx.Value().Sign() != 0 || len(tx.Data()) != 0 || tx.Gas() != cancellationGasLimit {
			return errors.New("invalid signed cancellation")
		}
	} else if *tx.To() != pending.req.To || tx.Value().Cmp(pending.value) != 0 || !bytes.Equal(tx.Data(), pending.req.Data) {
		return errors.New("signed transaction differs from journal request")
	}
	return nil
}

func journalInteger(value string) (*big.Int, error) {
	integer, ok := new(big.Int).SetString(value, 10)
	if !ok || integer.Sign() < 0 {
		return nil, errors.New("invalid nonnegative journal integer")
	}
	return integer, nil
}
