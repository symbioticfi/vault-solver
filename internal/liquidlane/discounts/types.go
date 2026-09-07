package discounts

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/parse"
)

const maxUint48 = int64(1<<48 - 1)

// Offer is one validated advertised discount. It is safe to pass into solver candidate construction.
type Offer struct {
	DiscountID         common.Hash
	Adapter            common.Address
	TokenToRedeem      common.Address
	Collateral         common.Address
	CollateralDecimals int
	Discount           *big.Int
	Deadline           int64
	// MaxRate is already net of Discount. The backend derives it from the
	// adapter oracle output and the advertised discount terms.
	MaxRate   *big.Int
	MaxAssets *big.Int
}

// Signed is one validated fill-time discount with both signatures decoded.
type Signed struct {
	DiscountID common.Hash
	Adapter    common.Address
	Terms      SignedTerms

	SignerSignature   []byte
	ProtocolDeadline  *big.Int
	ProtocolSignature []byte
}

type SignedTerms struct {
	TokenToRedeem common.Address
	Discount      *big.Int
	Signer        common.Address
	Protocol      common.Address
	Nonce         *big.Int
	Deadline      *big.Int
}

// offerDecoder accumulates independent field errors while constructing an owned
// domain value. The value is returned only after every required field is valid.
type offerDecoder struct{ errors []error }

func (d *offerDecoder) address(raw, name string) common.Address {
	address, err := parse.NonZeroAddress(raw, name)
	if err != nil {
		d.errors = append(d.errors, err)
	}
	return address
}

func (d *offerDecoder) integer(raw, name string, positive bool) *big.Int {
	value, err := parseNonNegativeDecimal(raw, name)
	if err == nil && positive && value.Sign() == 0 {
		err = errors.Errorf("%s: must be positive", name)
	}
	if err != nil {
		d.errors = append(d.errors, err)
	}
	return value
}

func (d *offerDecoder) signature(raw, name string) []byte {
	signature, err := hexutil.Decode(raw)
	if err == nil && len(signature) == 0 {
		err = errors.New("discount signatures must not be empty")
	}
	if err != nil {
		d.errors = append(d.errors, errors.Errorf("%s: %w", name, err))
	}
	return signature
}

func (d *offerDecoder) discount(raw string) *big.Int {
	value := d.integer(raw, "discount", false)
	if value != nil && value.Cmp(big.NewInt(liquidlane.DiscountPrecision)) > 0 {
		d.errors = append(d.errors, errors.Errorf("discount: must be <= %d", liquidlane.DiscountPrecision))
	}
	return value
}

func ParseOffer(item ListItem) (*Offer, error) {
	d := new(offerDecoder)
	id, err := parseHash(item.DiscountID, "discountId")
	if err != nil {
		d.errors = append(d.errors, err)
	}
	offer := &Offer{
		DiscountID: id, Adapter: d.address(item.Adapter, "adapter"),
		TokenToRedeem: d.address(item.TokenToRedeem, "tokenToRedeem"), Collateral: d.address(item.Collateral, "collateral"),
		Discount: d.discount(item.Discount), MaxRate: d.integer(item.MaxRate, "maxRate", true),
		MaxAssets: d.integer(item.MaxAssets, "maxAssets", true), CollateralDecimals: item.CollateralDecimals, Deadline: item.Deadline,
	}
	if item.CollateralDecimals < 0 || item.CollateralDecimals > 255 {
		d.errors = append(d.errors, errors.Errorf("collateralDecimals: must be in [0,255], got %d", item.CollateralDecimals))
	}
	if item.Deadline <= 0 {
		d.errors = append(d.errors, errors.New("deadline: must be positive"))
	}
	if err := errors.Join(d.errors...); err != nil {
		return nil, err
	}
	return offer, nil
}

func ParseSigned(resolved *Resolved) (*Signed, error) {
	if resolved == nil {
		return nil, errors.New("resolved discount is nil")
	}
	d := new(offerDecoder)
	id, idErr := parseHash(resolved.DiscountID, "discountId")
	nonce, nonceErr := parseUint256Decimal(resolved.Discount.Nonce, "nonce")
	signed := &Signed{
		DiscountID: id, Adapter: d.address(resolved.Discount.Adapter, "adapter"),
		Terms: SignedTerms{
			TokenToRedeem: d.address(resolved.Discount.TokenToRedeem, "tokenToRedeem"),
			Discount:      d.discount(resolved.Discount.Discount), Signer: d.address(resolved.Discount.Signer, "signer"),
			Protocol: d.address(resolved.Discount.Protocol, "protocol"), Nonce: nonce, Deadline: big.NewInt(resolved.Discount.Deadline),
		},
		SignerSignature:   d.signature(resolved.SignerSignature, "signerSignature"),
		ProtocolSignature: d.signature(resolved.ProtocolSignature, "protocolSignature"),
		ProtocolDeadline:  big.NewInt(resolved.ProtocolDeadline),
	}
	switch {
	case resolved.Discount.Deadline <= 0 || resolved.ProtocolDeadline <= 0:
		d.errors = append(d.errors, errors.New("discount deadlines must be positive"))
	case resolved.Discount.Deadline > maxUint48 || resolved.ProtocolDeadline > maxUint48:
		d.errors = append(d.errors, errors.New("discount deadlines exceed uint48"))
	}
	if err := errors.Join(append(d.errors, idErr, nonceErr)...); err != nil {
		return nil, err
	}
	return signed, nil
}

func parseHash(raw, field string) (common.Hash, error) {
	hash, err := parse.Hash(raw, field)
	if err != nil {
		return common.Hash{}, err
	}
	if hash == (common.Hash{}) {
		return common.Hash{}, errors.Errorf("%s: zero bytes32", field)
	}
	return hash, nil
}

func parseNonNegativeDecimal(raw, field string) (*big.Int, error) {
	value, err := parse.Big(raw, field)
	if err != nil || value.Sign() < 0 {
		return nil, errors.Errorf("%s: invalid non-negative decimal %q", field, raw)
	}
	return value, nil
}

func parseUint256Decimal(raw, field string) (*big.Int, error) { return parse.Uint(raw, field, 256) }

// Clone gives generated calldata a private copy of the mutable signed amounts.
func (terms SignedTerms) Clone() SignedTerms {
	terms.Discount, terms.Nonce, terms.Deadline = bigmath.Clone(terms.Discount), bigmath.Clone(terms.Nonce), bigmath.Clone(terms.Deadline)
	return terms
}
