package txmanager

import (
	"context"
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/tenderly"
)

func (m *Manager) currentFees(ctx context.Context, limit *big.Int) (feeQuote, error) {
	readCtx, cancel := context.WithTimeout(ctx, m.feeReadTimeout())
	defer cancel()
	head, err := m.backend.HeaderByNumber(readCtx, nil)
	if err != nil {
		return feeQuote{}, errors.Errorf("%w: header by number: %w", errFreshFeesUnavailable, err)
	}
	if head == nil || head.BaseFee == nil || head.BaseFee.Sign() < 0 {
		return feeQuote{}, errors.Errorf(
			"%w: latest header must contain a non-negative base fee",
			errFreshFeesUnavailable,
		)
	}
	baseFee := new(big.Int).Set(head.BaseFee)
	tipFloor := gweiToWei(m.cfg.TipGwei)
	tip, err := m.currentTip(readCtx, ctx, tipFloor)
	if err != nil {
		return feeQuote{}, err
	}
	return boundedFeeQuote(baseFee, tip, tipFloor, limit)
}

func boundedFeeQuote(baseFee, tip, tipFloor, limit *big.Int) (feeQuote, error) {
	maximum := new(big.Int).Add(new(big.Int).Mul(baseFee, big.NewInt(2)), tip)
	if limit != nil && maximum.Cmp(limit) > 0 {
		maximum.Set(limit)
	}
	maximumTip := new(big.Int).Sub(maximum, baseFee)
	if maximumTip.Sign() < 0 {
		return feeQuote{}, errors.Errorf(
			"fee limit reached: current base fee %s exceeds tx manager max fee %s",
			baseFee,
			maximum,
		)
	}
	if tipFloor.Sign() > 0 && tipFloor.Cmp(maximumTip) > 0 {
		return feeQuote{}, errors.Errorf(
			"fee limit reached: fee limit %s cannot cover base fee %s plus priority fee floor %s",
			maximum,
			baseFee,
			tipFloor,
		)
	}
	if tip.Cmp(maximumTip) > 0 {
		tip.Set(maximumTip)
	}
	return feeQuote{baseFee: baseFee, tip: tip, maxFee: maximum}, nil
}

func (m *Manager) currentTip(
	readCtx context.Context,
	parentCtx context.Context,
	floor *big.Int,
) (*big.Int, error) {
	if floor.Sign() == 0 {
		history, err := m.backend.FeeHistory(
			readCtx,
			feeHistoryBlocks,
			nil,
			[]float64{feeHistoryPercentile},
		)
		if err != nil {
			return nil, errors.Errorf("%w: fee history: %w", errFreshFeesUnavailable, err)
		}
		tip, valid := feeHistoryTip(history)
		if !valid {
			return nil, errors.Errorf("%w: invalid fee history rewards", errFreshFeesUnavailable)
		}
		return tip, nil
	}
	suggested, err := m.backend.SuggestGasTipCap(readCtx)
	if err == nil && suggested != nil && suggested.Sign() >= 0 {
		return maxBigCopy(suggested, floor), nil
	}
	if parentErr := parentCtx.Err(); parentErr != nil {
		return nil, errors.Errorf("%w: suggest gas tip: %w", errFreshFeesUnavailable, parentErr)
	}
	return new(big.Int).Set(floor), nil
}

func feeHistoryTip(history *ethereum.FeeHistory) (*big.Int, bool) {
	if history == nil || len(history.Reward) != feeHistoryBlocks {
		return nil, false
	}
	var minimum *big.Int
	for _, rewards := range history.Reward {
		if len(rewards) != 1 || rewards[0] == nil || rewards[0].Sign() < 0 {
			return nil, false
		}
		if minimum == nil || rewards[0].Cmp(minimum) < 0 {
			minimum = new(big.Int).Set(rewards[0])
		}
	}
	return minimum, true
}

func (m *Manager) estimateGas(ctx context.Context, request Request) (uint64, error) {
	estimate, err := m.backend.EstimateGas(ctx, ethereum.CallMsg{
		From: m.signer.Address(),
		To:   &request.To,
		Data: request.Data,
	})
	if err != nil {
		m.log.Error(
			err,
			"gas estimation failed",
			"label",
			request.Label,
			"tenderly",
			tenderly.SimulatorURL(
				m.chainID,
				m.signer.Address(),
				request.To,
				request.Data,
				nil,
			),
		)
		return 0, errors.Errorf("estimate gas %q: %w", request.Label, err)
	}
	return estimate + estimate/20, nil
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
	unsigned := types.NewTx(&types.DynamicFeeTx{
		ChainID:   m.chainID,
		Nonce:     nonce,
		GasTipCap: fees.tip,
		GasFeeCap: fees.maxFee,
		Gas:       gas,
		To:        &to,
		Value:     value,
		Data:      data,
	})
	signed, err := m.signer.SignTx(ctx, unsigned, m.chainID)
	if err != nil {
		return nil, errors.Errorf("sign transaction: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Errorf("sign transaction: %w", err)
	}
	broadcastErr := m.sendSigned(ctx, signed, existingLifecycle)
	rejected := errors.Is(broadcastErr, errNonceLanePaused) ||
		isDefiniteBroadcastRejection(broadcastErr) ||
		!existingLifecycle && isPendingNonceCollision(broadcastErr)
	if rejected {
		return nil, errors.Errorf("broadcast rejected before acceptance: %w", broadcastErr)
	}
	return signed, broadcastErr
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
	broadcastCtx, cancel := context.WithTimeout(ctx, m.cfg.BroadcastTimeout)
	defer cancel()
	err := m.backend.SendTransaction(broadcastCtx, signed)
	if !existingLifecycle && (isNonceConsumedError(err) || isPendingNonceCollision(err)) {
		m.markNonceConflict(signed.Nonce(), signed.Hash())
	}
	return err
}
