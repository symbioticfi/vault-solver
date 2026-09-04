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

// executorV6Domain is the RedStone Atom signature version string, the first field of the signed
// payload (see the verified Executor source, docs/OEV-PLAN.md §6.2).
const executorV6Domain = "EXECUTOR_V6"

// executorV6Args is the ABI tuple the Executor recovers the solver from:
//
//	keccak256(abi.encode("EXECUTOR_V6", chainId, operationCallback, keccak256(operationData),
//	                     bidAmount, nonce, maxTxGasPrice))
//
// standard (non-packed) ABI encoding, matching ethers AbiCoder.defaultAbiCoder().encode.
var executorV6Args = abi.Arguments{
	{Type: mustType("string")},
	{Type: mustType("uint256")}, // chainId
	{Type: mustType("address")}, // operationCallback
	{Type: mustType("bytes32")}, // keccak256(operationData)
	{Type: mustType("uint256")}, // bidAmount (wei)
	{Type: mustType("uint256")}, // nonce (strictly ascending)
	{Type: mustType("uint256")}, // maxTxGasPrice
}

// ExecutorV6Digest is the inner digest the Executor hashes before EIP-191 wrapping:
// keccak256(abi.encode("EXECUTOR_V6", chainId, callback, opDataHash, bid, nonce, maxTxGasPrice)).
func ExecutorV6Digest(chainID *big.Int, callback common.Address, opDataHash common.Hash, bid, nonce, maxTxGasPrice *big.Int) (common.Hash, error) {
	encoded, err := encodeExecutorV6(chainID, callback, opDataHash, bid, nonce, maxTxGasPrice)
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(encoded), nil
}

func encodeExecutorV6(
	chainID *big.Int,
	callback common.Address,
	operationHash common.Hash,
	bid, nonce, maxTxGasPrice *big.Int,
) ([]byte, error) {
	encoded, err := executorV6Args.Pack(
		executorV6Domain, chainID, callback, operationHash, bid, nonce, maxTxGasPrice,
	)
	if err != nil {
		return nil, errors.Errorf("encode EXECUTOR_V6: %w", err)
	}
	return encoded, nil
}

// SignBid produces the 65-byte EIP-191 (personal_sign) signature over the EXECUTOR_V6 digest that the
// auctioneer forwards and the Executor verifies via ECDSA.recover(toEthSignedMessageHash(digest)).
// The signer EOA must be the wallet holding the Executor deposit (§6.2).
func SignBid(sgnr signer.Signer, chainID *big.Int, callback common.Address, operationData []byte, bid, nonce, maxTxGasPrice *big.Int) ([]byte, error) {
	digest, err := ExecutorV6Digest(chainID, callback, crypto.Keccak256Hash(operationData), bid, nonce, maxTxGasPrice)
	if err != nil {
		return nil, err
	}
	return sgnr.SignHash(ethSignedMessageHash(digest))
}

// ethSignedMessageHash applies the EIP-191 personal_sign prefix to a 32-byte digest:
// keccak256("\x19Ethereum Signed Message:\n32" || digest) — Solady/OZ MessageHashUtils.toEthSignedMessageHash.
// accounts.TextHash computes exactly this prefix (len(digest)==32) for a 32-byte input.
func ethSignedMessageHash(digest common.Hash) common.Hash {
	return common.BytesToHash(accounts.TextHash(digest.Bytes()))
}

func mustType(t string) abi.Type {
	typ, err := abi.NewType(t, "", nil)
	if err != nil {
		panic("redstoneoev: abi type " + t + ": " + err.Error())
	}
	return typ
}
