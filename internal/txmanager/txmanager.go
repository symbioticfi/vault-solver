// Package txmanager owns the on-chain sending account and serializes all transactions through a
// single worker goroutine, so multiple solvers can never race on the account nonce. Solvers build
// calldata and hand it over via Send, TrySend, or SendAsync; they never sign or broadcast directly.
package txmanager

import (
	"context"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/go-errors/errors"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/tenderly"
)

// Backend is the subset of an EVM client the manager needs. *ethclient.Client satisfies it.
type Backend interface {
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	BlockNumber(ctx context.Context) (uint64, error)
}

// Config tunes fee selection and confirmation behavior.
type Config struct {
	Confirmations       uint64        // blocks to wait past inclusion before returning
	MaxFeeGwei          float64       // absolute max fee per gas; app config requires a positive value
	TipGwei             float64       // priority fee; 0 => use the node's suggestion
	PollInterval        time.Duration // receipt/confirmation poll cadence; 0 => 2s
	ReplacementInterval time.Duration // pending tx fee-bump cadence; 0 => 30s
	PendingTimeout      time.Duration // switch from replacing the call to cancelling its nonce; 0 => 5m
}

// Request is a transaction to send. Value nil means 0; GasLimit 0 means "estimate".
type Request struct {
	To            common.Address
	Data          []byte
	Value         *big.Int
	GasLimit      uint64
	MaxFeePerGas  *big.Int // optional hard EIP-1559 fee ceiling; fees are clamped to it or rejected below base fee
	Confirmations *uint64  // optional wait override; nil uses Config.Confirmations
	Label         string   // stable operation name for logs and metrics
}

// Outcome is the terminal transaction state observed by the manager.
type Outcome string

const (
	OutcomeConfirmed           Outcome = "confirmed"
	OutcomeIncludedUnconfirmed Outcome = "included_unconfirmed"
	OutcomeReverted            Outcome = "reverted"
	OutcomeCancelled           Outcome = "cancelled"
	OutcomeSubmissionError     Outcome = "submission_error"
	OutcomeTrackingStopped     Outcome = "tracking_stopped"
)

// Result carries the outcome of one transaction request.
type Result struct {
	Hash    common.Hash
	Receipt *types.Receipt
	Outcome Outcome
	Err     error
}

// EffectiveOutcome falls back to Result fields for lightweight test senders. Manager results always
// carry an explicit outcome because only the manager can distinguish a cancellation transaction.
func (r Result) EffectiveOutcome() Outcome {
	if r.Outcome != "" {
		return r.Outcome
	}
	switch {
	case r.Receipt != nil && r.Receipt.Status == types.ReceiptStatusFailed:
		return OutcomeReverted
	case r.Receipt != nil && r.Err != nil:
		return OutcomeIncludedUnconfirmed
	case r.Err != nil:
		return OutcomeSubmissionError
	default:
		return OutcomeConfirmed
	}
}

type feeQuote struct {
	baseFee *big.Int
	tip     *big.Int
	maxFee  *big.Int
}

type pendingTransaction struct {
	req      Request
	nonce    uint64
	gas      uint64
	value    *big.Int
	fees     feeQuote
	attempts []txAttempt
}

type txAttempt struct {
	hash         common.Hash
	cancellation bool
}

// Manager is the single-writer transaction sender.
type Manager struct {
	backend Backend
	signer  signer.Signer
	chainID *big.Int
	cfg     Config
	metrics *Metrics
	log     logr.Logger

	queue        chan job
	blockingSlot chan struct{}

	mu        sync.Mutex // guards the local nonce
	nonce     uint64
	nonceInit bool

	unminedMu     sync.Mutex
	unminedNonces map[uint64]struct{}
}

type job struct {
	req Request
	res chan Result
}

