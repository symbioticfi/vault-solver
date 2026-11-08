// Package merklesender adapts process-owned SDK work to the host's single sender.
package merklesender

import (
	"bytes"
	"context"
	"math/big"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/transaction"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type hostSender interface {
	Hold(string) error
	Release(string) error
	SendAsync(ctx context.Context, req txmanager.Request) (<-chan txmanager.Result, bool)
	ResumeOwned(ctx context.Context, req txmanager.Request, attempts []*types.Transaction) (<-chan txmanager.Result, bool)
	ConfirmOwnedReceipt(ctx context.Context, tx *types.Transaction) (*types.Receipt, error)
}

var nextOwner atomic.Uint64

// Sender owns one immutable SDK request in RAM. Its mutex serializes host calls;
// the capture mutex synchronizes the host's signing worker with those calls.
// Fees, signing, nonce allocation, broadcast and lifecycle workers remain in txmanager.
type Sender struct {
	Deadline      func() time.Time // Set before use; resolves the business submission deadline.
	mu            sync.Mutex
	manager       hostSender
	solver, owner string
	active        *ownedRequest
}

type ownedRequest struct {
	req      txmanager.Request
	results  <-chan txmanager.Result
	outcome  *txmanager.Result
	signed   chan struct{}
	mu       sync.Mutex
	attempts []*types.Transaction
}

var _ keeper.Sender = (*Sender)(nil)

func New(manager *txmanager.Manager, solverName string) *Sender {
	s := &Sender{solver: solverName, owner: "merkle/" + solverName + "/" + strconv.FormatUint(nextOwner.Add(1), 10)}
	if manager != nil {
		s.manager = manager
	}
	return s
}

func (s *Sender) Busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active != nil
}

