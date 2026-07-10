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

const (
	unsaltedDomainType = "EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"
	saltedDomainType   = "EIP712Domain(string name,string version,uint256 chainId,address verifyingContract,bytes32 salt)"
)

var (
	offerTypeHash      = crypto.Keccak256Hash([]byte(offerTypeString))
	unsaltedDomainHash = crypto.Keccak256Hash([]byte(unsaltedDomainType))
	saltedDomainHash   = crypto.Keccak256Hash([]byte(saltedDomainType))
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

// OfferDomain is the exact EIP-712 domain returned with one auction. Salt is optional: its presence
// changes both the encoded domain type and separator.
type OfferDomain struct {
	Name              string
	Version           string
	ChainID           *big.Int
	VerifyingContract common.Address
	Salt              *common.Hash
}

// OfferDigest computes the EIP-712 digest a maker signs for `offer` against the Request contract.
// The domain is per-Request: name/version/optional salt from the auction, chainID, and
// verifyingContract = the Request address. This is the digest grunt's OfferReceiver._validateOffer
// verifies, and which our adapter's EIP-1271 isValidSignature checks against offerSigner.
func OfferDigest(offer Offer, domain OfferDomain) common.Hash {
	ds := domainSeparator(domain)
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

func domainSeparator(domain OfferDomain) common.Hash {
	buf := make([]byte, 0, 6*32)
	typeHash := unsaltedDomainHash
	if domain.Salt != nil {
		typeHash = saltedDomainHash
	}
	buf = append(buf, typeHash.Bytes()...)
	buf = append(buf, crypto.Keccak256([]byte(domain.Name))...)
	buf = append(buf, crypto.Keccak256([]byte(domain.Version))...)
	buf = append(buf, word(domain.ChainID.Bytes())...)
	buf = append(buf, word(domain.VerifyingContract.Bytes())...)
	if domain.Salt != nil {
		buf = append(buf, domain.Salt.Bytes()...)
	}
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

// APIKeyDigest computes the EIP-712 digest a facilitator signs to generate a 3F API key (chainId 1).
func APIKeyDigest(facilitator common.Address, deadline *big.Int) common.Hash {
	sh := crypto.Keccak256Hash(apiKeyTypeHash.Bytes(), word(facilitator.Bytes()), word(deadline.Bytes()))
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, gruntAPIDomainSeparator(big.NewInt(apiKeyDomainChainID)).Bytes(), sh.Bytes())
}
