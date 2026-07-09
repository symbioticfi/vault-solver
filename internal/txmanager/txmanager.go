// Package txmanager owns the on-chain sending account. A single dispatcher serializes nonce
// allocation and initial broadcasts; manager-owned trackers supervise admitted transactions without
// blocking later nonces. Solvers build calldata and hand it over via Send; they never sign or
// broadcast directly.
package txmanager

import (
	"context"
	"math"
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
	Confirmations   uint64        // blocks to wait past inclusion before returning
	MaxFeeGwei      float64       // cap on max fee per gas; 0 => derive from base fee
	TipGwei         float64       // priority fee; 0 => use the node's suggestion
	PollInterval    time.Duration // receipt/confirmation poll cadence; 0 => 2s
	PendingInterval time.Duration // one pending-attempt window; 0 => 2m
	FeeBumpBps      uint64        // replacement fee increase in basis points; 0 => 1250
	MaxReplacements uint64        // replacements after the original attempt; 0 => 3
}

// Request is a transaction to send. Value nil means 0; GasLimit 0 means "estimate".
type Request struct {
	To       common.Address
	Data     []byte
	Value    *big.Int
	GasLimit uint64
	Label    string // for logs/metrics, e.g. "redeem"
}

// State is the explicit lifecycle outcome of a transaction request.
type State string

const (
	StateNotBroadcast     State = "not_broadcast"
	StateRejected         State = "rejected"
	StateBroadcastUnknown State = "broadcast_unknown"
	StatePending          State = "pending"
	StateConfirmed        State = "confirmed"
	StateReverted         State = "reverted"
	StateUnresolved       State = "unresolved"
)

var (
	ErrManagerStopped = errors.New("txmanager stopped")
	ErrUnresolved     = errors.New("transaction outcome unresolved")
)

// Result carries the final outcome of a Send.
type Result struct {
	State   State
	Nonce   uint64
	Hash    common.Hash
	Hashes  []common.Hash
	Receipt *types.Receipt
	Err     error
}

// SafeToRetry reports whether the logical request definitely was not admitted to the transaction
// pool and may safely be submitted again.
func (r Result) SafeToRetry() bool {
	return r.State == StateNotBroadcast || r.State == StateRejected
}

// Manager is the single-writer transaction dispatcher.
type Manager struct {
	backend Backend
	signer  signer.Signer
	chainID *big.Int
	cfg     Config
	log     logr.Logger

	queue chan job
	done  chan struct{}

	mu             sync.Mutex // guards the local nonce state
	nonce          uint64
	nonceInit      bool
	nonceFloor     uint64
	nonceFloorSet  bool
	nonceExhausted bool
}

type job struct {
	req Request
	res chan Result
}

const (
	defaultPollInterval    = 2 * time.Second
	defaultPendingInterval = 2 * time.Minute
	defaultFeeBumpBps      = 1_250
	defaultMaxReplacements = 3
)

// New constructs a Manager. Call Start to launch its worker.
func New(backend Backend, s signer.Signer, chainID *big.Int, cfg Config, log logr.Logger) *Manager {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	if cfg.PendingInterval <= 0 {
		cfg.PendingInterval = defaultPendingInterval
	}
	if cfg.FeeBumpBps == 0 {
		cfg.FeeBumpBps = defaultFeeBumpBps
	}
	if cfg.MaxReplacements == 0 {
		cfg.MaxReplacements = defaultMaxReplacements
	}
	return &Manager{
		backend: backend,
		signer:  s,
		chainID: new(big.Int).Set(chainID),
		cfg:     cfg,
		log:     log.WithName("txmanager"),
		queue:   make(chan job),
		done:    make(chan struct{}),
	}
}

