// Package txmanager owns the on-chain sending account. One worker serializes admission, fee
// selection, signing, nonce assignment, and broadcasts so solvers cannot race on the account nonce.
// Only one signed lifecycle may be unresolved at a time; solvers build calldata and hand it over via
// Send, TrySend, or SendAsync, but never sign or broadcast directly.
package txmanager

import (
	"context"
	"math/big"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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
	NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	FeeHistory(
		ctx context.Context,
		blockCount uint64,
		lastBlock *big.Int,
		rewardPercentiles []float64,
	) (*ethereum.FeeHistory, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error)
	EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
}

// Config tunes fee selection and confirmation behavior.
type Config struct {
	Confirmations       uint64        // blocks to wait past inclusion before returning
	MaxFeeGwei          float64       // absolute max fee per gas; app config requires a positive value
	TipGwei             float64       // minimum priority fee; 0 => derive it from recent fee history
	PollInterval        time.Duration // receipt/confirmation poll cadence; 0 => 2s
	ReplacementInterval time.Duration // pending tx fee-bump cadence; 0 => 30s
	PendingTimeout      time.Duration // switch from replacing the call to cancelling its nonce; 0 => 5m
	ShutdownTimeout     time.Duration // maximum graceful drain after manager cancellation; 0 => 1m
}

// Request is a transaction to send. Value nil means 0. Stateful solver calls leave GasLimit at 0 so
// gas estimation re-simulates their exact calldata after lifecycle admission and immediately before signing.
type Request struct {
	To            common.Address
	Data          []byte
	Value         *big.Int
	GasLimit      uint64
	MaxFeePerGas  *big.Int  // optional normal-lifecycle EIP-1559 fee ceiling; cancellation may use the global ceiling
	CancelAt      time.Time // optional deadline after which the manager replaces the call with a same-nonce cancellation
	Confirmations *uint64   // optional wait override; nil uses Config.Confirmations
	Label         string    // for logs/metrics, e.g. "redeem"
}

// Result carries the outcome of one transaction request. NotAdmitted identifies manager-level
// admission failures; ordinary fee, gas, signing, and definite broadcast failures remain submission
// failures even though they do not produce a tracked hash.
type Result struct {
	Hash        common.Hash
	Receipt     *types.Receipt
	Err         error
	NotAdmitted bool
}

type feeQuote struct {
	baseFee *big.Int
	tip     *big.Int
	maxFee  *big.Int
}

type pendingTransaction struct {
	req               Request
	nonce             uint64
	gas               uint64
	value             *big.Int
	fees              feeQuote
	attempts          []txAttempt
	receiptCursor     int
	nonceConflictHash common.Hash
	originalHash      common.Hash
	result            chan<- Result
	resultOnce        sync.Once
	cancelDeadline    time.Time
	cancelRequested   chan struct{}
	cancelOnce        sync.Once
}

type txAttempt struct {
	hash         common.Hash
	tx           *types.Transaction
	cancellation bool
}

// Manager serializes signed lifecycles and owns accepted work through its terminal result.
type Manager struct {
	backend Backend
	signer  signer.Signer
	chainID *big.Int
	cfg     Config
	log     logr.Logger

	queue           chan job
	lifecycleSlot   chan struct{}
	stopping        chan struct{}
	admissionDemand atomic.Int64

	availabilityMu          sync.Mutex
	availabilitySubscribers map[uint64]chan struct{}
	nextAvailabilityID      uint64

	mu        sync.Mutex // guards the local nonce and runtime nonce conflict
	nonce     uint64
	nonceInit bool
	conflict  *nonceConflict

	unminedMu   sync.Mutex
	unmined     *pendingTransaction
	lifecycleWG sync.WaitGroup
}

type job struct {
	req Request
	res chan Result
}

type nonceConflict struct {
	nonce uint64
	hash  common.Hash
}

const (
	defaultPollInterval        = 2 * time.Second
	defaultReplacementInterval = 30 * time.Second
	defaultPendingTimeout      = 5 * time.Minute
	defaultShutdownTimeout     = time.Minute
	maxFeeReadTimeout          = time.Second
	maxReceiptReadTimeout      = 2 * time.Second
	maxBroadcastTimeout        = 3 * time.Second
	feeHistoryBlocks           = 5
	feeHistoryPercentile       = 75.0
	replacementBumpNumerator   = 9
	replacementBumpDenominator = 8
	cancellationGasLimit       = 21_000
)

var (
	errFreshFeesUnavailable    = errors.New("fresh fees unavailable")
	errReplacementLimitReached = errors.New("replacement fee limit reached")
	errReceiptReorged          = errors.New("transaction receipt reorged")
	errNonceLanePaused         = errors.New("transaction manager nonce lane paused")
	errManagerStopped          = errors.New("transaction manager stopped")
	errShutdownTimeout         = errors.Errorf("transaction manager shutdown drain timed out: %w", context.DeadlineExceeded)
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
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = defaultShutdownTimeout
	}
	return &Manager{
		backend:                 backend,
		signer:                  s,
		chainID:                 chainID,
		cfg:                     cfg,
		log:                     log.WithName("txmanager"),
		queue:                   make(chan job),
		lifecycleSlot:           make(chan struct{}, 1),
		stopping:                make(chan struct{}),
		availabilitySubscribers: make(map[uint64]chan struct{}),
	}
}

// Confirmations returns the configured finality depth used by requests without an override.
func (m *Manager) Confirmations() uint64 {
	return m.cfg.Confirmations
}

