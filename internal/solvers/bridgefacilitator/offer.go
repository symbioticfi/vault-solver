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

// generateKeyDeadline is the EIP-712 `deadline` for the generate-key request. The 3F spec labels
// it the "Signature deadline" (how long the signed request is valid, for replay protection) and
// its example uses a year-2100 value — it is NOT documented as the API key's TTL. We set it far
// out so it's safe under both readings: a non-expiring signature window, or (if 3F ties key life
// to it) a long-lived key. Either way, reactive regeneration on a 401/403 covers revoke/expire.
const generateKeyDeadline = 100 * 365 * 24 * time.Hour

// buildSignedOffer prices and signs an offer for `request` at `principal`, with `maker` (the
// adapter) as the on-chain maker. `minYieldBps` is the adapter's on-chain minimum-return floor (0 =
// none); it returns ok=false (no error) when the auction's rate is below it, i.e. when the bot should
// simply not bid (the contract enforces the same floor at consume time).
func (s *Solver) buildSignedOffer(
	av auctionView, request, maker common.Address, principal, minYieldBps *big.Int,
) (threef.CreateOfferDto, bool, error) {
	auction := av.dto
	maxRate, ok := auction.GetMaxRateOk()
	if !ok || maxRate == nil {
		return threef.CreateOfferDto{}, false, nil
	}
	rateBps := float64(*maxRate)
	if minYieldBps != nil && minYieldBps.Sign() > 0 && rateBps < bpsToFloat(minYieldBps) {
		return threef.CreateOfferDto{}, false, nil // below the adapter's on-chain return floor
	}

	expectedReturn := offerExpectedReturn(principal, rateBps)

	domain, ok := auction.GetEip712DomainOk()
	if !ok || domain == nil {
		return threef.CreateOfferDto{}, false, errors.Errorf("auction %v: missing EIP-712 domain", auction.Id)
	}
	domainName, ok := domain.GetNameOk()
	if !ok || domainName == nil {
		return threef.CreateOfferDto{}, false, errors.Errorf("auction %v: missing EIP-712 domain name", auction.Id)
	}
	domainChainID, ok := domain.GetChainIdOk()
	if !ok || domainChainID == nil {
		return threef.CreateOfferDto{}, false, errors.Errorf("auction %v: missing EIP-712 domain chainId", auction.Id)
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
		return threef.CreateOfferDto{}, false, errors.Errorf("sign offer: %w", err)
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
	return *dto, true, nil
}
