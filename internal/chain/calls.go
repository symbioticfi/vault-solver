package chain

import (
	"context"
	"math/big"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// SendTransaction broadcasts a signed transaction through the write client. It overrides the
// promoted ethclient method.
func (c *Client) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return c.writeClient.SendTransaction(ctx, tx)
}

// NonceAt reads mined state through the sending endpoint for fresh nonce selection and to stop
// replacements after the nonce was consumed, outside the process.
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

// TransactionSenderBalanceAt prefers the write endpoint for sender funding, then falls back to the
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