// ValidateFeeHeadroom rejects a configured priority-fee floor that can never fit under the initial
// transaction cap after reserving one ordinary replacement and one cancellation bump.
func (m *Manager) ValidateFeeHeadroom() error {
	initialLimit := reserveFeeBump(m.normalFeeLimit(Request{}))
	tip := gweiToWei(m.cfg.TipGwei)
	if initialLimit != nil && tip.Sign() > 0 && tip.Cmp(initialLimit) >= 0 {
		return errors.Errorf(
			"tip floor %s leaves no base-fee headroom under initial fee limit %s after reserved replacement bumps",
			tip, initialLimit,
		)
	}
	return nil
}

// Available reports whether new logical transactions may enter the nonce lane. A nonce conflict
// pauses new work and further sends for the active lifecycle while exact signed hashes are polled.
// A benign inclusion race resumes once its exact receipt is proven canonical; an unresolved conflict
// remains fail-closed for operator investigation instead of replaying calldata at another nonce.
func (m *Manager) Available() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.conflict == nil
}

// Idle reports whether no request owns or is waiting for the single signed-lifecycle lane. It is
// intentionally independent of Available: a nonce conflict pauses admission without making an
// otherwise empty lane busy.
func (m *Manager) Idle() bool {
	return m.admissionDemand.Load() == 0
}

// SubscribeAvailability returns an independent, coalesced change stream. Consumers must call
// Available after every signal instead of assuming which edge occurred, and must call unsubscribe
// when they stop. Independent subscriptions prevent readiness and solvers from stealing signals
// from each other.
func (m *Manager) SubscribeAvailability() (<-chan struct{}, func()) {
	m.availabilityMu.Lock()
	id := m.nextAvailabilityID
	m.nextAvailabilityID++
	changes := make(chan struct{}, 1)
	m.availabilitySubscribers[id] = changes
	m.availabilityMu.Unlock()

	var once sync.Once
	return changes, func() {
		once.Do(func() {
			m.availabilityMu.Lock()
			delete(m.availabilitySubscribers, id)
			m.availabilityMu.Unlock()
		})
	}
}

// Initialize seeds the local nonce before solvers become ready. Startup fails closed when the
// account already has an unknown contiguous pending transaction. Standard nonce reads cannot expose
// a transaction queued beyond a gap, so safety also relies on exclusive EOA ownership and Start's
// invariant that later work cannot reach admission or signing until the active lifecycle is terminal.
func (m *Manager) Initialize(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.initializeNonceLocked(ctx)
}

// Start admits one signed lifecycle at a time. On cancellation, the active lifecycle is asked to
// cancel and drain. Once ShutdownTimeout elapses, its context is cancelled, its caller receives a
// terminal deadline result, and the worker returns without waiting on a stuck dependency.
func (m *Manager) Start(ctx context.Context) {
	m.log.Info("started", "from", m.signer.Address().Hex())
	lifecycleCtx, cancelLifecycle := context.WithCancelCause(context.WithoutCancel(ctx))
	defer cancelLifecycle(errManagerStopped)
	stop := func(reason error) {
		close(m.stopping)
		m.requestActiveCancellation()
		drained := make(chan struct{})
		go func() {
			m.lifecycleWG.Wait()
			close(drained)
		}()
		timer := time.NewTimer(m.cfg.ShutdownTimeout)
		defer timer.Stop()
		select {
		case <-drained:
		case <-timer.C:
			m.log.Error(errShutdownTimeout, "transaction lifecycle drain deadline reached",
				"timeout", m.cfg.ShutdownTimeout.String(),
			)
			cancelLifecycle(errShutdownTimeout)
			m.deliverActiveShutdownTimeout()
			reason = errShutdownTimeout
		}
		m.log.Info("stopped", "reason", reason.Error())
	}
	for {
		select {
		case <-ctx.Done():
			stop(ctx.Err())
			return
		case j := <-m.queue:
			if err := ctx.Err(); err != nil {
				j.res <- notAdmittedResult(err)
				m.releaseLifecycleSlot()
				stop(err)
				return
			}
			if err := m.nonceConflictError(); err != nil {
				j.res <- notAdmittedResult(err)
				m.releaseLifecycleSlot()
				continue
			}
			pending, err := m.broadcast(ctx, j.req)
			if err != nil {
				j.res <- Result{
					Err:         err,
					NotAdmitted: errors.Is(err, errNonceLanePaused) || ctx.Err() != nil,
				}
				m.releaseLifecycleSlot()
				continue
			}
			pending.result = j.res
			m.trackUnminedTransaction(pending)
			m.lifecycleWG.Go(func() {
				defer m.releaseLifecycleSlot()
				m.complete(lifecycleCtx, pending)
			})
		}
	}
}

// Send enqueues a transaction and blocks until it is confirmed or fails. Safe for concurrent
// callers; admission and the initial broadcast are serialized through the worker.
//
// ctx and CancelAt govern the pre-sign admission wait. Once enqueued, the worker broadcasts the tx on
// the manager's own long-lived context, so Send waits for and returns that real outcome — it must not
// report a cancellation while the transaction still lands on-chain, which a caller would read as
// "not sent". The worker owns fee replacement and same-nonce cancellation until it can deliver the
// real terminal receipt. Manager shutdown requests same-nonce cancellation instead of abandoning it.
func (m *Manager) Send(ctx context.Context, req Request) Result {
	result, accepted := m.sendAsync(ctx, req, false)
	if !accepted {
		return notAdmittedResult(ctx.Err())
	}
	return <-result
}

// TrySend submits only when no signed lifecycle is active.
func (m *Manager) TrySend(ctx context.Context, req Request) (Result, bool) {
	result, accepted := m.sendAsync(ctx, req, true)
	if !accepted {
		return Result{}, false
	}
	return <-result, true
}

