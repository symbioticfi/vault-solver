package bridgefacilitator

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

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

	got := OfferDigest(offer, OfferDomain{
		Name: domainName, Version: OfferDomainVersion, ChainID: chainID, VerifyingContract: request,
	})

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

func TestOfferDigestSaltedAndUnsaltedLargeChainParity(t *testing.T) {
	offer := Offer{
		Maker:          common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Amount:         big.NewInt(1_000_000_000),
		ExpectedReturn: big.NewInt(5_000_000),
		Nonce:          big.NewInt(1),
		Expiration:     big.NewInt(4_102_444_800),
		UseCallback:    true,
	}
	request := common.HexToAddress("0xd824000000000000000000000000000000000842")
	chainID := big.NewInt(9_007_199_254_740_993)
	salt := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	for _, tc := range []struct {
		name string
		salt *common.Hash
	}{
		{name: "unsalted"},
		{name: "salted", salt: &salt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			domain := OfferDomain{
				Name: "request-8185", Version: "1", ChainID: chainID,
				VerifyingContract: request, Salt: tc.salt,
			}
			got := OfferDigest(offer, domain)

			domainTypes := []apitypes.Type{
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			}
			typedDomain := apitypes.TypedDataDomain{
				Name: domain.Name, Version: domain.Version,
				ChainId: (*math.HexOrDecimal256)(chainID), VerifyingContract: request.Hex(),
			}
			if tc.salt != nil {
				domainTypes = append(domainTypes, apitypes.Type{Name: "salt", Type: "bytes32"})
				typedDomain.Salt = tc.salt.Hex()
			}
			typed := apitypes.TypedData{
				Types: apitypes.Types{
					"EIP712Domain": domainTypes,
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
				Domain:      typedDomain,
				Message: apitypes.TypedDataMessage{
					"maker": offer.Maker.Hex(), "amount": offer.Amount.String(),
					"expectedReturn": offer.ExpectedReturn.String(), "nonce": offer.Nonce.String(),
					"expiration": offer.Expiration.String(), "useCallback": offer.UseCallback,
				},
			}
			domainSep, err := typed.HashStruct("EIP712Domain", typed.Domain.Map())
			if err != nil {
				t.Fatal(err)
			}
			msgHash, err := typed.HashStruct("Offer", typed.Message)
			if err != nil {
				t.Fatal(err)
			}
			want := crypto.Keccak256Hash([]byte{0x19, 0x01}, domainSep, msgHash)
			if got != want {
				t.Fatalf("digest mismatch: got %s want %s", got, want)
			}
		})
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

func TestGetOffersDigest_Golden(t *testing.T) {
	maker := common.HexToAddress("0x0000000000000000000000000000000000000042")
	got := GetOffersDigest(maker, big.NewInt(4102444800), big.NewInt(apiKeyDomainChainID)).Hex()
	// GOLDEN: pinned from TestGetOffersDigest_MatchesApitypes cross-check (chainId 1).
	want := "0x9d4c2e5ccaaeb6884d2d2fd8e306e57cf781ef424db9e8801c703eac794fa6a5"
	if got != want {
		t.Fatalf("digest = %s, want %s", got, want)
	}
}

// TestGetOffersDigest_MatchesApitypes cross-checks our hand-rolled GetOffers digest against
// go-ethereum's independent EIP-712 implementation. The grunt-api domain has no verifyingContract
// (name/version/chainId=1 only), matching the same domain as APIKeyDigest.
func TestGetOffersDigest_MatchesApitypes(t *testing.T) {
	maker := common.HexToAddress("0x0000000000000000000000000000000000000042")
	deadline := big.NewInt(4102444800)

	got := GetOffersDigest(maker, deadline, big.NewInt(apiKeyDomainChainID))

	typed := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
			},
			"GetOffers": {
				{Name: "maker", Type: "address"},
				{Name: "deadline", Type: "uint256"},
			},
		},
		PrimaryType: "GetOffers",
		Domain: apitypes.TypedDataDomain{
			Name:    apiKeyDomainName,
			Version: apiKeyDomainVersion,
			ChainId: math.NewHexOrDecimal256(apiKeyDomainChainID),
		},
		Message: apitypes.TypedDataMessage{
			"maker":    maker.Hex(),
			"deadline": deadline.String(),
		},
	}
	domainSep, err := typed.HashStruct("EIP712Domain", typed.Domain.Map())
	if err != nil {
		t.Fatalf("hash domain: %v", err)
	}
	msgHash, err := typed.HashStruct("GetOffers", typed.Message)
	if err != nil {
		t.Fatalf("hash message: %v", err)
	}
	want := crypto.Keccak256Hash([]byte{0x19, 0x01}, domainSep, msgHash)

	if got != want {
		t.Fatalf("digest mismatch:\n manual   %s\n apitypes %s", got.Hex(), want.Hex())
	}
}

// TestGetOffersDigest_MatchesLiveAcceptedSignature verifies that the scaffolded GetOffers type
// string is accepted by the live 3F API. Skipped offline (SOLVER_LIVE_AUTH != "1").
// A correctly-formed sig returns 200/empty or 403 (unauthorized maker) — NOT a signature error.
// If the type string is wrong the API returns a 401/signature-error, which fails the test.
func TestGetOffersDigest_MatchesLiveAcceptedSignature(t *testing.T) {
	if os.Getenv("SOLVER_LIVE_AUTH") != "1" {
		t.Skip("set SOLVER_LIVE_AUTH=1 and SOLVER_PRIVATE_KEY to run the live 3F GetOffers auth check")
	}
	pk := os.Getenv("SOLVER_PRIVATE_KEY")
	if pk == "" {
		t.Fatal("SOLVER_PRIVATE_KEY not set")
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(pk, "0x"))
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	maker := crypto.PubkeyToAddress(key.PublicKey)
	deadline := big.NewInt(4_102_444_800)
	chainID := big.NewInt(11155111) // Sepolia; the grunt-api domain + query chainId must agree
	if v := os.Getenv("SOLVER_CHAIN_ID"); v != "" {
		chainID, _ = new(big.Int).SetString(v, 10)
	}

	sig, err := crypto.Sign(GetOffersDigest(maker, deadline, chainID).Bytes(), key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sig[64] += 27 // normalize V to {27,28}

	baseURL := os.Getenv("SOLVER_3F_BASE_URL")
	if baseURL == "" {
		baseURL = "https://bf.dev.gcp.3f.xyz"
	}

	url := fmt.Sprintf("%s/v1/offer?maker=%s&chainId=%s&deadline=%s",
		baseURL, strings.ToLower(maker.Hex()), chainID.String(), deadline.String())
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil) //nolint:gosec // G704: URL is operator-supplied via SOLVER_3F_BASE_URL in this live integration test
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+hexutil.Encode(sig))

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req) //nolint:gosec // G704: intentional operator-controlled target in live integration test
	if err != nil {
		t.Fatalf("GET /v1/offer: %v", err)
	}
	defer resp.Body.Close()

	// 200 (authorized) or 403 (maker not registered) both mean signature verification passed.
	// Anything in the 4xx range that is specifically a signature error means the type string is wrong.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unexpected status %d — expected 200 or 403 (sig accepted); a 401 means the type string may be wrong", resp.StatusCode)
	}
	t.Logf("GET /v1/offer status %d (maker=%s) — signature accepted by 3F API", resp.StatusCode, maker.Hex())
}
