// Package txmanager owns the on-chain sending account. One worker serializes admission, fee
// selection, signing, nonce assignment, and broadcasts so solvers cannot race on the account nonce.
// Only one signed lifecycle may be unresolved at a time; solvers build calldata and hand it over via
// Send or SendAsync, but never sign or broadcast directly.
package txmanager

import (
	"context"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

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

// Config tunes fee selection and confirmation behavior.
type Config struct {
	Confirmations       uint64        // blocks to wait past inclusion before returning
	MaxFeeGwei          float64       // absolute max fee per gas; app config requires a positive value
	TipGwei             float64       // minimum priority fee; 0 => derive it from recent fee history
	PollInterval        time.Duration // receipt/confirmation poll cadence; 0 => 2s
	BroadcastTimeout    time.Duration // maximum duration of one transaction submission RPC; 0 => 5s
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
	Label         string  // for logs/metrics, e.g. "redeem"
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
	defaultBroadcastTimeout    = 5 * time.Second
	maxFeeReadTimeout          = time.Second
	maxReceiptReadTimeout      = 2 * time.Second
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

// LaneReady reports whether the nonce lane is both safe and idle, so a solver can make an external
// commitment that requires prompt transaction admission.
func (m *Manager) LaneReady() bool {
	return m.Available() && m.admissionDemand.Load() == 0
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
	result, accepted := m.sendAsync(ctx, req)
	if !accepted {
		return notAdmittedResult(ctx.Err())
	}
	return <-result
}

// SendAsync waits without accepting or signing while another lifecycle is unresolved, then enqueues
// one transaction and returns its eventual receipt result. ctx and CancelAt can still stop this wait;
// a deadline or manager stop returns a terminal pre-admission error without signing. Once enqueued,
// the manager owns the broadcast and receipt lifecycle.
func (m *Manager) SendAsync(ctx context.Context, req Request) (<-chan Result, bool) {
	return m.sendAsync(ctx, req)
}
