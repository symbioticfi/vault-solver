// Package txmanager is the exclusive owner of one EVM account nonce lane.
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

// Backend is the read/write port used by the nonce state machine.
type Backend interface {
	NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	TransactionSenderBalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error)
	FeeHistory(
		ctx context.Context, blockCount uint64, lastBlock *big.Int, rewardPercentiles []float64,
	) (*ethereum.FeeHistory, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
}

type Config struct {
	Confirmations       uint64
	MaxFeeGwei          float64
	TipGwei             float64
	PollInterval        time.Duration
	BroadcastTimeout    time.Duration
	AccountPollInterval time.Duration
	ReplacementInterval time.Duration
	PendingTimeout      time.Duration
	ShutdownTimeout     time.Duration
}

type Request struct {
	To           common.Address
	Data         []byte
	MaxFeePerGas *big.Int
	CancelAt     time.Time
	Obsolete     func(context.Context) (bool, error)
	Label        string
}

type Result struct {
	Hash        common.Hash
	Receipt     *types.Receipt
	Outcome     Outcome
	Err         error
	NotAdmitted bool
}

type Outcome string

const (
	OutcomeConfirmed           Outcome = "confirmed"
	OutcomeIncludedUnconfirmed Outcome = "included_unconfirmed"
	OutcomeReverted            Outcome = "reverted"
	OutcomeCancelled           Outcome = "cancelled"
	OutcomeSubmissionError     Outcome = "submission_error"
	OutcomeTrackingStopped     Outcome = "tracking_stopped"
)

type feeQuote struct {
	baseFee *big.Int
	tip     *big.Int
	maxFee  *big.Int
}

type txAttempt struct {
	hash                    common.Hash
	tx                      *types.Transaction
	cancellation            bool
	exactRebroadcastPending bool
}

type pendingTransaction struct {
	req               Request
	nonce             uint64
	gas               uint64
	fees              feeQuote
	attempts          []txAttempt
	receiptCursor     int
	nonceConflictHash common.Hash
	originalHash      common.Hash
	cancelDeadline    time.Time
	lifecycle         lifecycleObservation
}

type nonceConflict struct {
	nonce uint64
	hash  common.Hash
}

type job struct {
	req              Request
	res              chan Result
	admissionStarted time.Time
}

// Manager contains three synchronized state domains: worker-owned admission, mutex-owned nonce
// identity, and mutex-owned active lifecycle. Network I/O never runs while those mutexes are held.
type Manager struct {
	backend Backend
	signer  signer.Signer
	chainID *big.Int
	cfg     Config
	metrics *Metrics
	log     logr.Logger

	queue           chan job
	stopping        chan struct{}
	stopOnce        sync.Once
	admissionDemand atomic.Int64

	laneStateMu          sync.Mutex
	laneStateSubscribers map[uint64]chan struct{}
	nextLaneStateID      uint64

	mu        sync.Mutex
	nonce     uint64
	nonceInit bool
	conflict  *nonceConflict
}

const (
	defaultPollInterval        = 2 * time.Second
	defaultReplacementInterval = 30 * time.Second
	defaultPendingTimeout      = 5 * time.Minute
	defaultShutdownTimeout     = time.Minute
	defaultBroadcastTimeout    = 5 * time.Second
	defaultAccountPollInterval = 30 * time.Second
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
	errShutdownTimeout         = errors.Errorf(
		"transaction manager shutdown drain timed out: %w",
		context.DeadlineExceeded,
	)
)

func New(
	backend Backend,
	transactionSigner signer.Signer,
	chainID *big.Int,
	config Config,
	log logr.Logger,
) *Manager {
	config = normalizedConfig(config)
	var immutableChainID *big.Int
	if chainID != nil {
		immutableChainID = new(big.Int).Set(chainID)
	}
	return &Manager{
		backend:              backend,
		signer:               transactionSigner,
		chainID:              immutableChainID,
		cfg:                  config,
		log:                  log.WithName("txmanager"),
		queue:                make(chan job),
		stopping:             make(chan struct{}),
		laneStateSubscribers: make(map[uint64]chan struct{}),
	}
}