// SendAsync waits without accepting or signing while another lifecycle is unresolved, then enqueues
// one transaction and returns its eventual receipt result. ctx and CancelAt can still stop this wait;
// a deadline or manager stop returns a terminal pre-admission error without signing. Once enqueued,
// the manager owns the broadcast and receipt lifecycle.
func (m *Manager) SendAsync(ctx context.Context, req Request) (<-chan Result, bool) {
	return m.sendAsync(ctx, req, false)
}

func (m *Manager) sendAsync(ctx context.Context, req Request, try bool) (<-chan Result, bool) {
	m.admissionDemand.Add(1)
	releaseDemandOnReturn := true
	defer func() {
		if releaseDemandOnReturn {
			m.releaseAdmissionDemand()
		}
	}()

	admissionCtx := ctx
	cancel := func() {}
	if !req.CancelAt.IsZero() {
		admissionCtx, cancel = context.WithDeadline(ctx, req.CancelAt)
	}
	defer cancel()
	if err := admissionCtx.Err(); err != nil {
		return admissionFailure(ctx, req, err)
	}
	select {
	case <-m.stopping:
		return admissionFailure(ctx, req, errManagerStopped)
	default:
	}
	if err := m.nonceConflictError(); err != nil {
		res := make(chan Result, 1)
		res <- notAdmittedResult(err)
		return res, true
	}
	if try {
		select {
		case m.lifecycleSlot <- struct{}{}:
		default:
			return nil, false
		}
	} else {
		select {
		case m.lifecycleSlot <- struct{}{}:
		case <-admissionCtx.Done():
			return admissionFailure(ctx, req, admissionCtx.Err())
		case <-m.stopping:
			return admissionFailure(ctx, req, errManagerStopped)
		}
	}
	res := make(chan Result, 1)
	select {
	case m.queue <- job{req: cloneRequest(req), res: res}:
		releaseDemandOnReturn = false
	case <-admissionCtx.Done():
		m.releaseLifecycleSlot()
		releaseDemandOnReturn = false
		return admissionFailure(ctx, req, admissionCtx.Err())
	case <-m.stopping:
		m.releaseLifecycleSlot()
		releaseDemandOnReturn = false
		return admissionFailure(ctx, req, errManagerStopped)
	}
	return res, true
}

func admissionFailure(ctx context.Context, req Request, err error) (<-chan Result, bool) {
	if ctx.Err() != nil {
		return nil, false
	}
	res := make(chan Result, 1)
	res <- notAdmittedResult(errors.Errorf("send %q before admission: %w", req.Label, err))
	return res, true
}

func notAdmittedResult(err error) Result {
	return Result{Err: err, NotAdmitted: true}
}

func (m *Manager) releaseLifecycleSlot() {
	<-m.lifecycleSlot
	m.releaseAdmissionDemand()
}

func (m *Manager) releaseAdmissionDemand() {
	if remaining := m.admissionDemand.Add(-1); remaining < 0 {
		panic("txmanager: negative admission demand")
	}
}

// MaxFeePerGas returns a profitability ceiling that includes one ordinary replacement when the
// configured limit permits it. Send recomputes the initial fees immediately before signing.
func (m *Manager) MaxFeePerGas(ctx context.Context) (*big.Int, error) {
	limit := m.normalFeeLimit(Request{})
	fees, err := m.currentFees(ctx, reserveFeeBump(limit))
	if err != nil {
		return nil, err
	}
	maxFee := bumpFee(fees.maxFee)
	if limit != nil && maxFee.Cmp(limit) > 0 {
		maxFee.Set(limit)
	}
	return maxFee, nil
}