const (
	defaultPollInterval        = 2 * time.Second
	defaultReplacementInterval = 30 * time.Second
	defaultPendingTimeout      = 5 * time.Minute
	replacementBumpNumerator   = 9
	replacementBumpDenominator = 8
	cancellationGasLimit       = 21_000
	maxNonceResyncs            = 1
)

// New constructs a Manager. Call Start to launch its worker.
func New(backend Backend, s signer.Signer, chainID *big.Int, cfg Config, log logr.Logger) *Manager {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	if cfg.ReplacementInterval <= 0 {
		cfg.ReplacementInterval = defaultReplacementInterval
	}
	if cfg.PendingTimeout <= 0 {
		cfg.PendingTimeout = defaultPendingTimeout
	}
	return &Manager{
		backend:       backend,
		signer:        s,
		chainID:       chainID,
		cfg:           cfg,
		log:           log.WithName("txmanager"),
		queue:         make(chan job),
		blockingSlot:  make(chan struct{}, 1),
		unminedNonces: make(map[uint64]struct{}),
	}
}

// NewWithMetrics constructs a Manager with transaction lifecycle metrics.
func NewWithMetrics(
	backend Backend,
	s signer.Signer,
	chainID *big.Int,
	cfg Config,
	metrics *Metrics,
	log logr.Logger,
) *Manager {
	manager := New(backend, s, chainID, cfg, log)
	manager.metrics = metrics
	return manager
}

// Confirmations returns the configured finality depth used by requests without an override.
func (m *Manager) Confirmations() uint64 {
	return m.cfg.Confirmations
}

// Start runs the worker until ctx is cancelled. Run it in its own goroutine.
func (m *Manager) Start(ctx context.Context) {
	m.log.Info("started", "from", m.signer.Address().Hex())
	for {
		select {
		case <-ctx.Done():
			m.log.Info("stopped", "reason", ctx.Err().Error())
			return
		case j := <-m.queue:
			m.metrics.requestStarted(j.req.Label)
			pending, err := m.broadcast(ctx, j.req)
			if err != nil {
				outcome := OutcomeSubmissionError
				if ctx.Err() != nil {
					outcome = OutcomeTrackingStopped
				}
				m.metrics.requestFinished(j.req.Label, outcome, nil)
				j.res <- Result{Outcome: outcome, Err: err}
				continue
			}
			m.addUnminedNonce(pending.nonce)
			go m.complete(ctx, pending, j.res)
		}
	}
}

// Send enqueues a transaction and blocks until it is confirmed or fails. Safe for concurrent
// callers; all requests are serialized through the single worker.
//
// ctx governs the enqueue only. Before the request is enqueued, a cancelled ctx aborts cleanly with
// no transaction sent. Once enqueued, the worker broadcasts the tx on the manager's own long-lived
// context, so Send waits for and returns that real outcome — it must not report a cancellation while
// the transaction still lands on-chain, which a caller would read as "not sent" (the caller's ctx is
// typically an errgroup child that cancels the instant any sibling solver errors, well before
// shutdown). The worker owns fee replacement and same-nonce cancellation until it can deliver the
// real receipt or the manager context ends.
func (m *Manager) Send(ctx context.Context, req Request) Result {
	select {
	case m.blockingSlot <- struct{}{}:
		defer func() { <-m.blockingSlot }()
	case <-ctx.Done():
		return Result{Outcome: OutcomeSubmissionError, Err: ctx.Err()}
	}
	return m.sendAccepted(ctx, req)
}

// TrySend submits only when no blocking Send or TrySend call owns the exclusive slot. Async
// transactions do not hold this slot; every accepted broadcast still receives a serialized nonce.
func (m *Manager) TrySend(ctx context.Context, req Request) (Result, bool) {
	select {
	case m.blockingSlot <- struct{}{}:
		defer func() { <-m.blockingSlot }()
	default:
		return Result{}, false
	}
	return m.sendAccepted(ctx, req), true
}

