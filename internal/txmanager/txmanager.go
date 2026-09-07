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

	"github.com/symbioticfi/vault-solver/internal/bigmath"
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

type transactionSenderBalanceBackend interface {
	TransactionSenderBalanceAt(
		ctx context.Context,
		account common.Address,
		blockNumber *big.Int,
	) (*big.Int, error)
}

type accountBalanceBackend interface {
	BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error)
}

// Config tunes fee selection and confirmation behavior.
type Config struct {
	Confirmations       uint64        // blocks to wait past inclusion before returning
	MaxFeeGwei          float64       // absolute max fee per gas; app config requires a positive value
	TipGwei             float64       // minimum priority fee; 0 => derive it from recent fee history
	PollInterval        time.Duration // receipt/confirmation poll cadence; 0 => 2s
	BroadcastTimeout    time.Duration // maximum duration of one transaction submission RPC; 0 => 5s
	AccountPollInterval time.Duration // signer balance/nonce metric refresh cadence; 0 => 30s
	ReplacementInterval time.Duration // pending tx fee-bump cadence; 0 => 30s
	PendingTimeout      time.Duration // switch from replacing the call to cancelling its nonce; 0 => 5m
	ShutdownTimeout     time.Duration // maximum graceful drain after manager cancellation; 0 => 1m
}

