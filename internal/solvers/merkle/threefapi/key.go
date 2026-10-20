package threefapi

import (
	"context"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethmath "github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/signer"
)

// GenerateKey explicitly rotates the facilitator's API key. Never call it during normal startup.
// The returned secret must be stored by the caller; the authorization expires after five minutes.
func GenerateKey(ctx context.Context, baseURL string, s signer.Signer) (string, error) {
	if err := validateURL(baseURL); err != nil {
		return "", err
	}
	if s == nil || s.Address() == (common.Address{}) {
		return "", errors.New("3f key generation requires a signer")
	}
	if ctx.Err() != nil {
		return "", apiError(ctx, "generate key", nil, ctx.Err())
	}
	deadline := big.NewInt(time.Now().Add(5 * time.Minute).Unix())
	digest, err := APIKeyDigest(s.Address(), deadline)
	if err != nil {
		return "", err
	}
	signature, err := s.SignHash(digest)
	if err != nil || len(signature) != 65 || (signature[64] != 27 && signature[64] != 28) {
		return "", errors.New("3f key generation signing failed")
	}
	dto := *threef.NewGenerateFacilitatorApiKeyDto(1, lower(s.Address()), deadline.String(), hexutil.Encode(signature))
	result, response, err := newAPIClient(baseURL, "").FacilitatorAPI.AdminControllerGenerateKeyV1(ctx).GenerateFacilitatorApiKeyDto(dto).Execute()
	closeResponse(response)
	if err != nil {
		return "", apiError(ctx, "generate key", response, err)
	}
	if result == nil || strings.TrimSpace(result.ApiKey) == "" || strings.ContainsAny(result.ApiKey, "\r\n") {
		return "", errors.New("3f invalid generated key response")
	}
	return result.ApiKey, nil
}

// APIKeyDigest follows GenerateFacilitatorApiKeyDto's exact grunt-api domain, always chain 1.
func APIKeyDigest(facilitator common.Address, deadline *big.Int) (common.Hash, error) {
	if facilitator == (common.Address{}) || deadline == nil || deadline.Sign() <= 0 || deadline.BitLen() > 256 {
		return common.Hash{}, errors.New("3f invalid key generation authorization")
	}
	typed := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain":              {{Name: "name", Type: "string"}, {Name: "version", Type: "string"}, {Name: "chainId", Type: "uint256"}},
			"GenerateFacilitatorApiKey": {{Name: "facilitator", Type: "address"}, {Name: "deadline", Type: "uint256"}},
		},
		PrimaryType: "GenerateFacilitatorApiKey",
		Domain:      apitypes.TypedDataDomain{Name: "grunt-api", Version: "1", ChainId: ethmath.NewHexOrDecimal256(1)},
		Message:     apitypes.TypedDataMessage{"facilitator": facilitator.Hex(), "deadline": deadline.String()},
	}
	digest, _, err := apitypes.TypedDataAndHash(typed)
	if err != nil {
		return common.Hash{}, errors.New("3f key generation digest failed")
	}
	return common.BytesToHash(digest), nil
}
