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
	ds := domainSeparator(domainName, domainVersion, chainID, request)
	sh := offerStructHash(offer)
	// keccak256(0x1901 || domainSeparator || structHash)
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, ds.Bytes(), sh.Bytes())
}

func offerStructHash(o Offer) common.Hash {
	buf := make([]byte, 0, 7*32)
	buf = append(buf, offerTypeHash.Bytes()...)
	buf = append(buf, word(o.Maker.Bytes())...)
	buf = append(buf, word(o.Amount.Bytes())...)
	buf = append(buf, word(o.ExpectedReturn.Bytes())...)
	buf = append(buf, word(o.Nonce.Bytes())...)
	buf = append(buf, word(o.Expiration.Bytes())...)
	buf = append(buf, boolWord(o.UseCallback)...)
	return crypto.Keccak256Hash(buf)
}

func domainSeparator(name, version string, chainID *big.Int, verifyingContract common.Address) common.Hash {
	buf := make([]byte, 0, 5*32)
	buf = append(buf, domainTypeHash.Bytes()...)
	buf = append(buf, crypto.Keccak256([]byte(name))...)
	buf = append(buf, crypto.Keccak256([]byte(version))...)
	buf = append(buf, word(chainID.Bytes())...)
	buf = append(buf, word(verifyingContract.Bytes())...)
	return crypto.Keccak256Hash(buf)
}

// word left-pads b to a 32-byte EIP-712 word.
func word(b []byte) []byte {
	return common.LeftPadBytes(b, 32)
}

func boolWord(v bool) []byte {
	w := make([]byte, 32)
	if v {
		w[31] = 1
	}
	return w
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
	return crypto.Keccak256Hash(
		apiKeyDomainTypeHash.Bytes(),
		crypto.Keccak256([]byte(apiKeyDomainName)),
		crypto.Keccak256([]byte(apiKeyDomainVersion)),
		word(chainID.Bytes()),
	)
}

// getOffersTypeHash is the EIP-712 type the maker signs to list its offers via the Authorization
// header; the field set is checked against the live 3F API in the GetOffers golden test.
var getOffersTypeHash = crypto.Keccak256Hash([]byte("GetOffers(address maker,uint256 deadline)"))

// GetOffersDigest computes the EIP-712 digest signed for an authenticated GET /v1/offer (maker=adapter)
// over the grunt-api domain at chainID (the bot's operating chain).
func GetOffersDigest(maker common.Address, deadline, chainID *big.Int) common.Hash {
	sh := crypto.Keccak256Hash(getOffersTypeHash.Bytes(), word(maker.Bytes()), word(deadline.Bytes()))
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, gruntAPIDomainSeparator(chainID).Bytes(), sh.Bytes())
}

// cancelOfferTypeHash is the EIP-712 type the maker signs to cancel an unaccepted offer via
// POST /v1/offer/cancel; the field set is checked against the live 3F API in the CancelOffer golden test.
var cancelOfferTypeHash = crypto.Keccak256Hash([]byte("CancelOffer(address maker,uint256 offerId,uint256 deadline)"))

// CancelOfferDigest computes the EIP-712 digest a maker signs to cancel offerID over the grunt-api
// domain at chainID (the bot's operating chain, matching GetOffersDigest).
func CancelOfferDigest(maker common.Address, offerID, deadline, chainID *big.Int) common.Hash {
	sh := crypto.Keccak256Hash(cancelOfferTypeHash.Bytes(), word(maker.Bytes()), word(offerID.Bytes()), word(deadline.Bytes()))
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, gruntAPIDomainSeparator(chainID).Bytes(), sh.Bytes())
}

// APIKeyDigest computes the EIP-712 digest a facilitator signs to generate a 3F API key (chainId 1).
func APIKeyDigest(facilitator common.Address, deadline *big.Int) common.Hash {
	sh := crypto.Keccak256Hash(apiKeyTypeHash.Bytes(), word(facilitator.Bytes()), word(deadline.Bytes()))
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, gruntAPIDomainSeparator(big.NewInt(apiKeyDomainChainID)).Bytes(), sh.Bytes())
}
