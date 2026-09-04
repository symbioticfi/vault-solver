// Package signer keeps secret key material behind the process signing port.
package signer

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Signer represents one immutable EVM identity. Implementations must support concurrent hash and
// transaction signing.
type Signer interface {
	Address() common.Address
	SignHash(common.Hash) ([]byte, error)
	SignTx(ctx context.Context, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error)
}
