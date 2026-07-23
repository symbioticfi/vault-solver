package discounts

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
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

func ParseOffer(item ListItem) (*Offer, error) {
	id, err := parseHash(item.DiscountID, "discountId")
	if err != nil {
		return nil, err
	}
	adapter, err := parseAddress(item.Adapter, "adapter")
	if err != nil {
		return nil, err
	}
	tokenToRedeem, err := parseAddress(item.TokenToRedeem, "tokenToRedeem")
	if err != nil {
		return nil, err
	}
	collateral, err := parseAddress(item.Collateral, "collateral")
	if err != nil {
		return nil, err
	}
	discount, err := parseNonNegativeDecimal(item.Discount, "discount")
	if err != nil {
		return nil, err
	}
	if discount.Cmp(big.NewInt(liquidlane.DiscountPrecision)) > 0 {
		return nil, errors.Errorf("discount: must be <= %d", liquidlane.DiscountPrecision)
	}
	maxRate, err := parsePositiveDecimal(item.MaxRate, "maxRate")
	if err != nil {
		return nil, err
	}
	maxAssets, err := parsePositiveDecimal(item.MaxAssets, "maxAssets")
	if err != nil {
		return nil, err
	}
	if item.CollateralDecimals < 0 || item.CollateralDecimals > 255 {
		return nil, errors.Errorf("collateralDecimals: must be in [0,255], got %d", item.CollateralDecimals)
	}
	if item.Deadline <= 0 {
		return nil, errors.New("deadline: must be positive")
	}
	return &Offer{
		DiscountID: id, Adapter: adapter, TokenToRedeem: tokenToRedeem,
		Collateral: collateral, CollateralDecimals: item.CollateralDecimals,
		Discount: discount, Deadline: item.Deadline,
		MaxRate: maxRate, MaxAssets: maxAssets,
	}, nil
}

func ParseSigned(resolved *Resolved) (*Signed, error) {
	if resolved == nil {
		return nil, errors.New("resolved discount is nil")
	}
	id, err := parseHash(resolved.DiscountID, "discountId")
	if err != nil {
		return nil, err
	}
	adapter, err := parseAddress(resolved.Discount.Adapter, "adapter")
	if err != nil {
		return nil, err
	}
	tokenToRedeem, err := parseAddress(resolved.Discount.TokenToRedeem, "tokenToRedeem")
	if err != nil {
		return nil, err
	}
	discount, err := parseNonNegativeDecimal(resolved.Discount.Discount, "discount")
	if err != nil {
		return nil, err
	}
	if discount.Cmp(big.NewInt(liquidlane.DiscountPrecision)) > 0 {
		return nil, errors.Errorf("discount: must be <= %d", liquidlane.DiscountPrecision)
	}
	signer, err := parseAddress(resolved.Discount.Signer, "signer")
	if err != nil {
		return nil, err
	}
	protocol, err := parseAddress(resolved.Discount.Protocol, "protocol")
	if err != nil {
		return nil, err
	}
	nonce, err := hexutil.DecodeBig(resolved.Discount.Nonce)
	if err != nil {
		return nil, errors.Errorf("nonce: %w", err)
	}
	if nonce.Sign() < 0 {
		return nil, errors.New("nonce: must be non-negative")
	}
	signerSignature, err := hexutil.Decode(resolved.SignerSignature)
	if err != nil {
		return nil, errors.Errorf("signerSignature: %w", err)
	}
	protocolSignature, err := hexutil.Decode(resolved.ProtocolSignature)
	if err != nil {
		return nil, errors.Errorf("protocolSignature: %w", err)
	}
	if len(signerSignature) == 0 || len(protocolSignature) == 0 {
		return nil, errors.New("discount signatures must not be empty")
	}
	if resolved.Discount.Deadline <= 0 || resolved.ProtocolDeadline <= 0 {
		return nil, errors.New("discount deadlines must be positive")
	}
	if resolved.Discount.Deadline > maxUint48 || resolved.ProtocolDeadline > maxUint48 {
		return nil, errors.New("discount deadlines exceed uint48")
	}
	return &Signed{
		DiscountID: id, Adapter: adapter,
		Terms: SignedTerms{
			TokenToRedeem: tokenToRedeem, Discount: discount, Signer: signer, Protocol: protocol,
			Nonce: nonce, Deadline: big.NewInt(resolved.Discount.Deadline),
		},
		SignerSignature: signerSignature, ProtocolDeadline: big.NewInt(resolved.ProtocolDeadline),
		ProtocolSignature: protocolSignature,
	}, nil
}

func parseAddress(raw, field string) (common.Address, error) {
	if !common.IsHexAddress(raw) {
		return common.Address{}, errors.Errorf("%s: invalid address %q", field, raw)
	}
	address := common.HexToAddress(raw)
	if address == (common.Address{}) {
		return common.Address{}, errors.Errorf("%s: zero address", field)
	}
	return address, nil
}

func parseHash(raw, field string) (common.Hash, error) {
	decoded, err := hexutil.Decode(raw)
	if err != nil || len(decoded) != common.HashLength {
		return common.Hash{}, errors.Errorf("%s: invalid bytes32 %q", field, raw)
	}
	hash := common.BytesToHash(decoded)
	if hash == (common.Hash{}) {
		return common.Hash{}, errors.Errorf("%s: zero bytes32", field)
	}
	return hash, nil
}

func parsePositiveDecimal(raw, field string) (*big.Int, error) {
	out, ok := new(big.Int).SetString(raw, 10)
	if !ok || out.Sign() <= 0 {
		return nil, errors.Errorf("%s: invalid positive decimal %q", field, raw)
	}
	return out, nil
}

func parseNonNegativeDecimal(raw, field string) (*big.Int, error) {
	out, ok := new(big.Int).SetString(raw, 10)
	if !ok || out.Sign() < 0 {
		return nil, errors.Errorf("%s: invalid non-negative decimal %q", field, raw)
	}
	return out, nil
}
