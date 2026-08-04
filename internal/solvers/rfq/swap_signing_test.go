package rfq

import (
	"bytes"
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/go-errors/errors"
	"github.com/google/uuid"

	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

const (
	signedSwapTypeHashHex = "0xffdb5fa9b52456ef5eb17369ab4182fbbad380b6a348af67c04fc73924d9bc77"
	swapNonceTypeHashHex  = "0x6011c54012434af706b36d8a23df1173ae1c8af38fa0f0dfb634193b013f6571"
	swapNonceGoldenHex    = "0x60e388e6f57a7f56b02157a0756e27a89e584029632d086f9a5c3fc1c47ab643"
)

func TestSignedSwapDigestMatchesEIP712Reference(t *testing.T) {
	domain := sampleSwapDomain()
	value := sampleSignedSwap(common.HexToAddress(testSwapper))
	got, err := signedSwapDigest(domain, value)
	if err != nil {
		t.Fatalf("signedSwapDigest: %v", err)
	}
	if signedSwapTypeHash.Hex() != signedSwapTypeHashHex {
		t.Fatalf("signed swap typehash = %s", signedSwapTypeHash.Hex())
	}

	typed := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {
				{Name: "name", Type: "string"}, {Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"}, {Name: "verifyingContract", Type: "address"},
			},
			"SignedSwap": {
				{Name: "recipient", Type: "address"}, {Name: "tokenIn", Type: "address"},
				{Name: "amountIn", Type: "uint256"}, {Name: "amountOut", Type: "uint256"},
				{Name: "caller", Type: "address"}, {Name: "signer", Type: "address"},
				{Name: "nonce", Type: "uint256"}, {Name: "deadline", Type: "uint48"},
			},
		},
		PrimaryType: "SignedSwap",
		Domain: apitypes.TypedDataDomain{
			Name: domain.Name, Version: domain.Version, ChainId: (*math.HexOrDecimal256)(domain.ChainID),
			VerifyingContract: domain.VerifyingContract.Hex(),
		},
		Message: apitypes.TypedDataMessage{
			"recipient": value.Recipient.Hex(), "tokenIn": value.TokenIn.Hex(), "amountIn": value.AmountIn.String(),
			"amountOut": value.AmountOut.String(), "caller": value.Caller.Hex(), "signer": value.Signer.Hex(),
			"nonce": value.Nonce.String(), "deadline": value.Deadline.String(),
		},
	}
	domainHash, err := typed.HashStruct("EIP712Domain", typed.Domain.Map())
	if err != nil {
		t.Fatalf("domain hash: %v", err)
	}
	messageHash, err := typed.HashStruct("SignedSwap", typed.Message)
	if err != nil {
		t.Fatalf("message hash: %v", err)
	}
	want := crypto.Keccak256Hash([]byte{0x19, 0x01}, domainHash, messageHash)
	if got != want {
		t.Fatalf("digest = %s, want %s", got.Hex(), want.Hex())
	}
}

func TestSignedSwapNonceMatchesGoldenVector(t *testing.T) {
	if swapNonceTypeHash.Hex() != swapNonceTypeHashHex {
		t.Fatalf("nonce typehash = %s", swapNonceTypeHash.Hex())
	}
	got := signedSwapNonce(
		uuid.MustParse(testBuildID), 1, common.HexToAddress(testAdapter), common.HexToAddress(testTokenIn), 0,
	)
	if common.BigToHash(got).Hex() != swapNonceGoldenHex {
		t.Fatalf("nonce = %s, want %s", common.BigToHash(got).Hex(), swapNonceGoldenHex)
	}
}

func TestPackSignedSwapCallUsesBindingSelectorAndFrameworkSigner(t *testing.T) {
	signer := newSwapTestSigner(t)
	value := sampleSignedSwap(signer.Address())
	data, err := packSignedSwapCall(signer, sampleSwapDomain(), value)
	if err != nil {
		t.Fatalf("packSignedSwapCall: %v", err)
	}
	if got := common.Bytes2Hex(data[:4]); got != "9a4568b6" {
		t.Fatalf("selector = 0x%s", got)
	}
	decoded, signature := unpackSignedCall(t, data)
	if decoded.Recipient != value.Recipient || decoded.Caller != value.Caller || decoded.Signer != signer.Address() ||
		decoded.AmountIn.Cmp(value.AmountIn) != 0 || decoded.AmountOut.Cmp(value.AmountOut) != 0 ||
		decoded.Nonce.Cmp(value.Nonce) != 0 || decoded.Deadline.Cmp(value.Deadline) != 0 {
		t.Fatalf("decoded signed swap = %+v", decoded)
	}
	digest, _ := signedSwapDigest(sampleSwapDomain(), value)
	recoverySignature := append([]byte(nil), signature...)
	recoverySignature[64] -= 27
	publicKey, err := crypto.SigToPub(digest.Bytes(), recoverySignature)
	if err != nil || crypto.PubkeyToAddress(*publicKey) != signer.Address() {
		t.Fatalf("signature recovery = %v, err %v", publicKey, err)
	}
}

func TestPackSignedSwapCallPropagatesSigningFailure(t *testing.T) {
	signer := newSwapTestSigner(t)
	signer.err = errors.New("sign failed")
	if _, err := packSignedSwapCall(signer, sampleSwapDomain(), sampleSignedSwap(signer.Address())); err == nil {
		t.Fatal("expected signing failure")
	}
}

