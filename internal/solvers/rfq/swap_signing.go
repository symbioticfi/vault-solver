package rfq

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	"github.com/google/uuid"

	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	frameworksigner "github.com/symbioticfi/vault-solver/internal/signer"
)

const (
	signedSwapTypeString = "SignedSwap(address recipient,address tokenIn,uint256 amountIn,uint256 amountOut,address caller,address signer,uint256 nonce,uint48 deadline)"
	swapNonceTypeString  = "VaultSolverSwapNonce(bytes16 buildId,uint256 chainId,address adapter,address tokenIn,uint256 callIndex)"
	swapDomainTypeString = "EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"
)

var (
	signedSwapTypeHash = crypto.Keccak256Hash([]byte(signedSwapTypeString))
	swapNonceTypeHash  = crypto.Keccak256Hash([]byte(swapNonceTypeString))
	swapDomainTypeHash = crypto.Keccak256Hash([]byte(swapDomainTypeString))
	swapAdapterBinding = adapter.NewLiquidLaneAdapter()
)

type swapDomain struct {
	Name              string
	Version           string
	ChainID           *big.Int
	VerifyingContract common.Address
}

func signedSwapDigest(domain swapDomain, value adapter.ILiquidLaneAdapterSignedSwap) (common.Hash, error) {
	if domain.Name == "" || domain.Version == "" || domain.ChainID == nil || domain.ChainID.Sign() <= 0 ||
		domain.VerifyingContract == (common.Address{}) {
		return common.Hash{}, errors.New("invalid signed swap domain")
	}
	if value.Recipient == (common.Address{}) || value.TokenIn == (common.Address{}) || value.Caller == (common.Address{}) ||
		value.Signer == (common.Address{}) || !validUint256(value.AmountIn) || !validUint256(value.AmountOut) ||
		!validUint256(value.Nonce) || value.Deadline == nil || value.Deadline.Sign() < 0 || value.Deadline.BitLen() > 48 {
		return common.Hash{}, errors.New("invalid signed swap value")
	}
	domainSeparator := swapDomainSeparator(domain)
	structHash := crypto.Keccak256Hash(
		signedSwapTypeHash.Bytes(), eip712Word(value.Recipient.Bytes()), eip712Word(value.TokenIn.Bytes()),
		eip712Word(value.AmountIn.Bytes()), eip712Word(value.AmountOut.Bytes()), eip712Word(value.Caller.Bytes()),
		eip712Word(value.Signer.Bytes()), eip712Word(value.Nonce.Bytes()), eip712Word(value.Deadline.Bytes()),
	)
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, domainSeparator.Bytes(), structHash.Bytes()), nil
}

func swapDomainSeparator(domain swapDomain) common.Hash {
	return crypto.Keccak256Hash(
		swapDomainTypeHash.Bytes(), crypto.Keccak256([]byte(domain.Name)), crypto.Keccak256([]byte(domain.Version)),
		eip712Word(domain.ChainID.Bytes()), eip712Word(domain.VerifyingContract.Bytes()),
	)
}

func signedSwapNonce(
	buildID uuid.UUID,
	chainID int64,
	adapterAddress common.Address,
	tokenIn common.Address,
	callIndex int,
) *big.Int {
	buildWord := make([]byte, 32)
	copy(buildWord, buildID[:])
	hash := crypto.Keccak256Hash(
		swapNonceTypeHash.Bytes(), buildWord, eip712Word(big.NewInt(chainID).Bytes()),
		eip712Word(adapterAddress.Bytes()), eip712Word(tokenIn.Bytes()), eip712Word(big.NewInt(int64(callIndex)).Bytes()),
	)
	return new(big.Int).SetBytes(hash.Bytes())
}

func packSignedSwapCall(
	signer frameworksigner.Signer,
	domain swapDomain,
	value adapter.ILiquidLaneAdapterSignedSwap,
) ([]byte, error) {
	if signer == nil || value.Signer != signer.Address() {
		return nil, errors.New("signed swap signer does not match framework signer")
	}
	digest, err := signedSwapDigest(domain, value)
	if err != nil {
		return nil, err
	}
	signature, err := signer.SignHash(digest)
	if err != nil {
		return nil, errors.Errorf("sign signed swap: %w", err)
	}
	if !validEthereumSignature(signature) {
		return nil, errors.New("framework signer returned an invalid Ethereum signature")
	}
	data, err := swapAdapterBinding.TryPackSwap1(value, signature)
	if err != nil {
		return nil, errors.Errorf("pack signed swap: %w", err)
	}
	return data, nil
}

func validEthereumSignature(signature []byte) bool {
	return len(signature) == 65 && (signature[64] == 27 || signature[64] == 28)
}

func validUint256(value *big.Int) bool {
	return value != nil && value.Sign() >= 0 && value.BitLen() <= 256
}

func eip712Word(value []byte) []byte { return common.LeftPadBytes(value, 32) }