// Request is a transaction to send. Value nil means 0. Stateful solver calls leave GasLimit at 0 so
// gas estimation re-simulates their exact calldata after lifecycle admission and immediately before signing.
type Request struct {
	To           common.Address
	Data         []byte
	Value        *big.Int
	GasLimit     uint64
	MaxFeePerGas *big.Int  // optional normal-lifecycle EIP-1559 fee ceiling; cancellation may use the global ceiling
	CancelAt     time.Time // optional deadline after which the manager replaces the call with a same-nonce cancellation
	// Obsolete optionally reports that the call can no longer succeed. It must honor ctx and have no
	// authorization role: errors preserve the current lifecycle. True before signing drops the call;
	// true after broadcast switches the owned nonce to cancellation.
	Obsolete      func(ctx context.Context) (bool, error)
	Confirmations *uint64 // optional wait override; nil uses Config.Confirmations
	Label         string  // stable operation name for logs and metrics
	Solver        string  // owning solver, so a shared manager's logs and Sentry events attribute to it
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

// Included reports whether the request reached the chain, even if confirmation tracking stopped.
func (o Outcome) Included() bool {
	return o == OutcomeConfirmed || o == OutcomeIncludedUnconfirmed
}

// Result carries the outcome of one transaction request. NotAdmitted identifies manager-level
// admission failures; ordinary fee, gas, signing, and definite broadcast failures remain submission
// failures even though they do not produce a tracked hash.
type Result struct {
	Hash        common.Hash
	Receipt     *types.Receipt
	Outcome     Outcome
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
	lifecycle         lifecycleObservation
	nonce             uint64
	gas               uint64
	value             *big.Int
	fees              feeQuote
	attempts          []txAttempt
	receiptCursor     int
	nonceConflictHash common.Hash
	originalHash      common.Hash
	receiptReads      readStreak
	obsolescenceReads readStreak
	log               logr.Logger // m.log stamped with the request's solver
	cancelDeadline    time.Time
	cancelRequested   chan struct{}
	cancelOnce        sync.Once
}

type txAttempt struct {
	hash                    common.Hash
	tx                      *types.Transaction
	cancellation            bool
	exactRebroadcastPending bool
}

// Manager serializes signed lifecycles and owns accepted work through its terminal result.
type Manager struct {
	backend Backend
	signer  signer.Signer
	chainID *big.Int
	cfg     Config
	metrics *Metrics
	log     logr.Logger

	queue           chan job
	lifecycleSlot   chan struct{}
	stopping        chan struct{}
	admissionDemand atomic.Int64

	laneStateMu          sync.Mutex
	laneStateSubscribers map[chan struct{}]struct{}

	mu        sync.Mutex // guards the local nonce and runtime nonce conflict
	nonce     uint64
	nonceInit bool
	conflict  *nonceConflict

	active      atomic.Pointer[pendingTransaction]
	lifecycleWG sync.WaitGroup
}

type job struct {
	*completion

	req              Request
	admissionStarted time.Time
}

type nonceConflict struct {
	nonce uint64
	hash  common.Hash
}

const (
	defaultPollInterval        = 2 * time.Second
	defaultAccountPollInterval = 30 * time.Second
	defaultReplacementInterval = 30 * time.Second
	defaultPendingTimeout      = 5 * time.Minute
	defaultShutdownTimeout     = time.Minute
	defaultBroadcastTimeout    = 5 * time.Second
	maxFeeReadTimeout          = time.Second
	maxReceiptReadTimeout      = 2 * time.Second
	accountRefreshTimeout      = 5 * time.Second
	feeHistoryBlocks           = 5
	feeHistoryPercentile       = 25.0
	replacementBumpNumerator   = 9
	replacementBumpDenominator = 8
	cancellationGasLimit       = 21_000
)

var (
	errFreshFeesUnavailable    = errors.New("fresh fees unavailable")
	errReplacementLimitReached = errors.New("replacement fee limit reached")
	errReceiptReorged          = errors.New("transaction receipt reorged")
	errRequestObsolete         = errors.New("transaction request is obsolete")
	errNonceLanePaused         = errors.New("transaction manager nonce lane paused")
	errManagerStopped          = errors.New("transaction manager stopped")
	errShutdownTimeout         = errors.Errorf("transaction manager shutdown drain timed out: %w", context.DeadlineExceeded)
)

// New constructs a Manager. Call Start to launch its worker.
func New(backend Backend, account signer.Signer, chainID *big.Int, cfg Config, log logr.Logger) *Manager {
	for _, duration := range []struct {
		value    *time.Duration
		fallback time.Duration
	}{
		{&cfg.PollInterval, defaultPollInterval}, {&cfg.BroadcastTimeout, defaultBroadcastTimeout},
		{&cfg.AccountPollInterval, defaultAccountPollInterval}, {&cfg.ReplacementInterval, defaultReplacementInterval},
		{&cfg.PendingTimeout, defaultPendingTimeout}, {&cfg.ShutdownTimeout, defaultShutdownTimeout},
	} {
		if *duration.value <= 0 {
			*duration.value = duration.fallback
		}
	}
	manager := &Manager{backend: backend, signer: account, cfg: cfg, log: log.WithName("txmanager"),
		queue: make(chan job), lifecycleSlot: make(chan struct{}, 1), stopping: make(chan struct{}),
		laneStateSubscribers: make(map[chan struct{}]struct{})}
	manager.chainID = bigmath.Clone(chainID)
	return manager
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

// Available reports nonce safety only; it does not report whether another lifecycle occupies the
// lane. Owned execution paths use it to keep progressing during contention, while producers of new
// external commitments use LaneReady. A nonce conflict pauses admission while exact signed hashes
// are polled. A benign inclusion race resumes once its exact receipt is proven canonical; an
// unresolved conflict remains fail-closed instead of replaying calldata at another nonce.
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

// LaneReady reports whether the nonce lane is both safe and idle, so a solver can make an external
// commitment that requires prompt transaction admission.
func (m *Manager) LaneReady() bool {
	return m.Available() && m.Idle()
}

// SubscribeLaneState returns an independent, coalesced change stream. Consumers must call
// LaneReady after every signal instead of assuming which edge occurred, and must call unsubscribe
// when they stop. Signals cover both nonce-conflict and admission-demand edges. Independent
// subscriptions prevent readiness and solvers from stealing signals from each other.
func (m *Manager) SubscribeLaneState() (<-chan struct{}, func()) {
	m.laneStateMu.Lock()
	changes := make(chan struct{}, 1)
	m.laneStateSubscribers[changes] = struct{}{}
	m.laneStateMu.Unlock()

	return changes, func() {
		m.laneStateMu.Lock()
		delete(m.laneStateSubscribers, changes)
		m.laneStateMu.Unlock()
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

func (m *Manager) monitorAccount(ctx context.Context) {
	if m.metrics == nil || !m.supportsAccountBalance() {
		return
	}
	m.refreshAccount(ctx)
	ticker := time.NewTicker(m.cfg.AccountPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.refreshAccount(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (m *Manager) supportsAccountBalance() bool {
	_, senderBalance := m.backend.(transactionSenderBalanceBackend)
	_, ordinaryBalance := m.backend.(accountBalanceBackend)
	return senderBalance || ordinaryBalance
}

func (m *Manager) refreshAccount(ctx context.Context) {
	readCtx, cancel := context.WithTimeout(ctx, accountRefreshTimeout)
	defer cancel()
	balance, err := m.transactionSenderBalance(readCtx)
	var mined, pending uint64
	if err == nil {
		if balance == nil || balance.Sign() < 0 {
			err = errors.New("txmanager: invalid account balance")
		} else {
			mined, pending, err = m.accountNonces(readCtx)
		}
	}
	if err == nil {
		m.metrics.observeAccount(balance, mined, pending)
		return
	}
	if ctx.Err() == nil {
		m.metrics.observeAccountRefreshError()
		m.log.V(1).Info("account metrics refresh failed", "error", err)
	}
}

func (m *Manager) accountNonces(ctx context.Context) (mined, pending uint64, err error) {
	account := m.signer.Address()
	mined, err = m.backend.NonceAt(ctx, account, nil)
	if err != nil {
		return 0, 0, errors.Errorf("latest mined nonce: %w", err)
	}
	pending, err = m.backend.PendingNonceAt(ctx, account)
	if err != nil {
		return 0, 0, errors.Errorf("pending nonce: %w", err)
	}
	return mined, pending, nil
}

func (m *Manager) transactionSenderBalance(ctx context.Context) (*big.Int, error) {
	if backend, ok := m.backend.(transactionSenderBalanceBackend); ok {
		return backend.TransactionSenderBalanceAt(ctx, m.signer.Address(), nil)
	}
	if backend, ok := m.backend.(accountBalanceBackend); ok {
		return backend.BalanceAt(ctx, m.signer.Address(), nil)
	}
	return nil, errors.New("txmanager: backend does not expose account balance")
}

// Start admits one signed lifecycle at a time. On cancellation, the active lifecycle is asked to
// cancel and drain. Once ShutdownTimeout elapses, its context is cancelled, its caller receives a
// terminal deadline result, and the worker returns without waiting on a stuck dependency.
func (m *Manager) Start(ctx context.Context) {
	m.metrics.bindAccount(m.signer.Address())
	monitorCtx, stopMonitor := context.WithCancel(ctx)
	monitorDone := make(chan struct{})
	go func() { defer close(monitorDone); m.monitorAccount(monitorCtx) }()
	defer func() { stopMonitor(); <-monitorDone }()
	defer m.log.Info("stopped")
	m.log.Info("started", "from", m.signer.Address().Hex())

	for {
		select {
		case <-ctx.Done():
			close(m.stopping)
			return
		case next := <-m.queue:
			if !m.runJob(ctx, next) {
				return
			}
		}
	}
}

// runJob owns a single accepted request from preparation to its terminal result. Only this worker
// can advance the queue. A separate deadline path can return a terminal result even when a custom
// signer ignores cancellation; signAndSend checks cancellation again before any later broadcast.
func (m *Manager) runJob(ctx context.Context, next job) bool {
	lifeCtx, stop := context.WithCancelCause(context.WithoutCancel(ctx))
	defer stop(errManagerStopped)
	finished := make(chan struct{})
	m.lifecycleWG.Go(func() {
		defer close(finished)
		defer m.releaseLifecycleSlot()
		m.executeJob(ctx, lifeCtx, next)
	})
	select {
	case <-finished:
		return true
	case <-ctx.Done():
		close(m.stopping)
		m.requestActiveCancellation()
		timer := time.NewTimer(m.cfg.ShutdownTimeout)
		defer timer.Stop()
		select {
		case <-finished:
		case <-timer.C:
			stop(errShutdownTimeout)
			hash := common.Hash{}
			if active := m.active.Load(); active != nil {
				hash = active.originalHash
			}
			next.finish(Result{Hash: hash, Outcome: OutcomeTrackingStopped, Err: errShutdownTimeout})
			m.log.Error(errShutdownTimeout, "transaction lifecycle drain deadline reached", "timeout", m.cfg.ShutdownTimeout)
		}
		return false
	}
}

func (m *Manager) executeJob(preparationCtx, lifeCtx context.Context, next job) {
	err := preparationCtx.Err()
	if err == nil {
		err = m.nonceConflictError()
	}
	m.metrics.finishAdmission(next.req.Label, next.admissionStarted, err)
	if err != nil {
		next.finish(notAdmittedResult(err))
		return
	}
	observation := m.metrics.beginLifecycle(next.req.Label)
	pending, err := m.broadcast(preparationCtx, next.req)
	if err != nil {
		outcome := OutcomeSubmissionError
		if preparationCtx.Err() != nil {
			outcome = OutcomeTrackingStopped
		}
		observation.finish(outcome, nil)
		next.finish(Result{
			Outcome: outcome, Err: err, NotAdmitted: errors.Is(err, errNonceLanePaused) || preparationCtx.Err() != nil,
		})
		return
	}
	pending.lifecycle = observation
	pending.lifecycle.transitionPhase(lifecyclePhasePending)
	m.trackUnminedTransaction(pending)
	// Cancellation may arrive while broadcast was resolving its signed hash, before publication.
	if preparationCtx.Err() != nil {
		requestCancellation(pending)
	}
	next.finish(m.complete(lifeCtx, pending))
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

// TrySend submits only when the nonce lane is available and no signed lifecycle is active. A false
// result from a busy lane is an expected availability probe and is not recorded as an admission
// rejection; terminal context failures are recorded.
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
	started := time.Now()
	m.addAdmissionDemand()
	acquired, transferred := false, false
	defer func() {
		if transferred {
			return
		}
		if acquired {
			<-m.lifecycleSlot
		}
		m.releaseAdmissionDemand()
	}()
	admissionCtx := ctx
	if !req.CancelAt.IsZero() {
		var cancel context.CancelFunc
		admissionCtx, cancel = context.WithDeadline(ctx, req.CancelAt)
		defer cancel()
	}
	var err error
	acquired, err = m.acquireLifecycle(admissionCtx, try)
	if err != nil {
		return m.admissionFailure(ctx, req, started, err)
	}
	if !acquired {
		return nil, false
	}
	result := make(chan Result, 1)
	next := job{completion: &completion{result: result}, req: cloneRequest(req), admissionStarted: started}
	select {
	case m.queue <- next:
		// From this point the worker owns both demand and slot, including during shutdown.
		transferred = true
		return result, true
	case <-admissionCtx.Done():
		return m.admissionFailure(ctx, req, started, admissionCtx.Err())
	case <-m.stopping:
		return m.admissionFailure(ctx, req, started, errManagerStopped)
	}
}

func (m *Manager) acquireLifecycle(ctx context.Context, try bool) (acquired bool, err error) {
	if err := m.admissionContextError(ctx); err != nil {
		return false, err
	}
	if try {
		if !m.Available() {
			return false, nil
		}
		select {
		case m.lifecycleSlot <- struct{}{}:
		default:
			return false, nil
		}
	} else {
		if err := m.waitForNonceLane(ctx); err != nil {
			return false, err
		}
		select {
		case m.lifecycleSlot <- struct{}{}:
		case <-ctx.Done():
			return false, ctx.Err()
		case <-m.stopping:
			return false, errManagerStopped
		}
	}
	// Admission may have paused while the previous owner released its slot.
	if try {
		if m.nonceConflictError() == nil {
			return true, nil
		}
	} else {
		err = m.waitForNonceLane(ctx)
		if err == nil {
			return true, nil
		}
	}
	<-m.lifecycleSlot
	return false, err
}

func (m *Manager) admissionContextError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-m.stopping:
		return errManagerStopped
	default:
		return nil
	}
}

func (m *Manager) waitForNonceLane(ctx context.Context) error {
	changes, unsubscribe := m.SubscribeLaneState()
	defer unsubscribe()
	for {
		if err := m.admissionContextError(ctx); err != nil {
			return err
		}
		if m.Available() {
			return nil
		}
		select {
		case <-changes:
		case <-ctx.Done():
			return ctx.Err()
		case <-m.stopping:
			return errManagerStopped
		}
	}
}

func (m *Manager) admissionFailure(
	ctx context.Context,
	req Request,
	started time.Time,
	err error,
) (<-chan Result, bool) {
	m.metrics.finishAdmission(req.Label, started, err)
	if ctx.Err() != nil {
		return nil, false
	}
	res := make(chan Result, 1)
	res <- notAdmittedResult(errors.Errorf("send %q before admission: %w", req.Label, err))
	return res, true
}

func notAdmittedResult(err error) Result {
	return Result{Outcome: OutcomeSubmissionError, Err: err, NotAdmitted: true}
}

func (m *Manager) releaseLifecycleSlot() {
	<-m.lifecycleSlot
	m.releaseAdmissionDemand()
}

func (m *Manager) addAdmissionDemand() {
	if m.admissionDemand.Add(1) == 1 {
		m.notifyLaneStateChange()
	}
}

func (m *Manager) releaseAdmissionDemand() {
	remaining := m.admissionDemand.Add(-1)
	if remaining < 0 {
		panic("txmanager: negative admission demand")
	}
	if remaining == 0 {
		m.notifyLaneStateChange()
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
func (m *Manager) requestLog(req Request) logr.Logger {
	if req.Solver == "" {
		return m.log
	}
	return m.log.WithValues("solver", req.Solver)
}

// Preparation builds one owned pending record. Only a signed transaction can
// commit its nonce: an ambiguous send keeps the signed bytes for reconciliation.
func (m *Manager) broadcast(ctx context.Context, req Request) (*pendingTransaction, error) {
	deadline := req.CancelAt
	if deadline.IsZero() {
		if inherited, ok := ctx.Deadline(); ok {
			deadline = inherited
		}
	}
	var cancel context.CancelFunc
	if deadline.IsZero() {
		ctx, cancel = context.WithCancel(ctx)
	} else {
		ctx, cancel = context.WithDeadline(ctx, deadline)
	}
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, errors.Errorf("send %q before broadcast: %w", req.Label, err)
	}
	if req.MaxFeePerGas != nil && req.MaxFeePerGas.Sign() <= 0 {
		return nil, errors.Errorf("send %q: request max fee per gas must be positive", req.Label)
	}
	pending := &pendingTransaction{req: req, log: m.requestLog(req), gas: req.GasLimit, value: new(big.Int)}
	if req.Value != nil {
		pending.value.Set(req.Value)
	}
	fees, err := m.currentFees(ctx, reserveFeeBump(m.normalFeeLimit(req)))
	if err != nil {
		return nil, errors.Errorf("send %q: %w", req.Label, err)
	}
	pending.fees = cloneFeeQuote(fees)
	if pending.gas == 0 {
		pending.gas, err = m.estimateGas(ctx, req)
		if err != nil {
			return nil, err
		}
	}
	obsolete, err := m.requestObsolete(ctx, req)
	switch {
	case err != nil:
		// Unknown obsolescence is not a rejection: execution contracts are authoritative.
		pending.log.Error(err, "transaction obsolescence check unavailable; continuing", "label", req.Label)
	case obsolete:
		return nil, errors.Errorf("send %q: %w", req.Label, errRequestObsolete)
	}
	pending.log.V(1).Info("transaction prepared", "label", req.Label, "to", req.To.Hex(),
		"value", pending.value.String(), "calldataBytes", len(req.Data), "gasLimit", pending.gas,
		"baseFeePerGas", fees.baseFee.String(), "maxPriorityFeePerGas", fees.tip.String(),
		"maxFeePerGas", fees.maxFee.String(), "requestMaxFeePerGas", optionalBigString(req.MaxFeePerGas))
	pending.nonce, err = m.nextNonce(ctx)
	if err != nil {
		return nil, err
	}
	signed, sendErr := m.signAndSend(ctx, pending.nonce, req.To, req.Data, pending.value, pending.gas, fees, false)
	if signed == nil {
		return nil, errors.Errorf("send %q: %w", req.Label, sendErr)
	}
	pending.originalHash = signed.Hash()
	uncertain := sendErr != nil && !isKnownTransactionError(sendErr)
	pending.attempts = []txAttempt{{hash: pending.originalHash, tx: signed, exactRebroadcastPending: uncertain}}
	log := pending.log.WithValues("label", req.Label, "hash", pending.originalHash.Hex(), "nonce", pending.nonce)
	switch {
	case uncertain:
		log.Error(sendErr, "transaction broadcast uncertain; tracking signed hash")
	case sendErr != nil:
		log.Info("transaction already known by write RPC", "rpcResult", sendErr.Error())
	default:
		log.Info("sent")
	}
	m.commitNonce(pending.nonce)
	return pending, nil
}

func (m *Manager) complete(ctx context.Context, pending *pendingTransaction) Result {
	defer m.removeUnminedTransaction(pending)
	outcome := m.waitForPendingTransaction(ctx, pending)
	pending.lifecycle.finish(outcome.Outcome, outcome.Receipt)
	if errors.Is(outcome.Err, errShutdownTimeout) {
		pending.log.Error(outcome.Err, "accepted transaction lifecycle did not drain before shutdown",
			"label", pending.req.Label,
			"nonce", pending.nonce,
			"hashes", attemptHashStrings(pending.attempts),
		)
	}
	if outcome.Receipt != nil {
		m.clearNonceConflict(pending.nonce)
	}
	return outcome
}

// completion is the sole terminal-result owner, shared by the worker and hard-stop path.
type completion struct {
	once   sync.Once
	result chan Result
}

func (c *completion) finish(result Result) { c.once.Do(func() { c.result <- result }) }

func (m *Manager) confirmations(req Request) uint64 {
	if req.Confirmations != nil {
		return *req.Confirmations
	}
	return m.cfg.Confirmations
}

// The admitted lifecycle owns all receipt, replacement and cancellation decisions.
// Cancellation is a one-way transition: its wakeup channels are disabled after entry.
func (m *Manager) waitForPendingTransaction(ctx context.Context, pending *pendingTransaction) Result {
	poll := time.NewTicker(m.cfg.PollInterval)
	defer poll.Stop()
	replace := time.NewTicker(m.cfg.ReplacementInterval)
	defer replace.Stop()
	deadline := time.NewTimer(max(time.Until(pending.cancelDeadline), 0))
	defer deadline.Stop()
	cancelC, deadlineC := pending.cancelRequested, deadline.C
	cancelling := false
	for {
		if result, done := m.receiptResult(ctx, pending); done {
			return result
		}
		reason := ""
		switch {
		case ctx.Err() != nil:
			return Result{Hash: pending.originalHash, Outcome: OutcomeTrackingStopped, Err: context.Cause(ctx)}
		default:
			select {
			case <-ctx.Done():
				return Result{Hash: pending.originalHash, Outcome: OutcomeTrackingStopped, Err: context.Cause(ctx)}
			case <-cancelC:
				reason = "shutdown"
			case <-deadlineC:
				reason = pending.cancellationReason()
			case <-poll.C:
				if cancelling || !m.pendingObsolete(ctx, pending) {
					continue
				}
				reason = "obsolete"
			case <-replace.C:
				if !cancelling && pending.cancellationDue(time.Now()) {
					reason = pending.cancellationReason()
				}
			}
		}
		if reason != "" && !cancelling {
			cancelling = true
			pending.logCancellation(reason, m.cfg.PendingTimeout)
		}
		if m.tryReplace(ctx, pending, cancelling) && !cancelling {
			// Fee lookup may cross CancelAt; the sender promotes that attempt to cancellation.
			cancelling = true
			pending.logCancellation(pending.cancellationReason(), m.cfg.PendingTimeout)
		}
		if cancelling {
			cancelC, deadlineC = nil, nil
		}
	}
}

func (pending *pendingTransaction) cancellationReason() string {
	select {
	case <-pending.cancelRequested:
		return "shutdown"
	default:
	}
	if pending.cancelDeadline.Equal(pending.req.CancelAt) {
		return "request_deadline"
	}
	return "pending_timeout"
}

func (pending *pendingTransaction) logCancellation(reason string, timeout time.Duration) {
	pending.log.Info("pending transaction cancellation requested", "label", pending.req.Label,
		"hash", pending.originalHash.Hex(), "nonce", pending.nonce, "reason", reason,
		"deadline", pending.cancelDeadline.UTC().Format(time.RFC3339Nano), "pendingTimeout", timeout.String())
}

func (m *Manager) pendingObsolete(ctx context.Context, pending *pendingTransaction) bool {
	if pending.req.Obsolete == nil {
		return false
	}
	obsolete, err := m.requestObsolete(ctx, pending.req)
	if err != nil {
		pending.obsolescenceReads.failed(pending.log, err,
			"pending transaction obsolescence check unavailable; retaining lifecycle",
			"label", pending.req.Label, "hash", pending.originalHash.Hex(), "nonce", pending.nonce)
		return false
	}
	pending.obsolescenceReads.recovered(pending.log, "pending transaction obsolescence checks recovered",
		"label", pending.req.Label, "nonce", pending.nonce)
	return obsolete
}

func (m *Manager) requestObsolete(ctx context.Context, req Request) (bool, error) {
	if req.Obsolete == nil {
		return false, nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	obsolete, err := req.Obsolete(checkCtx)
	if err != nil {
		return false, errors.Errorf("check transaction obsolescence: %w", err)
	}
	return obsolete, nil
}

func (m *Manager) receiptReadFailed(pending *pendingTransaction, attempt txAttempt, err error) {
	pending.receiptReads.failed(pending.log, err, "pending transaction receipt unavailable",
		"label", pending.req.Label,
		"hash", attempt.hash.Hex(),
		"originalHash", pending.originalHash.Hex(),
		"nonce", pending.nonce,
		"cancellation", attempt.cancellation,
		"rpcTimeout", m.receiptReadTimeout().String(),
	)
}

func (m *Manager) receiptReadsRecovered(pending *pendingTransaction) {
	pending.receiptReads.recovered(pending.log, "pending transaction receipt reads recovered",
		"label", pending.req.Label, "nonce", pending.nonce)
}

// pendingReceipt performs one bounded, rotating sweep. Read failures belong to the
// whole sweep, so a healthy hash cannot repeatedly reset another hash's failure streak.
func (m *Manager) pendingReceipt(ctx context.Context, pending *pendingTransaction) (*types.Receipt, txAttempt) {
	lookupCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	var firstErr error
	var failed txAttempt
	readOK := false
	count := len(pending.attempts)
	for range count {
		if lookupCtx.Err() != nil {
			break
		}
		index := pending.receiptCursor % count
		pending.receiptCursor = (index + 1) % count
		attempt := pending.attempts[index]
		receipt, err := m.backend.TransactionReceipt(lookupCtx, attempt.hash)
		if errors.Is(err, ethereum.NotFound) {
			readOK = true
			continue
		}
		if err != nil {
			if firstErr == nil {
				firstErr, failed = err, attempt
			}
			continue
		}
		readOK = true
		if err := validateReceipt(attempt.hash, receipt); err != nil {
			pending.log.Error(err, "invalid pending transaction receipt", "label", pending.req.Label, "hash", attempt.hash.Hex(), "nonce", pending.nonce)
			continue
		}
		m.receiptReadsRecovered(pending)
		return receipt, attempt
	}
	if firstErr != nil {
		m.receiptReadFailed(pending, failed, firstErr)
	} else if readOK {
		m.receiptReadsRecovered(pending)
	}
	return nil, txAttempt{}
}

func (m *Manager) receiptResult(ctx context.Context, pending *pendingTransaction) (Result, bool) {
	receipt, attempt := m.pendingReceipt(ctx, pending)
	if receipt == nil {
		return Result{}, false
	}
	if pending.nonceConflictHash != (common.Hash{}) && m.hasNonceConflict(pending.nonce) {
		if err := m.confirmCanonicalReceipt(ctx, receipt); err != nil {
			pending.log.Error(err, "owned receipt cannot reconcile nonce conflict", "label", pending.req.Label, "hash", attempt.hash.Hex(), "nonce", pending.nonce)
			return Result{}, false
		}
		m.clearNonceConflict(pending.nonce)
	}
	pending.lifecycle.transitionPhase(lifecyclePhaseConfirming)
	receipt, err := m.waitForConfirmations(ctx, pending.log, attempt.hash, receipt, m.confirmations(pending.req))
	if errors.Is(err, errReceiptReorged) {
		pending.lifecycle.transitionPhase(lifecyclePhasePending)
		if pending.nonceConflictHash != (common.Hash{}) {
			m.markNonceConflict(pending.nonce, pending.nonceConflictHash)
		}
		pending.log.Info("transaction inclusion reorged; resuming pending lifecycle", "label", pending.req.Label, "hash", attempt.hash.Hex(), "nonce", pending.nonce)
		return Result{}, false
	}
	result := Result{Hash: attempt.hash, Receipt: receipt, Outcome: OutcomeConfirmed, Err: err}
	switch {
	case receipt.Status == types.ReceiptStatusFailed:
		result.Outcome = OutcomeReverted
		result.Err = errors.Errorf("tx %s reverted on-chain", attempt.hash.Hex())
		if err != nil {
			result.Err = errors.Errorf("tx %s reverted on-chain; confirmation wait: %w", attempt.hash.Hex(), err)
		}
		pending.log.Error(result.Err, "transaction reverted", "label", pending.req.Label, "hash", attempt.hash.Hex(), "nonce", pending.nonce,
			"tenderly", tenderly.SimulatorURL(m.chainID, m.signer.Address(), pending.req.To, pending.req.Data, pending.req.Value))
	case attempt.cancellation:
		result.Outcome = OutcomeCancelled
		if err == nil {
			result.Err = errors.Errorf("send %q: pending transaction cancelled at nonce %d", pending.req.Label, pending.nonce)
		}
	case err != nil:
		result.Outcome = OutcomeIncludedUnconfirmed
	default:
		pending.log.V(1).Info("transaction confirmed", "label", pending.req.Label, "hash", attempt.hash.Hex(), "nonce", pending.nonce,
			"blockNumber", optionalBigString(receipt.BlockNumber), "gasUsed", receipt.GasUsed,
			"effectiveGasPrice", optionalBigString(receipt.EffectiveGasPrice), "confirmations", m.confirmations(pending.req))
	}
	return result, true
}

// tryReplace reports whether cancellation mode was entered, even if submission fails.
func (m *Manager) tryReplace(ctx context.Context, pending *pendingTransaction, cancellation bool) bool {
	if m.hasNonceConflict(pending.nonce) {
		return cancellation
	}
	cancellation = cancellation || pending.cancellationDue(time.Now())
	if !cancellation && m.rebroadcastUncertainAttempt(ctx, pending) {
		return false
	}
	var fees feeQuote
	for {
		limit := m.normalFeeLimit(pending.req)
		if cancellation {
			limit = m.globalFeeLimit()
		}
		var err error
		fees, err = m.nextReplacementFees(ctx, pending.fees, limit)
		// Pricing can cross the fill deadline. Reprice the cancellation once under
		// its own ceiling; never recursively re-enter the replacement lifecycle.
		if !cancellation && pending.cancellationDue(time.Now()) {
			cancellation = true
			continue
		}
		if err != nil {
			if !errors.Is(err, errReplacementLimitReached) || !m.rebroadcastLatestAttempt(ctx, pending, cancellation) {
				pending.log.Error(err, "cannot replace pending transaction", "label", pending.req.Label, "nonce", pending.nonce, "cancellation", cancellation)
			}
			return cancellation
		}
		break
	}
	to, data, value, gas := pending.req.To, pending.req.Data, pending.value, pending.gas
	if cancellation {
		to, data, value, gas = m.signer.Address(), nil, new(big.Int), cancellationGasLimit
	}
	sendCtx, cancelSend := replacementBroadcastContext(ctx, pending, cancellation)
	signed, sendErr := m.signAndSend(sendCtx, pending.nonce, to, data, value, gas, fees, true)
	cancelSend()
	log := pending.log.WithValues("label", pending.req.Label, "nonce", pending.nonce, "cancellation", cancellation)
	if signed == nil {
		log.Error(sendErr, "pending transaction replacement rejected")
		return cancellation
	}
	hash := signed.Hash()
	log = log.WithValues("hash", hash.Hex())
	known := isKnownTransactionError(sendErr)
	uncertain := sendErr != nil && !known
	pending.fees = cloneFeeQuote(fees)
	pending.attempts = append(pending.attempts, txAttempt{
		hash: hash, tx: signed, cancellation: cancellation, exactRebroadcastPending: uncertain,
	})
	if isNonceConsumedError(sendErr) {
		pending.nonceConflictHash = hash
		m.reconcileExistingLifecycleNonce(ctx, pending)
	}
	switch {
	case uncertain:
		log.Error(sendErr, "replacement broadcast uncertain; tracking signed hash")
	case known:
		log.Info("replacement already known by write RPC", "rpcResult", sendErr.Error())
	default:
		kind := replacementKindReplacement
		if cancellation {
			kind = replacementKindCancellation
		}
		m.metrics.replacement(pending.req.Label, kind)
		log.Info("pending transaction replaced", "maxFeePerGas", fees.maxFee.String(), "maxPriorityFeePerGas", fees.tip.String())
	}
	return cancellation
}

// rebroadcastUncertainAttempt gives a transport-ambiguous normal submission one exact-byte retry
// before escalating its fees. It never appends a duplicate attempt or changes the cached fee state.
func (m *Manager) rebroadcastUncertainAttempt(ctx context.Context, pending *pendingTransaction) bool {
	now := time.Now()
	if len(pending.attempts) == 0 || pending.cancellationDue(now) || !m.hasExactRebroadcastSlack(pending, now) {
		return false
	}
	latest := &pending.attempts[len(pending.attempts)-1]
	if latest.tx == nil || latest.cancellation || !latest.exactRebroadcastPending {
		return false
	}
	latest.exactRebroadcastPending = false
	m.rebroadcast(ctx, pending, *latest, true)
	return true
}

func (m *Manager) hasExactRebroadcastSlack(pending *pendingTransaction, now time.Time) bool {
	return pending.cancelDeadline.IsZero() || now.Add(m.cfg.BroadcastTimeout+m.cfg.ReplacementInterval).Before(pending.cancelDeadline)
}

func (m *Manager) rebroadcastLatestAttempt(ctx context.Context, pending *pendingTransaction, cancellation bool) bool {
	for index := len(pending.attempts); index > 0; index-- {
		attempt := pending.attempts[index-1]
		if attempt.tx != nil && attempt.cancellation == cancellation {
			m.rebroadcast(ctx, pending, attempt, false)
			return true
		}
	}
	return false
}

// Both retries resend stored bytes. They never advance the attempt list or its
// fee baseline. Nonce reconciliation belongs here for every exact rebroadcast.
func (m *Manager) rebroadcast(ctx context.Context, pending *pendingTransaction, attempt txAttempt, uncertain bool) {
	ctx, cancel := replacementBroadcastContext(ctx, pending, attempt.cancellation)
	defer cancel()
	err := m.sendSigned(ctx, attempt.tx, true)
	if isNonceConsumedError(err) {
		pending.nonceConflictHash = attempt.hash
		m.reconcileExistingLifecycleNonce(ctx, pending)
	}
	log := pending.log.WithValues("label", pending.req.Label, "hash", attempt.hash.Hex(), "nonce", pending.nonce)
	if !uncertain {
		log = log.WithValues("cancellation", attempt.cancellation)
		if err != nil {
			log.Error(err, "capped transaction rebroadcast failed")
		} else {
			log.Info("capped transaction rebroadcast")
		}
		return
	}
	log = log.WithValues("reason", "ambiguous-broadcast")
	switch {
	case err == nil:
		log.Info("uncertain transaction rebroadcast")
	case isKnownTransactionError(err):
		log.Info("uncertain transaction already known by write RPC", "rpcResult", err.Error())
	default:
		log.Error(err, "uncertain transaction exact rebroadcast failed; replacement deferred")
	}
}

// Normal replacement broadcasts must not outlive the request's cancellation deadline. Cancellation
// transactions use the lifecycle context because the request deadline has already elapsed for them.
func replacementBroadcastContext(
	ctx context.Context,
	pending *pendingTransaction,
	cancellation bool,
) (context.Context, context.CancelFunc) {
	if cancellation || pending.cancelDeadline.IsZero() {
		return ctx, func() {}
	}
	return context.WithDeadline(ctx, pending.cancelDeadline)
}

func (m *Manager) nextReplacementFees(ctx context.Context, previous feeQuote, limit *big.Int) (feeQuote, error) {
	fresh, err := m.currentFees(ctx, nil)
	if err == nil {
		return replacementFees(previous, &fresh, limit)
	}
	if !errors.Is(err, errFreshFeesUnavailable) {
		return feeQuote{}, err
	}
	m.log.V(1).Info("fresh replacement fees unavailable; using cached bump", "error", err)
	return replacementFees(previous, nil, limit)
}

// The last signed fees impose replacement floors even when fresh reads fail.
// A configured cap can clip fresh recommendations, never those protocol floors.
func replacementFees(previous feeQuote, fresh *feeQuote, limit *big.Int) (feeQuote, error) {
	floor := feeQuote{tip: bumpFee(previous.tip), maxFee: bumpFee(previous.maxFee)}
	next := feeQuote{baseFee: new(big.Int).Set(previous.baseFee), tip: new(big.Int).Set(floor.tip), maxFee: new(big.Int).Set(floor.maxFee)}
	if fresh != nil {
		next.baseFee.Set(fresh.baseFee)
		if fresh.maxFee.Cmp(next.maxFee) > 0 {
			next.maxFee.Set(fresh.maxFee)
		}
	}
	if limit != nil && next.maxFee.Cmp(limit) > 0 {
		next.maxFee.Set(limit)
	}
	headroom := new(big.Int).Sub(next.maxFee, next.baseFee)
	if headroom.Sign() < 0 {
		return feeQuote{}, errors.Errorf("replacement base fee %s exceeds fee limit %s", next.baseFee, next.maxFee)
	}
	if fresh != nil {
		recommended := new(big.Int).Set(fresh.tip)
		if recommended.Cmp(headroom) > 0 {
			recommended.Set(headroom)
		}
		if recommended.Cmp(next.tip) > 0 {
			next.tip.Set(recommended)
		}
	}
	if next.maxFee.Cmp(floor.maxFee) < 0 || next.tip.Cmp(next.maxFee) > 0 {
		return feeQuote{}, errors.Errorf("%w: previous max fee %s tip %s, limit %s", errReplacementLimitReached, previous.maxFee, previous.tip, feeLimitString(limit))
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
		pending.cancelRequested = make(chan struct{})
	}
	if !m.active.CompareAndSwap(nil, pending) {
		panic("txmanager: multiple signed lifecycles")
	}
}

func (m *Manager) removeUnminedTransaction(pending *pendingTransaction) {
	m.active.CompareAndSwap(pending, nil)
}

func (m *Manager) requestActiveCancellation() {
	if active := m.active.Load(); active != nil {
		requestCancellation(active)
	}
}

func requestCancellation(pending *pendingTransaction) {
	pending.cancelOnce.Do(func() { close(pending.cancelRequested) })
}

func (pending *pendingTransaction) cancellationDue(now time.Time) bool {
	if !pending.cancelDeadline.IsZero() && !now.Before(pending.cancelDeadline) {
		return true
	}
	select {
	case <-pending.cancelRequested:
		return true
	default:
		return false
	}
}

// currentFees computes the current EIP-1559 base fee, tip, and fee cap under the supplied lifecycle
// limit. A nil limit is unbounded.
func (m *Manager) currentFees(ctx context.Context, limit *big.Int) (feeQuote, error) {
	readCtx, cancel := context.WithTimeout(ctx, m.feeReadTimeout())
	defer cancel()
	head, err := m.backend.HeaderByNumber(readCtx, nil)
	if err != nil {
		return feeQuote{}, errors.Errorf("%w: header by number: %w", errFreshFeesUnavailable, err)
	}
	if head == nil || head.BaseFee == nil || head.BaseFee.Sign() < 0 {
		return feeQuote{}, errors.Errorf("%w: latest header must contain a non-negative base fee", errFreshFeesUnavailable)
	}
	floor := gweiToWei(m.cfg.TipGwei)
	tip, err := m.priorityFee(readCtx, floor)
	if err != nil {
		return feeQuote{}, err
	}
	// A configured floor remains usable when only the tip endpoint fails, but not when
	// the caller itself cancels this quote. This also preserves replacement fallback semantics.
	if err := ctx.Err(); err != nil {
		return feeQuote{}, errors.Errorf("%w: fee quote: %w", errFreshFeesUnavailable, err)
	}
	return priceFees(head.BaseFee, tip, floor, limit)
}

func (m *Manager) priorityFee(ctx context.Context, floor *big.Int) (*big.Int, error) {
	if floor.Sign() > 0 {
		tip, err := m.backend.SuggestGasTipCap(ctx)
		if err == nil && tip != nil && tip.Cmp(floor) > 0 {
			floor = tip
		}
		return bigmath.Clone(floor), nil
	}
	history, err := m.backend.FeeHistory(ctx, feeHistoryBlocks, nil, []float64{feeHistoryPercentile})
	if err != nil {
		return nil, errors.Errorf("%w: fee history: %w", errFreshFeesUnavailable, err)
	}
	tip, valid := feeHistoryTip(history)
	if !valid {
		return nil, errors.Errorf("%w: invalid fee history rewards", errFreshFeesUnavailable)
	}
	return tip, nil
}

// priceFees reserves room for one base-fee doubling and enforces a hard fee cap.
// A configured priority floor is a requirement; a dynamically observed tip may be clipped.
func priceFees(base, suggested, floor, limit *big.Int) (feeQuote, error) {
	fees := feeQuote{baseFee: new(big.Int).Set(base), tip: new(big.Int).Set(suggested)}
	fees.maxFee = new(big.Int).Add(new(big.Int).Lsh(base, 1), fees.tip)
	if limit != nil && fees.maxFee.Cmp(limit) > 0 {
		fees.maxFee.Set(limit)
	}
	headroom := new(big.Int).Sub(fees.maxFee, base)
	if headroom.Sign() < 0 {
		return feeQuote{}, errors.Errorf("fee limit reached: current base fee %s exceeds tx manager max fee %s", base, fees.maxFee)
	}
	if floor.Cmp(headroom) > 0 {
		return feeQuote{}, errors.Errorf("fee limit reached: fee limit %s cannot cover base fee %s plus priority fee floor %s", fees.maxFee, base, floor)
	}
	if fees.tip.Cmp(headroom) > 0 {
		fees.tip.Set(headroom)
	}
	return fees, nil
}

func feeHistoryTip(history *ethereum.FeeHistory) (*big.Int, bool) {
	if history == nil || len(history.Reward) != feeHistoryBlocks {
		return nil, false
	}
	rewards := make([]*big.Int, len(history.Reward))
	for index, row := range history.Reward {
		if len(row) != 1 || row[0] == nil || row[0].Sign() < 0 {
			return nil, false
		}
		rewards[index] = row[0]
	}
	return new(big.Int).Set(slices.MinFunc(rewards, (*big.Int).Cmp)), true
}

func (m *Manager) estimateGas(ctx context.Context, req Request) (uint64, error) {
	call := ethereum.CallMsg{From: m.signer.Address(), To: &req.To, Value: req.Value, Data: req.Data}
	estimate, err := m.backend.EstimateGas(ctx, call)
	if err != nil {
		// Replay the exact call to diagnose reverts without exposing signing material.
		m.requestLog(req).Error(err, "gas estimation failed", "label", req.Label,
			"tenderly", tenderly.SimulatorURL(m.chainID, call.From, req.To, call.Data, call.Value))
		return 0, errors.Errorf("estimate gas %q: %w", req.Label, err)
	}
	headroom := estimate / 20
	if estimate == 0 || estimate > ^uint64(0)-headroom {
		return 0, errors.Errorf("estimate gas %q: invalid estimate %d", req.Label, estimate)
	}
	return estimate + headroom, nil
}

func optionalBigString(value *big.Int) string {
	if value == nil {
		return "0"
	}
	return value.String()
}

func (m *Manager) signAndSend(ctx context.Context, nonce uint64, to common.Address, data []byte,
	value *big.Int, gas uint64, fees feeQuote, existingLifecycle bool) (*types.Transaction, error) {
	unsigned := types.NewTx(&types.DynamicFeeTx{ChainID: m.chainID, Nonce: nonce, To: &to,
		Data: data, Value: value, Gas: gas, GasTipCap: fees.tip, GasFeeCap: fees.maxFee})
	signed, err := m.signer.SignTx(ctx, unsigned, m.chainID)
	if err == nil {
		err = ctx.Err()
	}
	if err == nil && signed == nil {
		err = errors.New("signer returned no transaction")
	}
	if err != nil {
		return nil, errors.Errorf("sign transaction: %w", err)
	}
	err = m.sendSigned(ctx, signed, existingLifecycle)
	rejected := isDefiniteBroadcastRejection(err) || errors.Is(err, errNonceLanePaused)
	if !existingLifecycle {
		rejected = rejected || isPendingNonceCollision(err)
	}
	if rejected {
		return nil, errors.Errorf("broadcast rejected before acceptance: %w", err)
	}
	// An uncertain send retains the signed hash. It must never allocate another nonce.
	return signed, err
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
	sendCtx, cancel := context.WithTimeout(ctx, m.cfg.BroadcastTimeout)
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
	ctx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	for _, attempt := range pending.attempts {
		receipt, err := m.backend.TransactionReceipt(ctx, attempt.hash)
		if errors.Is(err, ethereum.NotFound) {
			continue
		}
		message := "tracked receipt unavailable during nonce reconciliation"
		if err == nil {
			message = "invalid tracked receipt during nonce reconciliation"
			err = validateReceipt(attempt.hash, receipt)
		}
		if err == nil {
			message = "tracked receipt is not canonically visible during nonce reconciliation"
			err = m.confirmCanonicalReceipt(ctx, receipt)
		}
		if err == nil {
			return true
		}
		pending.log.Error(err, message, "label", pending.req.Label, "hash", attempt.hash.Hex(), "nonce", pending.nonce)
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

func isKnownTransactionError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "already known")
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
		m.notifyLaneStateChange()
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
		m.notifyLaneStateChange()
	}
}

func (m *Manager) notifyLaneStateChange() {
	m.laneStateMu.Lock()
	defer m.laneStateMu.Unlock()
	for changes := range m.laneStateSubscribers {
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
	mined, pending, err := m.accountNonces(ctx)
	if err != nil {
		return err
	}
	if mined != pending {
		return errors.Errorf("unmanaged pending nonce gap: latest mined nonce %d, pending nonce %d", mined, pending)
	}
	m.nonce, m.nonceInit = mined, true
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
	log logr.Logger,
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

	var headReads, receiptReads, ancestryReads readStreak
	missing := 0
	for {
		headBefore, headErr := m.confirmationHead(ctx)
		if headErr != nil {
			headReads.failed(log, headErr, "confirmation head unavailable", "hash", hash.Hex())
		} else {
			headReads.recovered(log, "confirmation head reads recovered", "hash", hash.Hex())
		}
		refreshed, err := m.confirmationReceipt(ctx, hash)
		switch {
		case errors.Is(err, errReceiptReorged):
			// One null can be a lagging upstream rather than a reorg; require two in a row.
			missing++
			if missing >= 2 {
				return receipt, err
			}
			log.Info("confirmed receipt missing; rechecking before treating it as a reorg", "hash", hash.Hex())
		case err != nil:
			// A failed read says nothing about the receipt; only consecutive nulls count.
			missing = 0
			receiptReads.failed(log, err, "receipt confirmation check unavailable", "hash", hash.Hex())
		default:
			missing = 0
			receiptReads.recovered(log, "receipt confirmation reads recovered", "hash", hash.Hex())
			receipt = refreshed
			if headErr == nil {
				confirmed, proofErr := m.confirmedAtHead(ctx, headBefore, receipt, confirmations)
				if errors.Is(proofErr, errReceiptReorged) {
					return receipt, proofErr
				}
				if proofErr != nil {
					ancestryReads.failed(log, proofErr, "receipt ancestry check unavailable", "hash", hash.Hex())
				} else {
					ancestryReads.recovered(log, "receipt ancestry reads recovered", "hash", hash.Hex())
					if confirmed {
						return receipt, nil
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

// confirmedAtHead proves depth and ancestry against one head, then verifies the
// head did not move while its ancestry was read.
func (m *Manager) confirmedAtHead(ctx context.Context, head *types.Header, receipt *types.Receipt, confirmations uint64) (bool, error) {
	height, included := head.Number.Uint64(), receipt.BlockNumber.Uint64()
	if height < included || height-included < confirmations {
		return false, nil
	}
	if err := m.confirmReceiptAncestry(ctx, head, receipt); err != nil {
		return false, err
	}
	after, err := m.confirmationHead(ctx)
	if err != nil {
		return false, err
	}
	return head.Hash() == after.Hash(), nil
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

func (m *Manager) confirmReceiptAncestry(ctx context.Context, head *types.Header, receipt *types.Receipt) error {
	if head == nil || head.Number == nil || receipt == nil || receipt.BlockNumber == nil {
		return errors.New("confirmation ancestry requires head and receipt block numbers")
	}
	if !head.Number.IsUint64() || !receipt.BlockNumber.IsUint64() {
		return errors.New("confirmation ancestry block number exceeds uint64")
	}
	for head.Number.Cmp(receipt.BlockNumber) > 0 {
		parent, err := m.confirmationParent(ctx, head)
		if err != nil {
			return err
		}
		head = parent
	}
	if head.Number.Cmp(receipt.BlockNumber) == 0 && head.Hash() == receipt.BlockHash {
		return nil
	}
	return errors.Errorf("%w: receipt block %s is no longer canonical", errReceiptReorged, receipt.BlockHash.Hex())
}

func (m *Manager) confirmationParent(ctx context.Context, child *types.Header) (*types.Header, error) {
	readCtx, cancel := context.WithTimeout(ctx, m.receiptReadTimeout())
	defer cancel()
	parent, err := m.backend.HeaderByHash(readCtx, child.ParentHash)
	if err != nil {
		return nil, errors.Errorf("parent header %s: %w", child.ParentHash.Hex(), err)
	}
	if parent == nil || parent.Number == nil || !parent.Number.IsUint64() {
		return nil, errors.Errorf("parent header %s is invalid", child.ParentHash.Hex())
	}
	expected := new(big.Int).Sub(child.Number, big.NewInt(1))
	if parent.Hash() != child.ParentHash || parent.Number.Cmp(expected) != 0 {
		return nil, errors.Errorf("parent header %s does not link to block %s", parent.Hash(), child.Hash())
	}
	return parent, nil
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
	req.Value = bigmath.Clone(req.Value)
	req.MaxFeePerGas = bigmath.Clone(req.MaxFeePerGas)
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

func minPositiveDuration(fallback, candidate time.Duration) time.Duration {
	if candidate > 0 && candidate < fallback {
		return candidate
	}
	return fallback
}
