// Package txmanager owns the on-chain sending account and serializes all transactions through a
// single worker goroutine, so multiple solvers can never race on the account nonce. Solvers build
// calldata and hand it over via Send; they never sign or broadcast directly.
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
	Confirmations uint64        // blocks to wait past inclusion before returning
	MaxFeeGwei    float64       // cap on max fee per gas; 0 => derive from base fee
	TipGwei       float64       // priority fee; 0 => use the node's suggestion
	PollInterval  time.Duration // receipt/confirmation poll cadence; 0 => 2s
}

// Request is a transaction to send. Value nil means 0; GasLimit 0 means "estimate".
type Request struct {
	To       common.Address
	Data     []byte
	Value    *big.Int
	GasLimit uint64
	Label    string // for logs/metrics, e.g. "redeem"
}

// Result carries the outcome of a Send.
type Result struct {
	Hash    common.Hash
	Receipt *types.Receipt
	Err     error
}

// Manager is the single-writer transaction sender.
type Manager struct {
	backend Backend
	signer  signer.Signer
	chainID *big.Int
	cfg     Config
	log     logr.Logger

	queue chan job

	mu        sync.Mutex // guards the local nonce
	nonce     uint64
	nonceInit bool
}

type job struct {
	req Request
	res chan Result
}

const (
	defaultPollInterval = 2 * time.Second
	maxNonceResyncs     = 1
)

// New constructs a Manager. Call Start to launch its worker.
func New(backend Backend, s signer.Signer, chainID *big.Int, cfg Config, log logr.Logger) *Manager {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	return &Manager{
		backend: backend,
		signer:  s,
		chainID: chainID,
		cfg:     cfg,
		log:     log.WithName("txmanager"),
		queue:   make(chan job),
	}
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
			j.res <- m.execute(ctx, j.req)
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
// shutdown). The worker always delivers exactly one Result, so this wait cannot hang.
func (m *Manager) Send(ctx context.Context, req Request) Result {
	res := make(chan Result, 1)
	select {
	case m.queue <- job{req: req, res: res}:
	case <-ctx.Done():
		return Result{Err: ctx.Err()}
	}
	return <-res
}

// execute runs on the worker goroutine only, so nonce access is single-threaded here; the mutex
// guards against concurrent reads from a future status API.
func (m *Manager) execute(ctx context.Context, req Request) Result {
	tip, maxFee, err := m.fees(ctx)
	if err != nil {
		return Result{Err: err}
	}

	gas := req.GasLimit
	if gas == 0 {
		gas, err = m.estimateGas(ctx, req)
		if err != nil {
			return Result{Err: err}
		}
	}

	value := req.Value
	if value == nil {
		value = new(big.Int)
	}

	var lastErr error
	for attempt := 0; attempt <= maxNonceResyncs; attempt++ {
		nonce, nErr := m.nextNonce(ctx, attempt > 0)
		if nErr != nil {
			return Result{Err: nErr}
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

		signed, sErr := m.signer.SignTx(tx, m.chainID)
		if sErr != nil {
			return Result{Err: sErr}
		}

		if sendErr := m.backend.SendTransaction(ctx, signed); sendErr != nil {
			lastErr = sendErr
			if isNonceTooLow(sendErr) {
				m.log.Info("nonce too low; resyncing", "label", req.Label, "nonce", nonce)
				continue // retry with a freshly-synced nonce
			}
			return Result{Err: errors.Errorf("send %q: %w", req.Label, sendErr)}
		}

		m.commitNonce(nonce)
		hash := signed.Hash()
		m.log.Info("sent", "label", req.Label, "hash", hash.Hex(), "nonce", nonce)

		receipt, wErr := m.waitForReceipt(ctx, hash)
		return Result{Hash: hash, Receipt: receipt, Err: wErr}
	}
	return Result{Err: errors.Errorf("send %q: exhausted nonce resyncs: %w", req.Label, lastErr)}
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
		maxFee = new(big.Int).Set(tip)
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

func (m *Manager) waitForReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()

	var receipt *types.Receipt
	for {
		if receipt == nil {
			r, err := m.backend.TransactionReceipt(ctx, hash)
			if err == nil {
				receipt = r
				if receipt.Status == types.ReceiptStatusFailed {
					return receipt, errors.Errorf("tx %s reverted on-chain", hash.Hex())
				}
			} else if !errors.Is(err, ethereum.NotFound) {
				return nil, errors.Errorf("receipt %s: %w", hash.Hex(), err)
			}
		}
		if receipt != nil {
			head, err := m.backend.BlockNumber(ctx)
			if err != nil {
				return nil, errors.Errorf("block number: %w", err)
			}
			confirmed := receipt.BlockNumber.Uint64() + m.cfg.Confirmations
			if head >= confirmed {
				return receipt, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func gweiToWei(gwei float64) *big.Int {
	wei, _ := new(big.Float).Mul(big.NewFloat(gwei), big.NewFloat(params.GWei)).Int(nil)
	return wei
}

func isNonceTooLow(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "nonce too low")
}
