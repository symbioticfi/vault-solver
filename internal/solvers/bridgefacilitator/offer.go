package bridgefacilitator

import (
	"math/big"
	"time"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/symbioticfi/vault-solver/api/threef"
)

// offerTTL is how long a signed offer stays valid.
const offerTTL = 30 * time.Minute

// buildSignedOffer prices and signs an offer for `request` at `principal` and `rateBps`, with `maker`
// (the adapter) as the on-chain maker. The caller has already confirmed the rate clears the adapter's
// return floor (see offerAuction); the contract enforces it again at consume time.
func (s *Solver) buildSignedOffer(
	av auctionView, request, maker common.Address, principal *big.Int, rateBps float64,
) (threef.CreateOfferDto, error) {
	auction := av.dto
	expectedReturn := offerExpectedReturn(principal, rateBps)

	domain, ok := auction.GetEip712DomainOk()
	if !ok || domain == nil {
		return threef.CreateOfferDto{}, errors.Errorf("auction %v: missing EIP-712 domain", auction.Id)
	}
	domainName, ok := domain.GetNameOk()
	if !ok || domainName == nil {
		return threef.CreateOfferDto{}, errors.Errorf("auction %v: missing EIP-712 domain name", auction.Id)
	}
	domainChainID, ok := domain.GetChainIdOk()
	if !ok || domainChainID == nil {
		return threef.CreateOfferDto{}, errors.Errorf("auction %v: missing EIP-712 domain chainId", auction.Id)
	}
	chainID := big.NewInt(int64(*domainChainID))
	// The EIP-712 domain version comes from the auction; fall back to grunt's known default only when
	// the API omits it (the field is nullable). Name and chainId are required above — no fallback.
	domainVersion := OfferDomainVersion
	if v, hasVersion := domain.GetVersionOk(); hasVersion && v != nil && *v != "" {
		domainVersion = *v
	}

	nonce := new(big.Int).SetUint64(s.nextNonce())
	expiration := big.NewInt(time.Now().Add(offerTTL).Unix())

	offer := Offer{
		Maker:          maker,
		Amount:         principal,
		ExpectedReturn: expectedReturn,
		Nonce:          nonce,
		Expiration:     expiration,
		UseCallback:    true,
	}
	digest := OfferDigest(offer, *domainName, domainVersion, chainID, request)
	sig, err := s.deps.Signer.SignHash(digest)
	if err != nil {
		return threef.CreateOfferDto{}, errors.Errorf("sign offer: %w", err)
	}

	dto := threef.NewCreateOfferDto(
		auction.Id,
		lowerAddr(maker), // API rejects checksummed addresses (confirmed live)
		principal.String(),
		expectedReturn.String(),
		nonce.String(),
		expiration.String(),
		true, // useCallback
	)
	dto.SetChainId(float32(chainID.Int64()))
	dto.SetSignature(hexutil.Encode(sig))
	return *dto, nil
}
