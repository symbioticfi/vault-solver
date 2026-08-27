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
