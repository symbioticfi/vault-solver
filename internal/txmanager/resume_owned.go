package txmanager

import (
	"bytes"
	"context"
	"math/big"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
)

// ResumeOwned returns retained signed work to the same serialized host lifecycle.
// It never selects a fresh nonce or signs an initial transaction. Owner must still
// hold the lane; an expired business deadline permits only receipt observation.
func (m *Manager) ResumeOwned(ctx context.Context, req Request, attempts []*types.Transaction) (<-chan Result, bool) {
	if req.Owner == "" || req.BeforeBroadcast == nil || len(attempts) == 0 {
		return m.admissionFailure(ctx, req, time.Now(), errors.New("owned reservation, signed attempts and capture hook required"))
	}
	for _, tx := range attempts {
		if err := m.validateOwnedTransaction(tx); err != nil {
			return m.admissionFailure(ctx, req, time.Now(), err)
		}
	}
	latest := attempts[len(attempts)-1]
	for _, tx := range attempts {
		if tx.Nonce() != latest.Nonce() || tx.To() == nil || *tx.To() != req.To || !bytes.Equal(tx.Data(), req.Data) || tx.Value().Cmp(requestValue(req)) != 0 {
			return m.admissionFailure(ctx, req, time.Now(), errors.New("retained transaction does not match owned request"))
		}
	}
	now := time.Now()
	pending := &pendingTransaction{
		req: cloneRequest(req), nonce: latest.Nonce(), gas: latest.Gas(), value: latest.Value(),
		fees:         feeQuote{baseFee: new(big.Int), tip: latest.GasTipCap(), maxFee: latest.GasFeeCap()},
		originalHash: attempts[0].Hash(), deadline: maxDeadline(now, m.abandonmentDeadline(req)),
		horizon: horizonProgress{sentAt: now, lastEvaluation: now},
	}
	for _, tx := range attempts {
		pending.attempts = append(pending.attempts, txAttempt{hash: tx.Hash(), tx: tx})
	}
	pending.publishAttemptHashes()
	return m.enqueue(ctx, req, false, pending)
}

func requestValue(req Request) *big.Int {
	if req.Value != nil {
		return req.Value
	}
	return new(big.Int)
}

func maxDeadline(now, deadline time.Time) time.Time {
	if deadline.Before(now) {
		return now
	}
	return deadline
}

func (m *Manager) validateOwnedTransaction(tx *types.Transaction) error {
	if tx == nil || tx.Type() != types.DynamicFeeTxType || tx.ChainId().Cmp(m.chainID) != 0 || tx.Nonce() == ^uint64(0) {
		return errors.New("invalid owned transaction")
	}
	from, err := types.Sender(types.LatestSignerForChainID(m.chainID), tx)
	if err != nil || from != m.signer.Address() {
		return errors.New("retained transaction signer mismatch")
	}
	return nil
}

// ConfirmOwnedReceipt reuses the host's canonical witness proof. No receipt or
// insufficient depth returns nil; errors and foreign nonce consumption retain ownership.
func (m *Manager) ConfirmOwnedReceipt(ctx context.Context, tx *types.Transaction) (*types.Receipt, error) {
	if err := m.validateOwnedTransaction(tx); err != nil {
		return nil, err
	}
	readCtx, cancelRead := context.WithTimeout(ctx, m.receiptReadTimeout())
	receipt, err := m.backend.TransactionReceipt(readCtx, tx.Hash())
	cancelRead()
	if errors.Is(err, ethereum.NotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Errorf("read owned transaction receipt: %w", err)
	}
	if err := validateReceipt(tx.Hash(), receipt); err != nil {
		return nil, err
	}
	witness, err := confirmationWitnessNumber(receipt, m.cfg.Confirmations)
	if err != nil {
		return nil, err
	}
	head, err := m.confirmationHeader(ctx, nil)
	if err != nil {
		return nil, err
	}
	if head.Number.Cmp(witness) < 0 {
		return nil, nil
	}
	receipt, err = m.confirmReceiptWitness(ctx, receipt, m.cfg.Confirmations, m.backend.HeaderByHash)
	if err != nil {
		return nil, err
	}
	m.rememberConfirmedNonce(tx.Nonce())
	return receipt, nil
}
