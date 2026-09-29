// Package txmanager owns the on-chain sending account. One worker serializes admission, fee
// selection, signing, nonce assignment, and broadcasts so solvers cannot race on the account nonce.
// Only one signed lifecycle may be unresolved at a time; solvers build calldata and hand it over via
// Send, TrySend, or SendAsync, but never sign or broadcast directly.
package txmanager

import (
	"context"
	"math/big"
	"math/bits"
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

// cancellationBackend optionally routes same-nonce self-cancellations to a separate endpoint.
// Plain EVM backends keep using SendTransaction for every broadcast.
type cancellationBackend interface {
	SendCancellationTransaction(ctx context.Context, tx *types.Transaction) error
}

// Config tunes fee selection and confirmation behavior. Its zero value is the safe default (see
// WithDefaults): the legacy fee policy, the balance guard on and 500 bps of gas headroom.
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
	Fees                FeeConfig     // fee policy, validity horizons and the priority-fee ladder
	Gas                 GasConfig     // gas-limit headroom and next-block estimation
	Balance             BalanceConfig // per-attempt balance guard and lane funding gate
	Shadow              ShadowConfig  // metrics-only evaluator of both fee policies
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
	OutcomeConfirmed            Outcome = "confirmed"
	OutcomeIncludedUnconfirmed  Outcome = "included_unconfirmed"
	OutcomeReverted             Outcome = "reverted"
	OutcomeCancelled            Outcome = "cancelled"
	OutcomeCancelledUnconfirmed Outcome = "cancelled_unconfirmed"
	OutcomeSubmissionError      Outcome = "submission_error"
	OutcomeTrackingStopped      Outcome = "tracking_stopped"
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
	nonceReads        readStreak
	obsolescenceReads readStreak
	result            chan<- Result
	resultOnce        sync.Once
	// span is the caller's send span, so the shutdown drain can end it with the result it hands
	// the caller rather than leaving that to a complete that may conclude differently.
	span            trace.Span
	cancelDeadline  time.Time
	cancelRequested chan struct{}
	cancelOnce      sync.Once
	// sendHead is the head the first attempt was priced at and sentAt when it was sent; inclusion delay
	// and pending age are measured from them. Both are zero for a lifecycle built outside broadcast.
	// sendSeen is the newest block the first attempt's fee snapshot knew of, the header or its fee history
	// one block ahead: the horizon policy's pending evaluation counts only later blocks as missed.
	sendHead uint64
	sendSeen uint64
	sentAt   time.Time
	// balanceCapLogged keeps the balance-capped replacement log to once per lifecycle, and
	// reservedCapLogged the log of a cancellation capped at the reserved balance.
	balanceCapLogged  bool
	reservedCapLogged bool
	// reorged is the inclusion a reorg just removed, for the lifecycle loop to rebroadcast under the horizon
	// policy (see pendingEvaluator.reorged); the loop clears it.
	reorged *reorgedInclusion
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

	// balances is the pinned-balance reader of the balance guard; nil runs with the guard off.
	balances pinnedBalanceBackend
	// lastInclusion is the highest block that included an attempt of a finished lifecycle. Balance
	// pins below it are stale: they may predate that attempt's payment.
	lastInclusion atomic.Uint64
	// hashPinMisses counts the balance reads pinned by header hash in a row that ended not found.
	hashPinMisses atomic.Uint64
	// fundingGateOn is set once in New: balance.referenceGasUnits is positive and the backend can read
	// the signer balance on account polls. funding is the gate's state and fundingReads the account
	// poll's failure streak refreshing it (funding.go).
	fundingGateOn bool
	funding       fundingGate
	fundingReads  readStreak
	// balanceMu guards signerBalance, the signer balance the manager read last (SignerBalance).
	balanceMu     sync.Mutex
	signerBalance *big.Int

	// horizon is the horizon fee policy's configuration in wei, and snapshots the fee snapshots it prices
	// from, read with snapshotRewards' percentiles (fee_snapshot.go). nextBlock is the next-block gas
	// estimate capability, set in New under the horizon policy with gas.nextBlockEstimate on; overrides is the
	// capability probe's verdict and probeReads its failure streak, owned by the probe loop (gas_estimate.go).
	horizon         horizonPolicy
	snapshots       *feeSnapshotCache
	snapshotRewards snapshotRewards
	nextBlock       nextBlockEstimateBackend
	overrides       overridesState
	probeReads      readStreak

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
	span             trace.Span // the caller-derived send span, ended by whoever resolves the request
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
	errEstimateAbandoned       = errors.New("gas estimate abandoned: the send failed before it was needed")
	errShutdownTimeout         = errors.Errorf("transaction manager shutdown drain timed out: %w", context.DeadlineExceeded)
)

var (
	// errReplacementBaseAboveLimit is a replacement whose fee limit is below the latest base fee.
	errReplacementBaseAboveLimit = errors.New("replacement base fee exceeds fee limit")
	// errEstimateFailed ends a broadcast's fee and balance reads once its gas estimate has failed.
	errEstimateFailed = errors.New("gas estimate failed")
)

