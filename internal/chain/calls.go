package chain

import (
	"context"
	"math/big"
	"strings"
	"time"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

// EstimateGasNextBlock estimates a call against the latest state as if it were included in the block
// after parent: eth_estimateGas's blockOverrides set that block's number and timestamp, so
// time-dependent work (interest accrual, deadline checks) is sized for the block the transaction
// targets. An endpoint without blockOverrides support answers with an invalid-params error.
func (c *Client) EstimateGasNextBlock(
	ctx context.Context, msg ethereum.CallMsg, parent *types.Header, blockTime time.Duration,
) (uint64, error) {
	if parent == nil || parent.Number == nil {
		return 0, errors.New("chain: next-block gas estimate needs a parent header")
	}
	overrides := map[string]any{
		"number": (*hexutil.Big)(new(big.Int).Add(parent.Number, big.NewInt(1))),
		"time":   hexutil.Uint64(parent.Time + uint64(max(blockTime, time.Second)/time.Second)),
	}
	var gas hexutil.Uint64
	if err := c.Client.Client().CallContext(
		ctx, &gas, "eth_estimateGas", estimateGasArg(msg), "latest", nil, overrides,
	); err != nil {
		return 0, err
	}
	return uint64(gas), nil
}

// estimateGasArg encodes the call fields the transaction manager sets, the way ethclient does.
func estimateGasArg(msg ethereum.CallMsg) map[string]any {
	arg := map[string]any{"from": msg.From, "to": msg.To}
	if len(msg.Data) > 0 {
		arg["input"] = hexutil.Bytes(msg.Data)
	}
	if msg.Value != nil {
		arg["value"] = (*hexutil.Big)(msg.Value)
	}
	if msg.Gas != 0 {
		arg["gas"] = hexutil.Uint64(msg.Gas)
	}
	return arg
}

// SendTransaction broadcasts a signed transaction through the write client. It overrides the
// promoted ethclient method.
func (c *Client) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return c.writeClient.SendTransaction(ctx, tx)
}

// NonceAt reads mined state through the sending endpoint for fresh nonce selection and to stop
// replacements after the nonce was consumed, including by another replica.
func (c *Client) NonceAt(
	ctx context.Context, account common.Address, blockNumber *big.Int,
) (uint64, error) {
	return c.writeClient.NonceAt(ctx, account, blockNumber)
}

// PendingNonceAt reads the pending nonce through the write client so a private write endpoint can
// report transactions that are not visible to the primary RPC. It overrides the promoted ethclient
// method. When no separate write endpoint is configured, it targets the primary endpoint. Fresh
// transaction nonce selection uses NonceAt instead.
func (c *Client) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return c.writeClient.PendingNonceAt(ctx, account)
}

// TransactionSenderBalanceAt prefers the write endpoint for sender telemetry, then falls back to the
// ordinary read client when a distinct submission endpoint rejects or cannot serve eth_getBalance.
func (c *Client) TransactionSenderBalanceAt(
	ctx context.Context,
	account common.Address,
	blockNumber *big.Int,
) (*big.Int, error) {
	balance, writeErr := c.writeClient.BalanceAt(ctx, account, blockNumber)
	if writeErr == nil || c.writeClient == c.Client {
		return balance, writeErr
	}
	balance, readErr := c.BalanceAt(ctx, account, blockNumber)
	if readErr == nil {
		return balance, nil
	}
	return nil, errors.Errorf(
		"chain: transaction sender balance: %w",
		errors.Join(
			errors.Errorf("write endpoint: %w", writeErr),
			errors.Errorf("read endpoints: %w", readErr),
		),
	)
}

// ReadBalanceAt serves sender balance telemetry from the read endpoints only, so a submission-only
// relay never pays for it. Admission and replacement keep their write-endpoint reads.
func (c *Client) ReadBalanceAt(ctx context.Context, account common.Address) (*big.Int, error) {
	return c.BalanceAt(ctx, account, nil)
}

// ReadNonces serves nonce telemetry from the read endpoints only: the mined and pending nonces as the
// primary RPC sees them. The pending nonce may lag a privately submitted transaction until the primary
// RPC sees it, which is acceptable for a gauge and never for admission.
func (c *Client) ReadNonces(ctx context.Context, account common.Address) (latestNonce, pendingNonce uint64, err error) {
	if latestNonce, err = c.Client.NonceAt(ctx, account, nil); err != nil {
		return 0, 0, err
	}
	if pendingNonce, err = c.Client.PendingNonceAt(ctx, account); err != nil {
		return 0, 0, err
	}
	return latestNonce, pendingNonce, nil
}

// IsBlockOverrideUnsupported recognizes an endpoint that rejects the blockOverrides parameter of
// eth_estimateGas or eth_call, as opposed to a call that reverts or a transport failure.
func IsBlockOverrideUnsupported(err error) bool {
	// A revert ran the call, so the overrides were accepted, whatever its reason text says.
	if _, reverted := RevertData(err); reverted {
		return false
	}
	var rpcErr rpc.Error
	if errors.As(err, &rpcErr) && (rpcErr.ErrorCode() == -32602 || rpcErr.ErrorCode() == -32601) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"too many arguments", "invalid params", "invalid argument", "unknown field"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

// ReplayMinedCall re-executes a mined transaction's call on its parent block's state with the mined
// block's number and timestamp, so deadline and decay checks see the time it ran at and later
// transactions in that block do not leak in. Earlier transactions in the block are still missing.
// An endpoint without blockOverrides support gets a plain parent-state eth_call.
func (c *Client) ReplayMinedCall(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	header, err := c.HeaderByNumber(ctx, block)
	if err != nil {
		return nil, errors.Errorf("read block %s header: %w", block, err)
	}
	parent := new(big.Int).Sub(block, big.NewInt(1))
	overrides := map[string]any{"number": (*hexutil.Big)(block), "time": hexutil.Uint64(header.Time)}
	var result hexutil.Bytes
	err = c.Client.Client().CallContext(ctx, &result, "eth_call", estimateGasArg(msg), hexutil.EncodeBig(parent), nil, overrides)
	if err != nil && IsBlockOverrideUnsupported(err) {
		return c.CallContract(ctx, msg, parent)
	}
	return result, err
}
