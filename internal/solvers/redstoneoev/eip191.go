package redstoneoev

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/signer"
)

// This signature payload is opaque bytes in the Executor ABI. Its version and
// ordered fields are pinned to verified Executor V6 (docs/OEV-PLAN.md §6.2).
const executorV6Domain = "EXECUTOR_V6"

var executorV6Args = func() abi.Arguments {
	schema := []string{"string", "uint256", "address", "bytes32", "uint256", "uint256", "uint256"}
	arguments := make(abi.Arguments, len(schema))
	for index, kind := range schema {
		field, err := abi.NewType(kind, "", nil)
		if err != nil {
			panic("Executor V6 signature schema: " + err.Error())
		}
		arguments[index].Type = field
	}
	return arguments
}()

// ExecutorV6Digest hashes standard ABI encoding, never packed encoding. Bounds
// are checked before the ABI encoder can truncate a signed or oversized integer.
func ExecutorV6Digest(chainID *big.Int, callback common.Address, operationHash common.Hash,
	bid, nonce, gasPrice *big.Int) (common.Hash, error) {
	for _, field := range []struct {
		name  string
		value *big.Int
	}{
		{"chainId", chainID}, {"bid", bid}, {"nonce", nonce}, {"maxTxGasPrice", gasPrice},
	} {
		if field.value == nil || field.value.Sign() < 0 || field.value.BitLen() > 256 {
			return common.Hash{}, errors.Errorf("EXECUTOR_V6: %s is not uint256", field.name)
		}
	}
	payload, err := executorV6Args.Pack(executorV6Domain, chainID, callback, operationHash, bid, nonce, gasPrice)
	if err != nil {
		return common.Hash{}, errors.Errorf("encode EXECUTOR_V6: %w", err)
	}
	return crypto.Keccak256Hash(payload), nil
}

// SignBid signs the EIP-191 envelope using the EOA that holds the Executor deposit.
func SignBid(account signer.Signer, chainID *big.Int, callback common.Address, operation []byte,
	bid, nonce, gasPrice *big.Int) ([]byte, error) {
	if account == nil {
		return nil, errors.New("bid signer is missing")
	}
	digest, err := ExecutorV6Digest(chainID, callback, crypto.Keccak256Hash(operation), bid, nonce, gasPrice)
	if err != nil {
		return nil, err
	}
	signature, err := account.SignHash(ethSignedMessageHash(digest))
	if err != nil {
		return nil, errors.Errorf("sign EXECUTOR_V6: %w", err)
	}
	return signature, nil
}

func ethSignedMessageHash(digest common.Hash) common.Hash {
	return common.Hash(accounts.TextHash(digest[:]))
}
