package bridgefacilitator

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// OfferDomainVersion is grunt's known EIP-712 domain version (solady `_domainNameAndVersion`). It is
// only a fallback default: buildSignedOffer signs with the version reported in the auction's
// eip712Domain and uses this constant solely when the API omits it (the field is nullable).
const OfferDomainVersion = "0.0.1"

// offerTypeString is the EIP-712 type of grunt's Offer struct (see IOfferReceiver.sol).
const offerTypeString = "Offer(address maker,uint256 amount,uint256 expectedReturn," +
	"uint256 nonce,uint256 expiration,bool useCallback)"

// eip712DomainTypeString is the standard EIP-712 domain type used by solady's EIP712.
const eip712DomainTypeString = "EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"

// offerTypeHash / domainTypeHash are the keccak256 of the type strings above.
var (
	offerTypeHash  = crypto.Keccak256Hash([]byte(offerTypeString))
	domainTypeHash = crypto.Keccak256Hash([]byte(eip712DomainTypeString))
)

// Offer is the on-chain Offer tuple the maker signs.
type Offer struct {
	Maker          common.Address
	Amount         *big.Int
	ExpectedReturn *big.Int
	Nonce          *big.Int
	Expiration     *big.Int
	UseCallback    bool
}

// OfferDigest computes the EIP-712 digest a maker signs for `offer` against the Request contract.
// The domain is per-Request: name/version from the Request, chainID, verifyingContract = the
// Request address. This is the digest grunt's OfferReceiver._validateOffer verifies, and which our
// adapter's EIP-1271 isValidSignature checks against offerSigner.
func OfferDigest(offer Offer, domainName, domainVersion string, chainID *big.Int, request common.Address) common.Hash {
	return hashTypedMessage(domainSeparator(domainName, domainVersion, chainID, request), offerStructHash(offer))
}

func offerStructHash(offer Offer) common.Hash {
	callback := byte(0)
	if offer.UseCallback {
		callback = 1
	}
	return hashWords(offerTypeHash[:], offer.Maker.Bytes(), offer.Amount.Bytes(), offer.ExpectedReturn.Bytes(),
		offer.Nonce.Bytes(), offer.Expiration.Bytes(), []byte{callback})
}

func domainSeparator(name, version string, chainID *big.Int, verifyingContract common.Address) common.Hash {
	return hashWords(domainTypeHash[:], crypto.Keccak256([]byte(name)), crypto.Keccak256([]byte(version)),
		chainID.Bytes(), verifyingContract.Bytes())
}

// hashWords hashes static ABI words in field order; fixed-width values are padded once.
func hashWords(values ...[]byte) common.Hash {
	for index, value := range values {
		values[index] = common.LeftPadBytes(value, common.HashLength)
	}
	return crypto.Keccak256Hash(values...)
}

func hashTypedMessage(domain, message common.Hash) common.Hash {
	var envelope [66]byte
	envelope[0], envelope[1] = 0x19, 0x01
	copy(envelope[2:34], domain[:])
	copy(envelope[34:], message[:])
	return crypto.Keccak256Hash(envelope[:])
}

// grunt-api EIP-712 domain (no verifyingContract). chainId is per-flow: the (test-only) API-key
// generation domain uses 1; the GetOffers listing domain uses the bot's operating chain.
const (
	apiKeyDomainName    = "grunt-api"
	apiKeyDomainVersion = "1"
	apiKeyDomainChainID = 1
)

var (
	apiKeyTypeHash = crypto.Keccak256Hash(
		[]byte("GenerateFacilitatorApiKey(address facilitator,uint256 deadline)"))
	apiKeyDomainTypeHash = crypto.Keccak256Hash(
		[]byte("EIP712Domain(string name,string version,uint256 chainId)"))
)

// gruntAPIDomainSeparator builds the grunt-api domain separator (name/version, no verifyingContract)
// for chainID; the 3F server rebuilds it from the request's chainId query param to verify the signature.
func gruntAPIDomainSeparator(chainID *big.Int) common.Hash {
	return hashWords(apiKeyDomainTypeHash[:], crypto.Keccak256([]byte(apiKeyDomainName)),
		crypto.Keccak256([]byte(apiKeyDomainVersion)), chainID.Bytes())
}

// getOffersTypeHash is the EIP-712 type the maker signs to list its offers via the Authorization
// header; the field set is checked against the live 3F API in the GetOffers golden test.
var getOffersTypeHash = crypto.Keccak256Hash([]byte("GetOffers(address maker,uint256 deadline)"))

// GetOffersDigest computes the EIP-712 digest signed for an authenticated GET /v1/offer (maker=adapter)
// over the grunt-api domain at chainID (the bot's operating chain).
func GetOffersDigest(maker common.Address, deadline, chainID *big.Int) common.Hash {
	return hashTypedMessage(gruntAPIDomainSeparator(chainID), hashWords(getOffersTypeHash[:], maker.Bytes(), deadline.Bytes()))
}

// cancelOfferTypeHash is the EIP-712 type the maker signs to cancel an unaccepted offer via
// POST /v1/offer/cancel; the field set is checked against the live 3F API in the CancelOffer golden test.
var cancelOfferTypeHash = crypto.Keccak256Hash([]byte("CancelOffer(address maker,uint256 offerId,uint256 deadline)"))

// CancelOfferDigest computes the EIP-712 digest a maker signs to cancel offerID over the grunt-api
// domain at chainID (the bot's operating chain, matching GetOffersDigest).
func CancelOfferDigest(maker common.Address, offerID, deadline, chainID *big.Int) common.Hash {
	return hashTypedMessage(gruntAPIDomainSeparator(chainID), hashWords(cancelOfferTypeHash[:], maker.Bytes(), offerID.Bytes(), deadline.Bytes()))
}

// APIKeyDigest computes the EIP-712 digest a facilitator signs to generate a 3F API key (chainId 1).
func APIKeyDigest(facilitator common.Address, deadline *big.Int) common.Hash {
	domain := gruntAPIDomainSeparator(big.NewInt(apiKeyDomainChainID))
	return hashTypedMessage(domain, hashWords(apiKeyTypeHash[:], facilitator.Bytes(), deadline.Bytes()))
}
