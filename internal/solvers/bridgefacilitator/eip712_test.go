package bridgefacilitator

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// The canonical Offer typehash, pinned from grunt v1.1.0 OfferReceiver.sol.
const goldenOfferTypeHash = "0x3ded0c963332962cf2d273c8fb4f3e69f4ef33407ca72484fcebb56263ad0664"

func TestOfferTypeHash_MatchesGolden(t *testing.T) {
	if got := offerTypeHash.Hex(); got != goldenOfferTypeHash {
		t.Fatalf("offer typehash drift:\n got  %s\n want %s\n(type string: %q)", got, goldenOfferTypeHash, offerTypeString)
	}
}

// TestOfferDigest_MatchesApitypes cross-checks our hand-rolled digest against go-ethereum's
// independent EIP-712 implementation (apitypes). Two implementations agreeing is strong evidence
// the digest matches what grunt's Request verifies on-chain.
func TestOfferDigest_MatchesApitypes(t *testing.T) {
	offer := Offer{
		Maker:          common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Amount:         big.NewInt(1_000_000_000),
		ExpectedReturn: big.NewInt(5_000_000),
		Nonce:          big.NewInt(1),
		Expiration:     big.NewInt(4_102_444_800),
		UseCallback:    true,
	}
	const domainName = "request-8185"
	chainID := big.NewInt(11155111)
	request := common.HexToAddress("0xd824000000000000000000000000000000000842")

	got := OfferDigest(offer, domainName, OfferDomainVersion, chainID, request)

	typed := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"Offer": {
				{Name: "maker", Type: "address"},
				{Name: "amount", Type: "uint256"},
				{Name: "expectedReturn", Type: "uint256"},
				{Name: "nonce", Type: "uint256"},
				{Name: "expiration", Type: "uint256"},
				{Name: "useCallback", Type: "bool"},
			},
		},
		PrimaryType: "Offer",
		Domain: apitypes.TypedDataDomain{
			Name:              domainName,
			Version:           OfferDomainVersion,
			ChainId:           (*math.HexOrDecimal256)(chainID),
			VerifyingContract: request.Hex(),
		},
		Message: apitypes.TypedDataMessage{
			"maker":          offer.Maker.Hex(),
			"amount":         offer.Amount.String(),
			"expectedReturn": offer.ExpectedReturn.String(),
			"nonce":          offer.Nonce.String(),
			"expiration":     offer.Expiration.String(),
			"useCallback":    offer.UseCallback,
		},
	}
	domainSep, err := typed.HashStruct("EIP712Domain", typed.Domain.Map())
	if err != nil {
		t.Fatalf("hash domain: %v", err)
	}
	msgHash, err := typed.HashStruct("Offer", typed.Message)
	if err != nil {
		t.Fatalf("hash message: %v", err)
	}
	want := crypto.Keccak256Hash([]byte{0x19, 0x01}, domainSep, msgHash)

	if got != want {
		t.Fatalf("digest mismatch:\n manual   %s\n apitypes %s", got.Hex(), want.Hex())
	}
}

// TestAPIKeyDigest_MatchesLiveAcceptedSignature reproduces the exact signature the live 3F dev API
// accepted for generate-key (it returned 403 "Facilitator not registered" — past signature
// verification — rather than a signature error). This pins our EIP-712 to the on-wire schema.
func TestAPIKeyDigest_MatchesLiveAcceptedSignature(t *testing.T) {
	// anvil account #0 (throwaway), the address+deadline used in the live validation call.
	key, err := crypto.HexToECDSA("ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	facilitator := common.HexToAddress("0xf39Fd6e51aad88F6F4ce6aB8827279cfFFb92266")
	deadline := big.NewInt(4_102_444_800)

	sig, err := crypto.Sign(APIKeyDigest(facilitator, deadline).Bytes(), key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sig[64] += 27 // normalize V to {27,28}, as SignHash does

	const wantSig = "0x804755534b98cba2ac903a07923c4e7e80a02d5ffdc699280dd799faec905af0" +
		"4de6933caf26995e9e41eb4f69d8ac2f600d2c08c610a9548a9b1072a3c1757c1c"
	if got := hexutil.Encode(sig); got != wantSig {
		t.Fatalf("generate-key signature mismatch (digest drift):\n got  %s\n want %s", got, wantSig)
	}
}

func TestOfferExpectedReturn(t *testing.T) {
	// 100,000 USDC (6 dp) at 200 bps (2%) => 2,000 USDC.
	principal := new(big.Int).SetUint64(100_000_000_000)
	got := offerExpectedReturn(principal, 200)
	want := new(big.Int).SetUint64(2_000_000_000)
	if got.Cmp(want) != 0 {
		t.Fatalf("expected %s, got %s", want, got)
	}
}