// New constructs a Manager. Call Start to launch its worker.
func New(backend Backend, s signer.Signer, chainID *big.Int, cfg Config, log logr.Logger) *Manager {
	m := &Manager{
		backend:              backend,
		signer:               s,
		chainID:              chainID,
		cfg:                  cfg.WithDefaults(),
		log:                  log.WithName("txmanager"),
		queue:                make(chan job),
		lifecycleSlot:        make(chan struct{}, 1),
		stopping:             make(chan struct{}),
		laneStateSubscribers: make(map[uint64]chan struct{}),
		fundingReads:         readStreak{quietStart: true},
		probeReads:           readStreak{quietStart: true},
	}
	if balances, ok := backend.(pinnedBalanceBackend); ok && !m.cfg.Balance.GuardDisabled {
		m.balances = balances
	}
	m.fundingGateOn = m.cfg.Balance.ReferenceGasUnits > 0 && m.supportsAccountBalance()
	m.horizon = newHorizonPolicy(m.cfg.Fees)
	m.snapshotRewards = newSnapshotRewards(m.cfg.Fees.CongestedRewardPercentile)
	m.snapshots = &feeSnapshotCache{read: func(ctx context.Context) (*feeSnapshot, error) {
		return m.readFeeSnapshot(ctx, m.snapshotBlocks())
	}}
	if estimator, ok := backend.(nextBlockEstimateBackend); ok &&
		m.cfg.Fees.Policy == FeePolicyHorizon && !m.cfg.Gas.NextBlockEstimateDisabled {
		m.nextBlock = estimator
	}
	return m
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

// ReferenceGasUnits is the fill gas limit the lane funding gate and quote pricing assume
// (txManager.balance.referenceGasUnits); zero means the operator has not set one and the gate is off.
func (m *Manager) ReferenceGasUnits() uint64 {
	return m.cfg.Balance.ReferenceGasUnits
}

// ValidateFeeHeadroom rejects a fee configuration whose priority fees can never fit under the
// initial transaction cap after reserving one ordinary replacement and one cancellation bump: the
// tipGwei floor and fees.tipFloorGwei (which the balance guard's refusal floor assumes) under both
// policies, and the whole tip ladder up to fees.congestedTipCapGwei under the horizon policy. It also
// rejects a fees.tipFloorGwei below one wei (the rest of the ladder is at least the floor), an unknown
// policy, and a positive tipGwei under the horizon policy.
func (m *Manager) ValidateFeeHeadroom() error {
	cfg := m.cfg.WithDefaults()
	if cfg.Fees.Policy != FeePolicyLegacy && cfg.Fees.Policy != FeePolicyHorizon {
		return errors.Errorf("unknown fee policy %q", cfg.Fees.Policy)
	}
	// The horizon policy's tip comes from the fees ladder alone; a mandatory tipGwei would be ignored.
	if cfg.Fees.Policy == FeePolicyHorizon && cfg.TipGwei != 0 {
		return errors.Errorf("tipGwei %v must be 0 under the horizon fee policy, whose tip comes from fees.*TipGwei", cfg.TipGwei)
	}
	// Checked in wei, as it is signed: a positive gwei value below one wei truncates to a zero tip,
	// which relays reject and which would leave the refusal floor on the base fee alone.
	if gweiToWei(cfg.Fees.TipFloorGwei).Sign() <= 0 {
		return errors.Errorf("fees.tipFloorGwei %v must be at least one wei (0.000000001 gwei)", cfg.Fees.TipFloorGwei)
	}
	initialLimit := reserveFeeBump(m.normalFeeLimit(Request{}))
	if initialLimit == nil {
		return nil
	}
	tip := gweiToWei(cfg.TipGwei)
	if tip.Sign() > 0 && tip.Cmp(initialLimit) >= 0 {
		return errors.Errorf(
			"tip floor %s leaves no base-fee headroom under initial fee limit %s after reserved replacement bumps",
			tip, initialLimit,
		)
	}
	if floor := gweiToWei(cfg.Fees.TipFloorGwei); floor.Cmp(initialLimit) >= 0 {
		return errors.Errorf(
			"fees.tipFloorGwei %s leaves no base-fee headroom under initial fee limit %s after reserved replacement bumps",
			floor, initialLimit,
		)
	}
	if cfg.Fees.Policy == FeePolicyHorizon {
		if tipCap := gweiToWei(cfg.Fees.CongestedTipCapGwei); tipCap.Cmp(initialLimit) >= 0 {
			return errors.Errorf(
				"fees.congestedTipCapGwei %s leaves no base-fee headroom under initial fee limit %s after reserved replacement bumps",
				tipCap, initialLimit,
			)
		}
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

// SubscribeLaneState returns an independent, coalesced change stream. Consumers must re-read
// LaneReady (and Fundable, if they gate on it) after every signal instead of assuming which edge
// occurred, and must call unsubscribe when they stop. Signals cover nonce-conflict, admission-demand
// and funding-gate edges. Independent subscriptions prevent readiness and solvers from stealing
// signals from each other.
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

// monitorAccount polls the signer account while account metrics or the funding gate need it.
func (m *Manager) monitorAccount(ctx context.Context) {
	if (m.metrics == nil && !m.fundingGateOn) || !m.supportsAccountBalance() {
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
// context carries no span, so each poll is its own trace. account_refreshes_total counts the telemetry
// snapshot alone; a funding-gate failure is logged once per run of failures (fundingPollFailed).
func (m *Manager) refreshAccount(ctx context.Context) {
	pollCtx, end := tracer.Start(ctx, "txmanager.account_poll")
	telemetryErr, fundingErr := m.readAccount(pollCtx)
	end(errors.Join(fundingErr, telemetryErr))
	if ctx.Err() != nil {
		return
	}
	if telemetryErr != nil {
		m.metrics.observeAccountRefreshError()
		observability.Log(ctx).V(1).Info("account metrics refresh failed", "error", telemetryErr)
	}
	if !m.fundingGateOn {
		return
	}
	if fundingErr != nil {
		m.fundingPollFailed(ctx, fundingErr)
		return
	}
	m.fundingPollRecovered(ctx)
}

// readAccount refreshes the funding gate when it is on, then reads the account telemetry snapshot when
// metrics are on. Each has its own read budget and error, so a failure of one neither skips nor
// miscounts the other; a failed read keeps the previous gate state or snapshot. The gate goes first: its
// balance is pinned to the fee history's newest block, and telemetry then reads latest, so the two
// balances are read in block order.
func (m *Manager) readAccount(ctx context.Context) (telemetryErr, fundingErr error) {
	if m.fundingGateOn {
		fundingCtx, cancel := context.WithTimeout(ctx, accountRefreshTimeout)
		fundingErr = m.refreshFunding(fundingCtx)
		cancel()
	}
	if m.metrics != nil {
		telemetryCtx, cancel := context.WithTimeout(ctx, accountRefreshTimeout)
		telemetryErr = m.refreshTelemetry(telemetryCtx)
		cancel()
	}
	return telemetryErr, fundingErr
}

// refreshTelemetry reads the account telemetry snapshot and exports it.
func (m *Manager) refreshTelemetry(ctx context.Context) error {
	reading, err := m.readAccountTelemetry(ctx)
	if err != nil {
		return err
	}
	if reading.balance == nil || reading.balance.Sign() < 0 {
		return errors.New("txmanager: invalid account balance")
	}
	m.metrics.observeAccount(reading.balance, reading.latestNonce, reading.pendingNonce)
	m.observeSignerBalance(reading.balance)
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

// Start admits one signed lifecycle at a time. On cancellation, the active lifecycle is asked to
// cancel and drain. Once ShutdownTimeout elapses, its context is cancelled, its caller receives a
// terminal deadline result, and the worker returns without waiting on a stuck dependency.
func (m *Manager) Start(ctx context.Context) {
	// The worker's own context carries the manager logger; per-request contexts replace it with the
	// job's solver-stamped one below.
	ctx = observability.WithLogger(ctx, m.log)
	m.metrics.bindAccount(m.signer.Address())
	if m.cfg.Balance.TargetEth > 0 {
		m.metrics.setBalanceTarget(ethToWei(m.cfg.Balance.TargetEth))
	}
	m.exportFundingGate()
	accountMonitorDone := make(chan struct{})
	go func() {
		defer close(accountMonitorDone)
		m.monitorAccount(ctx)
	}()
	defer func() { <-accountMonitorDone }()
	probeDone := make(chan struct{})
	go func() {
		defer close(probeDone)
		m.probeBlockOverrides(ctx)
	}()
	defer func() { <-probeDone }()

	observability.Log(ctx).Info("started",
		"from", m.signer.Address().Hex(),
		"feePolicy", string(m.cfg.Fees.Policy),
		"balanceGuard", m.guardEnabled(),
		"fundingGate", m.fundingGateOn,
		"referenceGasUnits", m.cfg.Balance.ReferenceGasUnits,
	)
	if !m.cfg.Balance.GuardDisabled && !m.guardEnabled() {
		observability.Log(ctx).Info("balance guard disabled: the backend cannot read a balance pinned to a block; " +
			"attempts are signed without checking that the signer can fund them")
	}
	if m.cfg.Balance.ReferenceGasUnits > 0 && !m.fundingGateOn {
		observability.Log(ctx).Info("funding gate disabled: the backend cannot read the signer balance; " +
			"Fundable always reports true")
	}
	if m.cfg.Fees.Policy == FeePolicyHorizon && !m.cfg.Gas.NextBlockEstimateDisabled && m.nextBlock == nil {
		observability.Log(ctx).Info("next-block gas estimates unavailable: the backend cannot estimate with block "+
			"overrides; horizon sends estimate at latest with gas.fallbackHeadroomBps",
			"fallbackHeadroomBps", m.fallbackGasHeadroomBps())
	}
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
			if err := m.nonceConflictError(); err != nil {
				m.metrics.finishAdmission(j.req.Label, j.admissionStarted, err)
				deliverJobResult(j, notAdmittedResult(err))
				m.releaseLifecycleSlot()
				continue
			}
			m.metrics.finishAdmission(j.req.Label, j.admissionStarted, nil)
			lifecycle := m.metrics.beginLifecycle(j.req.Label)
			pending, err := m.broadcast(spanCtx, j.req)
			if err != nil {
				outcome := OutcomeSubmissionError
				if ctx.Err() != nil {
					outcome = OutcomeTrackingStopped
				}
				refusal, refused := guardRefusalReason(err)
				if refused {
					m.metrics.guardRefusal(j.req.Label, refusal)
				}
				lifecycle.finish(outcome, nil)
				deliverJobResult(j, Result{
					Outcome:     outcome,
					Err:         err,
					NotAdmitted: errors.Is(err, errNonceLanePaused) || ctx.Err() != nil || refused,
				})
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
		if m.nonceConflictError() != nil {
			return declineBusyLane()
		}
		select {
		case m.lifecycleSlot <- struct{}{}:
		default:
			return declineBusyLane()
		}
		if m.nonceConflictError() != nil {
			<-m.lifecycleSlot
			return declineBusyLane()
		}
	} else {
		if err := m.waitForNonceLane(admissionCtx); err != nil {
			return failAdmission(err)
		}
		select {
		case m.lifecycleSlot <- struct{}{}:
		case <-admissionCtx.Done():
			return failAdmission(admissionCtx.Err())
		case <-m.stopping:
			return failAdmission(errManagerStopped)
		}
		if err := m.waitForNonceLane(admissionCtx); err != nil {
			<-m.lifecycleSlot
			return failAdmission(err)
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
// configured limit permits it. Send recomputes the initial fees immediately before signing. Under the
// horizon policy it is bump(fee(fees.pricingHorizonBlocks, tip)) for a fill of balance.referenceGasUnits
// gas, priced from the cached fee snapshot; passed back as Request.MaxFeePerGas it still fills after the
// base fee grew at the maximum rate for pricingHorizonBlocks − minHorizonBlocks blocks.
func (m *Manager) MaxFeePerGas(ctx context.Context) (*big.Int, error) {
	if m.cfg.Fees.Policy == FeePolicyHorizon {
		return m.horizonMaxFeePerGas(ctx)
	}
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
// estimation, signing, and nonce assignment stay serialized. The order is: the fee policy prices the
// attempt (a fresh fee snapshot, the signer balance pinned to its head and the gas estimate, then the
// balance guard); Obsolete; the nonce; then sign and send. A guard refusal therefore happens before
// anything is signed and leaves the nonce unconsumed.
func (m *Manager) broadcast(ctx context.Context, req Request) (pending *pendingTransaction, err error) {
	sendSpan := trace.SpanFromContext(ctx)

	broadcastCtx := ctx
	cancel := func() {}
	if !req.CancelAt.IsZero() {
		broadcastCtx, cancel = context.WithDeadline(ctx, req.CancelAt)
	}
	defer cancel()
	broadcastCtx, end := tracer.Start(broadcastCtx, "txmanager.broadcast")
	var priced pricedAttempt
	defer func() {
		// A guard refusal is an expected skip of an underfunded or briefly stale lane, not a failure of
		// the broadcast or anything to page on; the solver decides what it means.
		if reason, refused := guardRefusalReason(err); refused {
			observability.Log(ctx).Info("transaction refused before signing",
				"label", req.Label,
				"reason", string(reason),
				"head", priced.snapshot.head,
				"nextBaseFee", optionalBigString(priced.snapshot.nextBase),
				"headLagBlocks", priced.snapshot.lag,
				"error", err.Error(),
			)
			observability.Decline(broadcastCtx, "not_admitted", string(reason))
			end(nil)
			return
		}
		end(err)
	}()
	if err := broadcastCtx.Err(); err != nil {
		return nil, errors.Errorf("send %q before broadcast: %w", req.Label, err)
	}

	if req.MaxFeePerGas != nil && req.MaxFeePerGas.Sign() <= 0 {
		return nil, errors.Errorf("send %q: request max fee per gas must be positive", req.Label)
	}
	value := req.Value
	if value == nil {
		value = new(big.Int)
	}
	if m.cfg.Fees.Policy == FeePolicyHorizon {
		priced, err = m.priceHorizonAttempt(broadcastCtx, req, value)
	} else {
		priced, err = m.priceLegacyAttempt(broadcastCtx, req, value)
	}
	// A refusal is annotated too: the next base fee and what the balance funded are what explain it.
	annotateBroadcast(broadcastCtx, priced, value)
	if err != nil {
		return nil, err
	}
	fees, gas := priced.fees, priced.gas
	obsolete, obsoleteErr := m.requestObsolete(broadcastCtx, req)
	if obsoleteErr != nil {
		// Obsolescence is only a liveness optimization. The solver already validated the call,
		// and execution-time contracts remain authoritative, so an unknown check keeps it alive.
		observability.Log(ctx).Error(obsoleteErr, "transaction obsolescence check unavailable; continuing",
			"label", req.Label)
	} else if obsolete {
		return nil, errors.Errorf("send %q: %w", req.Label, errRequestObsolete)
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

	nonce, err := m.nextNonce(broadcastCtx)
	if err != nil {
		return nil, err
	}
	signed, sendErr := m.signAndSend(
		broadcastCtx, nonce, req.To, req.Data, value, gas, fees, false, false,
	)
	if signed == nil {
		return nil, errors.Errorf("send %q: %w", req.Label, sendErr)
	}
	sentAt := time.Now()
	hash := signed.Hash()
	// Both spans: the broadcast span is short-lived, the send span keeps the identity for the whole
	// lifecycle (endSendSpan later overwrites tx.hash with the attempt that actually landed).
	txIdentity := []attribute.KeyValue{
		observability.AttrTxHash.String(hash.Hex()),
		observability.AttrTxNonce.Int64(int64(nonce)),
	}
	observability.SetAttributes(broadcastCtx, txIdentity...)
	sendSpan.SetAttributes(txIdentity...)
	m.metrics.observeAttempt(req.Label, false, fees, gas)
	if priced.snapshot.nextBase != nil {
		m.metrics.observeHorizon(req.Label, priced.horizon)
	}
	// The fee fields are logged at Info: RFQ and LI.FI run without --debug, and they are what explains
	// a fill that did not land.
	sentFields := []any{
		"label", req.Label, "hash", hash.Hex(), "nonce", nonce,
		"gasLimit", gas, "estimateMode", priced.estimateMode,
		"tip", fees.tip.String(), "maxFee", fees.maxFee.String(),
		"requiredBalance", requiredBalance(gas, fees.maxFee, value).String(),
	}
	if priced.snapshot.nextBase != nil {
		sentFields = append(sentFields, "nextBaseFee", priced.snapshot.nextBase.String(), "horizonBlocks", priced.horizon)
		if priced.balance != nil {
			sentFields = append(sentFields, "balance", priced.balance.String(), "balanceBound", priced.balanceBound)
		}
		sentFields = append(sentFields, "headLagBlocks", priced.snapshot.lag)
	}
	broadcastUncertain := sendErr != nil && !isKnownTransactionError(sendErr)
	if broadcastUncertain {
		observability.Log(ctx).Error(sendErr, "transaction broadcast uncertain; tracking signed hash", sentFields...)
	} else if sendErr != nil {
		observability.Log(ctx).Info("transaction already known by write RPC",
			append(sentFields, "rpcResult", sendErr.Error())...)
	} else {
		observability.Log(ctx).Info("sent", sentFields...)
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
		span:         sendSpan,
		sendHead:     priced.snapshot.head,
		sendSeen:     max(priced.snapshot.head, priced.snapshot.historyHead),
		sentAt:       sentAt,
	}, nil
}

// priceLegacyAttempt prices a new attempt under the legacy policy: a fee snapshot (with the balance guard's
// stale-head wait) and the signer balance pinned to its head, overlapped with the gas estimate; then the
// legacy quote and the guard. The returned attempt carries the snapshot even on error, for the refusal log.
func (m *Manager) priceLegacyAttempt(ctx context.Context, req Request, value *big.Int) (priced pricedAttempt, err error) {
	// The estimate needs neither the fees nor the balance, so it overlaps their reads. A failed estimate
	// fails the send whatever they return, so it also ends them: the send then reports the estimate's
	// error at once instead of waiting out a stale head and returning a refusal a solver would retry.
	readCtx, stopReads := context.WithCancelCause(ctx)
	defer stopReads(nil)
	estimate := m.estimateAsync(ctx, req, func(ctx context.Context) (uint64, string, error) {
		gas, err := m.estimateGas(ctx, req)
		return gas, estimateModeLatest, err
	}, func() { stopReads(errEstimateFailed) })
	defer estimate.stop()

	snapshot, err := m.sendSnapshot(readCtx)
	priced.snapshot = snapshot
	if err != nil {
		if estimateErr := estimate.failure(); estimateErr != nil {
			return priced, estimateErr
		}
		return priced, errors.Errorf("send %q: %w", req.Label, err)
	}
	fees, err := m.legacyQuote(snapshot.reading, reserveFeeBump(m.normalFeeLimit(req)))
	if err != nil {
		return priced, errors.Errorf("send %q: %w", req.Label, err)
	}
	if snapshot.guarded() {
		if priced.balance, err = m.pinnedBalance(readCtx, snapshot.pin); err != nil {
			if estimateErr := estimate.failure(); estimateErr != nil {
				return priced, estimateErr
			}
			return priced, errors.Errorf("send %q: %w", req.Label, err)
		}
		// The pinned balance and the next base fee are a fee snapshot of the lane: the funding gate
		// follows it between account polls, so a refusal below closes the gate at once.
		m.evaluateFunding(ctx, priced.balance, snapshot.nextBase, snapshot.historyHead)
	}
	if priced.gas, priced.estimateMode, err = estimate.wait(); err != nil {
		return priced, err
	}
	if snapshot.guarded() {
		guard, err := applyBalanceGuard(guardInput{
			fees:       fees,
			nextBase:   snapshot.nextBase,
			lag:        snapshot.lag,
			minHorizon: m.cfg.Fees.MinHorizonBlocks,
			floorTip:   m.floorTip(),
			balance:    priced.balance,
			value:      value,
			gas:        priced.gas,
		})
		m.observeFeeSnapshot(snapshot.nextBase, priced.gas)
		if err != nil {
			return priced, errors.Errorf("send %q: %w", req.Label, err)
		}
		fees = guard.fees
		priced.horizon, priced.balanceBound = guard.horizon, guard.clamped
	}
	priced.fees = fees
	return priced, nil
}

// annotateBroadcast records how an attempt was priced on the broadcast span, as far as pricing got: an
// attempt refused or failed before signing carries what was read before it stopped, and a fee horizon only
// once a fee cap was priced.
func annotateBroadcast(ctx context.Context, priced pricedAttempt, value *big.Int) {
	var attrs []attribute.KeyValue
	if priced.estimateMode != "" {
		attrs = append(attrs, attribute.String("gas.estimate_mode", priced.estimateMode))
	}
	if priced.snapshot.nextBase != nil {
		attrs = append(attrs, attribute.String("fee.next_base", priced.snapshot.nextBase.String()))
		if priced.fees.maxFee != nil {
			attrs = append(attrs, attribute.Int64("fee.horizon", int64(min(priced.horizon, maxReportedHorizonBlocks))))
		}
	}
	if priced.balance != nil && priced.gas > 0 {
		attrs = append(attrs, attribute.String("balance.affordable",
			affordableString(affordableMaxFee(priced.balance, value, priced.gas))))
	}
	observability.SetAttributes(ctx, attrs...)
}

// observeFeeSnapshot exports the next base fee a send was priced or guarded at, or an account poll of the
// funding gate read, and what a reference fill needs then: balance.referenceGasUnits, or this attempt's gas
// limit while that is unset.
func (m *Manager) observeFeeSnapshot(nextBase *big.Int, gas uint64) {
	if m.metrics == nil {
		return
	}
	reference := m.cfg.Balance.ReferenceGasUnits
	if reference == 0 {
		reference = gas
	}
	floorTip := m.floorTip()
	need := func(blocks uint64) *big.Int {
		return requiredBalance(reference, horizonFee(nextBase, blocks, floorTip), new(big.Int))
	}
	m.metrics.observeFeeSnapshot(nextBase, laneRequirements{
		min:   need(m.cfg.Fees.MinHorizonBlocks),
		quote: need(m.cfg.Fees.PricingHorizonBlocks),
		full:  need(m.cfg.Fees.MaxHorizonBlocks),
	})
}

// asyncEstimate is a gas estimate running beside the fee and balance reads of one broadcast. Its
// methods are called from the worker goroutine only; the estimate goroutine writes gas, mode and err before
// it closes done, and nothing writes them after.
type asyncEstimate struct {
	done   chan struct{}
	gas    uint64
	mode   string
	err    error
	failed bool // the estimate failed on its own, not because its context ended
	cancel context.CancelCauseFunc
}

// estimateAsync starts estimate for req, unless the request supplies its gas limit. onFailure runs on the
// estimate goroutine when the estimate fails on its own, after failure reports it.
func (m *Manager) estimateAsync(ctx context.Context, req Request, estimate gasEstimator, onFailure func()) *asyncEstimate {
	pending := &asyncEstimate{done: make(chan struct{}), cancel: func(error) {}}
	if req.GasLimit != 0 {
		pending.gas, pending.mode = req.GasLimit, estimateModeSupplied
		close(pending.done)
		return pending
	}
	estimateCtx, cancel := context.WithCancelCause(ctx)
	pending.cancel = cancel
	go func() {
		pending.gas, pending.mode, pending.err = estimate(estimateCtx)
		pending.failed = pending.err != nil && estimateCtx.Err() == nil
		close(pending.done)
		if pending.failed {
			onFailure()
		}
	}()
	return pending
}

// wait returns the estimate and its mode once it has finished.
func (e *asyncEstimate) wait() (uint64, string, error) {
	<-e.done
	return e.gas, e.mode, e.err
}

// failure returns the estimate's error if it has already failed on its own, without waiting for it.
func (e *asyncEstimate) failure() error {
	select {
	case <-e.done:
		if e.failed {
			return e.err
		}
	default:
	}
	return nil
}

// stop cancels an estimate still running and waits for its goroutine, so the estimate never outlives
// broadcast. It reports whether the estimate had already failed on its own, whose error then stands.
func (e *asyncEstimate) stop() bool {
	e.cancel(errEstimateAbandoned)
	<-e.done
	return e.failed
}

func (m *Manager) complete(ctx context.Context, pending *pendingTransaction) {
	defer m.removeUnminedTransaction(pending)
	outcome := m.waitForPendingTransaction(ctx, pending)
	m.metrics.clearPendingAge(pending.req.Label)
	m.recordInclusion(pending, outcome)
	pending.lifecycle.finish(outcome.Outcome, outcome.Receipt)
	if errors.Is(outcome.Err, errShutdownTimeout) {
		observability.Log(ctx).Error(outcome.Err, "accepted transaction lifecycle did not drain before shutdown",
			"label", pending.req.Label,
			"nonce", pending.nonce,
			"hashes", attemptHashStrings(pending.attempts),
		)
	}
	if outcome.Receipt != nil {
		m.clearNonceConflict(pending.nonce)
	}
	// End before delivering, so the caller never resumes its trace while the span it nests under is
	// still open. A pending built outside Start carries no span and gets the no-op one from ctx.
	endSendSpan(trace.SpanFromContext(ctx), outcome)
	pending.deliver(outcome)
}

// recordInclusion raises the balance-pin floor to the block that included the lifecycle, and records
// how it landed relative to its first signed attempt.
func (m *Manager) recordInclusion(pending *pendingTransaction, outcome Result) {
	if outcome.Receipt == nil || outcome.Receipt.BlockNumber == nil || !outcome.Receipt.BlockNumber.IsUint64() {
		return
	}
	included := outcome.Receipt.BlockNumber.Uint64()
	m.noteInclusion(included)
	if pending.sendHead == 0 || included < pending.sendHead {
		return
	}
	landed, delay := classifyFirstAttempt(pending.attempts, outcome.Hash, included-pending.sendHead)
	m.metrics.observeInclusion(pending.req.Label, landed, delay)
}

// classifyFirstAttempt says how a lifecycle landed: cancelled when its cancellation was included,
// replaced when any replacement or cancellation was signed before its call landed, and otherwise first
// or late depending on whether the first signed attempt took more than firstAttemptBlocks blocks. An
// exact rebroadcast of the same bytes signs nothing new, so it keeps a first attempt first.
func classifyFirstAttempt(attempts []txAttempt, landed common.Hash, delay uint64) (firstAttemptOutcome, uint64) {
	for _, attempt := range attempts {
		if attempt.hash == landed && attempt.cancellation {
			return firstAttemptCancelled, delay
		}
	}
	switch {
	case len(attempts) > 1:
		return firstAttemptReplaced, delay
	case delay > firstAttemptBlocks:
		return firstAttemptLate, delay
	default:
		return firstAttemptFirst, delay
	}
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
	replace := time.NewTicker(m.pendingTick())
	defer replace.Stop()
	timeout := time.NewTimer(max(time.Until(pending.cancelDeadline), 0))
	defer timeout.Stop()

	var replacementStarted time.Time
	replaceWith := func(cancellation bool, plan *replacementPlan) bool {
		replacementStarted = time.Now()
		attrs := []attribute.KeyValue{
			observability.AttrTxAttempt.Int(len(pending.attempts) + 1),
			attribute.Bool("tx.cancellation", cancellation),
		}
		if plan != nil {
			attrs = append(attrs, attribute.String("tx.reprice_reason", plan.reason))
		}
		replaceCtx, end := tracer.Start(ctx, "txmanager.replace", attrs...)
		cancelling, err := m.replace(replaceCtx, pending, cancellation, plan)
		end(err)
		return cancelling
	}
	tryReplace := func(cancellation bool) bool { return replaceWith(cancellation, nil) }
	cancelling := false
	var cancellationStarted time.Time
	cancelRequested, timeoutC := pending.cancelRequested, timeout.C
	// evaluator drives the lifecycle under the horizon policy (pending.go); nil under the legacy policy.
	var evaluator *pendingEvaluator
	startCancellation := func(reason string) {
		if cancelling {
			return
		}
		// A re-estimate still in flight decided about the call, which the cancellation now replaces.
		evaluator.abandonEstimate()
		cancellationStarted = time.Now()
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
		observability.Log(ctx).Info("pending transaction cancellation requested",
			"label", pending.req.Label,
			"hash", pending.originalHash.Hex(),
			"nonce", pending.nonce,
			"reason", reason,
			"deadline", pending.cancelDeadline.UTC().Format(time.RFC3339Nano),
			"pendingTimeout", m.cfg.PendingTimeout.String(),
		)
	}
	if m.cfg.Fees.Policy == FeePolicyHorizon {
		evaluator = m.newPendingEvaluator(pending, pendingActions{
			replace: func(cancellation bool, plan *replacementPlan) {
				if replaceWith(cancellation, plan) {
					startCancellation("")
				}
			},
			cancel: func(reason string) {
				startCancellation(reason)
				tryReplace(true)
			},
		})
		defer evaluator.abandonEstimate()
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
				result, done := m.confirmPendingReceipt(ctx, pending, read.attempt, read.receipt)
				if done {
					return result
				}
				// A reorg or an untrusted receipt keeps ownership and resumes polling.
				sweep = nil
				if reorged := pending.reorged; reorged != nil {
					pending.reorged = nil
					if evaluator != nil {
						evaluator.reorged(ctx, *reorged)
					}
				}
			} else if sweep.nextIndex(pending) < 0 {
				m.finishReceiptSweep(ctx, pending, sweep)
				// Include superseded variants considered by the priority path.
				knownAttempts = sweep.knownAttempts
				sweep = nil
				// A terminal protocol status may reflect our own transaction.
				// Give receipts precedence before checking obsolescence.
				if !cancelling && m.pendingRequestObsolete(ctx, pending) {
					startCancellation("obsolete")
					tryReplace(true)
				}
			}
		case <-ctx.Done():
			return Result{
				Hash:    pending.attempts[0].hash,
				Outcome: OutcomeTrackingStopped,
				Err:     context.Cause(ctx),
			}
		case <-cancelRequested:
			startCancellation("shutdown")
			tryReplace(true)
		case <-poll.C:
			if sweep == nil {
				knownAttempts = len(pending.attempts)
				sweep = newReceiptSweep(pending, knownAttempts)
			}
			if cancelling {
				m.metrics.observePendingAge(pending.req.Label, true, time.Since(cancellationStarted))
			} else if !pending.sentAt.IsZero() {
				m.metrics.observePendingAge(pending.req.Label, false, time.Since(pending.sentAt))
			}
		case <-evaluator.estimated():
			evaluator.finishEstimate(ctx, cancelling)
		case tick := <-replace.C:
			// A cancellation deadline may coincide with this tick. Do not send a
			// second replacement for a tick already covered by that broadcast.
			if !tick.After(replacementStarted) {
				continue
			}
			if !cancelling && pending.cancellationDue(time.Now()) {
				startCancellation("")
			} else if evaluator != nil {
				evaluator.tick(ctx, cancelling)
				continue
			}
			if tryReplace(cancelling) {
				// A fee lookup can cross the deadline and promote this replacement to
				// cancellation. Disarm the expired timer before the next select.
				startCancellation("")
			}
		case <-timeoutC:
			startCancellation("")
			tryReplace(true)
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
		"cancellation", read.attempt.cancellation,
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
	if pending.nonceConflictHash != (common.Hash{}) && m.hasNonceConflict(pending.nonce) {
		if err := m.confirmCanonicalReceipt(ctx, receipt); err != nil {
			observability.Log(ctx).Error(err, "owned receipt cannot reconcile nonce conflict",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
			)
			return Result{}, false
		}
		m.clearNonceConflict(pending.nonce)
	}
	pending.lifecycle.transitionPhase(lifecyclePhaseConfirming)
	m.metrics.clearPendingAge(pending.req.Label)
	confirmations := m.confirmations(pending.req)
	receipt, err := m.waitForConfirmations(ctx, attempt.hash, receipt, confirmations)
	if errors.Is(err, errReceiptReorged) {
		pending.reorged = &reorgedInclusion{attempt: attempt, block: blockNumber(receipt.BlockNumber)}
		pending.lifecycle.transitionPhase(lifecyclePhasePending)
		if pending.nonceConflictHash != (common.Hash{}) {
			m.markNonceConflict(pending.nonce, pending.nonceConflictHash)
		}
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
		outcome := OutcomeIncludedUnconfirmed
		if attempt.cancellation {
			outcome = OutcomeCancelledUnconfirmed
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

// tryReplace reports whether cancellation mode was entered, even if submission fails, plus the
// replacement failure for the calling span. Both are already logged. It is the legacy policy's timed
// replacement, and under either policy the first cancellation (see replace).
func (m *Manager) tryReplace(
	ctx context.Context, pending *pendingTransaction, cancellation bool,
) (bool, error) {
	return m.replace(ctx, pending, cancellation, nil)
}

// replacementPlan is a replacement the horizon policy's pending evaluation decided (strategy §2.7): the fee
// snapshot to price it from, the gas limit of a fill replacement, and why.
type replacementPlan struct {
	// snapshot prices the replacement with repriceFees or cancellationFees; nil prices it with the legacy
	// cached bump, which is the fallback after evaluation reads failed.
	snapshot     *feeSnapshot
	gas          uint64 // fill gas limit; 0 keeps the pending one
	reason       string // repricings_total reason
	cancellation bool   // planned for the cancellation rather than the call
}

// replace signs and broadcasts the next same-nonce attempt, or rebroadcasts one, and reports whether
// cancellation mode was entered (see tryReplace). Every path first checks the mined nonce. Without a plan a
// call is replaced with the legacy cached bump, and a cancellation under the horizon policy is priced from a
// fresh fee snapshot (the legacy bump when that read fails); a plan fixes the snapshot, the gas limit and
// the reason. A cancellation deadline reached meanwhile promotes a call's replacement to cancellation.
func (m *Manager) replace(
	ctx context.Context, pending *pendingTransaction, cancellation bool, plan *replacementPlan,
) (bool, error) {
	if m.hasNonceConflict(pending.nonce) {
		return cancellation, nil
	}
	cancellation = cancellation || pending.cancellationDue(time.Now())
	if available, err := m.replacementNonceAvailable(ctx, pending); err != nil || !available {
		return cancellation, err
	}
	if !cancellation && m.rebroadcastUncertainAttempt(ctx, pending) {
		return false, nil
	}
	fees, gas, err := m.replacementPricing(ctx, pending, cancellation, plan)
	if !cancellation && pending.cancellationDue(time.Now()) {
		return m.replace(ctx, pending, true, plan)
	}
	if err != nil {
		if errors.Is(err, errReplacementLimitReached) &&
			m.rebroadcastLatestAttempt(ctx, pending, cancellation) {
			return cancellation, nil
		}
		observability.Log(ctx).Error(err, "cannot replace pending transaction",
			"label", pending.req.Label,
			"nonce", pending.nonce,
			"cancellation", cancellation,
		)
		return cancellation, err
	}
	to := pending.req.To
	data := pending.req.Data
	value := pending.value
	if cancellation {
		to = m.signer.Address()
		data = nil
		value = new(big.Int)
	}
	if !cancellation && pending.cancellationDue(time.Now()) {
		return m.replace(ctx, pending, true, plan)
	}
	sendCtx, cancelSend := replacementBroadcastContext(ctx, pending, cancellation)
	signed, sendErr := m.signAndSend(sendCtx, pending.nonce, to, data, value, gas, fees, true, cancellation)
	cancelSend()
	if signed == nil {
		observability.Log(ctx).Error(sendErr, "pending transaction replacement rejected",
			"label", pending.req.Label,
			"nonce", pending.nonce,
			"cancellation", cancellation,
		)
		return cancellation, sendErr
	}
	hash := signed.Hash()
	broadcastUncertain := sendErr != nil && !isKnownTransactionError(sendErr)
	pending.fees = cloneFeeQuote(fees)
	if !cancellation {
		pending.gas = gas
	}
	pending.attempts = append(pending.attempts, txAttempt{
		hash: hash, tx: signed, cancellation: cancellation, exactRebroadcastPending: broadcastUncertain,
	})
	m.metrics.observeAttempt(pending.req.Label, cancellation, fees, gas)
	reason := ""
	if plan != nil && plan.cancellation == cancellation {
		reason = plan.reason
		m.metrics.repricing(pending.req.Label, reason)
	}
	if isNonceConsumedError(sendErr) {
		pending.nonceConflictHash = hash
		m.reconcileExistingLifecycleNonce(ctx, pending)
	}
	if broadcastUncertain {
		observability.Log(ctx).Error(sendErr, "replacement broadcast uncertain; tracking signed hash",
			"label", pending.req.Label,
			"hash", hash.Hex(),
			"nonce", pending.nonce,
			"cancellation", cancellation,
		)
		return cancellation, sendErr
	}
	if sendErr != nil {
		observability.Log(ctx).Info("replacement already known by write RPC",
			"label", pending.req.Label,
			"hash", hash.Hex(),
			"nonce", pending.nonce,
			"cancellation", cancellation,
			"rpcResult", sendErr.Error(),
		)
		return cancellation, nil
	}
	kind := replacementKindReplacement
	if cancellation {
		kind = replacementKindCancellation
	}
	m.metrics.replacement(pending.req.Label, kind)
	fields := []any{
		"label", pending.req.Label,
		"hash", hash.Hex(),
		"nonce", pending.nonce,
		"cancellation", cancellation,
		"maxFeePerGas", fees.maxFee.String(),
		"maxPriorityFeePerGas", fees.tip.String(),
	}
	if reason != "" {
		fields = append(fields, "reason", reason, "gasLimit", gas)
	}
	observability.Log(ctx).Info("pending transaction replaced", fields...)
	return cancellation, nil
}

// replacementPricing prices the next same-nonce attempt and returns its fees and gas limit: 21000 for a
// cancellation, the plan's for a planned fill replacement, the pending call's otherwise. A horizon snapshot
// (see replacementSnapshot) prices it with repriceFees or cancellationFees; without one it is the legacy
// cached bump of replacementFees.
func (m *Manager) replacementPricing(
	ctx context.Context, pending *pendingTransaction, cancellation bool, plan *replacementPlan,
) (feeQuote, uint64, error) {
	gas := pending.gas
	switch {
	case cancellation:
		gas = cancellationGasLimit
	case plan != nil && plan.gas > 0:
		gas = plan.gas
	}
	limit := m.normalFeeLimit(pending.req)
	if cancellation {
		limit = m.globalFeeLimit()
	}
	snapshot := m.replacementSnapshot(ctx, pending, cancellation, plan)
	if snapshot == nil {
		fees, err := m.replacementFees(ctx, pending, cancellation, limit)
		return fees, gas, err
	}
	fees, err := m.horizonReplacementFees(ctx, pending, cancellation, gas, limit, snapshot)
	return fees, gas, err
}

// replacementSnapshot is the fee snapshot a horizon-policy replacement is priced from, or nil for the legacy
// cached bump: the plan's snapshot; for an unplanned cancellation (the deadline, shutdown or Obsolete) a
// fresh read; and nil under the legacy policy, for the fallback's planless plan, for an unplanned call
// replacement, or when the cancellation's read fails, so the deadline cancel still fires.
func (m *Manager) replacementSnapshot(
	ctx context.Context, pending *pendingTransaction, cancellation bool, plan *replacementPlan,
) *feeSnapshot {
	switch {
	case m.cfg.Fees.Policy != FeePolicyHorizon:
		return nil
	case plan != nil:
		return plan.snapshot
	case !cancellation:
		return nil
	}
	snapshot, err := m.snapshots.get(ctx, 0)
	if err != nil {
		observability.Log(ctx).Info("cancellation fee snapshot unavailable; bumping the latest fees",
			"label", pending.req.Label, "nonce", pending.nonce, "error", err.Error())
		return nil
	}
	return snapshot
}

// horizonReplacementFees prices a same-nonce replacement under the horizon policy from snapshot: repriceFees
// for the call at gas, cancellationFees for a cancellation (strategy §2.7, §2.8). With the balance guard on,
// the fee cap is also bounded by the signer balance pinned to the snapshot's head (replacementFundedCap, so a
// cancellation falls back to the balance its lifecycle reserved). When the balance or limit cannot fund the
// bump, or the balance cannot be read for a call, the error wraps errReplacementLimitReached, which leads to
// the capped exact rebroadcast, and "replacement capped" is logged once per lifecycle.
func (m *Manager) horizonReplacementFees(
	ctx context.Context, pending *pendingTransaction, cancellation bool, gas uint64, limit *big.Int, snapshot *feeSnapshot,
) (feeQuote, error) {
	value := pending.value
	if cancellation {
		value = new(big.Int)
	}
	required := bumpFee(pending.fees.maxFee)
	var affordable *big.Int
	source := fundedCapBalance
	if m.guardEnabled() {
		var err error
		affordable, source, err = m.replacementFundedCap(ctx, pending, cancellation, gas, value,
			func(ctx context.Context) (*big.Int, error) { return m.snapshotBalance(ctx, snapshot) })
		if err != nil {
			m.logReplacementCapped(ctx, pending, cancellation, source, nil, required, err)
			return feeQuote{}, errors.Errorf("%w: signer balance unavailable: %w", errReplacementLimitReached, err)
		}
	}
	var (
		fees    feeQuote
		binding feeBinding
		err     error
	)
	if cancellation {
		fees, binding, err = cancellationFees(pending.fees, snapshot, affordable, limit, m.horizon)
	} else {
		fees, binding, err = repriceFees(pending.fees, snapshot, gas, affordable, limit, m.horizon)
	}
	if errors.Is(err, errReplacementLimitReached) {
		reason, capped := string(feeBindingCap), limit
		if binding == feeBindingBalance {
			reason, capped = source, affordable
		}
		m.logReplacementCapped(ctx, pending, cancellation, reason, capped, required, nil)
	}
	return fees, err
}

// snapshotBalance reads the signer balance pinned to a fee snapshot's head, as a new send pins it (strategy
// §2.4): a snapshot trailing the next block by more than fees.maxHeadLagBlocks, or ending below the previous
// inclusion block, has no usable pin and is ErrStaleHead.
func (m *Manager) snapshotBalance(ctx context.Context, snapshot *feeSnapshot) (*big.Int, error) {
	pin, err := m.snapshotPin(snapshot.header, snapshot.historyHead, snapshot.lag(time.Now(), m.cfg.Fees.BlockTime))
	if err != nil {
		return nil, errors.Errorf("%w: %w", ErrStaleHead, err)
	}
	return m.pinnedBalance(ctx, pin)
}

// rebroadcastUncertainAttempt gives a transport-ambiguous normal submission one exact-byte retry
// before escalating its fees. It never appends a duplicate attempt or changes the cached fee state.
func (m *Manager) rebroadcastUncertainAttempt(ctx context.Context, pending *pendingTransaction) bool {
	if !m.uncertainRebroadcastDue(pending, time.Now()) {
		return false
	}
	attempt := &pending.attempts[len(pending.attempts)-1]
	attempt.exactRebroadcastPending = false
	err := m.sendSigned(ctx, attempt.tx, true, false)
	known := isKnownTransactionError(err)
	if isNonceConsumedError(err) {
		pending.nonceConflictHash = attempt.hash
		m.reconcileExistingLifecycleNonce(ctx, pending)
	}
	if err == nil || known {
		m.metrics.rebroadcast(pending.req.Label, rebroadcastUncertain)
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

// uncertainRebroadcastDue reports whether the latest attempt is a call whose broadcast was ambiguous and
// that still has the slack for its one exact retry before the cancellation deadline.
func (m *Manager) uncertainRebroadcastDue(pending *pendingTransaction, now time.Time) bool {
	if len(pending.attempts) == 0 || pending.cancellationDue(now) || !m.hasExactRebroadcastSlack(pending, now) {
		return false
	}
	latest := pending.attempts[len(pending.attempts)-1]
	return !latest.cancellation && latest.tx != nil && latest.exactRebroadcastPending
}

// hasExactRebroadcastSlack reports whether an exact rebroadcast still leaves a broadcast and the next
// decision before the cancellation deadline: BroadcastTimeout plus replacementInterval under the legacy
// policy, whose next bump is a replacement interval away, and plus one block time under the horizon policy,
// whose next decision comes with the next head (strategy §2.12 derived timeouts).
func (m *Manager) hasExactRebroadcastSlack(pending *pendingTransaction, now time.Time) bool {
	if pending.cancelDeadline.IsZero() {
		return true
	}
	next := m.cfg.ReplacementInterval
	if m.cfg.Fees.Policy == FeePolicyHorizon {
		next = m.cfg.Fees.BlockTime
	}
	return pending.cancelDeadline.Sub(now) > m.cfg.BroadcastTimeout+next
}

func (m *Manager) rebroadcastLatestAttempt(
	ctx context.Context,
	pending *pendingTransaction,
	cancellation bool,
) bool {
	for _, attempt := range slices.Backward(pending.attempts) {
		if attempt.cancellation != cancellation || attempt.tx == nil {
			continue
		}
		sendCtx, cancelSend := replacementBroadcastContext(ctx, pending, cancellation)
		err := m.sendSigned(sendCtx, attempt.tx, true, cancellation)
		cancelSend()
		if isNonceConsumedError(err) {
			pending.nonceConflictHash = attempt.hash
			m.reconcileExistingLifecycleNonce(ctx, pending)
		}
		if err != nil {
			observability.Log(ctx).Error(err, "capped transaction rebroadcast failed",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
				"cancellation", cancellation,
			)
		} else {
			m.metrics.rebroadcast(pending.req.Label, rebroadcastCapped)
			observability.Log(ctx).Info("capped transaction rebroadcast",
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

// replacementFees prices the next same-nonce attempt under limit. With the balance guard on, the limit
// is also capped at what the signer balance funds at the current head; when that cannot fund the
// required 12.5% bump, or lies below the latest base fee, no new attempt is signed and the capped exact
// rebroadcast of the latest attempt takes over, which becomes valid again if the base fee recedes. An
// unreadable balance stops a fill replacement the same way; a cancellation is instead capped at the
// balance the lifecycle already reserved (see replacementFundedCap), so the deadline cancel still fires.
func (m *Manager) replacementFees(
	ctx context.Context,
	pending *pendingTransaction,
	cancellation bool,
	limit *big.Int,
) (feeQuote, error) {
	if !m.guardEnabled() {
		return m.nextReplacementFees(ctx, pending.fees, limit)
	}
	gas, value := pending.gas, pending.value
	if cancellation {
		gas, value = cancellationGasLimit, new(big.Int)
	}
	required := bumpFee(pending.fees.maxFee)
	affordable, source, err := m.replacementFundedCap(ctx, pending, cancellation, gas, value, m.currentHeadBalance)
	if err != nil {
		m.logReplacementCapped(ctx, pending, cancellation, source, nil, required, err)
		return feeQuote{}, errors.Errorf("%w: signer balance unavailable: %w", errReplacementLimitReached, err)
	}
	if affordable.Cmp(required) < 0 {
		m.logReplacementCapped(ctx, pending, cancellation, source, affordable, required, nil)
		return feeQuote{}, errors.Errorf(
			"%w: signer balance funds %s wei per gas at gas limit %d, below the required bump %s",
			errReplacementLimitReached, affordableString(affordable), gas, required,
		)
	}
	balanceBound := limit == nil || affordable.Cmp(limit) < 0
	if balanceBound {
		limit = affordable
	}
	fees, err := m.nextReplacementFees(ctx, pending.fees, limit)
	if balanceBound && errors.Is(err, errReplacementBaseAboveLimit) {
		// The balance funds the bump but not the latest base fee: hold the latest attempt, which becomes
		// valid again when the base fee recedes, exactly as when the bump itself is unaffordable.
		m.logReplacementCapped(ctx, pending, cancellation, source, affordable, required, nil)
		return feeQuote{}, errors.Errorf("%w: %w", errReplacementLimitReached, err)
	}
	return fees, err
}

// Sources of the fee cap a guarded replacement is funded to, for the "replacement capped" log.
const (
	fundedCapBalance            = "balance"
	fundedCapReserved           = "reserved_balance"
	fundedCapBalanceUnavailable = "balance_unavailable"
)

// replacementFundedCap is the largest fee cap a replacement at gas and value can be funded to, and where
// it came from. It is normally the signer balance readBalance reads at the current head: pinned to the
// latest header under the legacy policy, to the evaluation's fee snapshot under the horizon policy. When
// that read fails, a
// cancellation falls back to the balance the lifecycle already reserved: every attempt was checked
// against the signer balance when it was signed, and their shared nonce is not mined (the replacement
// nonce check ran first), so that balance still holds the costliest of them. A cancellation must not
// wait for a read endpoint to recover: it is what settles the nonce at the request's deadline.
func (m *Manager) replacementFundedCap(
	ctx context.Context, pending *pendingTransaction, cancellation bool, gas uint64, value *big.Int,
	readBalance func(context.Context) (*big.Int, error),
) (*big.Int, string, error) {
	balance, err := readBalance(ctx)
	if err == nil {
		return affordableMaxFee(balance, value, gas), fundedCapBalance, nil
	}
	if !cancellation {
		return nil, fundedCapBalanceUnavailable, err
	}
	reserved, ok := reservedFeeCap(pending.attempts, gas, value)
	if !ok {
		return nil, fundedCapBalanceUnavailable, err
	}
	if !pending.reservedCapLogged {
		pending.reservedCapLogged = true
		observability.Log(ctx).Info("cancellation capped at the balance its lifecycle reserved",
			"label", pending.req.Label,
			"nonce", pending.nonce,
			"reservedMaxFeePerGas", affordableString(reserved),
			"error", err.Error(),
		)
	}
	return reserved, fundedCapReserved, nil
}

// reservedFeeCap is the largest fee cap per gas at gas and value that the costliest signed attempt's
// gasLimit × maxFee + value covers. It reports false when no attempt carries its signed transaction.
func reservedFeeCap(attempts []txAttempt, gas uint64, value *big.Int) (*big.Int, bool) {
	var reserved *big.Int
	for _, attempt := range attempts {
		if attempt.tx == nil {
			continue
		}
		if cost := attempt.tx.Cost(); reserved == nil || cost.Cmp(reserved) > 0 {
			reserved = cost
		}
	}
	if reserved == nil {
		return nil, false
	}
	return affordableMaxFee(reserved, value, gas), true
}

// logReplacementCapped reports, once per lifecycle, that the balance stopped a replacement from being
// signed. The capped exact rebroadcast that follows logs every attempt itself.
func (m *Manager) logReplacementCapped(
	ctx context.Context,
	pending *pendingTransaction,
	cancellation bool,
	reason string,
	affordable, required *big.Int,
	err error,
) {
	if pending.balanceCapLogged {
		return
	}
	pending.balanceCapLogged = true
	fields := []any{
		"label", pending.req.Label,
		"nonce", pending.nonce,
		"cancellation", cancellation,
		"reason", reason,
		"requiredMaxFeePerGas", required.String(),
	}
	if affordable != nil {
		fields = append(fields, "affordableMaxFeePerGas", affordableString(affordable))
	}
	if err != nil {
		fields = append(fields, "error", err.Error())
	}
	observability.Log(ctx).Info("replacement capped", fields...)
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
		observability.Log(ctx).V(1).Info("fresh replacement fees unavailable; using cached bump", "error", err)
	}
	if limit != nil && next.maxFee.Cmp(limit) > 0 {
		next.maxFee.Set(limit)
	}
	effectiveTipLimit := new(big.Int).Sub(next.maxFee, next.baseFee)
	if effectiveTipLimit.Sign() < 0 {
		return feeQuote{}, errors.Errorf(
			"%w: base fee %s, fee limit %s", errReplacementBaseAboveLimit, next.baseFee, next.maxFee,
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
	reading, err := m.readFees(ctx)
	if err != nil {
		return feeQuote{}, err
	}
	return m.legacyQuote(reading, limit)
}

// readFees reads the legacy fee rule's inputs within the fee-read budget: the latest header and either
// the fee-history rewards (tipGwei 0) or the node's tip suggestion floored at tipGwei.
func (m *Manager) readFees(ctx context.Context) (feeReading, error) {
	feeCtx, cancel := context.WithTimeout(ctx, m.feeReadTimeout())
	defer cancel()

	head, err := m.backend.HeaderByNumber(feeCtx, nil)
	if err != nil {
		return feeReading{}, errors.Errorf("%w: header by number: %w", errFreshFeesUnavailable, err)
	}
	if head == nil || head.BaseFee == nil || head.BaseFee.Sign() < 0 {
		return feeReading{}, errors.Errorf("%w: latest header must contain a non-negative base fee", errFreshFeesUnavailable)
	}

	tipFloor := gweiToWei(m.cfg.TipGwei)
	if tipFloor.Sign() == 0 {
		history, historyErr := m.backend.FeeHistory(
			feeCtx, feeHistoryBlocks, nil, []float64{feeHistoryPercentile},
		)
		if historyErr != nil {
			return feeReading{}, errors.Errorf("%w: fee history: %w", errFreshFeesUnavailable, historyErr)
		}
		tip, valid := feeHistoryTip(history)
		if !valid {
			return feeReading{}, errors.Errorf("%w: invalid fee history rewards", errFreshFeesUnavailable)
		}
		return feeReading{head: head, tip: tip, history: history}, nil
	}
	suggestedTip, tipErr := m.backend.SuggestGasTipCap(feeCtx)
	switch {
	case tipErr == nil && suggestedTip != nil && suggestedTip.Sign() >= 0:
		return feeReading{head: head, tip: maxBigCopy(suggestedTip, tipFloor)}, nil
	case ctx.Err() != nil:
		return feeReading{}, errors.Errorf("%w: suggest gas tip: %w", errFreshFeesUnavailable, ctx.Err())
	default:
		return feeReading{head: head, tip: tipFloor}, nil
	}
}

// legacyQuote prices one fee reading under the legacy rule: 2×base + tip, capped at limit, with the tip
// clamped under the cap and a positive tipGwei kept mandatory.
func (m *Manager) legacyQuote(reading feeReading, limit *big.Int) (feeQuote, error) {
	baseFee := new(big.Int).Set(reading.head.BaseFee)
	tip := new(big.Int).Set(reading.tip)
	tipFloor := gweiToWei(m.cfg.TipGwei)

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
	tips := make([]*big.Int, 0, len(history.Reward))
	for _, blockRewards := range history.Reward {
		if len(blockRewards) != 1 || blockRewards[0] == nil || blockRewards[0].Sign() < 0 {
			return nil, false
		}
		tips = append(tips, blockRewards[0])
	}
	slices.SortFunc(tips, func(a, b *big.Int) int { return a.Cmp(b) })
	return new(big.Int).Set(tips[len(tips)/2]), true
}

// estimateGas is the legacy policy's estimate: a plain estimate at latest with gas.headroomBps.
func (m *Manager) estimateGas(ctx context.Context, req Request) (uint64, error) {
	started := time.Now()
	gas, err := m.backend.EstimateGas(ctx, m.callMsg(req))
	m.observeGasEstimate(ctx, req.Label, estimateModeLatest, started, err)
	if err != nil {
		// An estimate broadcast abandoned because the send failed first is not a failure of its own.
		if !errors.Is(context.Cause(ctx), errEstimateAbandoned) {
			// Calldata can contain unpublished authorizations. Keep it out of error logs and Sentry.
			observability.Log(ctx).Error(err, "gas estimation failed",
				"label", req.Label,
			)
		}
		return 0, errors.Errorf("estimate gas %q: %w", req.Label, err)
	}
	limit, err := gasLimitWithHeadroom(gas, m.gasHeadroomBps())
	if err != nil {
		return 0, errors.Errorf("estimate gas %q: %w", req.Label, err)
	}
	return limit, nil
}

// gasLimitWithHeadroom adds bps basis points of the estimate to it, rounding the headroom down, so the
// default 500 bps signs exactly the estimate + estimate/20 the manager always has. The product is
// computed in 128 bits; a limit that does not fit in 64 bits is an error rather than a wrapped value.
func gasLimitWithHeadroom(estimate, bps uint64) (uint64, error) {
	hi, lo := bits.Mul64(estimate, bps)
	if hi >= basisPoints {
		return 0, errors.Errorf("gas limit overflows: estimate %d with %d bps headroom", estimate, bps)
	}
	headroom, _ := bits.Div64(hi, lo, basisPoints)
	limit, carry := bits.Add64(estimate, headroom, 0)
	if carry != 0 {
		return 0, errors.Errorf("gas limit overflows: estimate %d with %d bps headroom", estimate, bps)
	}
	return limit, nil
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
	cancellation bool,
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
	sendErr := m.sendSigned(ctx, signed, existingLifecycle, cancellation)
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
	cancellation bool,
) error {
	if !existingLifecycle {
		if err := m.nonceConflictError(); err != nil {
			return err
		}
	}
	sendCtx, cancel := context.WithTimeout(ctx, m.broadcastTimeout())
	defer cancel()
	if cancellation {
		if backend, ok := m.backend.(cancellationBackend); ok {
			return backend.SendCancellationTransaction(sendCtx, signed)
		}
	}
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
			observability.Log(ctx).Error(err, "tracked receipt unavailable during nonce reconciliation",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
			)
			continue
		}
		if err := validateReceipt(attempt.hash, receipt); err != nil {
			observability.Log(ctx).Error(err, "invalid tracked receipt during nonce reconciliation",
				"label", pending.req.Label,
				"hash", attempt.hash.Hex(),
				"nonce", pending.nonce,
			)
			continue
		}
		if err := m.confirmCanonicalReceipt(lookupCtx, receipt); err != nil {
			observability.Log(ctx).Error(err, "tracked receipt is not canonically visible during nonce reconciliation",
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

// pendingTick is the cadence of a pending lifecycle's replacement ticker, which also sets the read budgets
// below: under the horizon policy it is blockTime/2, so every new head is evaluated within half a slot
// (strategy §2.7, §2.12 derived timeouts); under the legacy policy it is replacementInterval, the fee-bump
// cadence.
func (m *Manager) pendingTick() time.Duration {
	if m.cfg.Fees.Policy == FeePolicyHorizon {
		return max(m.cfg.Fees.BlockTime/2, time.Millisecond)
	}
	return m.cfg.ReplacementInterval
}

// feeReadTimeout bounds one fee read: min(1s, tick/2).
func (m *Manager) feeReadTimeout() time.Duration {
	return minPositiveDuration(maxFeeReadTimeout, m.pendingTick()/2)
}

// receiptReadTimeout bounds one receipt, mined-nonce or obsolescence read: min(2s, tick/2).
func (m *Manager) receiptReadTimeout() time.Duration {
	return minPositiveDuration(maxReceiptReadTimeout, m.pendingTick()/2)
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