// Submit waits for captured signing or a definite pre-sign failure. Once signed,
// a canceled caller and any uncertain host result leave Busy true for Poll.
func (s *Sender) Submit(ctx context.Context, req transaction.Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != nil {
		return transaction.ErrBusy
	}
	if s.manager == nil || s.solver == "" {
		return errors.New("host transaction manager and solver name required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if req.To == (common.Address{}) {
		return errors.New("nonzero transaction destination required")
	}
	if err := s.manager.Hold(s.owner); err != nil {
		return err
	}
	work := &ownedRequest{signed: make(chan struct{})}
	work.req = txmanager.Request{Owner: s.owner, To: req.To, Data: bytes.Clone(req.Data), Label: "execute_recipe", Solver: s.solver, BeforeBroadcast: work.capture}
	if s.Deadline != nil {
		work.req.Deadline = s.Deadline()
	}
	s.active = work
	results, accepted := s.manager.SendAsync(ctx, work.req)
	if !accepted {
		return s.rejectUnsigned(work, errors.Errorf("host transaction admission failed: %w", ctx.Err()))
	}
	work.results = results
	// The host accepted the request on its lifecycle context. It must report the
	// real signing outcome even when the caller's operation context expires.
	select {
	case <-work.signed:
		return ctx.Err()
	case outcome := <-results:
		work.results, work.outcome = nil, &outcome
		if len(work.snapshot()) == 0 {
			return s.rejectUnsigned(work, outcome.Err)
		}
		return outcome.Err
	}
}

func (s *Sender) rejectUnsigned(work *ownedRequest, cause error) error {
	if cause == nil {
		cause = errors.New("host transaction failed before signing")
	}
	if err := s.manager.Release(s.owner); err != nil {
		return errors.Join(cause, err)
	}
	if s.active == work {
		s.active = nil
	}
	return cause
}

func (work *ownedRequest) capture(unsigned, signed *types.Transaction) error {
	if unsigned == nil || signed == nil || unsigned.Type() != types.DynamicFeeTxType || signed.Type() != types.DynamicFeeTxType || signed.ChainId().Sign() <= 0 {
		return errors.New("host signer returned an invalid transaction")
	}
	signer := types.LatestSignerForChainID(signed.ChainId())
	if signer.Hash(unsigned) != signer.Hash(signed) || unsigned.To() == nil || *unsigned.To() != work.req.To || !bytes.Equal(unsigned.Data(), work.req.Data) || unsigned.Value().Sign() != 0 {
		return errors.New("host signer changed the authorized transaction")
	}
	if _, err := types.Sender(signer, signed); err != nil {
		return errors.New("host signer returned an invalid signature")
	}
	work.mu.Lock()
	defer work.mu.Unlock()
	if len(work.attempts) > 0 {
		if signed.Nonce() != work.attempts[0].Nonce() {
			return errors.New("replacement changed owned nonce")
		}
		for _, tx := range work.attempts {
			if tx.Hash() == signed.Hash() {
				return nil
			}
		}
	}
	work.attempts = append(work.attempts, signed)
	if len(work.attempts) == 1 {
		close(work.signed)
	}
	return nil
}

func (work *ownedRequest) snapshot() []*types.Transaction {
	work.mu.Lock()
	defer work.mu.Unlock()
	return append([]*types.Transaction(nil), work.attempts...)
}

// Poll accepts only canonically confirmed owned receipt evidence. Ambiguous host
// outcomes re-enter the same host lifecycle with every captured signed variant.
func (s *Sender) Poll(ctx context.Context) (*transaction.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	work := s.active
	if work == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if work.results != nil {
		select {
		case outcome := <-work.results:
			work.results, work.outcome = nil, &outcome
		default:
			return nil, nil
		}
	}
	attempts := work.snapshot()
	if len(attempts) == 0 {
		var cause error
		if work.outcome != nil {
			cause = work.outcome.Err
		}
		return nil, s.rejectUnsigned(work, cause)
	}
	var receiptErrors []error
	for _, attempt := range slices.Backward(attempts) {
		receipt, err := s.manager.ConfirmOwnedReceipt(ctx, attempt)
		if err != nil {
			receiptErrors = append(receiptErrors, err)
			continue
		}
		if receipt == nil {
			continue
		}
		execution, err := executionFromReceipt(receipt, attempt.Hash())
		if err != nil {
			return nil, err
		}
		if err := s.manager.Release(s.owner); err != nil {
			return nil, err
		}
		s.active = nil
		return &transaction.Result{Hash: attempt.Hash(), Execution: execution, Reverted: receipt.Status == types.ReceiptStatusFailed}, nil
	}
	if len(receiptErrors) > 0 {
		return nil, errors.Join(receiptErrors...)
	}
	var cause error
	if work.outcome != nil {
		cause = work.outcome.Err
		if work.outcome.Outcome == txmanager.OutcomeNonceConsumed {
			cause = errors.Join(transaction.ErrNonceConsumed, cause)
		}
	}
	results, accepted := s.manager.ResumeOwned(ctx, work.req, attempts)
	if accepted {
		work.results, work.outcome = results, nil
	} else {
		cause = errors.Join(cause, errors.New("host owned transaction resumption not admitted"))
	}
	return nil, cause
}

func executionFromReceipt(receipt *types.Receipt, hash common.Hash) (*provider.Execution, error) {
	if receipt == nil || receipt.TxHash != hash || receipt.BlockNumber == nil || !receipt.BlockNumber.IsUint64() || receipt.Status > 1 {
		return nil, errors.New("invalid owned transaction receipt")
	}
	execution := &provider.Execution{TxHash: hash, BlockNumber: new(big.Int).Set(receipt.BlockNumber), BlockHash: receipt.BlockHash, Logs: make([]types.Log, len(receipt.Logs))}
	for i, log := range receipt.Logs {
		if log == nil || log.TxIndex != receipt.TransactionIndex {
			return nil, errors.New("invalid owned receipt log")
		}
		execution.Logs[i] = *log
	}
	if err := execution.Validate(); err != nil {
		return nil, err
	}
	return execution.Clone(), nil
}
