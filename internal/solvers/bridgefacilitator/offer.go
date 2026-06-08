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
// adapter) as the on-chain maker. It returns ok=false (no error) when the auction's rate is below
// the configured return floor, i.e. when the bot should simply not bid.
func (s *Solver) buildSignedOffer(
	av auctionView, request, maker common.Address, principal *big.Int,
) (threef.CreateOfferDto, bool, error) {
	auction := av.dto
	if auction.MaxRate == nil {
		return threef.CreateOfferDto{}, false, nil
	}
	rateBps := float64(*auction.MaxRate)
	if rateBps < s.cfg.MinReturnBps {
		return threef.CreateOfferDto{}, false, nil // below our return floor
	}

	expectedReturn := offerExpectedReturn(principal, rateBps)

	domain := auction.Eip712Domain
	if domain == nil || domain.Name == nil || domain.ChainId == nil {
		return threef.CreateOfferDto{}, false, errors.Errorf("auction %v: missing EIP-712 domain", auction.Id)
	}
	chainID := big.NewInt(int64(*domain.ChainId))
	// The EIP-712 domain version comes from the auction; fall back to grunt's known default only when
	// the API omits it (the field is nullable). Name and chainId are required above — no fallback.
	domainVersion := OfferDomainVersion
	if domain.Version != nil && *domain.Version != "" {
		domainVersion = *domain.Version
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
	digest := OfferDigest(offer, *domain.Name, domainVersion, chainID, request)
	sig, err := s.deps.Signer.SignHash(digest)
	if err != nil {
		return threef.CreateOfferDto{}, false, errors.Errorf("sign offer: %w", err)
	}

	chainIDf := float32(chainID.Int64())
	sigHex := hexutil.Encode(sig)
	return threef.CreateOfferDto{
		AuctionId:      auction.Id,
		Maker:          lowerAddr(maker), // API rejects checksummed addresses (confirmed live)
		Amount:         principal.String(),
		ExpectedReturn: expectedReturn.String(),
		Nonce:          nonce.String(),
		Expiration:     expiration.String(),
		UseCallback:    true,
		ChainId:        &chainIDf,
		Signature:      &sigHex,
	}, true, nil
}