func TestPackDiscountSwapCallUsesExactResolvedPayload(t *testing.T) {
	signed := sampleSignedDiscount()
	recipient := common.HexToAddress(testRouter)
	amountIn := big.NewInt(100)

	data, err := packDiscountSwapCall(signed, recipient, amountIn)
	if err != nil {
		t.Fatalf("packDiscountSwapCall: %v", err)
	}
	if got := common.Bytes2Hex(data[:4]); got != "8fa5c671" {
		t.Fatalf("selector = 0x%s", got)
	}
	decoded, protocolSignature, gotRecipient, gotAmountIn := unpackDiscountCall(t, data)
	if decoded.Discount.TokenToRedeem != signed.Terms.TokenToRedeem ||
		decoded.Discount.Discount.Cmp(signed.Terms.Discount) != 0 ||
		decoded.Discount.Signer != signed.Terms.Signer || decoded.Discount.Protocol != signed.Terms.Protocol ||
		decoded.Discount.Nonce.Cmp(signed.Terms.Nonce) != 0 ||
		decoded.Discount.Deadline.Cmp(signed.Terms.Deadline) != 0 ||
		!bytes.Equal(decoded.SignerSignature, signed.SignerSignature) ||
		decoded.ProtocolDeadline.Cmp(signed.ProtocolDeadline) != 0 {
		t.Fatalf("decoded discount swap = %+v", decoded)
	}
	if !bytes.Equal(protocolSignature, signed.ProtocolSignature) || gotRecipient != recipient ||
		gotAmountIn.Cmp(amountIn) != 0 {
		t.Fatalf(
			"outer discount arguments = protocolSig %x recipient %v amountIn %v",
			protocolSignature,
			gotRecipient,
			gotAmountIn,
		)
	}
}

type swapTestSigner struct {
	key   *ecdsa.PrivateKey
	err   error
	calls int
}

func newSwapTestSigner(t *testing.T) *swapTestSigner {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return &swapTestSigner{key: key}
}

func (s *swapTestSigner) Address() common.Address { return crypto.PubkeyToAddress(s.key.PublicKey) }

func (s *swapTestSigner) SignHash(hash common.Hash) ([]byte, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	signature, err := crypto.Sign(hash.Bytes(), s.key)
	if err == nil {
		signature[64] += 27
	}
	return signature, err
}

func (s *swapTestSigner) SignTx(*types.Transaction, *big.Int) (*types.Transaction, error) {
	return nil, errors.New("not used")
}

func sampleSwapDomain() swapDomain {
	return swapDomain{
		Name: "LiquidLaneAdapter", Version: "1", ChainID: big.NewInt(1),
		VerifyingContract: common.HexToAddress(testAdapter),
	}
}

func sampleSignedSwap(signer common.Address) adapter.ILiquidLaneAdapterSignedSwap {
	return adapter.ILiquidLaneAdapterSignedSwap{
		Recipient: common.HexToAddress(testRouter), TokenIn: common.HexToAddress(testTokenIn),
		AmountIn: big.NewInt(10), AmountOut: big.NewInt(19), Caller: common.HexToAddress(testRouter),
		Signer: signer, Nonce: big.NewInt(9), Deadline: big.NewInt(2_000_000_000),
	}
}

func sampleSignedDiscount() *discounts.Signed {
	return &discounts.Signed{
		DiscountID: common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Adapter:    common.HexToAddress(testAdapter),
		Terms: discounts.SignedTerms{
			TokenToRedeem: common.HexToAddress(testTokenIn),
			Discount:      big.NewInt(123),
			Signer:        common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			Protocol:      common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
			Nonce:         big.NewInt(9),
			Deadline:      big.NewInt(2_000_000_000),
		},
		SignerSignature:   []byte{0x01, 0x02, 0x03},
		ProtocolDeadline:  big.NewInt(2_000_000_001),
		ProtocolSignature: []byte{0x04, 0x05, 0x06},
	}
}

func unpackSignedCall(t *testing.T, data []byte) (adapter.ILiquidLaneAdapterSignedSwap, []byte) {
	t.Helper()
	parsed, err := adapter.LiquidLaneAdapterMetaData.ParseABI()
	if err != nil {
		t.Fatal(err)
	}
	values, err := parsed.Methods["swap1"].Inputs.Unpack(data[4:])
	if err != nil {
		t.Fatal(err)
	}
	value := *abi.ConvertType(values[0], new(adapter.ILiquidLaneAdapterSignedSwap)).(*adapter.ILiquidLaneAdapterSignedSwap)
	return value, values[1].([]byte)
}

func unpackDiscountCall(
	t *testing.T,
	data []byte,
) (adapter.ILiquidLaneAdapterDiscountSwap, []byte, common.Address, *big.Int) {
	t.Helper()
	parsed, err := adapter.LiquidLaneAdapterMetaData.ParseABI()
	if err != nil {
		t.Fatal(err)
	}
	values, err := parsed.Methods["swap0"].Inputs.Unpack(data[4:])
	if err != nil {
		t.Fatal(err)
	}
	value := *abi.ConvertType(
		values[0], new(adapter.ILiquidLaneAdapterDiscountSwap),
	).(*adapter.ILiquidLaneAdapterDiscountSwap)
	return value, values[1].([]byte), values[2].(common.Address), values[3].(*big.Int)
}
