package txmanager

import (
	"context"
	"math/big"
	"time"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// trackedTx is owned by exactly one tracker goroutine after the dispatcher hands it off.
type trackedTx struct {
	req          Request
	nonce        uint64
	state        State
	attempts     []*types.Transaction
	admissionErr error
}

func (t *trackedTx) hashes() []common.Hash {
	out := make([]common.Hash, len(t.attempts))
	for i, tx := range t.attempts {
		out[i] = tx.Hash()
	}
	return out
}

func (m *Manager) track(ctx context.Context, tracked *trackedTx) Result {
	poll := time.NewTicker(m.cfg.PollInterval)
	defer poll.Stop()
	pending := time.NewTimer(m.cfg.PendingInterval)
	defer pending.Stop()
	var feeCap *big.Int
	if m.cfg.MaxFeeGwei > 0 {
		feeCap = gweiToWei(m.cfg.MaxFeeGwei)
	}

	lastErr := tracked.admissionErr
	replacements := uint64(0)
	rebroadcast := false
	for {
		if result, err := m.pollAttempts(ctx, tracked); result != nil {
			return *result
		} else if err != nil {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			return unresolvedResult(tracked, errors.Join(ErrUnresolved, ctx.Err(), lastErr))
		case <-poll.C:
			continue
		case <-pending.C:
			if err := ctx.Err(); err != nil {
				return unresolvedResult(tracked, errors.Join(ErrUnresolved, err, lastErr))
			}
			if replacements >= m.cfg.MaxReplacements {
				return unresolvedResult(tracked, errors.Join(ErrUnresolved, lastErr))
			}

			if tracked.state == StateBroadcastUnknown && !rebroadcast {
				rebroadcast = true
				if err := m.rebroadcast(ctx, tracked.attempts[0]); err != nil {
					if errors.Is(err, ErrUnresolved) {
						return unresolvedResult(tracked, errors.Join(err, lastErr))
					}
					lastErr = err
				}
			}

			replacements++
			if err := m.replace(ctx, tracked, feeCap); err != nil {
				if errors.Is(err, ErrUnresolved) {
					return unresolvedResult(tracked, errors.Join(err, lastErr))
				}
				lastErr = err
			}
			pending.Reset(m.cfg.PendingInterval)
		}
	}
}

func (m *Manager) pollAttempts(ctx context.Context, tracked *trackedTx) (*Result, error) {
	var latestErr error
	var final *Result
	for _, attempt := range tracked.attempts {
		receipt, err := m.backend.TransactionReceipt(ctx, attempt.Hash())
		if err != nil {
			latestErr = errors.Errorf("receipt %s: %w", attempt.Hash().Hex(), err)
			continue
		}
		canonical, err := m.receiptIsCanonical(ctx, attempt, receipt)
		if err != nil {
			latestErr = err
			continue
		}
		if !canonical {
			continue
		}

		switch receipt.Status {
		case types.ReceiptStatusSuccessful:
			result := finalResult(tracked, StateConfirmed, receipt, nil)
			final = &result
		case types.ReceiptStatusFailed:
			result := finalResult(
				tracked,
				StateReverted,
				receipt,
				errors.Errorf("tx %s reverted on-chain", receipt.TxHash.Hex()),
			)
			final = &result
		default:
			latestErr = errors.Errorf("receipt %s has invalid status %d", attempt.Hash().Hex(), receipt.Status)
		}
	}
	return final, latestErr
}

func (m *Manager) receiptIsCanonical(
	ctx context.Context,
	attempt *types.Transaction,
	receipt *types.Receipt,
) (bool, error) {
	if receipt == nil {
		return false, errors.Errorf("receipt %s is nil", attempt.Hash().Hex())
	}
	if receipt.TxHash != attempt.Hash() {
		return false, errors.Errorf(
			"receipt for %s reports transaction hash %s",
			attempt.Hash().Hex(),
			receipt.TxHash.Hex(),
		)
	}
	if receipt.BlockNumber == nil || receipt.BlockNumber.Sign() < 0 {
		return false, errors.Errorf("receipt %s has invalid block number", attempt.Hash().Hex())
	}

	header, err := m.backend.HeaderByNumber(ctx, receipt.BlockNumber)
	if err != nil {
		return false, errors.Errorf("header for receipt %s: %w", attempt.Hash().Hex(), err)
	}
	if header == nil {
		return false, errors.Errorf("header for receipt %s is nil", attempt.Hash().Hex())
	}
	if header.Hash() != receipt.BlockHash {
		return false, errors.Errorf("receipt %s block hash is not canonical", attempt.Hash().Hex())
	}

	head, err := m.backend.BlockNumber(ctx)
	if err != nil {
		return false, errors.Errorf("block number for receipt %s: %w", attempt.Hash().Hex(), err)
	}
	required := new(big.Int).Add(
		new(big.Int).Set(receipt.BlockNumber),
		new(big.Int).SetUint64(m.cfg.Confirmations),
	)
	return new(big.Int).SetUint64(head).Cmp(required) >= 0, nil
}

func (m *Manager) rebroadcast(ctx context.Context, tx *types.Transaction) error {
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrUnresolved, err)
	}
	if err := m.backend.SendTransaction(ctx, tx); err != nil {
		return errors.Errorf("re-broadcast %s: %w", tx.Hash().Hex(), err)
	}
	return nil
}

