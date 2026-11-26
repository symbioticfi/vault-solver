package connectortest

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

// RevertError is a node's execution revert (JSON-RPC code 3) carrying Data as its payload.
type RevertError struct{ Data []byte }

func (e *RevertError) Error() string  { return "execution reverted" }
func (e *RevertError) ErrorCode() int { return 3 }
func (e *RevertError) ErrorData() any { return hexutil.Encode(e.Data) }

// Revert is a revert of the argument-less custom error signature, e.g. "InvalidDiscount()".
func Revert(signature string) *RevertError {
	return &RevertError{Data: crypto.Keccak256([]byte(signature))[:4]}
}