// Start runs the dispatcher until ctx is cancelled, then joins every transaction tracker before
// returning. Run it in its own goroutine.
func (m *Manager) Start(ctx context.Context) error {
	m.log.Info("started", "from", m.signer.Address().Hex())
	var trackers sync.WaitGroup
	defer func() {
		trackers.Wait()
		close(m.done)
		m.log.Info("stopped")
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case j := <-m.queue:
			tracked, immediate := m.dispatch(ctx, j.req)
			if immediate != nil {
				j.res <- *immediate
				continue
			}
			trackers.Add(1)
			go func(tracked *trackedTx, result chan<- Result) {
				defer trackers.Done()
				result <- m.track(ctx, tracked)
			}(tracked, j.res)
		}
	}
}

// Send enqueues a transaction and blocks until it reaches a final state. Safe for concurrent
// callers; preparation and initial broadcast are serialized through the dispatcher while receipt
// tracking runs concurrently.
//
// ctx governs the enqueue only. Before the request is enqueued, a cancelled ctx aborts cleanly with
// no transaction sent. Once enqueued, the worker broadcasts the tx on the manager's own long-lived
// context, so Send waits for and returns that real outcome — it must not report a cancellation while
// the transaction still lands on-chain, which a caller would read as "not sent" (the caller's ctx is
// typically an errgroup child that cancels the instant any sibling solver errors, well before
// shutdown). The worker always delivers exactly one Result, so this wait cannot hang.
func (m *Manager) Send(ctx context.Context, req Request) Result {
	if err := ctx.Err(); err != nil {
		return Result{State: StateNotBroadcast, Err: err}
	}
	res := make(chan Result, 1)
	select {
	case m.queue <- job{req: req, res: res}:
	case <-ctx.Done():
		return Result{State: StateNotBroadcast, Err: ctx.Err()}
	case <-m.done:
		return Result{State: StateNotBroadcast, Err: ErrManagerStopped}
	}
	return <-res
}

// dispatch runs only on the dispatcher goroutine. It prepares, signs, and initially broadcasts one
// logical transaction before handing admitted or ambiguous outcomes to a tracker.
func (m *Manager) dispatch(ctx context.Context, req Request) (*trackedTx, *Result) {
	tip, maxFee, err := m.fees(ctx)
	if err != nil {
		result := rejectedResult(0, nil, err)
		return nil, &result
	}

	gas := req.GasLimit
	if gas == 0 {
		gas, err = m.estimateGas(ctx, req)
		if err != nil {
			result := rejectedResult(0, nil, err)
			return nil, &result
		}
	}

	value := req.Value
	if value == nil {
		value = new(big.Int)
	}

	nonce, err := m.seedNonce(ctx)
	if err != nil {
		result := rejectedResult(0, nil, err)
		return nil, &result
	}

	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   m.chainID,
		Nonce:     nonce,
		GasTipCap: tip,
		GasFeeCap: maxFee,
		Gas:       gas,
		To:        &req.To,
		Value:     value,
		Data:      req.Data,
	})

	signed, err := m.signer.SignTx(tx, m.chainID)
	if err != nil {
		result := rejectedResult(nonce, nil, errors.Errorf("sign tx %q: %w", req.Label, err))
		return nil, &result
	}

	sendErr := m.backend.SendTransaction(ctx, signed)
	class := classifyBroadcastError(sendErr)
	if class == broadcastRejected {
		result := rejectedResult(nonce, signed, errors.Errorf("send %q: %w", req.Label, sendErr))
		return nil, &result
	}

	m.commitNonce(nonce)
	tracked := &trackedTx{
		req:      req,
		nonce:    nonce,
		state:    StatePending,
		attempts: []*types.Transaction{signed},
	}
	if class == broadcastAmbiguous {
		tracked.state = StateBroadcastUnknown
		tracked.admissionErr = errors.Errorf("send %q: %w", req.Label, sendErr)
		if isNonceTooLow(sendErr) {
			m.invalidateNonceSeed()
		}
		m.log.Info("broadcast outcome unknown", "label", req.Label, "hash", signed.Hash().Hex(), "nonce", nonce)
	} else {
		m.log.Info("sent", "label", req.Label, "hash", signed.Hash().Hex(), "nonce", nonce)
	}
	return tracked, nil
}

