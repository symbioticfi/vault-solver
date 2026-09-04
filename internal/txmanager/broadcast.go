package txmanager

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
)

type preparedBroadcast struct {
	gas  uint64
	fees feeQuote
}

// broadcast is called only by the admission worker, preserving sign/broadcast nonce order.
func (m *Manager) broadcast(ctx context.Context, request Request) (*pendingTransaction, error) {
	broadcastCtx, cancel := admissionContext(ctx, request)
	defer cancel()
	if err := broadcastCtx.Err(); err != nil {
		return nil, errors.Errorf("send %q before broadcast: %w", request.Label, err)
	}
	prepared, err := m.prepareBroadcast(broadcastCtx, request)
	if err != nil {
		return nil, err
	}
	nonce, err := m.nextNonce(broadcastCtx)
	if err != nil {
		return nil, err
	}
	signed, sendErr := m.signAndSend(
		broadcastCtx,
		nonce,
		request.To,
		request.Data,
		new(big.Int),
		prepared.gas,
		prepared.fees,
		false,
	)
	if signed == nil {
		return nil, errors.Errorf("send %q: %w", request.Label, sendErr)
	}
	hash := signed.Hash()
	uncertain := sendErr != nil && !isKnownTransactionError(sendErr)
	m.logAttempt("transaction", request.Label, nonce, hash, false, sendErr, uncertain)
	m.commitNonce(nonce)
	return &pendingTransaction{
		req:   request,
		nonce: nonce,
		gas:   prepared.gas,
		fees:  cloneFeeQuote(prepared.fees),
		attempts: []txAttempt{{
			hash:                    hash,
			tx:                      signed,
			exactRebroadcastPending: uncertain,
		}},
		originalHash:   hash,
		cancelDeadline: m.cancellationDeadline(request),
	}, nil
}

func (m *Manager) logAttempt(
	event, label string,
	nonce uint64,
	hash common.Hash,
	cancellation bool,
	err error,
	uncertain bool,
) {
	fields := []any{"label", label, "hash", hash.Hex(), "nonce", nonce}
	if cancellation {
		fields = append(fields, "cancellation", true)
	}
	switch {
	case uncertain:
		m.log.Error(err, event+" broadcast uncertain; tracking signed hash", fields...)
	case err != nil:
		m.log.Info(event+" already known by write RPC", append(fields, "rpcResult", err.Error())...)
	default:
		m.log.Info(event+" sent", fields...)
	}
}

func (m *Manager) prepareBroadcast(ctx context.Context, request Request) (preparedBroadcast, error) {
	if request.MaxFeePerGas != nil && request.MaxFeePerGas.Sign() <= 0 {
		return preparedBroadcast{}, errors.Errorf(
			"send %q: request max fee per gas must be positive",
			request.Label,
		)
	}
	fees, err := m.currentFees(ctx, reserveFeeBump(m.normalFeeLimit(request)))
	if err != nil {
		return preparedBroadcast{}, errors.Errorf("send %q: %w", request.Label, err)
	}
	gas, err := m.estimateGas(ctx, request)
	if err != nil {
		return preparedBroadcast{}, err
	}
	obsolete, obsoleteErr := m.requestObsolete(ctx, request)
	if obsoleteErr != nil {
		m.log.Error(
			obsoleteErr,
			"transaction obsolescence check unavailable; continuing",
			"label",
			request.Label,
		)
	} else if obsolete {
		return preparedBroadcast{}, errors.Errorf("send %q: %w", request.Label, errRequestObsolete)
	}
	m.log.V(1).Info(
		"transaction prepared",
		"label",
		request.Label,
		"to",
		request.To.Hex(),
		"calldataBytes",
		len(request.Data),
		"gasLimit",
		gas,
		"baseFeePerGas",
		fees.baseFee.String(),
		"maxPriorityFeePerGas",
		fees.tip.String(),
		"maxFeePerGas",
		fees.maxFee.String(),
		"requestMaxFeePerGas",
		optionalBigString(request.MaxFeePerGas),
	)
	return preparedBroadcast{gas: gas, fees: fees}, nil
}
