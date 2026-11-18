package threef

import (
	"math/big"
	"regexp"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/threef"
	threefrates "github.com/symbioticfi/vault-solver/internal/threef"
)

var uintPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
var ratePattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)

func uintAmount(text string) (*big.Int, error) {
	if len(text) > 78 || !uintPattern.MatchString(text) {
		return nil, errors.New("expected canonical uint256 decimal")
	}
	value, ok := new(big.Int).SetString(text, 10)
	if !ok || value.BitLen() > 256 {
		return nil, errors.New("quantity exceeds uint256")
	}
	return value, nil
}

func dtoID(value float32) (uint64, error) {
	if err := validateID(value); err != nil {
		return 0, err
	}
	return uint64(value), nil
}

func address(text string) (common.Address, error) {
	if !common.IsHexAddress(text) || common.HexToAddress(text) == (common.Address{}) {
		return common.Address{}, errors.New("3F address must be nonzero Ethereum address")
	}
	return common.HexToAddress(text), nil
}

func depositAsset(auction threef.AuctionDto) (common.Address, error) {
	asset, ok := auction.GetDepositAssetOk()
	if !ok || asset == nil {
		return common.Address{}, errors.New("3F auction deposit asset is unavailable")
	}
	return address(asset.Address)
}

func minInt(values ...*big.Int) *big.Int {
	result := new(big.Int).Set(values[0])
	for _, value := range values[1:] {
		if value.Cmp(result) < 0 {
			result.Set(value)
		}
	}
	return result
}

func minimumReturn(principal, rate *big.Int) *big.Int {
	product := new(big.Int).Mul(principal, rate)
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(product, big.NewInt(100000000), remainder)
	if remainder.Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}

func fitsTerms(m offerMarket, principal, yield *big.Int, auction threef.AuctionDto) (bool, error) {
	if principal.Sign() <= 0 || yield.Sign() <= 0 || principal.BitLen() > 128 || yield.BitLen() > 128 || principal.Cmp(m.MaxPrincipal) > 0 || yield.Cmp(minimumReturn(principal, m.MinimumYield1e8)) < 0 {
		return false, nil
	}
	minimum, maximum, err := auctionReturnBounds(principal, auction)
	if err != nil {
		return false, err
	}
	return yield.Cmp(minimum) >= 0 && yield.Cmp(maximum) <= 0, nil
}

func auctionReturnBounds(principal *big.Int, auction threef.AuctionDto) (minimum, maximum *big.Int, err error) {
	basis, err := rateBasis(auction)
	if err != nil {
		return nil, nil, err
	}
	// The HTTP boundary preserves the original JSON number before DTO float decoding.
	// Never reconstruct an economic limit from the generated float32 value.
	exact, present := auction.AdditionalProperties[exactMaxRateKey]
	if !present {
		return nil, nil, errors.New("3F auction maxRate lacks exact decimal source")
	}
	text, ok := exact.(string)
	if !ok || len(text) > 128 || !ratePattern.MatchString(text) {
		return nil, nil, errors.New("3F auction maxRate is not an exact nonnegative decimal")
	}
	rate, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, nil, errors.New("3F auction maxRate is invalid")
	}
	return threefrates.ReturnBounds(principal, rate, basis.Int64())
}

// rateBasis uses the raw API period before generated float32 conversion.
func rateBasis(auction threef.AuctionDto) (*big.Int, error) {
	var value *big.Rat
	if exact, present := auction.AdditionalProperties[exactRateBasisKey]; present {
		var ok bool
		value, ok = new(big.Rat).SetString(exact.(string))
		if !ok {
			return nil, errors.New("3F invalid auction rateBasisSeconds")
		}
	} else {
		value = new(big.Rat).SetFloat64(float64(auction.GetRateBasisSeconds()))
	}
	if value == nil || !value.IsInt() || value.Sign() < 0 || !value.Num().IsInt64() {
		return nil, errors.New("3F auction rateBasisSeconds must be a nonnegative int64")
	}
	return value.Num(), nil
}
