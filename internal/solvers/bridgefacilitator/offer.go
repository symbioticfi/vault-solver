package bridgefacilitator

import (
	"math/big"
	"time"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/types"
)

// buildSignedOffer signs a trusted strategy execution offer. Strategy owns pricing and sizing; solver
// only supplies the auction EIP-712 domain and signature.
func (s *Solver) buildSignedOffer(
	av auctionView, offer types.OfferExecution,
) (threef.CreateOfferDto, error) {
	auction := av.dto
	if offer.Principal == nil || offer.ExpectedReturn == nil {
		return threef.CreateOfferDto{}, errors.Errorf("auction %v: strategy offer is missing amounts", auction.Id)
	}

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
	expiration := offerExpiration(av, s.cfg.OfferExpiryBuffer, time.Now())

	signedOffer := Offer{
		Maker:          offer.Maker,
		Amount:         offer.Principal,
		ExpectedReturn: offer.ExpectedReturn,
		Nonce:          nonce,
		Expiration:     expiration,
		UseCallback:    true,
	}
	digest := OfferDigest(signedOffer, *domainName, domainVersion, chainID, offer.Request)
	sig, err := s.deps.Signer.SignHash(digest)
	if err != nil {
		return threef.CreateOfferDto{}, errors.Errorf("sign offer: %w", err)
	}

	dto := threef.NewCreateOfferDto(
		auction.Id,
		lowerAddr(offer.Maker), // API rejects checksummed addresses (confirmed live)
		offer.Principal.String(),
		offer.ExpectedReturn.String(),
		nonce.String(),
		expiration.String(),
		true, // useCallback
	)
	dto.SetChainId(float32(chainID.Int64()))
	dto.SetSignature(hexutil.Encode(sig))
	return *dto, nil
}

// offerExpiration anchors a signed offer's expiration to the auction's solve_start_time plus buffer.
// If the auction omits solve_start_time, the offer expires now+buffer.
// The buffer is long enough to cover a full auction solve window plus slack.
func offerExpiration(av auctionView, buffer time.Duration, now time.Time) *big.Int {
	exp := now.Add(buffer)
	if s, ok := av.dto.GetSolveStartTimeOk(); ok && s != nil && *s != "" {
		if t, err := time.Parse(time.RFC3339, *s); err == nil {
			exp = t.Add(buffer)
		}
	}
	return big.NewInt(exp.Unix())
}
