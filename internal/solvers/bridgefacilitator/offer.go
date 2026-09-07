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
	av auction, offer types.OfferExecution,
) (threef.CreateOfferDto, error) {
	if offer.Principal == nil || offer.ExpectedReturn == nil {
		return threef.CreateOfferDto{}, errors.Errorf("auction %v: strategy offer is missing amounts", av.id)
	}

	if offer.Request != av.request {
		return threef.CreateOfferDto{}, errors.New("strategy request differs from auction request")
	}
	if av.domainName == "" || av.domainChain == nil {
		return threef.CreateOfferDto{}, errors.Errorf("auction %d: missing EIP-712 domain name or chainId", av.id)
	}
	chainID := av.domainChain
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
	digest := OfferDigest(signedOffer, av.domainName, av.domainVersion, chainID, offer.Request)
	sig, err := s.offerSigner.SignHash(digest)
	if err != nil {
		return threef.CreateOfferDto{}, errors.Errorf("sign offer: %w", err)
	}

	dto := threef.NewCreateOfferDto(
		float32(av.id),
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
func offerExpiration(av auction, buffer time.Duration, now time.Time) *big.Int {
	if !av.solveStart.IsZero() {
		now = av.solveStart
	}
	return big.NewInt(now.Add(buffer).Unix())
}