func NewWithMetrics(
	backend Backend,
	transactionSigner signer.Signer,
	chainID *big.Int,
	config Config,
	metrics *Metrics,
	log logr.Logger,
) *Manager {
	manager := New(backend, transactionSigner, chainID, config, log)
	manager.metrics = metrics
	return manager
}

func normalizedConfig(config Config) Config {
	if config.PollInterval <= 0 {
		config.PollInterval = defaultPollInterval
	}
	if config.BroadcastTimeout <= 0 {
		config.BroadcastTimeout = defaultBroadcastTimeout
	}
	if config.AccountPollInterval <= 0 {
		config.AccountPollInterval = defaultAccountPollInterval
	}
	if config.ReplacementInterval <= 0 {
		config.ReplacementInterval = defaultReplacementInterval
	}
	if config.PendingTimeout <= 0 {
		config.PendingTimeout = defaultPendingTimeout
	}
	if config.ShutdownTimeout <= 0 {
		config.ShutdownTimeout = defaultShutdownTimeout
	}
	return config
}

func (m *Manager) Confirmations() uint64 {
	return m.cfg.Confirmations
}

func (m *Manager) ValidateFeeHeadroom() error {
	initialLimit := reserveFeeBump(m.normalFeeLimit(Request{}))
	tipFloor := gweiToWei(m.cfg.TipGwei)
	if initialLimit != nil && tipFloor.Sign() > 0 && tipFloor.Cmp(initialLimit) >= 0 {
		return errors.Errorf(
			"tip floor %s leaves no base-fee headroom under initial fee limit %s after reserved replacement bumps",
			tipFloor,
			initialLimit,
		)
	}
	return nil
}

func (m *Manager) Available() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.conflict == nil
}

func (m *Manager) LaneReady() bool {
	return m.Available() && m.admissionDemand.Load() == 0
}

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

func (m *Manager) Initialize(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.initializeNonceLocked(ctx)
}

// Start serializes the complete lifecycle of each accepted transaction. One worker owns both the
// nonce and the active transaction, so no second lifecycle lock or active-transaction registry is needed.
func (m *Manager) Start(ctx context.Context) {
	m.metrics.bindAccount(m.signer.Address())
	accountDone := make(chan struct{})
	go func() { defer close(accountDone); m.monitorAccount(ctx) }()
	defer func() { <-accountDone }()
	m.log.Info("started", "from", m.signer.Address().Hex())
	defer m.markStopped()
	for {
		select {
		case <-ctx.Done():
			m.log.Info("stopped", "reason", ctx.Err().Error())
			return
		case work := <-m.queue:
			if err := ctx.Err(); err != nil {
				m.metrics.finishAdmission(work.req.Label, work.admissionStarted, errManagerStopped)
				work.res <- notAdmittedResult(err)
				m.releaseAdmissionDemand()
				m.log.Info("stopped", "reason", err.Error())
				return
			}
			if err := m.nonceConflictError(); err != nil {
				m.metrics.finishAdmission(work.req.Label, work.admissionStarted, err)
				work.res <- notAdmittedResult(err)
				m.releaseAdmissionDemand()
				continue
			}
			m.metrics.finishAdmission(work.req.Label, work.admissionStarted, nil)
			lifecycle := m.metrics.beginLifecycle(work.req.Label)
			pending, err := m.broadcast(ctx, work.req)
			if err != nil {
				outcome := OutcomeSubmissionError
				if ctx.Err() != nil {
					outcome = OutcomeTrackingStopped
				}
				lifecycle.finish(outcome, nil)
				work.res <- Result{
					Outcome:     outcome,
					Err:         err,
					NotAdmitted: errors.Is(err, errNonceLanePaused) || ctx.Err() != nil,
				}
				m.releaseAdmissionDemand()
				continue
			}
			lifecycle.transitionPhase(lifecyclePhasePending)
			pending.lifecycle = lifecycle
			result := m.completeLifecycle(ctx, pending)
			pending.lifecycle.finish(result.Outcome, result.Receipt)
			work.res <- result
			m.releaseAdmissionDemand()
			if err := ctx.Err(); err != nil {
				m.log.Info("stopped", "reason", err.Error())
				return
			}
		}
	}
}

func (m *Manager) markStopped() {
	m.stopOnce.Do(func() {
		close(m.stopping)
		m.notifyLaneStateChange()
	})
}