// fees computes the EIP-1559 tip and max-fee-per-gas.
func (m *Manager) fees(ctx context.Context) (tip, maxFee *big.Int, err error) {
	if m.cfg.TipGwei > 0 {
		tip = gweiToWei(m.cfg.TipGwei)
	} else {
		tip, err = m.backend.SuggestGasTipCap(ctx)
		if err != nil {
			return nil, nil, errors.Errorf("suggest gas tip: %w", err)
		}
	}

	head, err := m.backend.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, nil, errors.Errorf("header by number: %w", err)
	}
	baseFee := head.BaseFee
	if baseFee == nil {
		baseFee = new(big.Int)
	}

	if m.cfg.MaxFeeGwei > 0 {
		maxFee = gweiToWei(m.cfg.MaxFeeGwei)
	} else {
		// 2*baseFee + tip leaves headroom for one base-fee doubling between now and inclusion.
		maxFee = new(big.Int).Add(new(big.Int).Mul(baseFee, big.NewInt(2)), tip)
	}
	if maxFee.Cmp(tip) < 0 {
		return nil, nil, errors.Errorf("selected gas tip %s wei exceeds max fee %s wei", tip, maxFee)
	}
	return tip, maxFee, nil
}

func (m *Manager) estimateGas(ctx context.Context, req Request) (uint64, error) {
	gas, err := m.backend.EstimateGas(ctx, ethereum.CallMsg{
		From:  m.signer.Address(),
		To:    &req.To,
		Value: req.Value,
		Data:  req.Data,
	})
	if err != nil {
		return 0, errors.Errorf("estimate gas %q: %w", req.Label, err)
	}
	// 20% headroom over the estimate.
	return gas + gas/5, nil
}

// seedNonce returns the current dispatcher-owned nonce candidate, seeding it from the backend when
// necessary without ever going below the persistent committed floor.
func (m *Manager) seedNonce(ctx context.Context) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nonceExhausted {
		return 0, errors.New("transaction nonce space exhausted")
	}
	if m.nonceInit {
		return m.nonce, nil
	}

	pending, err := m.backend.PendingNonceAt(ctx, m.signer.Address())
	if err != nil {
		return 0, errors.Errorf("pending nonce: %w", err)
	}
	if m.nonceFloorSet && pending < m.nonceFloor {
		pending = m.nonceFloor
	}
	m.nonce = pending
	m.nonceInit = true
	return m.nonce, nil
}

func (m *Manager) commitNonce(used uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if used == math.MaxUint64 {
		m.nonce = used
		m.nonceInit = false
		m.nonceFloor = used
		m.nonceFloorSet = true
		m.nonceExhausted = true
		return
	}

	next := used + 1
	if !m.nonceFloorSet || next > m.nonceFloor {
		m.nonceFloor = next
		m.nonceFloorSet = true
	}
	m.nonce = next
	m.nonceInit = true
}

func (m *Manager) invalidateNonceSeed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nonceInit = false
}

func rejectedResult(nonce uint64, signed *types.Transaction, err error) Result {
	result := Result{State: StateRejected, Nonce: nonce, Err: err}
	if signed != nil {
		result.Hash = signed.Hash()
		result.Hashes = []common.Hash{signed.Hash()}
	}
	return result
}

type broadcastClass uint8

const (
	broadcastAdmitted broadcastClass = iota
	broadcastRejected
	broadcastAmbiguous
)

func classifyBroadcastError(err error) broadcastClass {
	if err == nil {
		return broadcastAdmitted
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "already known") {
		return broadcastAdmitted
	}
	for _, rejection := range []string{
		"insufficient funds",
		"intrinsic gas too low",
		"invalid sender",
		"max fee per gas less than block base fee",
		"max priority fee per gas higher than max fee per gas",
		"transaction type not supported",
	} {
		if strings.Contains(message, rejection) {
			return broadcastRejected
		}
	}
	return broadcastAmbiguous
}

func gweiToWei(gwei float64) *big.Int {
	wei, _ := new(big.Float).Mul(big.NewFloat(gwei), big.NewFloat(params.GWei)).Int(nil)
	return wei
}

func isNonceTooLow(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "nonce too low")
}
