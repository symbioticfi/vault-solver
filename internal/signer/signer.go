package signer

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Signer signs digests and transactions for a single address. Implementations must be safe for
// concurrent use: the txmanager and solver offer-signing can call it from different goroutines.
type Signer interface {
	// Address returns the signing address.
	Address() common.Address

	// SignHash signs a 32-byte digest and returns a 65-byte [R || S || V] ECDSA signature with
	// V in {27, 28} — the form expected by Ethereum ecrecover / EIP-1271 verifiers. Callers pass
	// an already-computed EIP-712 digest.
	SignHash(hash common.Hash) ([]byte, error)

	// SignTx signs an EVM transaction for the given chain and returns the signed transaction.
	SignTx(tx *types.Transaction, chainID *big.Int) (*types.Transaction, error)
}