func (m *Manager) sendAccepted(ctx context.Context, req Request) Result {
	result, accepted := m.SendAsync(ctx, req)
	if !accepted {
		return Result{Outcome: OutcomeSubmissionError, Err: ctx.Err()}
	}
	return <-result
}

// SendAsync enqueues one transaction for nonce-serialized broadcast and returns its eventual
// receipt result without waiting for it. Once accepted, the manager's long-lived context owns the
// broadcast and receipt wait, matching Send's cancellation contract.
func (m *Manager) SendAsync(ctx context.Context, req Request) (<-chan Result, bool) {
	res := make(chan Result, 1)
	select {
	case m.queue <- job{req: cloneRequest(req), res: res}:
	case <-ctx.Done():
		return nil, false
	}
	return res, true
}

// MaxFeePerGas returns the conservative per-gas fee cap that the next transaction would use. Solvers
// use it only for profitability calculations; Send recomputes fees immediately before signing.
func (m *Manager) MaxFeePerGas(ctx context.Context) (*big.Int, error) {
	fees, err := m.currentFees(ctx)
	if err != nil {
		return nil, err
	}
	return fees.maxFee, nil
}

// broadcast runs on the worker goroutine only, so fee selection, signing, and nonce assignment stay
// serialized even while earlier transactions wait for receipts concurrently.
func (m *Manager) broadcast(ctx context.Context, req Request) (*pendingTransaction, error) {
	fees, err := m.currentFees(ctx)
	if err != nil {
		return nil, err
	}
	if req.MaxFeePerGas != nil {
		feeCap := new(big.Int).Set(req.MaxFeePerGas)
		if feeCap.Sign() <= 0 {
			return nil, errors.Errorf("send %q: request max fee per gas must be positive", req.Label)
		}
		if feeCap.Cmp(fees.baseFee) < 0 {
			return nil, errors.Errorf(
				"send %q: current base fee per gas %s exceeds request cap %s", req.Label, fees.baseFee, feeCap,
			)
		}
		if fees.maxFee.Cmp(feeCap) > 0 {
			fees.maxFee.Set(feeCap)
		}
		maxTip := new(big.Int).Sub(feeCap, fees.baseFee)
		if fees.tip.Cmp(maxTip) > 0 {
			fees.tip.Set(maxTip)
		}
	}

	gas := req.GasLimit
	if gas == 0 {
		gas, err = m.estimateGas(ctx, req)
		if err != nil {
			return nil, err
		}
	}

	value := req.Value
	if value == nil {
		value = new(big.Int)
	}
	m.log.V(1).Info(
		"transaction prepared",
		"label", req.Label,
		"to", req.To.Hex(),
		"value", value.String(),
		"calldataBytes", len(req.Data),
		"gasLimit", gas,
		"baseFeePerGas", fees.baseFee.String(),
		"maxPriorityFeePerGas", fees.tip.String(),
		"maxFeePerGas", fees.maxFee.String(),
		"requestMaxFeePerGas", optionalBigString(req.MaxFeePerGas),
	)

	var lastErr error
	for attempt := 0; attempt <= maxNonceResyncs; attempt++ {
		nonce, nErr := m.nextNonce(ctx, attempt > 0)
		if nErr != nil {
			return nil, nErr
		}

		hash, sendErr := m.signAndSend(
			ctx, nonce, req.To, req.Data, value, gas, fees,
		)
		if sendErr != nil {
			lastErr = sendErr
			if isNonceTooLow(sendErr) {
				m.log.Info("nonce too low; resyncing", "label", req.Label, "nonce", nonce)
				continue // retry with a freshly-synced nonce
			}
			return nil, errors.Errorf("send %q: %w", req.Label, sendErr)
		}

		m.commitNonce(nonce)
		m.log.Info("sent", "label", req.Label, "hash", hash.Hex(), "nonce", nonce)
		return &pendingTransaction{
			req:      req,
			nonce:    nonce,
			gas:      gas,
			value:    new(big.Int).Set(value),
			fees:     cloneFeeQuote(fees),
			attempts: []txAttempt{{hash: hash}},
		}, nil
	}
	return nil, errors.Errorf("send %q: exhausted nonce resyncs: %w", req.Label, lastErr)
}

