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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/signer"
)

// Backend is the EVM client surface the manager needs. NonceAt must read the sending endpoint
// when selecting the first unconsumed nonce; pending state is used only by account telemetry.
type Backend interface {
	NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	FeeHistory(
		ctx context.Context,
		blockCount uint64,
		lastBlock *big.Int,
		rewardPercentiles []float64,
	) (*ethereum.FeeHistory, error)
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

// accountTelemetryBackend serves the periodic account snapshot from the read endpoints, so telemetry
// never spends a submission relay's request budget. Without it the manager reads through the same
// write-endpoint methods admission uses.
type accountTelemetryBackend interface {
	ReadBalanceAt(ctx context.Context, account common.Address) (*big.Int, error)
	ReadNonces(ctx context.Context, account common.Address) (latestNonce, pendingNonce uint64, err error)
}

// accountReading is one complete telemetry snapshot.
type accountReading struct {
	balance                   *big.Int
	latestNonce, pendingNonce uint64
}

// Config tunes fee selection and confirmation behavior.
type Config struct {
	Confirmations        uint64        // blocks to wait past inclusion before returning
	MaxFeeGwei           float64       // absolute max fee per gas; app config requires a positive value
	PollInterval         time.Duration // receipt/confirmation poll cadence; 0 => 2s
	BroadcastTimeout     time.Duration // maximum duration of one transaction submission RPC; 0 => 5s
	AccountPollInterval  time.Duration // signer balance/nonce metric refresh cadence; 0 => 30s
	ReplacementInterval  time.Duration // fallback fee-bump cadence while fee windows are unreadable; 0 => 30s
	PendingTimeout       time.Duration // abandon unresolved calls and reuse their nonce for fresh requests; 0 => 5m
	ShutdownTimeout      time.Duration // maximum graceful drain after manager cancellation; 0 => 1m
	LateReceiptTimeout   time.Duration // passive receipt observation after uncertain release; 0 => 10m
	LateReceiptMaxHashes int           // bound on passive hashes and receipt deduplication; 0 => 1024
	Horizon              HorizonConfig // fee horizon, tip and gas-estimate tuning; zero values select defaults
}

// Request is a transaction to send. Value nil means 0. Stateful solver calls leave GasLimit at 0 so
// gas estimation re-simulates their exact calldata after lifecycle admission and immediately before signing.
type Request struct {
	To           common.Address
	Data         []byte
	Value        *big.Int
	GasLimit     uint64
	MaxFeePerGas *big.Int  // optional EIP-1559 fee ceiling
	Deadline     time.Time // optional latest time to submit this call; later work may reuse its nonce
	// Obsolete optionally reports that the call can no longer succeed. It must honor ctx and have no
	// authorization role: errors preserve the current lifecycle. True before signing drops the call;
	// true after broadcast abandons tracking without proving whether it executed.
	Obsolete      func(ctx context.Context) (bool, error)
	Confirmations *uint64 // optional wait override; nil uses Config.Confirmations
	Label         string  // stable operation name for logs and metrics
	Solver        string  // owning solver, so a shared manager's logs and Sentry events attribute to it
	// ObserveReceipt is telemetry-only: it observes an owned mined receipt once per locally retained
	// hash, including receipts found after an uncertain result. It must not perform I/O or mutate
	// business state; the manager may invoke it after the request's caller has resumed.
	ObserveReceipt func(context.Context, Result)
}

// Outcome is the terminal transaction state observed by the manager.
type Outcome string

const (
	OutcomeConfirmed           Outcome = "confirmed"
	OutcomeIncludedUnconfirmed Outcome = "included_unconfirmed"
	OutcomeReverted            Outcome = "reverted"
	OutcomeAbandoned           Outcome = "abandoned"
	OutcomeSubmissionError     Outcome = "submission_error"
	OutcomeTrackingStopped     Outcome = "tracking_stopped"
	OutcomeNonceConsumed       Outcome = "nonce_consumed"
	OutcomeNonceConflict       Outcome = "nonce_conflict"
)

// Included reports whether the request reached the chain, even if confirmation tracking stopped.
func (o Outcome) Included() bool {
	return o == OutcomeConfirmed || o == OutcomeIncludedUnconfirmed
}

// NonceUncertain reports an execution-unknown nonce race or abandoned request. The owning solver
// must read current protocol state before preparing another transaction for the order.
func (o Outcome) NonceUncertain() bool {
	return o == OutcomeNonceConflict || o == OutcomeNonceConsumed || o == OutcomeAbandoned
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
	broadcastErr      error // initial nonce race; returned immediately without owning the competing transaction
	attempts          []txAttempt
	receiptCursor     int
	originalHash      common.Hash
	receiptReads      readStreak
	nonceReads        readStreak
	obsolescenceReads readStreak
	result            chan<- Result
	resultOnce        sync.Once
	// span is the caller's send span, so the shutdown drain can end it with the result it hands
	// the caller rather than leaving that to a complete that may conclude differently.
	span     trace.Span
	deadline time.Time
	horizon  horizonProgress
}

type txAttempt struct {
	hash                    common.Hash
	tx                      *types.Transaction
	exactRebroadcastPending bool
}

// Manager serializes signed lifecycles and owns accepted work through its terminal result.
type Manager struct {
	backend Backend
	signer  signer.Signer
	chainID *big.Int
	cfg     Config
	horizon horizonPolicy
	metrics *Metrics
	log     logr.Logger

	// lastGas is the gas limit of the latest signed call, which horizon pricing uses to judge whether
	// recent blocks had room for a fill. overrideFallbackLogged limits the next-block estimate
	// fallback notice to once per process.
	lastGas                atomic.Uint64
	overrideFallbackLogged atomic.Bool

	queue           chan job
	lifecycleSlot   chan struct{}
	stopping        chan struct{}
	admissionDemand atomic.Int64

	laneStateMu          sync.Mutex
	laneStateSubscribers map[uint64]chan struct{}
	nextLaneStateID      uint64

	// mu guards initialization and the reusable nonce hint. The worker selects and updates the hint;
	// the one lifecycle owner remembers abandonment before releasing the serialized lane.
	mu          sync.Mutex
	initialized bool
	reusable    *reusableNonce

	unminedMu   sync.Mutex
	unmined     *pendingTransaction
	lifecycleWG sync.WaitGroup

	// receiptMu guards passive hash observations and the bounded accounted-hash ledger. The
	// lifecycle owners register work or account ordinary receipts; one passive observer does only
	// reads. No passive operation can acquire or alter the nonce lane or its fee hint.
	receiptMu          sync.Mutex
	lateReceiptQueue   []lateReceiptObservation
	lateReceiptCursor  int
	accountedReceipts  map[common.Hash]time.Time
	lateReceiptWake    chan struct{}
	lateReceiptStopped bool
	lateReceiptHeaders map[common.Hash]lateReceiptHeader
}

type job struct {
	req              Request
	res              chan Result
	admissionStarted time.Time
	span             trace.Span // the caller-derived send span, ended by whoever resolves the request
}

const (
	defaultPollInterval         = 2 * time.Second
	defaultAccountPollInterval  = 30 * time.Second
	defaultReplacementInterval  = 30 * time.Second
	defaultPendingTimeout       = 5 * time.Minute
	defaultShutdownTimeout      = time.Minute
	defaultBroadcastTimeout     = 5 * time.Second
	defaultLateReceiptTimeout   = 10 * time.Minute
	defaultLateReceiptMaxHashes = 1024
	maxFeeReadTimeout           = time.Second
	maxReceiptReadTimeout       = 2 * time.Second
	maxGasEstimateTimeout       = 5 * time.Second
	accountRefreshTimeout       = 5 * time.Second
	replacementBumpNumerator    = 9
	replacementBumpDenominator  = 8
)

// ErrRequestObsolete marks a request whose Obsolete hook reported that it can no longer succeed. A
// result wraps it when the request was dropped before signing or abandoned after broadcast.
// Abandonment is still execution-unknown, so solvers must reconcile current protocol state.
var ErrRequestObsolete = errors.New("transaction request is obsolete")

// ErrAbandoned means tracking stopped so fresh business work can reuse an unused nonce. The old
// signed call can still execute; this never proves that it failed or was removed from a relay.
var ErrAbandoned = errors.New("transaction tracking abandoned")

// ErrNonceConsumed means the sending endpoint reports a mined nonce above our signed nonce,
// but no owned receipt is available. This is an uncertain execution result, not a successful fill.
var ErrNonceConsumed = errors.New("transaction nonce consumed without an owned receipt")

// ErrNonceConflict marks an initial nonce-too-low or underpriced replacement response. Another
// replica may have sent this order, so retry requires fresh protocol state rather than calldata replay.
var ErrNonceConflict = errors.New("transaction nonce raced with another sender")

var (
	errFreshFeesUnavailable    = errors.New("fresh fees unavailable")
	errReplacementLimitReached = errors.New("replacement fee limit reached")
	errReceiptReorged          = errors.New("transaction receipt reorged")
	errManagerStopped          = errors.New("transaction manager stopped")
	errShutdownTimeout         = errors.Errorf("transaction manager shutdown drain timed out: %w", context.DeadlineExceeded)
)

// New constructs a Manager. Call Start to launch its worker.
func New(backend Backend, s signer.Signer, chainID *big.Int, cfg Config, log logr.Logger) *Manager {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	if cfg.BroadcastTimeout <= 0 {
		cfg.BroadcastTimeout = defaultBroadcastTimeout
	}
	if cfg.AccountPollInterval <= 0 {
		cfg.AccountPollInterval = defaultAccountPollInterval
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
	if cfg.LateReceiptTimeout <= 0 {
		cfg.LateReceiptTimeout = defaultLateReceiptTimeout
	}
	if cfg.LateReceiptMaxHashes <= 0 {
		cfg.LateReceiptMaxHashes = defaultLateReceiptMaxHashes
	}
	cfg.Horizon = cfg.Horizon.withDefaults()
	return &Manager{
		backend:              backend,
		signer:               s,
		chainID:              chainID,
		cfg:                  cfg,
		horizon:              newHorizonPolicy(cfg.Horizon),
		log:                  log.WithName("txmanager"),
		queue:                make(chan job),
		lifecycleSlot:        make(chan struct{}, 1),
		stopping:             make(chan struct{}),
		laneStateSubscribers: make(map[uint64]chan struct{}),
		accountedReceipts:    make(map[common.Hash]time.Time),
		lateReceiptWake:      make(chan struct{}, 1),
		lateReceiptHeaders:   make(map[common.Hash]lateReceiptHeader),
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

// ValidateFeeHeadroom rejects a configured congested tip cap, the highest tip the manager prices,
// that can never fit under the initial transaction cap after reserving one ordinary replacement.
func (m *Manager) ValidateFeeHeadroom() error {
	initialLimit := reserveFeeBump(m.normalFeeLimit(Request{}))
	if initialLimit != nil && m.horizon.congestedTipCap.Cmp(initialLimit) >= 0 {
		return errors.Errorf(
			"congested tip cap %s leaves no base-fee headroom under initial fee limit %s after reserved replacement bumps",
			m.horizon.congestedTipCap, initialLimit,
		)
	}
	return nil
}

// Available reports whether the sending endpoint has supplied an initial mined nonce. Pending
// transactions and nonce races do not pause admission. Use LaneReady to also check local work.
func (m *Manager) Available() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.initialized
}

// Idle reports whether no request owns or is waiting for the local signed-lifecycle lane.
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
// when they stop. Signals cover initialization and admission-demand edges. Independent
// subscriptions prevent readiness and solvers from stealing signals from each other.
func (m *Manager) SubscribeLaneState() (<-chan struct{}, func()) {
	m.laneStateMu.Lock()
	id := m.nextLaneStateID
	m.nextLaneStateID++
	changes := make(chan struct{}, 1)
	m.laneStateSubscribers[id] = changes
	m.laneStateMu.Unlock()

	var once sync.Once
	return changes, func() {
		once.Do(func() {
			m.laneStateMu.Lock()
			delete(m.laneStateSubscribers, id)
			m.laneStateMu.Unlock()
		})
	}
}

// Initialize verifies that the sending endpoint exposes mined nonce state. Every new send reads
// it again; pending transactions never move a fresh request past the first unused nonce.
func (m *Manager) Initialize(ctx context.Context) error {
	_, err := m.freshMinedNonce(ctx)
	return err
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
	_, snapshot := m.backend.(accountTelemetryBackend)
	_, senderBalance := m.backend.(transactionSenderBalanceBackend)
	_, ordinaryBalance := m.backend.(accountBalanceBackend)
	return snapshot || senderBalance || ordinaryBalance
}

// refreshAccount runs one account poll. It is periodic background work, and the manager's lifetime
// context carries no span, so each poll is its own trace.
func (m *Manager) refreshAccount(ctx context.Context) {
	pollCtx, end := tracer.Start(ctx, "txmanager.account_poll")
	err := m.readAccount(pollCtx)
	end(err)
	if err != nil && ctx.Err() == nil {
		m.metrics.observeAccountRefreshError()
		observability.Log(ctx).V(1).Info("account metrics refresh failed", "error", err)
	}
}

func (m *Manager) readAccount(ctx context.Context) error {
	refreshCtx, cancel := context.WithTimeout(ctx, accountRefreshTimeout)
	defer cancel()
	reading, err := m.readAccountTelemetry(refreshCtx)
	if err != nil {
		return err
	}
	if reading.balance == nil || reading.balance.Sign() < 0 {
		return errors.New("txmanager: invalid account balance")
	}
	m.metrics.observeAccount(reading.balance, reading.latestNonce, reading.pendingNonce)
	return nil
}

func (m *Manager) readAccountTelemetry(ctx context.Context) (accountReading, error) {
	var reading accountReading
	var err error
	if backend, ok := m.backend.(accountTelemetryBackend); ok {
		if reading.balance, err = backend.ReadBalanceAt(ctx, m.signer.Address()); err != nil {
			return accountReading{}, err
		}
		reading.latestNonce, reading.pendingNonce, err = backend.ReadNonces(ctx, m.signer.Address())
		return reading, err
	}
	if reading.balance, err = m.transactionSenderBalance(ctx); err != nil {
		return accountReading{}, err
	}
	if reading.latestNonce, err = m.backend.NonceAt(ctx, m.signer.Address(), nil); err != nil {
		return accountReading{}, err
	}
	reading.pendingNonce, err = m.backend.PendingNonceAt(ctx, m.signer.Address())
	return reading, err
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

// Start admits one signed lifecycle at a time. On shutdown, accepted work drains without changing
// its calldata. Once ShutdownTimeout elapses, its context is cancelled, its caller receives a
// terminal deadline result, and the worker returns without waiting on a stuck dependency.
func (m *Manager) Start(ctx context.Context) {
	// The worker's own context carries the manager logger; per-request contexts replace it with the
	// job's solver-stamped one below.
	ctx = observability.WithLogger(ctx, m.log)
	m.metrics.bindAccount(m.signer.Address())
	accountMonitorDone := make(chan struct{})
	go func() {
		defer close(accountMonitorDone)
		m.monitorAccount(ctx)
	}()
	defer func() { <-accountMonitorDone }()
	lateObserverDone := make(chan struct{})
	go func() {
		defer close(lateObserverDone)
		m.monitorLateReceipts(ctx)
	}()
	defer func() { <-lateObserverDone }()

	observability.Log(ctx).Info("started", "from", m.signer.Address().Hex())
	lifecycleCtx, cancelLifecycle := context.WithCancelCause(context.WithoutCancel(ctx))
	defer cancelLifecycle(errManagerStopped)
	stop := func(reason error) {
		close(m.stopping)
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
			observability.Log(ctx).Error(errShutdownTimeout, "transaction lifecycle drain deadline reached",
				"timeout", m.cfg.ShutdownTimeout.String(),
			)
			cancelLifecycle(errShutdownTimeout)
			m.deliverActiveShutdownTimeout()
			reason = errShutdownTimeout
		}
		observability.Log(ctx).Info("stopped", "reason", reason.Error())
	}
	for {
		select {
		case <-ctx.Done():
			stop(ctx.Err())
			return
		case j := <-m.queue:
			// The send span, not the caller's context, carries the trace across the detached lifecycle.
			spanCtx := m.jobContext(ctx, j)
			if err := ctx.Err(); err != nil {
				m.metrics.finishAdmission(j.req.Label, j.admissionStarted, errManagerStopped)
				deliverJobResult(j, notAdmittedResult(err))
				m.releaseLifecycleSlot()
				stop(err)
				return
			}

			m.metrics.finishAdmission(j.req.Label, j.admissionStarted, nil)
			lifecycle := m.metrics.beginLifecycle(j.req.Label)
			pending, err := m.broadcast(spanCtx, j.req)
			if err != nil {
				outcome := OutcomeSubmissionError
				if ctx.Err() != nil {
					outcome = OutcomeTrackingStopped
				}
				lifecycle.finish(outcome)
				deliverJobResult(j, Result{
					Outcome:     outcome,
					Err:         err,
					NotAdmitted: ctx.Err() != nil,
				})
				m.releaseLifecycleSlot()
				continue
			}
			if pending.broadcastErr != nil {
				result := Result{Hash: pending.originalHash, Outcome: OutcomeNonceConflict,
					Err: errors.Errorf("send %q at nonce %d: %w: %w", j.req.Label, pending.nonce, ErrNonceConflict, pending.broadcastErr)}
				lifecycle.finish(result.Outcome)
				deliverJobResult(j, result)
				m.releaseLifecycleSlot()
				continue
			}
			lifecycle.transitionPhase(lifecyclePhasePending)
			pending.lifecycle = lifecycle
			pending.result = j.res
			m.trackUnminedTransaction(pending)
			lifecycleSpanCtx := m.jobContext(lifecycleCtx, j)
			m.lifecycleWG.Go(func() {
				defer m.releaseLifecycleSlot()
				m.complete(lifecycleSpanCtx, pending)
			})
		}
	}
}

// Send enqueues a transaction and blocks until it is confirmed or fails. Safe for concurrent
// callers; admission and the initial broadcast are serialized through the worker.
//
// ctx and Deadline govern the pre-sign admission wait. Once enqueued, the worker broadcasts the tx on
// the manager's own long-lived context, so Send waits for and returns that real outcome — it must not
// report a cancellation while the transaction still lands on-chain, which a caller would read as
// "not sent". The worker owns fee replacement until a receipt or an explicitly execution-unknown
// abandonment result. Shutdown drains accepted work without sending a different transaction.
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
// one transaction and returns its eventual receipt result. ctx and Deadline can still stop this wait;
// a deadline or manager stop returns a terminal pre-admission error without signing. Once enqueued,
// the manager owns the broadcast and receipt lifecycle.
func (m *Manager) SendAsync(ctx context.Context, req Request) (<-chan Result, bool) {
	return m.sendAsync(ctx, req, false)
}

func (m *Manager) sendAsync(ctx context.Context, req Request, try bool) (<-chan Result, bool) {
	admissionStarted := time.Now()
	m.addAdmissionDemand()
	releaseDemandOnReturn := true
	defer func() {
		if releaseDemandOnReturn {
			m.releaseAdmissionDemand()
		}
	}()

	admissionCtx := ctx
	cancel := func() {}
	if !req.Deadline.IsZero() {
		admissionCtx, cancel = context.WithDeadline(ctx, req.Deadline)
	}
	defer cancel()
	if err := admissionCtx.Err(); err != nil {
		return m.admissionFailure(ctx, req, admissionStarted, err)
	}

	// From here on the span owns the request: every return either ends it or hands it to the worker.
	spanCtx, span := startSendSpan(ctx, req)
	failAdmission := func(err error) (<-chan Result, bool) {
		endSendSpan(span, Result{Outcome: OutcomeSubmissionError, Err: err})
		return m.admissionFailure(ctx, req, admissionStarted, err)
	}
	// A busy lane is an expected probe result, not a failure, so the span carries no error status.
	declineBusyLane := func() (<-chan Result, bool) {
		observability.Decline(spanCtx, "not_admitted", "lane_busy")
		span.End()
		return nil, false
	}

	select {
	case <-m.stopping:
		return failAdmission(errManagerStopped)
	default:
	}
	if try {
		select {
		case m.lifecycleSlot <- struct{}{}:
		default:
			return declineBusyLane()
		}
	} else {
		select {
		case m.lifecycleSlot <- struct{}{}:
		case <-admissionCtx.Done():
			return failAdmission(admissionCtx.Err())
		case <-m.stopping:
			return failAdmission(errManagerStopped)
		}
	}
	res := make(chan Result, 1)
	select {
	case m.queue <- job{
		req: cloneRequest(req), res: res, admissionStarted: admissionStarted,
		span: span,
	}:
		releaseDemandOnReturn = false
	case <-admissionCtx.Done():
		m.releaseLifecycleSlot()
		releaseDemandOnReturn = false
		return failAdmission(admissionCtx.Err())
	case <-m.stopping:
		m.releaseLifecycleSlot()
		releaseDemandOnReturn = false
		return failAdmission(errManagerStopped)
	}
	return res, true
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

// deliverJobResult ends the send span before the caller observes the result, so a caller resuming
// its own trace never races the span it is nested under.
func deliverJobResult(j job, result Result) {
	endSendSpan(j.span, result)
	j.res <- result
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
// configured limit permits it: one bump over the horizon fee cap for the size of the latest signed
// call. Send recomputes the initial fees immediately before signing.
func (m *Manager) MaxFeePerGas(ctx context.Context) (*big.Int, error) {
	limit := m.normalFeeLimit(Request{})
	snapshot, _, err := m.readFeeSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	fees, err := horizonFees(snapshot, m.lastGas.Load(), reserveFeeBump(limit), m.horizon)
	if err != nil {
		return nil, err
	}
	remembered, err := m.unusedReusableNonce(ctx)
	if err != nil {
		return nil, err
	}
	if remembered != nil {
		fees, err = replacementFloor(fees, remembered.fees, limit)
		if err != nil {
			return nil, err
		}
	}
	maxFee := bumpFee(fees.maxFee)
	if limit != nil && maxFee.Cmp(limit) > 0 {
		maxFee.Set(limit)
	}
	return maxFee, nil
}

// requestLog is the manager logger named after the solver the request serves. The worker stores it
// on every context of the request's lifecycle, so Log(ctx) reaches it from anywhere below.
func (m *Manager) requestLog(req Request) logr.Logger {
	if req.Solver == "" {
		return m.log
	}
	return m.log.WithValues("solver", req.Solver)
}

// jobContext derives base into the context the job's work runs on: the send span carries the
// caller's trace across the detached lifecycle, and the request logger is what Log(ctx) stamps.
func (m *Manager) jobContext(base context.Context, j job) context.Context {
	return observability.WithLogger(trace.ContextWithSpan(base, j.span), m.requestLog(j.req))
}

// broadcast runs on the worker goroutine only, after lifecycle admission, so fee selection, gas
// estimation, signing, and nonce assignment stay serialized.
func (m *Manager) broadcast(ctx context.Context, req Request) (pending *pendingTransaction, err error) {
	sendSpan := trace.SpanFromContext(ctx)

	broadcastCtx := ctx
	cancel := func() {}
	if !req.Deadline.IsZero() {
		broadcastCtx, cancel = context.WithDeadline(ctx, req.Deadline)
	}
	defer cancel()
	broadcastCtx, end := tracer.Start(broadcastCtx, "txmanager.broadcast")
	defer func() { end(err) }()
	if err := broadcastCtx.Err(); err != nil {
		return nil, errors.Errorf("send %q before broadcast: %w", req.Label, err)
	}

	if req.MaxFeePerGas != nil && req.MaxFeePerGas.Sign() <= 0 {
		return nil, errors.Errorf("send %q: request max fee per gas must be positive", req.Label)
	}
	quote, err := m.quoteCall(broadcastCtx, req, reserveFeeBump(m.normalFeeLimit(req)))
	if err != nil {
		return nil, err
	}
	fees, gas := quote.fees, quote.gas
	obsolete, obsoleteErr := m.requestObsolete(broadcastCtx, req)
	if obsoleteErr != nil {
		// Obsolescence is only a liveness optimization. The solver already validated the call,
		// and execution-time contracts remain authoritative, so an unknown check keeps it alive.
		observability.Log(ctx).Error(obsoleteErr, "transaction obsolescence check unavailable; continuing",
			"label", req.Label)
	} else if obsolete {
		return nil, errors.Errorf("send %q: %w", req.Label, ErrRequestObsolete)
	}

	value := req.Value
	if value == nil {
		value = new(big.Int)
	}
	observability.Log(ctx).V(1).Info(
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

	nonce, floor, err := m.selectNonce(broadcastCtx)
	if err != nil {
		return nil, err
	}
	if floor != nil {
		fees, err = replacementFloor(fees, *floor, m.normalFeeLimit(req))
		if err != nil {
			return nil, errors.Errorf("send %q: %w", req.Label, err)
		}
	}
	signed, sendErr := m.signAndSend(
		broadcastCtx, nonce, req.To, req.Data, value, gas, fees, false,
	)
	if signed == nil {
		return nil, errors.Errorf("send %q: %w", req.Label, sendErr)
	}
	if floor != nil || isPendingNonceCollision(sendErr) {
		m.rememberReusable(nonce, fees)
	}
	hash := signed.Hash()
	// Both spans: the broadcast span is short-lived, the send span keeps the identity for the whole
	// lifecycle (endSendSpan later overwrites tx.hash with the attempt that actually landed).
	txIdentity := []attribute.KeyValue{
		observability.AttrTxHash.String(hash.Hex()),
		observability.AttrTxNonce.Int64(int64(nonce)),
	}
	observability.SetAttributes(broadcastCtx, txIdentity...)
	sendSpan.SetAttributes(txIdentity...)
	broadcastUncertain := sendErr != nil && !isKnownTransactionError(sendErr)
	if broadcastUncertain {
		if isNonceConsumedError(sendErr) || isPendingNonceCollision(sendErr) {
			observability.Log(ctx).Info("transaction nonce race; releasing local lane",
				"label", req.Label, "hash", hash.Hex(), "nonce", nonce, "rpcResult", sendErr.Error())
		} else {
			observability.Log(ctx).Error(sendErr, "transaction broadcast uncertain; tracking signed hash",
				"label", req.Label, "hash", hash.Hex(), "nonce", nonce)
		}
	} else if sendErr != nil {
		observability.Log(ctx).Info("transaction already known by write RPC",
			"label", req.Label, "hash", hash.Hex(), "nonce", nonce, "rpcResult", sendErr.Error())
	} else {
		observability.Log(ctx).Info("sent", "label", req.Label, "hash", hash.Hex(), "nonce", nonce)
	}
	m.lastGas.Store(gas)
	pending = &pendingTransaction{
		req:   req,
		nonce: nonce,
		gas:   gas,
		value: new(big.Int).Set(value),
		fees:  cloneFeeQuote(fees),
		attempts: []txAttempt{{
			hash: hash, tx: signed, exactRebroadcastPending: broadcastUncertain,
		}},
		originalHash: hash,
		span:         sendSpan,
		horizon: horizonProgress{
			sentHead: quote.head, sentAt: time.Now(), lastHead: quote.head, lastEvaluation: time.Now(),
		},
	}
	if isNonceConsumedError(sendErr) || isPendingNonceCollision(sendErr) {
		pending.broadcastErr = sendErr
	}
	return pending, nil
}

func (m *Manager) complete(ctx context.Context, pending *pendingTransaction) {
	defer m.removeUnminedTransaction(pending)
	outcome := m.waitForPendingTransaction(ctx, pending)
	if outcome.Outcome.Included() || outcome.Outcome == OutcomeReverted || outcome.Outcome == OutcomeNonceConsumed {
		m.forgetReusable(pending.nonce)
	}
	pending.lifecycle.finish(outcome.Outcome)
	m.recordReceipt(ctx, m.receiptMetadata(pending.req, pending.nonce), outcome, false)
	if outcome.Outcome == OutcomeAbandoned || outcome.Outcome == OutcomeNonceConsumed {
		m.retainLateReceipts(ctx, pending)
	}
	if errors.Is(outcome.Err, errShutdownTimeout) {
		observability.Log(ctx).Error(outcome.Err, "accepted transaction lifecycle did not drain before shutdown",
			"label", pending.req.Label,
			"nonce", pending.nonce,
			"hashes", attemptHashStrings(pending.attempts),
		)
	}
	// End before delivering, so the caller never resumes its trace while the span it nests under is
	// still open. A pending built outside Start carries no span and gets the no-op one from ctx.
	endSendSpan(trace.SpanFromContext(ctx), outcome)
	pending.deliver(outcome)
}

// deliver hands the caller its one terminal result and reports whether this call was the one that
// delivered it. Every later result is dropped, so only the winner describes what the caller acted on.
func (pending *pendingTransaction) deliver(result Result) bool {
	if pending.result == nil {
		return false
	}
	delivered := false
	pending.resultOnce.Do(func() {
		pending.result <- result
		delivered = true
	})
	return delivered
}

func (m *Manager) confirmations(req Request) uint64 {
	if req.Confirmations != nil {
		return *req.Confirmations
	}
	return m.cfg.Confirmations
}

func (m *Manager) waitForPendingTransaction(ctx context.Context, pending *pendingTransaction) Result {
	reader := m.startReceiptReader(ctx)
	defer reader.stop()
	knownAttempts := len(pending.attempts)
	sweep := newReceiptSweep(pending, knownAttempts)
	var receiptResults <-chan receiptRead
	poll := time.NewTicker(m.cfg.PollInterval)
	defer poll.Stop()
	replace := time.NewTicker(m.replacementTick())
	defer replace.Stop()
	timeout := time.NewTimer(max(time.Until(pending.deadline), 0))
	defer timeout.Stop()

	var replacementStarted time.Time
	tryReplace := func(intent replaceIntent) bool {
		replacementStarted = time.Now()
		replaceCtx, end := tracer.Start(ctx, "txmanager.replace",
			observability.AttrTxAttempt.Int(len(pending.attempts)+1),
			attribute.String("tx.replace_reason", intent.reason),
		)
		expired, err := m.tryReplace(replaceCtx, pending, intent)
		end(err)
		return expired
	}
	abandon, timeoutC := false, timeout.C
	var finalReceiptTimer *time.Timer
	var finalReceiptC <-chan time.Time
	finalReceiptDeadline := pending.deadline.Add(m.receiptReadTimeout())
	defer func() {
		if finalReceiptTimer != nil {
			finalReceiptTimer.Stop()
		}
	}()
	requestAbandonment := func() {
		if !abandon {
			// The grace is measured from the original deadline, including time spent in a
			// synchronous fee read. Many historical hashes never extend the nonce lane.
			finalReceiptTimer = time.NewTimer(max(time.Until(finalReceiptDeadline), 0))
			finalReceiptC = finalReceiptTimer.C
			if sweep == nil {
				knownAttempts = len(pending.attempts)
				sweep = newReceiptSweep(pending, knownAttempts)
			}
		}
		abandon, timeoutC = true, nil
	}
	confirmRead := func(read receiptRead) (Result, bool) {
		return m.confirmPendingReceipt(ctx, pending, read.attempt, read.receipt)
	}
	for {
		// New variants arriving between sweeps also get an immediate priority read.
		if sweep == nil && knownAttempts != len(pending.attempts) {
			sweep = newReceiptSweep(pending, knownAttempts)
			knownAttempts = len(pending.attempts)
		}
		var reads chan<- txAttempt
		var next txAttempt
		var nextIndex int
		if sweep != nil && receiptResults == nil {
			nextIndex = sweep.nextIndex(pending)
			if nextIndex >= 0 {
				reads = reader.requests
				next = pending.attempts[nextIndex]
			}
		}
		select {
		case reads <- next:
			receiptResults = reader.results
			sweep.dispatched(pending, nextIndex)
		case read := <-receiptResults:
			receiptResults = nil
			if m.observeReceiptRead(ctx, pending, sweep, read) {
				result, done := confirmRead(read)
				if done {
					return result
				}
				// A reorg or an untrusted receipt resumes polling only within the final
				// receipt grace once the request deadline has elapsed.
				sweep = nil
				if pending.abandonmentDue(time.Now()) {
					requestAbandonment()
				}
			} else if sweep.nextIndex(pending) < 0 {
				m.finishReceiptSweep(ctx, pending, sweep)
				reconcileCtx := ctx
				cancelReconcile := func() {}
				if pending.abandonmentDue(time.Now()) {
					reconcileCtx, cancelReconcile = context.WithDeadline(ctx, finalReceiptDeadline)
				}
				if sweep.allAttemptsMissing(pending) {
					if result, consumed := m.confirmConsumedNonce(reconcileCtx, pending); consumed {
						cancelReconcile()
						return result
					}
				}
				// Include superseded variants considered by the priority path.
				knownAttempts = sweep.knownAttempts
				sweep = nil
				// A terminal protocol status may reflect our own transaction.
				// Give receipts precedence before checking obsolescence.
				obsolete := m.pendingRequestObsolete(reconcileCtx, pending)
				cancelReconcile()
				if obsolete {
					return m.abandonPending(ctx, pending, "obsolete", ErrRequestObsolete)
				}
				if abandon || pending.abandonmentDue(time.Now()) {
					return m.abandonPending(ctx, pending, pending.abandonmentReason(), nil)
				}
			}
		case <-ctx.Done():
			return Result{
				Hash:    pending.attempts[0].hash,
				Outcome: OutcomeTrackingStopped,
				Err:     context.Cause(ctx),
			}
		case <-poll.C:
			if sweep == nil {
				knownAttempts = len(pending.attempts)
				sweep = newReceiptSweep(pending, knownAttempts)
			}
		case tick := <-replace.C:
			// A request deadline may coincide with this tick. Do not send a
			// second replacement for a tick already covered by that broadcast.
			if !tick.After(replacementStarted) {
				continue
			}
			if abandon || pending.abandonmentDue(time.Now()) {
				requestAbandonment()
			} else if m.evaluateHorizon(ctx, pending, tryReplace) {
				requestAbandonment()
			}
		case <-timeoutC:
			requestAbandonment()
		case <-finalReceiptC:
			// Prefer a receipt already delivered by the reader at the grace boundary. A
			// canonical included candidate continues the existing confirmation policy.
			select {
			case read := <-receiptResults:
				if m.observeReceiptRead(ctx, pending, sweep, read) {
					if result, done := confirmRead(read); done {
						return result
					}
				}
			default:
			}
			return m.abandonPending(ctx, pending, pending.abandonmentReason(), nil)
		}
	}
}

func (m *Manager) pendingRequestObsolete(ctx context.Context, pending *pendingTransaction) bool {
	obsolete, err := m.requestObsolete(ctx, pending.req)
	if err != nil {
		pending.obsolescenceReads.failed(observability.Log(ctx), err,
			"pending transaction obsolescence check unavailable; retaining lifecycle",
			"label", pending.req.Label, "hash", pending.originalHash.Hex(), "nonce", pending.nonce)
		return false
	}
	pending.obsolescenceReads.recovered(observability.Log(ctx), "pending transaction obsolescence checks recovered",
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

func (m *Manager) receiptReadFailed(ctx context.Context, pending *pendingTransaction, sweep *receiptSweep) {
	read := sweep.firstError
	diagnostic := sweep.diagnostics
	pending.receiptReads.failed(observability.Log(ctx), read.err, "pending transaction receipt unavailable",
		"label", pending.req.Label,
		"hash", read.attempt.hash.Hex(),
		"originalHash", pending.originalHash.Hex(),
		"nonce", pending.nonce,
		"rpcTimeout", m.receiptReadTimeout().String(),
		"reason_code", read.reason(),
		"rpcBudgetTotalMs", diagnostic.budget.Milliseconds(),
		"sweepElapsedMs", time.Since(diagnostic.started).Milliseconds(),
		"hashesChecked", len(diagnostic.hashes),
		"hashesTotal", len(pending.attempts),
		"rpcChecks", diagnostic.reads,
		"lastRPCDurationMs", diagnostic.lastRPC.Milliseconds(),
		"cancelCause", read.cancelCause,
	)
}

func (m *Manager) receiptReadsRecovered(ctx context.Context, pending *pendingTransaction) {
	pending.receiptReads.recovered(observability.Log(ctx), "pending transaction receipt reads recovered",
		"label", pending.req.Label, "nonce", pending.nonce)
}

// confirmPendingReceipt runs only in the lifecycle owner, after receipt validation.
func (m *Manager) confirmPendingReceipt(ctx context.Context, pending *pendingTransaction, attempt txAttempt, receipt *types.Receipt) (Result, bool) {
	// A shaped receipt alone cannot reserve confirmation ownership: a displaced fork's
	// receipt can persist while a stalled head never reaches the requested depth. First
	// establish canonical inclusion under one overall read budget, also capped by the
	// final abandonment grace. Proven inclusion then retains the ordinary depth wait.
	canonicalDeadline := time.Now().Add(m.receiptReadTimeout())
	if !pending.deadline.IsZero() {
		finalDeadline := pending.deadline.Add(m.receiptReadTimeout())
		if finalDeadline.Before(canonicalDeadline) {
			canonicalDeadline = finalDeadline
		}
	}
	canonicalCtx, cancelCanonical := context.WithDeadline(ctx, canonicalDeadline)
	canonicalErr := canonicalCtx.Err()
	if canonicalErr == nil {
		canonicalErr = m.confirmCanonicalReceipt(canonicalCtx, receipt)
	}
	cancelCanonical()
	if canonicalErr != nil {
		observability.Log(ctx).Error(canonicalErr, "owned receipt is not canonical",
			"label", pending.req.Label, "hash", attempt.hash.Hex(), "nonce", pending.nonce)
		return Result{}, false
	}
	pending.lifecycle.transitionPhase(lifecyclePhaseConfirming)
	confirmations := m.confirmations(pending.req)
	receipt, err := m.waitForConfirmations(ctx, attempt.hash, receipt, confirmations)
	if errors.Is(err, errReceiptReorged) {
		pending.lifecycle.transitionPhase(lifecyclePhasePending)
		observability.Log(ctx).Info("transaction inclusion reorged; resuming pending lifecycle",
			"label", pending.req.Label,
			"hash", attempt.hash.Hex(),
			"nonce", pending.nonce,
		)
		return Result{}, false
	}
	if receipt.Status == types.ReceiptStatusFailed {
		revertErr := errors.Errorf("tx %s reverted on-chain", attempt.hash.Hex())
		if err != nil {
			revertErr = errors.Errorf("tx %s reverted on-chain; confirmation wait: %w", attempt.hash.Hex(), err)
		}
		observability.Log(ctx).Error(revertErr, "transaction reverted",
			"label", pending.req.Label,
			"hash", attempt.hash.Hex(),
			"nonce", pending.nonce,
		)
		return Result{
			Hash:    attempt.hash,
			Receipt: receipt,
			Outcome: OutcomeReverted,
			Err:     revertErr,
		}, true
	}
	if err != nil {
		return Result{Hash: attempt.hash, Receipt: receipt, Outcome: OutcomeIncludedUnconfirmed, Err: err}, true
	}
	observability.Log(ctx).V(1).Info(
		"transaction confirmed",
		"label", pending.req.Label,
		"hash", attempt.hash.Hex(),
		"nonce", pending.nonce,
		"blockNumber", optionalBigString(receipt.BlockNumber),
		"gasUsed", receipt.GasUsed,
		"effectiveGasPrice", optionalBigString(receipt.EffectiveGasPrice),
		"confirmations", confirmations,
	)
	return Result{Hash: attempt.hash, Receipt: receipt, Outcome: OutcomeConfirmed}, true
}

// replaceIntent says why a fresh fee or gas replacement is needed.
type replaceIntent struct {
	reason string
	gas    uint64
}

// tryReplace reports whether the deadline was reached while preparing a replacement. It never
// changes the business call; a later request owns any fresh calldata at the reusable nonce.
func (m *Manager) tryReplace(ctx context.Context, pending *pendingTransaction, intent replaceIntent) (bool, error) {
	if pending.abandonmentDue(time.Now()) {
		return true, nil
	}
	if available, err := m.replacementNonceAvailable(ctx, pending); err != nil || !available {
		return false, err
	}
	if m.rebroadcastUncertainAttempt(ctx, pending) {
		return false, nil
	}
	gas := pending.gas
	if intent.gas > 0 {
		gas = intent.gas
	}
	fees, err := m.nextReplacementFees(ctx, pending.fees, m.normalFeeLimit(pending.req), gas)
	if pending.abandonmentDue(time.Now()) {
		return true, nil
	}
	if err != nil {
		if errors.Is(err, errReplacementLimitReached) && m.rebroadcastLatestAttempt(ctx, pending) {
			return false, nil
		}
		observability.Log(ctx).Error(err, "cannot replace pending transaction", "label", pending.req.Label, "nonce", pending.nonce)
		return false, err
	}
	sendCtx, cancelSend := replacementBroadcastContext(ctx, pending)
	signed, sendErr := m.signAndSend(sendCtx, pending.nonce, pending.req.To, pending.req.Data, pending.value, gas, fees, true)
	cancelSend()
	if signed == nil {
		observability.Log(ctx).Error(sendErr, "pending transaction replacement rejected", "label", pending.req.Label, "nonce", pending.nonce)
		return pending.abandonmentDue(time.Now()), sendErr
	}
	hash := signed.Hash()
	broadcastUncertain := sendErr != nil && !isKnownTransactionError(sendErr)
	pending.fees = cloneFeeQuote(fees)
	pending.gas = gas
	pending.horizon.sent(pending.horizon.lastHead)
	pending.horizon.stallRebroadcasts = 0
	pending.attempts = append(pending.attempts, txAttempt{hash: hash, tx: signed, exactRebroadcastPending: broadcastUncertain})
	if broadcastUncertain {
		observability.Log(ctx).Error(sendErr, "replacement broadcast uncertain; tracking signed hash", "label", pending.req.Label, "hash", hash.Hex(), "nonce", pending.nonce)
		return false, sendErr
	}
	m.metrics.replacement(pending.req.Label, replacementKindReplacement, intent.reason)
	observability.Log(ctx).Info("pending transaction replaced", "label", pending.req.Label, "hash", hash.Hex(), "nonce", pending.nonce, "reason", intent.reason, "gasLimit", gas, "maxFeePerGas", fees.maxFee.String(), "maxPriorityFeePerGas", fees.tip.String())
	return false, nil
}

// rebroadcastUncertainAttempt gives a transport-ambiguous normal submission one exact-byte retry
// before escalating its fees. It never appends a duplicate attempt or changes the cached fee state.
func (m *Manager) rebroadcastUncertainAttempt(ctx context.Context, pending *pendingTransaction) bool {
	now := time.Now()
	if pending.abandonmentDue(now) || !m.hasExactRebroadcastSlack(pending, now) {
		return false
	}
	if len(pending.attempts) == 0 {
		return false
	}
	attempt := &pending.attempts[len(pending.attempts)-1]
	if attempt.tx == nil || !attempt.exactRebroadcastPending {
		return false
	}
	attempt.exactRebroadcastPending = false
	err := m.sendSigned(ctx, attempt.tx)
	known := isKnownTransactionError(err)
	if err == nil || known {
		m.metrics.replacement(pending.req.Label, replacementKindRebroadcast, rebroadcastReasonUncertain)
	}
	switch {
	case err == nil:
		observability.Log(ctx).Info("uncertain transaction rebroadcast",
			"label", pending.req.Label,
			"hash", attempt.hash.Hex(),
			"nonce", pending.nonce,
			"reason", "ambiguous-broadcast",
		)
	case known:
		observability.Log(ctx).Info("uncertain transaction already known by write RPC",
			"label", pending.req.Label,
			"hash", attempt.hash.Hex(),
			"nonce", pending.nonce,
			"reason", "ambiguous-broadcast",
			"rpcResult", err.Error(),
		)
	default:
		observability.Log(ctx).Error(err, "uncertain transaction exact rebroadcast failed; replacement deferred",
			"label", pending.req.Label,
			"hash", attempt.hash.Hex(),
			"nonce", pending.nonce,
			"reason", "ambiguous-broadcast",
		)
	}
	return true
}

func (m *Manager) hasExactRebroadcastSlack(pending *pendingTransaction, now time.Time) bool {
	if pending.deadline.IsZero() {
		return true
	}
	return pending.deadline.Sub(now) > m.cfg.BroadcastTimeout+m.replacementCadence()
}

func (m *Manager) rebroadcastLatestAttempt(
	ctx context.Context,
	pending *pendingTransaction,
) bool {
	for _, attempt := range slices.Backward(pending.attempts) {
		if attempt.tx == nil {
			continue
		}
		sendCtx, cancelSend := replacementBroadcastContext(ctx, pending)
		err := m.sendSigned(sendCtx, attempt.tx)
		cancelSend()
		if err == nil || isKnownTransactionError(err) {
			m.metrics.replacement(pending.req.Label, replacementKindRebroadcast, rebroadcastReasonCapped)
		}
		if err != nil {
			observability.Log(ctx).Error(err, "capped transaction rebroadcast failed",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
			)
		} else {
			observability.Log(ctx).Info("capped transaction rebroadcast",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
			)
		}
		return true
	}
	return false
}

// Replacement broadcasts must not outlive the abandonment deadline.
func replacementBroadcastContext(ctx context.Context, pending *pendingTransaction) (context.Context, context.CancelFunc) {
	if pending.deadline.IsZero() {
		return ctx, func() {}
	}
	return context.WithDeadline(ctx, pending.deadline)
}

// freshFees prices a replacement of gas units from current chain state, without a limit: the fees a
// new call of that size would get now.
func (m *Manager) freshFees(ctx context.Context, gas uint64) (feeQuote, error) {
	snapshot, _, err := m.readFeeSnapshot(ctx)
	if err != nil {
		return feeQuote{}, err
	}
	return horizonFees(snapshot, gas, nil, m.horizon)
}

func (m *Manager) nextReplacementFees(
	ctx context.Context,
	previous feeQuote,
	limit *big.Int,
	gas uint64,
) (feeQuote, error) {
	current, err := m.freshFees(ctx, gas)
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
		observability.Log(ctx).V(1).Info("fresh replacement fees unavailable; using cached bump", "error", err)
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
	limit := m.globalFeeLimit()
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
	if pending.deadline.IsZero() {
		pending.deadline = m.abandonmentDeadline(pending.req)
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

func (m *Manager) deliverActiveShutdownTimeout() {
	m.unminedMu.Lock()
	pending := m.unmined
	m.unminedMu.Unlock()
	if pending == nil {
		return
	}
	result := Result{
		Hash:    pending.originalHash,
		Outcome: OutcomeTrackingStopped,
		Err:     errShutdownTimeout,
	}
	if !pending.deliver(result) || pending.span == nil {
		return
	}
	// The caller acted on this result, so the span reports it too. complete ends the span again
	// with whatever its own wait concluded; the SDK ignores that second end.
	endSendSpan(pending.span, result)
}

// latestAttempt is the most recently signed variant of the pending nonce.
func (pending *pendingTransaction) latestAttempt() txAttempt {
	return pending.attempts[len(pending.attempts)-1]
}

func (pending *pendingTransaction) abandonmentDue(now time.Time) bool {
	return !pending.deadline.IsZero() && !now.Before(pending.deadline)
}

func (pending *pendingTransaction) abandonmentReason() string {
	if pending.deadline.Equal(pending.req.Deadline) {
		return "request_deadline"
	}
	return "pending_timeout"
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
	sendErr := m.sendSigned(ctx, signed)
	if !existingLifecycle && (isNonceConsumedError(sendErr) || isPendingNonceCollision(sendErr)) {
		return signed, sendErr // Return the attempted identity without owning the competing transaction.
	}
	if isDefiniteBroadcastRejection(sendErr) {
		return nil, errors.Errorf("broadcast rejected before acceptance: %w", sendErr)
	}
	return signed, sendErr
}

func (m *Manager) sendSigned(ctx context.Context, signed *types.Transaction) error {
	sendCtx, cancel := context.WithTimeout(ctx, m.broadcastTimeout())
	defer cancel()
	return m.backend.SendTransaction(sendCtx, signed)
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

func (m *Manager) notifyLaneStateChange() {
	m.laneStateMu.Lock()
	defer m.laneStateMu.Unlock()
	for _, changes := range m.laneStateSubscribers {
		select {
		case changes <- struct{}{}:
		default:
		}
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
	log := observability.Log(ctx)
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
				head := headBefore.Number.Uint64()
				included := receipt.BlockNumber.Uint64()
				if head >= included && head-included >= confirmations {
					if err := m.confirmReceiptAncestry(ctx, headBefore, receipt); err != nil {
						if errors.Is(err, errReceiptReorged) {
							return receipt, err
						}
						ancestryReads.failed(log, err, "receipt ancestry check unavailable", "hash", hash.Hex())
					} else {
						ancestryReads.recovered(log, "receipt ancestry reads recovered", "hash", hash.Hex())
						headAfter, afterErr := m.confirmationHead(ctx)
						if afterErr != nil {
							headReads.failed(log, afterErr, "confirmation head unavailable", "hash", hash.Hex())
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
	return m.confirmReceiptAncestryUsing(ctx, head, receipt, m.backend.HeaderByHash)
}

func (m *Manager) confirmReceiptAncestryUsing(
	ctx context.Context,
	head *types.Header,
	receipt *types.Receipt,
	parentHeader func(context.Context, common.Hash) (*types.Header, error),
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
		parent, err := parentHeader(lookupCtx, current.ParentHash)
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
	if receipt.Status != types.ReceiptStatusSuccessful && receipt.Status != types.ReceiptStatusFailed {
		return errors.Errorf("transaction receipt %s has invalid status %d", hash.Hex(), receipt.Status)
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

func (m *Manager) abandonmentDeadline(req Request) time.Time {
	deadline := time.Now().Add(m.cfg.PendingTimeout)
	if !req.Deadline.IsZero() && req.Deadline.Before(deadline) {
		return req.Deadline
	}
	return deadline
}

func (m *Manager) feeReadTimeout() time.Duration {
	return minPositiveDuration(maxFeeReadTimeout, m.cfg.ReplacementInterval/2)
}

func (m *Manager) receiptReadTimeout() time.Duration {
	return minPositiveDuration(maxReceiptReadTimeout, m.cfg.ReplacementInterval/2)
}

func (m *Manager) gasEstimateTimeout() time.Duration {
	return minPositiveDuration(maxGasEstimateTimeout, m.cfg.ReplacementInterval/2)
}

func (m *Manager) broadcastTimeout() time.Duration {
	return m.cfg.BroadcastTimeout
}

func minPositiveDuration(fallback, candidate time.Duration) time.Duration {
	if candidate > 0 && candidate < fallback {
		return candidate
	}
	return fallback
}
