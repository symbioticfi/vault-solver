// Package txmanager owns the on-chain sending account. One worker serializes admission, fee
// selection, signing, nonce assignment, and broadcasts so solvers cannot race on the account nonce.
// Only one signed lifecycle may be unresolved at a time; solvers build calldata and hand it over via
// Send, TrySend, or SendAsync, but never sign or broadcast directly.
package txmanager

import (
	"context"
	"math/big"
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
	// receiptReadFailures counts consecutive failed receipt lookups. The first failure of a streak
	// is logged at error, the rest at debug, and the error repeats every
	// receiptFailureReminderInterval while the streak lasts, so a stuck RPC produces one alert per
	// pending transaction plus a periodic reminder rather than one per poll, and an outage that
	// never recovers cannot go quiet.
	receiptReadFailures    int
	receiptReadFailedAt    time.Time
	receiptReadLastAlertAt time.Time
	result                 chan<- Result
	resultOnce             sync.Once
	cancelDeadline         time.Time
	cancelRequested        chan struct{}
	cancelOnce             sync.Once
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
	laneStateSubscribers map[uint64]chan struct{}
	nextLaneStateID      uint64

	mu        sync.Mutex // guards the local nonce and runtime nonce conflict
	nonce     uint64
	nonceInit bool
	conflict  *nonceConflict

	unminedMu   sync.Mutex
	unmined     *pendingTransaction
	lifecycleWG sync.WaitGroup
}

type job struct {
	req              Request
	res              chan Result
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
	return &Manager{
		backend:              backend,
		signer:               s,
		chainID:              chainID,
		cfg:                  cfg,
		log:                  log.WithName("txmanager"),
		queue:                make(chan job),
		lifecycleSlot:        make(chan struct{}, 1),
		stopping:             make(chan struct{}),
		laneStateSubscribers: make(map[uint64]chan struct{}),
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
	refreshCtx, cancel := context.WithTimeout(ctx, accountRefreshTimeout)
	defer cancel()
	balance, err := m.transactionSenderBalance(refreshCtx)
	if err == nil && (balance == nil || balance.Sign() < 0) {
		err = errors.New("txmanager: invalid account balance")
	}
	if err == nil {
		var latestNonce, pendingNonce uint64
		latestNonce, err = m.backend.NonceAt(refreshCtx, m.signer.Address(), nil)
		if err == nil {
			pendingNonce, err = m.backend.PendingNonceAt(refreshCtx, m.signer.Address())
		}
		if err == nil {
			m.metrics.observeAccount(balance, latestNonce, pendingNonce)
			return
		}
	}
	if ctx.Err() == nil {
		m.metrics.observeAccountRefreshError()
		m.log.V(1).Info("account metrics refresh failed", "error", err)
	}
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
	accountMonitorDone := make(chan struct{})
	go func() {
		defer close(accountMonitorDone)
		m.monitorAccount(ctx)
	}()
	defer func() { <-accountMonitorDone }()

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
				m.metrics.finishAdmission(j.req.Label, j.admissionStarted, errManagerStopped)
				j.res <- notAdmittedResult(err)
				m.releaseLifecycleSlot()
				stop(err)
				return
			}
			if err := m.nonceConflictError(); err != nil {
				m.metrics.finishAdmission(j.req.Label, j.admissionStarted, err)
				j.res <- notAdmittedResult(err)
				m.releaseLifecycleSlot()
				continue
			}
			m.metrics.finishAdmission(j.req.Label, j.admissionStarted, nil)
			lifecycle := m.metrics.beginLifecycle(j.req.Label)
			pending, err := m.broadcast(ctx, j.req)
			if err != nil {
				outcome := OutcomeSubmissionError
				if ctx.Err() != nil {
					outcome = OutcomeTrackingStopped
				}
				lifecycle.finish(outcome, nil)
				j.res <- Result{
					Outcome:     outcome,
					Err:         err,
					NotAdmitted: errors.Is(err, errNonceLanePaused) || ctx.Err() != nil,
				}
				m.releaseLifecycleSlot()
				continue
			}
			lifecycle.transitionPhase(lifecyclePhasePending)
			pending.lifecycle = lifecycle
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
	if !req.CancelAt.IsZero() {
		admissionCtx, cancel = context.WithDeadline(ctx, req.CancelAt)
	}
	defer cancel()
	if err := admissionCtx.Err(); err != nil {
		return m.admissionFailure(ctx, req, admissionStarted, err)
	}
	select {
	case <-m.stopping:
		return m.admissionFailure(ctx, req, admissionStarted, errManagerStopped)
	default:
	}
	if try {
		if m.nonceConflictError() != nil {
			return nil, false
		}
		select {
		case m.lifecycleSlot <- struct{}{}:
		default:
			return nil, false
		}
		if m.nonceConflictError() != nil {
			<-m.lifecycleSlot
			return nil, false
		}
	} else {
		if err := m.waitForNonceLane(admissionCtx); err != nil {
			return m.admissionFailure(ctx, req, admissionStarted, err)
		}
		select {
		case m.lifecycleSlot <- struct{}{}:
		case <-admissionCtx.Done():
			return m.admissionFailure(ctx, req, admissionStarted, admissionCtx.Err())
		case <-m.stopping:
			return m.admissionFailure(ctx, req, admissionStarted, errManagerStopped)
		}
		if err := m.waitForNonceLane(admissionCtx); err != nil {
			<-m.lifecycleSlot
			return m.admissionFailure(ctx, req, admissionStarted, err)
		}
	}
	res := make(chan Result, 1)
	select {
	case m.queue <- job{req: cloneRequest(req), res: res, admissionStarted: admissionStarted}:
		releaseDemandOnReturn = false
	case <-admissionCtx.Done():
		m.releaseLifecycleSlot()
		releaseDemandOnReturn = false
		return m.admissionFailure(ctx, req, admissionStarted, admissionCtx.Err())
	case <-m.stopping:
		m.releaseLifecycleSlot()
		releaseDemandOnReturn = false
		return m.admissionFailure(ctx, req, admissionStarted, errManagerStopped)
	}
	return res, true
}