func (m *Manager) complete(ctx context.Context, pending *pendingTransaction, result chan<- Result) {
	defer m.removeUnminedNonce(pending.nonce)
	completed := m.waitForPendingTransaction(ctx, pending)
	m.metrics.requestFinished(pending.req.Label, completed.Outcome, completed.Receipt)
	result <- completed
}

func (m *Manager) confirmations(req Request) uint64 {
	if req.Confirmations != nil {
		return *req.Confirmations
	}
	return m.cfg.Confirmations
}

func (m *Manager) waitForPendingTransaction(ctx context.Context, pending *pendingTransaction) Result {
	poll := time.NewTicker(m.cfg.PollInterval)
	defer poll.Stop()
	replace := time.NewTicker(m.cfg.ReplacementInterval)
	defer replace.Stop()
	timeout := time.NewTimer(m.cfg.PendingTimeout)
	defer timeout.Stop()

	cancelling := false
	for {
		if receiptResult, done := m.receiptResult(ctx, pending); done {
			return receiptResult
		}
		select {
		case <-ctx.Done():
			return Result{
				Hash:    pending.attempts[0].hash,
				Outcome: OutcomeTrackingStopped,
				Err:     ctx.Err(),
			}
		case <-poll.C:
		case <-replace.C:
			m.tryReplace(ctx, pending, cancelling)
		case <-timeout.C:
			if !m.isLowestUnminedNonce(pending.nonce) {
				m.log.Info("pending timeout deferred behind lower nonce",
					"label", pending.req.Label,
					"nonce", pending.nonce,
				)
				timeout.Reset(m.cfg.PendingTimeout)
				continue
			}
			cancelling = true
			m.log.Info("pending transaction timed out; cancelling nonce",
				"label", pending.req.Label,
				"nonce", pending.nonce,
				"timeout", m.cfg.PendingTimeout.String(),
			)
			m.tryReplace(ctx, pending, true)
		}
	}
}

func (m *Manager) receiptResult(ctx context.Context, pending *pendingTransaction) (Result, bool) {
	for i := len(pending.attempts) - 1; i >= 0; i-- {
		attempt := pending.attempts[i]
		receipt, err := m.backend.TransactionReceipt(ctx, attempt.hash)
		if errors.Is(err, ethereum.NotFound) {
			continue
		}
		if err != nil {
			m.log.Error(err, "pending transaction receipt unavailable",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
			)
			continue
		}
		m.removeUnminedNonce(pending.nonce)
		if receipt.Status == types.ReceiptStatusFailed {
			m.log.Error(errors.Errorf("tx %s reverted on-chain", attempt.hash.Hex()), "transaction reverted",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
				"tenderly", tenderly.SimulatorURL(m.chainID, m.signer.Address(), pending.req.To, pending.req.Data, pending.req.Value),
			)
			return Result{
				Hash:    attempt.hash,
				Receipt: receipt,
				Outcome: OutcomeReverted,
				Err:     errors.Errorf("tx %s reverted on-chain", attempt.hash.Hex()),
			}, true
		}
		if err := m.waitForConfirmations(ctx, receipt, m.confirmations(pending.req)); err != nil {
			outcome := OutcomeIncludedUnconfirmed
			if attempt.cancellation {
				outcome = OutcomeCancelled
			}
			return Result{Hash: attempt.hash, Receipt: receipt, Outcome: outcome, Err: err}, true
		}
		if attempt.cancellation {
			return Result{
				Hash:    attempt.hash,
				Receipt: receipt,
				Outcome: OutcomeCancelled,
				Err: errors.Errorf(
					"send %q: pending transaction cancelled at nonce %d after %s",
					pending.req.Label, pending.nonce, m.cfg.PendingTimeout,
				),
			}, true
		}
		m.log.V(1).Info(
			"transaction confirmed",
			"label", pending.req.Label,
			"hash", attempt.hash.Hex(),
			"nonce", pending.nonce,
			"blockNumber", optionalBigString(receipt.BlockNumber),
			"gasUsed", receipt.GasUsed,
			"effectiveGasPrice", optionalBigString(receipt.EffectiveGasPrice),
			"confirmations", m.confirmations(pending.req),
		)
		return Result{Hash: attempt.hash, Receipt: receipt, Outcome: OutcomeConfirmed}, true
	}
	return Result{}, false
}