func (m *Manager) replace(ctx context.Context, tracked *trackedTx, feeCap *big.Int) error {
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrUnresolved, err)
	}
	previous := tracked.attempts[len(tracked.attempts)-1]
	tip, fee, err := m.replacementFees(previous, feeCap)
	if err != nil {
		return err
	}
	unsigned := replacementTx(previous, tip, fee)
	signed, err := m.signer.SignTx(unsigned, m.chainID)
	if err != nil {
		return errors.Errorf("sign replacement %q: %w", tracked.req.Label, err)
	}
	tracked.attempts = append(tracked.attempts, signed)
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrUnresolved, err)
	}
	if err := m.backend.SendTransaction(ctx, signed); err != nil {
		return errors.Errorf("send replacement %s: %w", signed.Hash().Hex(), err)
	}
	return nil
}

func (m *Manager) replacementFees(
	previous *types.Transaction,
	feeCap *big.Int,
) (tip, fee *big.Int, err error) {
	nextTip := bumpedFee(previous.GasTipCap(), m.cfg.FeeBumpBps)
	nextFee := bumpedFee(previous.GasFeeCap(), m.cfg.FeeBumpBps)
	if feeCap == nil {
		return nextTip, nextFee, nil
	}
	if nextFee.Cmp(feeCap) > 0 {
		nextFee = new(big.Int).Set(feeCap)
	}
	if nextTip.Cmp(feeCap) > 0 || nextFee.Cmp(previous.GasFeeCap()) <= 0 {
		return nil, nil, errors.Errorf("explicit max fee prevents strict replacement fee increase: %w", ErrUnresolved)
	}
	return nextTip, nextFee, nil
}

func finalResult(tracked *trackedTx, state State, receipt *types.Receipt, err error) Result {
	return Result{
		State:   state,
		Nonce:   tracked.nonce,
		Hash:    receipt.TxHash,
		Hashes:  tracked.hashes(),
		Receipt: receipt,
		Err:     err,
	}
}

func unresolvedResult(tracked *trackedTx, err error) Result {
	hashes := tracked.hashes()
	return Result{
		State:  StateUnresolved,
		Nonce:  tracked.nonce,
		Hash:   hashes[len(hashes)-1],
		Hashes: hashes,
		Err:    err,
	}
}

func bumpedFee(old *big.Int, bumpBps uint64) *big.Int {
	numerator := new(big.Int).Mul(old, new(big.Int).SetUint64(bumpBps))
	delta := new(big.Int).Quo(
		new(big.Int).Add(numerator, big.NewInt(9_999)),
		big.NewInt(10_000),
	)
	if delta.Sign() == 0 {
		delta.SetInt64(1)
	}
	return new(big.Int).Add(old, delta)
}

func replacementTx(previous *types.Transaction, tip, fee *big.Int) *types.Transaction {
	to := previous.To()
	return types.NewTx(&types.DynamicFeeTx{
		ChainID:    previous.ChainId(),
		Nonce:      previous.Nonce(),
		GasTipCap:  tip,
		GasFeeCap:  fee,
		Gas:        previous.Gas(),
		To:         to,
		Value:      previous.Value(),
		Data:       previous.Data(),
		AccessList: previous.AccessList(),
	})
}