func (m *Manager) waitForNonceLane(ctx context.Context) error {
	changes, unsubscribe := m.SubscribeLaneState()
	defer unsubscribe()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case <-m.stopping:
			return errManagerStopped
		default:
		}
		if m.nonceConflictError() == nil {
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
	obsolete, obsoleteErr := m.requestObsolete(broadcastCtx, req)
	if obsoleteErr != nil {
		// Obsolescence is only a liveness optimization. The solver already validated the call,
		// and execution-time contracts remain authoritative, so an unknown check keeps it alive.
		m.log.Error(obsoleteErr, "transaction obsolescence check unavailable; continuing",
			"label", req.Label)
	} else if obsolete {
		return nil, errors.Errorf("send %q: %w", req.Label, errRequestObsolete)
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
	broadcastUncertain := sendErr != nil && !isKnownTransactionError(sendErr)
	if broadcastUncertain {
		m.log.Error(sendErr, "transaction broadcast uncertain; tracking signed hash",
			"label", req.Label, "hash", hash.Hex(), "nonce", nonce)
	} else if sendErr != nil {
		m.log.Info("transaction already known by write RPC",
			"label", req.Label, "hash", hash.Hex(), "nonce", nonce, "rpcResult", sendErr.Error())
	} else {
		m.log.Info("sent", "label", req.Label, "hash", hash.Hex(), "nonce", nonce)
	}
	m.commitNonce(nonce)
	return &pendingTransaction{
		req:   req,
		nonce: nonce,
		gas:   gas,
		value: new(big.Int).Set(value),
		fees:  cloneFeeQuote(fees),
		attempts: []txAttempt{{
			hash: hash, tx: signed, exactRebroadcastPending: broadcastUncertain,
		}},
		originalHash: hash,
	}, nil
}

func (m *Manager) complete(ctx context.Context, pending *pendingTransaction) {
	defer m.removeUnminedTransaction(pending)
	outcome := m.waitForPendingTransaction(ctx, pending)
	pending.lifecycle.finish(outcome.Outcome, outcome.Receipt)
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
	cancelRequested, timeoutC := pending.cancelRequested, timeout.C
	startCancellation := func(reason string) {
		if cancelling {
			return
		}
		if reason == "" {
			select {
			case <-pending.cancelRequested:
				reason = "shutdown"
			default:
				reason = "pending_timeout"
				if pending.cancelDeadline.Equal(pending.req.CancelAt) {
					reason = "request_deadline"
				}
			}
		}
		cancelling, cancelRequested, timeoutC = true, nil, nil
		m.log.Info("pending transaction cancellation requested",
			"label", pending.req.Label,
			"hash", pending.originalHash.Hex(),
			"nonce", pending.nonce,
			"reason", reason,
			"deadline", pending.cancelDeadline.UTC().Format(time.RFC3339Nano),
			"pendingTimeout", m.cfg.PendingTimeout.String(),
		)
	}
	for {
		if receiptResult, done := m.receiptResult(ctx, pending); done {
			return receiptResult
		}
		select {
		case <-ctx.Done():
			return Result{
				Hash:    pending.attempts[0].hash,
				Outcome: OutcomeTrackingStopped,
				Err:     context.Cause(ctx),
			}
		case <-cancelRequested:
			startCancellation("shutdown")
			m.tryReplace(ctx, pending, true)
		case <-poll.C:
			if cancelling || pending.req.Obsolete == nil {
				continue
			}
			obsolete, err := m.requestObsolete(ctx, pending.req)
			if err != nil {
				m.log.Error(err, "pending transaction obsolescence check unavailable; retaining lifecycle",
					"label", pending.req.Label,
					"hash", pending.originalHash.Hex(),
					"nonce", pending.nonce,
				)
				continue
			}
			if !obsolete {
				continue
			}
			startCancellation("obsolete")
			m.tryReplace(ctx, pending, true)
		case <-replace.C:
			if !cancelling && pending.cancellationDue(time.Now()) {
				startCancellation("")
			}
			if m.tryReplace(ctx, pending, cancelling) {
				// A fee lookup can cross the deadline and promote this replacement to
				// cancellation. Disarm the expired timer before the next select.
				startCancellation("")
			}
		case <-timeoutC:
			startCancellation("")
			m.tryReplace(ctx, pending, true)
		}
	}
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

// noteReceiptReadFailed records a failed receipt lookup: the first failure of a streak is an error,
// later ones are debug with the running count, since the condition is already reported.
func (m *Manager) noteReceiptReadFailed(pending *pendingTransaction, attempt txAttempt, err error) {
	pending.receiptReadFailures++
	fields := make([]any, 0, 16)
	fields = append(fields,
		"label", pending.req.Label,
		"hash", attempt.hash.Hex(),
		"originalHash", pending.originalHash.Hex(),
		"nonce", pending.nonce,
		"cancellation", attempt.cancellation,
		"rpcTimeout", m.receiptReadTimeout().String(),
		"consecutiveFailures", pending.receiptReadFailures,
	)
	now := time.Now()
	if pending.receiptReadFailures == 1 {
		pending.receiptReadFailedAt, pending.receiptReadLastAlertAt = now, now
		m.log.Error(err, "pending transaction receipt unavailable", fields...)
		return
	}
	if now.Sub(pending.receiptReadLastAlertAt) >= receiptFailureReminderInterval {
		pending.receiptReadLastAlertAt = now
		m.log.Error(err, "pending transaction receipt still unavailable",
			append(fields, "since", now.Sub(pending.receiptReadFailedAt).Round(time.Second).String())...)
		return
	}
	m.log.V(1).Info("pending transaction receipt still unavailable", append(fields, "error", err.Error())...)
}

// receiptFailureReminderInterval is how often an unrecovered receipt read streak is re-raised at
// error level. A variable so tests can shorten it.
var receiptFailureReminderInterval = 5 * time.Minute

// noteReceiptReadRecovered closes a failure streak, if one was open, with how long it lasted.
func (m *Manager) noteReceiptReadRecovered(pending *pendingTransaction) {
	if pending.receiptReadFailures == 0 {
		return
	}
	m.log.Info("pending transaction receipt reads recovered",
		"label", pending.req.Label,
		"nonce", pending.nonce,
		"consecutiveFailures", pending.receiptReadFailures,
		"outage", time.Since(pending.receiptReadFailedAt).Round(time.Millisecond).String(),
	)
	pending.receiptReadFailures = 0
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
			m.noteReceiptReadRecovered(pending)
			continue
		}
		if err != nil {
			m.noteReceiptReadFailed(pending, attempt, err)
			continue
		}
		m.noteReceiptReadRecovered(pending)
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
		pending.lifecycle.transitionPhase(lifecyclePhaseConfirming)
		confirmations := m.confirmations(pending.req)
		receipt, err = m.waitForConfirmations(ctx, attempt.hash, receipt, confirmations)
		if errors.Is(err, errReceiptReorged) {
			pending.lifecycle.transitionPhase(lifecyclePhasePending)
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
		if receipt.Status == types.ReceiptStatusFailed {
			revertErr := errors.Errorf("tx %s reverted on-chain", attempt.hash.Hex())
			if err != nil {
				revertErr = errors.Errorf("tx %s reverted on-chain; confirmation wait: %w", attempt.hash.Hex(), err)
			}
			m.log.Error(revertErr, "transaction reverted",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
				"tenderly", tenderly.SimulatorURL(m.chainID, m.signer.Address(), pending.req.To, pending.req.Data, pending.req.Value),
			)
			return Result{
				Hash:    attempt.hash,
				Receipt: receipt,
				Outcome: OutcomeReverted,
				Err:     revertErr,
			}, true
		}
		if err != nil {
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
		return Result{Hash: attempt.hash, Receipt: receipt, Outcome: OutcomeConfirmed}, true
	}
	return Result{}, false
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
	limit := m.normalFeeLimit(pending.req)
	if cancellation {
		limit = m.globalFeeLimit()
	}
	fees, err := m.nextReplacementFees(ctx, pending.fees, limit)
	if !cancellation && pending.cancellationDue(time.Now()) {
		return m.tryReplace(ctx, pending, true)
	}
	if err != nil {
		if errors.Is(err, errReplacementLimitReached) &&
			m.rebroadcastLatestAttempt(ctx, pending, cancellation) {
			return cancellation
		}
		m.log.Error(err, "cannot replace pending transaction",
			"label", pending.req.Label,
			"nonce", pending.nonce,
			"cancellation", cancellation,
		)
		return cancellation
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
	if !cancellation && pending.cancellationDue(time.Now()) {
		return m.tryReplace(ctx, pending, true)
	}
	sendCtx, cancelSend := replacementBroadcastContext(ctx, pending, cancellation)
	signed, sendErr := m.signAndSend(sendCtx, pending.nonce, to, data, value, gas, fees, true)
	cancelSend()
	if signed == nil {
		m.log.Error(sendErr, "pending transaction replacement rejected",
			"label", pending.req.Label,
			"nonce", pending.nonce,
			"cancellation", cancellation,
		)
		return cancellation
	}
	hash := signed.Hash()
	broadcastUncertain := sendErr != nil && !isKnownTransactionError(sendErr)
	pending.fees = cloneFeeQuote(fees)
	pending.attempts = append(pending.attempts, txAttempt{
		hash: hash, tx: signed, cancellation: cancellation, exactRebroadcastPending: broadcastUncertain,
	})
	if isNonceConsumedError(sendErr) {
		pending.nonceConflictHash = hash
		m.reconcileExistingLifecycleNonce(ctx, pending)
	}
	if broadcastUncertain {
		m.log.Error(sendErr, "replacement broadcast uncertain; tracking signed hash",
			"label", pending.req.Label,
			"hash", hash.Hex(),
			"nonce", pending.nonce,
			"cancellation", cancellation,
		)
		return cancellation
	}
	if sendErr != nil {
		m.log.Info("replacement already known by write RPC",
			"label", pending.req.Label,
			"hash", hash.Hex(),
			"nonce", pending.nonce,
			"cancellation", cancellation,
			"rpcResult", sendErr.Error(),
		)
		return cancellation
	}
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
	return cancellation
}

// rebroadcastUncertainAttempt gives a transport-ambiguous normal submission one exact-byte retry
// before escalating its fees. It never appends a duplicate attempt or changes the cached fee state.
func (m *Manager) rebroadcastUncertainAttempt(ctx context.Context, pending *pendingTransaction) bool {
	now := time.Now()
	if pending.cancellationDue(now) || !m.hasExactRebroadcastSlack(pending, now) {
		return false
	}
	if len(pending.attempts) == 0 {
		return false
	}
	attempt := &pending.attempts[len(pending.attempts)-1]
	if attempt.cancellation || attempt.tx == nil || !attempt.exactRebroadcastPending {
		return false
	}
	attempt.exactRebroadcastPending = false
	err := m.sendSigned(ctx, attempt.tx, true)
	known := isKnownTransactionError(err)
	if isNonceConsumedError(err) {
		pending.nonceConflictHash = attempt.hash
		m.reconcileExistingLifecycleNonce(ctx, pending)
	}
	switch {
	case err == nil:
		m.log.Info("uncertain transaction rebroadcast",
			"label", pending.req.Label,
			"hash", attempt.hash.Hex(),
			"nonce", pending.nonce,
			"reason", "ambiguous-broadcast",
		)
	case known:
		m.log.Info("uncertain transaction already known by write RPC",
			"label", pending.req.Label,
			"hash", attempt.hash.Hex(),
			"nonce", pending.nonce,
			"reason", "ambiguous-broadcast",
			"rpcResult", err.Error(),
		)
	default:
		m.log.Error(err, "uncertain transaction exact rebroadcast failed; replacement deferred",
			"label", pending.req.Label,
			"hash", attempt.hash.Hex(),
			"nonce", pending.nonce,
			"reason", "ambiguous-broadcast",
		)
	}
	return true
}

func (m *Manager) hasExactRebroadcastSlack(pending *pendingTransaction, now time.Time) bool {
	if pending.cancelDeadline.IsZero() {
		return true
	}
	return pending.cancelDeadline.Sub(now) > m.cfg.BroadcastTimeout+m.cfg.ReplacementInterval
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
		sendCtx, cancelSend := replacementBroadcastContext(ctx, pending, cancellation)
		err := m.sendSigned(sendCtx, attempt.tx, true)
		cancelSend()
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
		pending.cancelRequested = make(chan struct{})
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
		pending.deliver(Result{
			Hash:    pending.originalHash,
			Outcome: OutcomeTrackingStopped,
			Err:     errShutdownTimeout,
		})
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
	if history == nil || len(history.Reward) != feeHistoryBlocks {
		return nil, false
	}
	var tip *big.Int
	for _, blockRewards := range history.Reward {
		if len(blockRewards) != 1 || blockRewards[0] == nil || blockRewards[0].Sign() < 0 {
			return nil, false
		}
		if tip == nil || blockRewards[0].Cmp(tip) < 0 {
			tip = new(big.Int).Set(blockRewards[0])
		}
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
	// 5% headroom over the estimate.
	return gas + gas/20, nil
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
	for _, changes := range m.laneStateSubscribers {
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
	return m.cfg.BroadcastTimeout
}

func minPositiveDuration(fallback, candidate time.Duration) time.Duration {
	if candidate > 0 && candidate < fallback {
		return candidate
	}
	return fallback
}