func (m *Manager) tryReplace(ctx context.Context, pending *pendingTransaction, cancellation bool) {
	limit := m.normalFeeLimit(pending.req)
	if cancellation {
		limit = m.globalFeeLimit()
	}
	fees, err := m.nextReplacementFees(ctx, pending.fees, limit)
	if err != nil {
		m.log.Error(err, "cannot replace pending transaction",
			"label", pending.req.Label,
			"nonce", pending.nonce,
			"cancellation", cancellation,
		)
		return
	}
	to := pending.req.To
	data := pending.req.Data
	value := pending.value
	gas := pending.gas
	if cancellation {
		to = m.signer.Address()
		data = nil
		value = new(big.Int)
		gas = cancellationGasLimit
	}
	hash, err := m.signAndSend(ctx, pending.nonce, to, data, value, gas, fees)
	if err != nil {
		m.log.Error(err, "pending transaction replacement failed",
			"label", pending.req.Label,
			"nonce", pending.nonce,
			"cancellation", cancellation,
		)
		return
	}
	pending.fees = cloneFeeQuote(fees)
	pending.attempts = append(pending.attempts, txAttempt{hash: hash, cancellation: cancellation})
	kind := replacementKindReplacement
	if cancellation {
		kind = replacementKindCancellation
	}
	m.metrics.replacement(pending.req.Label, kind)
	m.log.Info("pending transaction replaced",
		"label", pending.req.Label,
		"hash", hash.Hex(),
		"nonce", pending.nonce,
		"cancellation", cancellation,
		"maxFeePerGas", fees.maxFee.String(),
		"maxPriorityFeePerGas", fees.tip.String(),
	)
}

func (m *Manager) nextReplacementFees(
	ctx context.Context,
	previous feeQuote,
	limit *big.Int,
) (feeQuote, error) {
	current, err := m.currentFees(ctx)
	if err != nil {
		return feeQuote{}, err
	}
	next := feeQuote{
		baseFee: current.baseFee,
		tip:     maxBig(current.tip, bumpFee(previous.tip)),
		maxFee:  maxBig(current.maxFee, bumpFee(previous.maxFee)),
	}
	if limit != nil && next.maxFee.Cmp(limit) > 0 {
		next.maxFee.Set(limit)
	}
	maxTip := new(big.Int).Sub(next.maxFee, next.baseFee)
	if maxTip.Sign() < 0 {
		return feeQuote{}, errors.Errorf(
			"replacement base fee %s exceeds fee limit %s", next.baseFee, next.maxFee,
		)
	}
	if next.tip.Cmp(maxTip) > 0 {
		next.tip.Set(maxTip)
	}
	if next.maxFee.Cmp(previous.maxFee) <= 0 || next.tip.Cmp(previous.tip) <= 0 {
		return feeQuote{}, errors.Errorf(
			"replacement fee limit reached: previous max fee %s tip %s, limit %s",
			previous.maxFee, previous.tip, feeLimitString(limit),
		)
	}
	return next, nil
}

func (m *Manager) normalFeeLimit(req Request) *big.Int {
	limit := reserveCancellationBump(m.globalFeeLimit())
	if req.MaxFeePerGas != nil && (limit == nil || req.MaxFeePerGas.Cmp(limit) < 0) {
		limit = new(big.Int).Set(req.MaxFeePerGas)
	}
	return limit
}