// broadcast runs on the worker goroutine only, after lifecycle admission, so fee selection, gas
// estimation, signing, and nonce assignment stay serialized.
func (m *Manager) broadcast(ctx context.Context, req Request) (*pendingTransaction, error) {
	broadcastCtx := ctx
	cancel := func() {}
	if !req.CancelAt.IsZero() {
		broadcastCtx, cancel = context.WithDeadline(ctx, req.CancelAt)
	}
	defer cancel()
	if err := broadcastCtx.Err(); err != nil {
		return nil, errors.Errorf("send %q before broadcast: %w", req.Label, err)
	}

	if req.MaxFeePerGas != nil && req.MaxFeePerGas.Sign() <= 0 {
		return nil, errors.Errorf("send %q: request max fee per gas must be positive", req.Label)
	}
	normalLimit := m.normalFeeLimit(req)
	fees, err := m.currentFees(broadcastCtx, reserveFeeBump(normalLimit))
	if err != nil {
		return nil, errors.Errorf("send %q: %w", req.Label, err)
	}

	gas := req.GasLimit
	if gas == 0 {
		gas, err = m.estimateGas(broadcastCtx, req)
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

	nonce, err := m.nextNonce(broadcastCtx)
	if err != nil {
		return nil, err
	}
	signed, sendErr := m.signAndSend(
		broadcastCtx, nonce, req.To, req.Data, value, gas, fees, false,
	)
	if signed == nil {
		return nil, errors.Errorf("send %q: %w", req.Label, sendErr)
	}
	hash := signed.Hash()
	if sendErr != nil {
		m.log.Error(sendErr, "transaction broadcast uncertain; tracking signed hash",
			"label", req.Label, "hash", hash.Hex(), "nonce", nonce)
	} else {
		m.log.Info("sent", "label", req.Label, "hash", hash.Hex(), "nonce", nonce)
	}
	m.commitNonce(nonce)
	return &pendingTransaction{
		req:          req,
		nonce:        nonce,
		gas:          gas,
		value:        new(big.Int).Set(value),
		fees:         cloneFeeQuote(fees),
		attempts:     []txAttempt{{hash: hash, tx: signed}},
		originalHash: hash,
	}, nil
}

func (m *Manager) complete(ctx context.Context, pending *pendingTransaction) {
	defer m.removeUnminedTransaction(pending)
	outcome := m.waitForPendingTransaction(ctx, pending)
	if errors.Is(outcome.Err, errShutdownTimeout) {
		m.log.Error(outcome.Err, "accepted transaction lifecycle did not drain before shutdown",
			"label", pending.req.Label,
			"nonce", pending.nonce,
			"hashes", attemptHashStrings(pending.attempts),
		)
	}
	if outcome.Receipt != nil {
		m.clearNonceConflict(pending.nonce)
	}
	pending.deliver(outcome)
}

func (pending *pendingTransaction) deliver(result Result) {
	if pending.result == nil {
		return
	}
	pending.resultOnce.Do(func() { pending.result <- result })
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
	timeout := time.NewTimer(max(time.Until(pending.cancelDeadline), 0))
	defer timeout.Stop()

	cancelling := false
	for {
		if receiptResult, done := m.receiptResult(ctx, pending); done {
			return receiptResult
		}
		select {
		case <-ctx.Done():
			return Result{Hash: pending.attempts[0].hash, Err: context.Cause(ctx)}
		case <-pending.cancelRequested:
			cancelling = true
			m.tryReplace(ctx, pending, true)
		case <-poll.C:
		case <-replace.C:
			if !pending.req.CancelAt.IsZero() && !time.Now().Before(pending.req.CancelAt) {
				cancelling = true
			}
			m.tryReplace(ctx, pending, cancelling)
		case <-timeout.C:
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
	lookupCtx, cancelLookup := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancelLookup()
	attempts := len(pending.attempts)
	if attempts == 0 {
		return Result{}, false
	}
	start := pending.receiptCursor % attempts
	for checked := range attempts {
		if lookupCtx.Err() != nil {
			break
		}
		i := (start + checked) % attempts
		pending.receiptCursor = (i + 1) % attempts
		attempt := pending.attempts[i]
		receipt, err := m.backend.TransactionReceipt(lookupCtx, attempt.hash)
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
		if err := validateReceipt(attempt.hash, receipt); err != nil {
			m.log.Error(err, "invalid pending transaction receipt",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
			)
			continue
		}
		cancelLookup()
		if pending.nonceConflictHash != (common.Hash{}) && m.hasNonceConflict(pending.nonce) {
			if err := m.confirmCanonicalReceipt(ctx, receipt); err != nil {
				m.log.Error(err, "owned receipt cannot reconcile nonce conflict",
					"label", pending.req.Label,
					"hash", attempt.hash.Hex(),
					"nonce", pending.nonce,
				)
				continue
			}
			m.clearNonceConflict(pending.nonce)
		}
		confirmations := m.confirmations(pending.req)
		receipt, err = m.waitForConfirmations(ctx, attempt.hash, receipt, confirmations)
		if errors.Is(err, errReceiptReorged) {
			if pending.nonceConflictHash != (common.Hash{}) {
				m.markNonceConflict(pending.nonce, pending.nonceConflictHash)
			}
			m.log.Info("transaction inclusion reorged; resuming pending lifecycle",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
			)
			return Result{}, false
		}
		if err != nil {
			return Result{Hash: attempt.hash, Receipt: receipt, Err: err}, true
		}
		if receipt.Status == types.ReceiptStatusFailed {
			revertErr := errors.Errorf("tx %s reverted on-chain", attempt.hash.Hex())
			m.log.Error(revertErr, "transaction reverted",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
				"tenderly", tenderly.SimulatorURL(m.chainID, m.signer.Address(), pending.req.To, pending.req.Data, pending.req.Value),
			)
			return Result{
				Hash:    attempt.hash,
				Receipt: receipt,
				Err:     revertErr,
			}, true
		}
		if attempt.cancellation {
			return Result{
				Hash:    attempt.hash,
				Receipt: receipt,
				Err: errors.Errorf(
					"send %q: pending transaction cancelled at nonce %d",
					pending.req.Label, pending.nonce,
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
			"confirmations", confirmations,
		)
		return Result{Hash: attempt.hash, Receipt: receipt}, true
	}
	return Result{}, false
}

func (m *Manager) tryReplace(ctx context.Context, pending *pendingTransaction, cancellation bool) {
	if m.hasNonceConflict(pending.nonce) {
		return
	}
	limit := m.normalFeeLimit(pending.req)
	if cancellation {
		limit = m.globalFeeLimit()
	}
	fees, err := m.nextReplacementFees(ctx, pending.fees, limit)
	if err != nil {
		if errors.Is(err, errReplacementLimitReached) &&
			m.rebroadcastLatestAttempt(ctx, pending, cancellation) {
			return
		}
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
	signed, sendErr := m.signAndSend(ctx, pending.nonce, to, data, value, gas, fees, true)
	if signed == nil {
		m.log.Error(sendErr, "pending transaction replacement rejected",
			"label", pending.req.Label,
			"nonce", pending.nonce,
			"cancellation", cancellation,
		)
		return
	}
	hash := signed.Hash()
	pending.fees = cloneFeeQuote(fees)
	pending.attempts = append(pending.attempts, txAttempt{
		hash: hash, tx: signed, cancellation: cancellation,
	})
	if isNonceConsumedError(sendErr) {
		pending.nonceConflictHash = hash
		m.reconcileExistingLifecycleNonce(ctx, pending)
	}
	if sendErr != nil {
		m.log.Error(sendErr, "replacement broadcast uncertain; tracking signed hash",
			"label", pending.req.Label,
			"hash", hash.Hex(),
			"nonce", pending.nonce,
			"cancellation", cancellation,
		)
		return
	}
	m.log.Info("pending transaction replaced",
		"label", pending.req.Label,
		"hash", hash.Hex(),
		"nonce", pending.nonce,
		"cancellation", cancellation,
		"maxFeePerGas", fees.maxFee.String(),
		"maxPriorityFeePerGas", fees.tip.String(),
	)
}

func (m *Manager) rebroadcastLatestAttempt(
	ctx context.Context,
	pending *pendingTransaction,
	cancellation bool,
) bool {
	for i := len(pending.attempts) - 1; i >= 0; i-- {
		attempt := pending.attempts[i]
		if attempt.cancellation != cancellation || attempt.tx == nil {
			continue
		}
		err := m.sendSigned(ctx, attempt.tx, true)
		if isNonceConsumedError(err) {
			pending.nonceConflictHash = attempt.hash
			m.reconcileExistingLifecycleNonce(ctx, pending)
		}
		if err != nil {
			m.log.Error(err, "capped transaction rebroadcast failed",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
				"cancellation", cancellation,
			)
		} else {
			m.log.Info("capped transaction rebroadcast",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
				"cancellation", cancellation,
			)
		}
		return true
	}
	return false
}

func (m *Manager) nextReplacementFees(
	ctx context.Context,
	previous feeQuote,
	limit *big.Int,
) (feeQuote, error) {
	current, err := m.currentFees(ctx, nil)
	if err != nil && !errors.Is(err, errFreshFeesUnavailable) {
		return feeQuote{}, err
	}
	requiredTip := bumpFee(previous.tip)
	requiredMaxFee := bumpFee(previous.maxFee)
	next := feeQuote{
		baseFee: new(big.Int).Set(previous.baseFee),
		tip:     new(big.Int).Set(requiredTip),
		maxFee:  new(big.Int).Set(requiredMaxFee),
	}
	if err == nil {
		next.baseFee.Set(current.baseFee)
		next.maxFee = maxBigCopy(current.maxFee, next.maxFee)
	} else {
		m.log.V(1).Info("fresh replacement fees unavailable; using cached bump", "error", err)
	}
	if limit != nil && next.maxFee.Cmp(limit) > 0 {
		next.maxFee.Set(limit)
	}
	effectiveTipLimit := new(big.Int).Sub(next.maxFee, next.baseFee)
	if effectiveTipLimit.Sign() < 0 {
		return feeQuote{}, errors.Errorf(
			"replacement base fee %s exceeds fee limit %s", next.baseFee, next.maxFee,
		)
	}
	if err == nil {
		freshTip := new(big.Int).Set(current.tip)
		if freshTip.Cmp(effectiveTipLimit) > 0 {
			freshTip.Set(effectiveTipLimit)
		}
		next.tip = maxBigCopy(freshTip, requiredTip)
	}
	if next.maxFee.Cmp(requiredMaxFee) < 0 || next.tip.Cmp(next.maxFee) > 0 {
		return feeQuote{}, errors.Errorf(
			"%w: previous max fee %s tip %s, limit %s",
			errReplacementLimitReached,
			previous.maxFee, previous.tip, feeLimitString(limit),
		)
	}
	return next, nil
}

func (m *Manager) normalFeeLimit(req Request) *big.Int {
	limit := reserveFeeBump(m.globalFeeLimit())
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

func (m *Manager) trackUnminedTransaction(pending *pendingTransaction) {
	if pending.cancelDeadline.IsZero() {
		pending.cancelDeadline = m.cancellationDeadline(pending.req)
	}
	if pending.cancelRequested == nil {
		pending.cancelRequested = make(chan struct{}, 1)
	}
	m.unminedMu.Lock()
	defer m.unminedMu.Unlock()
	if m.unmined != nil {
		panic("txmanager: multiple signed lifecycles")
	}
	m.unmined = pending
}

func (m *Manager) removeUnminedTransaction(pending *pendingTransaction) {
	m.unminedMu.Lock()
	defer m.unminedMu.Unlock()
	if m.unmined == pending {
		m.unmined = nil
	}
}

func (m *Manager) requestActiveCancellation() {
	m.unminedMu.Lock()
	defer m.unminedMu.Unlock()
	if m.unmined != nil {
		requestCancellation(m.unmined)
	}
}

func (m *Manager) deliverActiveShutdownTimeout() {
	m.unminedMu.Lock()
	pending := m.unmined
	m.unminedMu.Unlock()
	if pending != nil {
		pending.deliver(Result{Hash: pending.originalHash, Err: errShutdownTimeout})
	}
}

func requestCancellation(pending *pendingTransaction) {
	pending.cancelOnce.Do(func() { pending.cancelRequested <- struct{}{} })
}

// currentFees computes the current EIP-1559 base fee, tip, and fee cap under the supplied lifecycle
// limit. A nil limit is unbounded.
func (m *Manager) currentFees(ctx context.Context, limit *big.Int) (feeQuote, error) {
	feeCtx, cancel := context.WithTimeout(ctx, m.feeReadTimeout())
	defer cancel()

	head, err := m.backend.HeaderByNumber(feeCtx, nil)
	if err != nil {
		return feeQuote{}, errors.Errorf("%w: header by number: %w", errFreshFeesUnavailable, err)
	}
	if head == nil || head.BaseFee == nil || head.BaseFee.Sign() < 0 {
		return feeQuote{}, errors.Errorf("%w: latest header must contain a non-negative base fee", errFreshFeesUnavailable)
	}
	baseFee := new(big.Int).Set(head.BaseFee)

	tipFloor := gweiToWei(m.cfg.TipGwei)
	var tip *big.Int
	if tipFloor.Sign() == 0 {
		history, historyErr := m.backend.FeeHistory(
			feeCtx, feeHistoryBlocks, nil, []float64{feeHistoryPercentile},
		)
		if historyErr != nil {
			return feeQuote{}, errors.Errorf("%w: fee history: %w", errFreshFeesUnavailable, historyErr)
		}
		var valid bool
		tip, valid = feeHistoryTip(history)
		if !valid {
			return feeQuote{}, errors.Errorf("%w: invalid fee history rewards", errFreshFeesUnavailable)
		}
	} else {
		suggestedTip, tipErr := m.backend.SuggestGasTipCap(feeCtx)
		if tipErr == nil && suggestedTip != nil && suggestedTip.Sign() >= 0 {
			tip = maxBigCopy(suggestedTip, tipFloor)
		} else if ctx.Err() != nil {
			return feeQuote{}, errors.Errorf("%w: suggest gas tip: %w", errFreshFeesUnavailable, ctx.Err())
		} else {
			tip = tipFloor
		}
	}

	// 2*baseFee + tip leaves headroom for one base-fee doubling between now and inclusion.
	maxFee := new(big.Int).Add(new(big.Int).Mul(baseFee, big.NewInt(2)), tip)
	if limit != nil {
		if maxFee.Cmp(limit) > 0 {
			maxFee.Set(limit)
		}
	}
	maxTip := new(big.Int).Sub(maxFee, baseFee)
	if maxTip.Sign() < 0 {
		return feeQuote{}, errors.Errorf(
			"fee limit reached: current base fee %s exceeds tx manager max fee %s", baseFee, maxFee,
		)
	}
	if tipFloor.Sign() > 0 && tipFloor.Cmp(maxTip) > 0 {
		return feeQuote{}, errors.Errorf(
			"fee limit reached: fee limit %s cannot cover base fee %s plus priority fee floor %s",
			maxFee, baseFee, tipFloor,
		)
	}
	if tip.Cmp(maxTip) > 0 {
		tip.Set(maxTip)
	}
	return feeQuote{baseFee: baseFee, tip: tip, maxFee: maxFee}, nil
}

func feeHistoryTip(history *ethereum.FeeHistory) (*big.Int, bool) {
	if history == nil || len(history.Reward) == 0 || len(history.Reward) > feeHistoryBlocks {
		return nil, false
	}
	rewards := make([]*big.Int, len(history.Reward))
	for i, blockRewards := range history.Reward {
		if len(blockRewards) != 1 || blockRewards[0] == nil || blockRewards[0].Sign() < 0 {
			return nil, false
		}
		rewards[i] = new(big.Int).Set(blockRewards[0])
	}
	slices.SortFunc(rewards, func(left, right *big.Int) int { return left.Cmp(right) })
	middle := len(rewards) / 2
	tip := new(big.Int).Set(rewards[middle])
	if len(rewards)%2 == 0 {
		tip.Add(tip, rewards[middle-1]).Div(tip, big.NewInt(2))
	}
	return tip, true
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
	existingLifecycle bool,
) (*types.Transaction, error) {
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
	signed, err := m.signer.SignTx(ctx, tx, m.chainID)
	if err != nil {
		return nil, errors.Errorf("sign transaction: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Errorf("sign transaction: %w", err)
	}
	sendErr := m.sendSigned(ctx, signed, existingLifecycle)
	if errors.Is(sendErr, errNonceLanePaused) || isDefiniteBroadcastRejection(sendErr) ||
		(!existingLifecycle && isPendingNonceCollision(sendErr)) {
		return nil, errors.Errorf("broadcast rejected before acceptance: %w", sendErr)
	}
	return signed, sendErr
}

func (m *Manager) sendSigned(
	ctx context.Context,
	signed *types.Transaction,
	existingLifecycle bool,
) error {
	if !existingLifecycle {
		if err := m.nonceConflictError(); err != nil {
			return err
		}
	}
	sendCtx, cancel := context.WithTimeout(ctx, m.broadcastTimeout())
	defer cancel()
	err := m.backend.SendTransaction(sendCtx, signed)
	if !existingLifecycle && (isNonceConsumedError(err) || isPendingNonceCollision(err)) {
		m.markNonceConflict(signed.Nonce(), signed.Hash())
	}
	return err
}

// reconcileExistingLifecycleNonce distinguishes the benign race where one of this lifecycle's
// exact signed attempts was included just before a replacement from unexplained nonce consumption.
// Only a receipt proven canonical against a stable head can resume the lane.
func (m *Manager) reconcileExistingLifecycleNonce(ctx context.Context, pending *pendingTransaction) {
	if m.hasCanonicalTrackedReceipt(ctx, pending) {
		m.clearNonceConflict(pending.nonce)
		return
	}
	m.markNonceConflict(pending.nonce, pending.nonceConflictHash)
}

func (m *Manager) hasCanonicalTrackedReceipt(ctx context.Context, pending *pendingTransaction) bool {
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	for _, attempt := range pending.attempts {
		receipt, err := m.backend.TransactionReceipt(lookupCtx, attempt.hash)
		if errors.Is(err, ethereum.NotFound) {
			continue
		}
		if err != nil {
			m.log.Error(err, "tracked receipt unavailable during nonce reconciliation",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
			)
			continue
		}
		if err := validateReceipt(attempt.hash, receipt); err != nil {
			m.log.Error(err, "invalid tracked receipt during nonce reconciliation",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
			)
			continue
		}
		if err := m.confirmCanonicalReceipt(lookupCtx, receipt); err != nil {
			m.log.Error(err, "tracked receipt is not canonically visible during nonce reconciliation",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
			)
			continue
		}
		return true
	}
	return false
}

// isDefiniteBroadcastRejection is intentionally narrow. Once bytes have been signed and submitted,
// transport, decoding, nonce, fee, and generic RPC errors are ambiguous and the exact hash must stay
// tracked. These validation failures cannot have entered a node's transaction pool and cannot be
// repaired by replacing the same lifecycle.
func isDefiniteBroadcastRejection(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "insufficient funds") ||
		strings.Contains(message, "intrinsic gas too low") ||
		strings.Contains(message, "invalid sender") ||
		strings.Contains(message, "transaction type not supported")
}

func isNonceConsumedError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "nonce too low") ||
		strings.Contains(message, "nonce is too low") ||
		strings.Contains(message, "nonce has already been used")
}

func isPendingNonceCollision(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "replacement transaction underpriced")
}

// nextNonce returns the nonce to use, failing closed if startup discovers an unknown pending
// transaction that the in-memory manager cannot safely replace or cancel.
func (m *Manager) nextNonce(ctx context.Context) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.nonceConflictErrorLocked(); err != nil {
		return 0, err
	}
	if err := m.initializeNonceLocked(ctx); err != nil {
		return 0, err
	}
	return m.nonce, nil
}

func (m *Manager) markNonceConflict(nonce uint64, hash common.Hash) {
	m.mu.Lock()
	if m.conflict != nil && m.conflict.nonce != nonce {
		panic("txmanager: multiple nonce conflicts")
	}
	first := m.conflict == nil
	m.conflict = &nonceConflict{nonce: nonce, hash: hash}
	m.mu.Unlock()
	if first {
		m.notifyAvailabilityChange()
		m.log.Error(errors.New("nonce ownership is uncertain"),
			"transaction manager paused pending nonce reconciliation",
			"nonce", nonce,
			"hash", hash.Hex(),
		)
	}
}

func (m *Manager) clearNonceConflict(nonce uint64) {
	m.mu.Lock()
	existed := m.conflict != nil && m.conflict.nonce == nonce
	if existed {
		m.conflict = nil
	}
	m.mu.Unlock()
	if existed {
		m.notifyAvailabilityChange()
	}
}

func (m *Manager) notifyAvailabilityChange() {
	m.availabilityMu.Lock()
	defer m.availabilityMu.Unlock()
	for _, changes := range m.availabilitySubscribers {
		select {
		case changes <- struct{}{}:
		default:
		}
	}
}

func (m *Manager) nonceConflictError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nonceConflictErrorLocked()
}

func (m *Manager) nonceConflictErrorLocked() error {
	if m.conflict == nil {
		return nil
	}
	return errors.Errorf(
		"%w: nonce %d has uncertain ownership; attempted signed hash %s has no receipt",
		errNonceLanePaused, m.conflict.nonce, m.conflict.hash.Hex(),
	)
}

func (m *Manager) hasNonceConflict(nonce uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.conflict != nil && m.conflict.nonce == nonce
}

func (m *Manager) initializeNonceLocked(ctx context.Context) error {
	if m.nonceInit {
		return nil
	}
	latest, err := m.backend.NonceAt(ctx, m.signer.Address(), nil)
	if err != nil {
		return errors.Errorf("latest mined nonce: %w", err)
	}
	pending, err := m.backend.PendingNonceAt(ctx, m.signer.Address())
	if err != nil {
		return errors.Errorf("pending nonce: %w", err)
	}
	if pending != latest {
		return errors.Errorf(
			"unmanaged pending nonce gap: latest mined nonce %d, pending nonce %d",
			latest, pending,
		)
	}
	m.nonce = pending
	m.nonceInit = true
	return nil
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
	hash common.Hash,
	receipt *types.Receipt,
	confirmations uint64,
) (*types.Receipt, error) {
	if receipt == nil || receipt.BlockNumber == nil {
		return receipt, errors.New("receipt block number is required")
	}
	if confirmations == 0 {
		return receipt, nil
	}
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()

	for {
		headBefore, headErr := m.confirmationHead(ctx)
		if headErr != nil {
			m.log.Error(headErr, "confirmation head unavailable", "hash", hash.Hex())
		}
		refreshed, err := m.confirmationReceipt(ctx, hash)
		if err != nil {
			if errors.Is(err, errReceiptReorged) {
				return receipt, err
			}
			m.log.Error(err, "receipt confirmation check unavailable", "hash", hash.Hex())
		} else {
			receipt = refreshed
			if headErr == nil {
				head := headBefore.Number.Uint64()
				included := receipt.BlockNumber.Uint64()
				if head >= included && head-included >= confirmations {
					if err := m.confirmReceiptAncestry(ctx, headBefore, receipt); err != nil {
						if errors.Is(err, errReceiptReorged) {
							return receipt, err
						}
						m.log.Error(err, "receipt ancestry check unavailable", "hash", hash.Hex())
					} else {
						headAfter, afterErr := m.confirmationHead(ctx)
						if afterErr != nil {
							m.log.Error(afterErr, "confirmation head unavailable", "hash", hash.Hex())
						} else if headBefore.Hash() == headAfter.Hash() {
							return receipt, nil
						}
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return receipt, context.Cause(ctx)
		case <-ticker.C:
		}
	}
}

func (m *Manager) confirmationHead(ctx context.Context) (*types.Header, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	header, err := m.backend.HeaderByNumber(lookupCtx, nil)
	if err != nil {
		return nil, err
	}
	if header == nil || header.Number == nil {
		return nil, errors.New("latest header number is required")
	}
	return header, nil
}

func (m *Manager) confirmationReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	receiptCtx, cancelReceipt := context.WithTimeout(ctx, m.receiptReadTimeout())
	receipt, err := m.backend.TransactionReceipt(receiptCtx, hash)
	cancelReceipt()
	if errors.Is(err, ethereum.NotFound) {
		return nil, errors.Errorf("%w: receipt %s disappeared", errReceiptReorged, hash.Hex())
	}
	if err != nil {
		return nil, errors.Errorf("transaction receipt %s: %w", hash.Hex(), err)
	}
	if err := validateReceipt(hash, receipt); err != nil {
		return nil, err
	}
	return receipt, nil
}

func (m *Manager) confirmCanonicalReceipt(ctx context.Context, receipt *types.Receipt) error {
	headBefore, err := m.confirmationHead(ctx)
	if err != nil {
		return errors.Errorf("confirmation head before ancestry check: %w", err)
	}
	if err := m.confirmReceiptAncestry(ctx, headBefore, receipt); err != nil {
		return err
	}
	headAfter, err := m.confirmationHead(ctx)
	if err != nil {
		return errors.Errorf("confirmation head after ancestry check: %w", err)
	}
	if headBefore.Hash() != headAfter.Hash() {
		return errors.New("confirmation head changed during ancestry check")
	}
	return nil
}

func (m *Manager) confirmReceiptAncestry(
	ctx context.Context,
	head *types.Header,
	receipt *types.Receipt,
) error {
	if head == nil || head.Number == nil || receipt == nil || receipt.BlockNumber == nil {
		return errors.New("confirmation ancestry requires head and receipt block numbers")
	}
	if !head.Number.IsUint64() || !receipt.BlockNumber.IsUint64() {
		return errors.New("confirmation ancestry block number exceeds uint64")
	}
	included := receipt.BlockNumber.Uint64()
	current := head
	for current.Number.Uint64() > included {
		lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
		parent, err := m.backend.HeaderByHash(lookupCtx, current.ParentHash)
		cancel()
		if err != nil {
			return errors.Errorf("parent header %s: %w", current.ParentHash.Hex(), err)
		}
		if parent == nil || parent.Number == nil || !parent.Number.IsUint64() {
			return errors.Errorf("parent header %s is invalid", current.ParentHash.Hex())
		}
		if parent.Hash() != current.ParentHash || parent.Number.Uint64() != current.Number.Uint64()-1 {
			return errors.Errorf("parent header %s does not link to block %s", parent.Hash(), current.Hash())
		}
		current = parent
	}
	if current.Number.Uint64() != included || current.Hash() != receipt.BlockHash {
		return errors.Errorf(
			"%w: receipt block %s is no longer canonical", errReceiptReorged, receipt.BlockHash.Hex(),
		)
	}
	return nil
}

func validateReceipt(hash common.Hash, receipt *types.Receipt) error {
	if receipt == nil || receipt.BlockNumber == nil {
		return errors.Errorf("transaction receipt %s has no block number", hash.Hex())
	}
	if receipt.TxHash != hash {
		return errors.Errorf(
			"transaction receipt %s returned mismatched hash %s", hash.Hex(), receipt.TxHash.Hex(),
		)
	}
	if receipt.BlockHash == (common.Hash{}) {
		return errors.Errorf("transaction receipt %s has no block hash", hash.Hex())
	}
	return nil
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

func reserveFeeBump(limit *big.Int) *big.Int {
	if limit == nil {
		return nil
	}
	reserved := new(big.Int).Mul(limit, big.NewInt(replacementBumpDenominator))
	return reserved.Div(reserved, big.NewInt(replacementBumpNumerator))
}

func maxBigCopy(a, b *big.Int) *big.Int {
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

func attemptHashStrings(attempts []txAttempt) []string {
	hashes := make([]string, len(attempts))
	for i, attempt := range attempts {
		hashes[i] = attempt.hash.Hex()
	}
	return hashes
}

func gweiToWei(gwei float64) *big.Int {
	wei, _ := new(big.Float).Mul(big.NewFloat(gwei), big.NewFloat(params.GWei)).Int(nil)
	return wei
}

func (m *Manager) cancellationDeadline(req Request) time.Time {
	deadline := time.Now().Add(m.cfg.PendingTimeout)
	if !req.CancelAt.IsZero() && req.CancelAt.Before(deadline) {
		return req.CancelAt
	}
	return deadline
}

func (m *Manager) feeReadTimeout() time.Duration {
	return minPositiveDuration(maxFeeReadTimeout, m.cfg.ReplacementInterval/2)
}

func (m *Manager) receiptReadTimeout() time.Duration {
	return minPositiveDuration(maxReceiptReadTimeout, m.cfg.ReplacementInterval/2)
}

func (m *Manager) broadcastTimeout() time.Duration {
	return minPositiveDuration(maxBroadcastTimeout, m.cfg.ReplacementInterval/2)
}

func minPositiveDuration(fallback, candidate time.Duration) time.Duration {
	if candidate > 0 && candidate < fallback {
		return candidate
	}
	return fallback
}