func (m *Manager) globalFeeLimit() *big.Int {
	if m.cfg.MaxFeeGwei <= 0 {
		return nil
	}
	return gweiToWei(m.cfg.MaxFeeGwei)
}

func (m *Manager) addUnminedNonce(nonce uint64) {
	m.unminedMu.Lock()
	defer m.unminedMu.Unlock()
	m.unminedNonces[nonce] = struct{}{}
}

func (m *Manager) removeUnminedNonce(nonce uint64) {
	m.unminedMu.Lock()
	defer m.unminedMu.Unlock()
	delete(m.unminedNonces, nonce)
}

func (m *Manager) isLowestUnminedNonce(nonce uint64) bool {
	m.unminedMu.Lock()
	defer m.unminedMu.Unlock()
	for unmined := range m.unminedNonces {
		if unmined < nonce {
			return false
		}
	}
	return true
}

// currentFees computes the current EIP-1559 base fee, tip, and normal-send fee cap.
func (m *Manager) currentFees(ctx context.Context) (feeQuote, error) {
	var tip *big.Int
	if m.cfg.TipGwei > 0 {
		tip = gweiToWei(m.cfg.TipGwei)
	} else {
		var err error
		tip, err = m.backend.SuggestGasTipCap(ctx)
		if err != nil {
			return feeQuote{}, errors.Errorf("suggest gas tip: %w", err)
		}
	}
	if tip == nil || tip.Sign() < 0 {
		return feeQuote{}, errors.New("gas tip must be non-negative")
	}
	tip = new(big.Int).Set(tip)

	head, err := m.backend.HeaderByNumber(ctx, nil)
	if err != nil {
		return feeQuote{}, errors.Errorf("header by number: %w", err)
	}
	var baseFee *big.Int
	if head.BaseFee == nil {
		baseFee = new(big.Int)
	} else {
		baseFee = new(big.Int).Set(head.BaseFee)
	}

	// 2*baseFee + tip leaves headroom for one base-fee doubling between now and inclusion.
	maxFee := new(big.Int).Add(new(big.Int).Mul(baseFee, big.NewInt(2)), tip)
	if limit := m.normalFeeLimit(Request{}); limit != nil {
		if maxFee.Cmp(limit) > 0 {
			maxFee.Set(limit)
		}
	}
	maxTip := new(big.Int).Sub(maxFee, baseFee)
	if maxTip.Sign() < 0 {
		return feeQuote{}, errors.Errorf("current base fee %s exceeds tx manager max fee %s", baseFee, maxFee)
	}
	if tip.Cmp(maxTip) > 0 {
		tip.Set(maxTip)
	}
	return feeQuote{baseFee: baseFee, tip: tip, maxFee: maxFee}, nil
}

func (m *Manager) estimateGas(ctx context.Context, req Request) (uint64, error) {
	gas, err := m.backend.EstimateGas(ctx, ethereum.CallMsg{
		From:  m.signer.Address(),
		To:    &req.To,
		Value: req.Value,
		Data:  req.Data,
	})
	if err != nil {
		// A revert here surfaces from eth_estimateGas with almost no detail; the Tenderly link replays
		// the exact call so the operator can see the trace (harmless for a non-revert RPC error).
		m.log.Error(err, "gas estimation failed",
			"label", req.Label,
			"tenderly", tenderly.SimulatorURL(m.chainID, m.signer.Address(), req.To, req.Data, req.Value),
		)
		return 0, errors.Errorf("estimate gas %q: %w", req.Label, err)
	}
	// 20% headroom over the estimate.
	return gas + gas/5, nil
}

func optionalBigString(value *big.Int) string {
	if value == nil {
		return "0"
	}
	return value.String()
}

func (m *Manager) signAndSend(
	ctx context.Context,
	nonce uint64,
	to common.Address,
	data []byte,
	value *big.Int,
	gas uint64,
	fees feeQuote,
) (common.Hash, error) {
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   m.chainID,
		Nonce:     nonce,
		GasTipCap: fees.tip,
		GasFeeCap: fees.maxFee,
		Gas:       gas,
		To:        &to,
		Value:     value,
		Data:      data,
	})
	signed, err := m.signer.SignTx(tx, m.chainID)
	if err != nil {
		return common.Hash{}, errors.Errorf("sign transaction: %w", err)
	}
	if err := m.backend.SendTransaction(ctx, signed); err != nil {
		return common.Hash{}, err
	}
	return signed.Hash(), nil
}

// nextNonce returns the nonce to use, seeding or resyncing from the pending nonce when needed.
func (m *Manager) nextNonce(ctx context.Context, resync bool) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if resync || !m.nonceInit {
		pending, err := m.backend.PendingNonceAt(ctx, m.signer.Address())
		if err != nil {
			return 0, errors.Errorf("pending nonce: %w", err)
		}
		m.nonce = pending
		m.nonceInit = true
	}
	return m.nonce, nil
}

func (m *Manager) commitNonce(used uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if used >= m.nonce {
		m.nonce = used + 1
	}
}

func (m *Manager) waitForConfirmations(
	ctx context.Context,
	receipt *types.Receipt,
	confirmations uint64,
) error {
	if receipt == nil || receipt.BlockNumber == nil {
		return errors.New("receipt block number is required")
	}
	if confirmations == 0 {
		return nil
	}
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()

	confirmed := receipt.BlockNumber.Uint64() + confirmations
	for {
		head, err := m.backend.BlockNumber(ctx)
		if err != nil {
			m.log.V(1).Info("confirmation head unavailable; retrying",
				"error", err,
				"tx", receipt.TxHash.Hex(),
			)
		} else if head >= confirmed {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func cloneRequest(req Request) Request {
	req.Data = append([]byte(nil), req.Data...)
	if req.Value != nil {
		req.Value = new(big.Int).Set(req.Value)
	}
	if req.MaxFeePerGas != nil {
		req.MaxFeePerGas = new(big.Int).Set(req.MaxFeePerGas)
	}
	if req.Confirmations != nil {
		confirmations := *req.Confirmations
		req.Confirmations = &confirmations
	}
	return req
}

func cloneFeeQuote(fees feeQuote) feeQuote {
	return feeQuote{
		baseFee: new(big.Int).Set(fees.baseFee),
		tip:     new(big.Int).Set(fees.tip),
		maxFee:  new(big.Int).Set(fees.maxFee),
	}
}

func bumpFee(value *big.Int) *big.Int {
	numerator := new(big.Int).Mul(value, big.NewInt(replacementBumpNumerator))
	numerator.Add(numerator, big.NewInt(replacementBumpDenominator-1))
	bumped := numerator.Div(numerator, big.NewInt(replacementBumpDenominator))
	if bumped.Cmp(value) <= 0 {
		bumped.Add(value, big.NewInt(1))
	}
	return bumped
}

func reserveCancellationBump(limit *big.Int) *big.Int {
	if limit == nil {
		return nil
	}
	reserved := new(big.Int).Mul(limit, big.NewInt(replacementBumpDenominator))
	return reserved.Div(reserved, big.NewInt(replacementBumpNumerator))
}

func maxBig(a, b *big.Int) *big.Int {
	if a.Cmp(b) >= 0 {
		return new(big.Int).Set(a)
	}
	return new(big.Int).Set(b)
}

func feeLimitString(limit *big.Int) string {
	if limit == nil {
		return "unbounded"
	}
	return limit.String()
}

func gweiToWei(gwei float64) *big.Int {
	wei, _ := new(big.Float).Mul(big.NewFloat(gwei), big.NewFloat(params.GWei)).Int(nil)
	return wei
}

func isNonceTooLow(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "nonce too low")
}
