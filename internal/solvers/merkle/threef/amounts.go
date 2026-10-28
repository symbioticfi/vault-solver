package threef

import (
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/threef"
)

var uintPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
var periodPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)
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
	if principal.BitLen() > 128 || yield.BitLen() > 128 || principal.Cmp(m.MaxPrincipal) > 0 || yield.Cmp(minimumReturn(principal, m.MinimumYield1e8)) < 0 {
		return false, nil
	}
	basis, err := rateBasis(auction)
	if err != nil {
		return false, err
	}
	// The HTTP boundary preserves the original JSON number before DTO float decoding.
	// Never reconstruct an economic limit from the generated float32 value.
	exact, present := auction.AdditionalProperties[exactMaxRateKey]
	if !present {
		if value, _ := auction.GetMaxRateOk(); value != nil {
			return false, errors.New("3F auction maxRate lacks exact decimal source")
		}
		return true, nil
	}
	text, ok := exact.(string)
	if !ok || len(text) > 128 || !ratePattern.MatchString(text) {
		return false, errors.New("3F auction maxRate is not an exact nonnegative decimal")
	}
	rate, ok := new(big.Rat).SetString(text)
	if !ok {
		return false, errors.New("3F auction maxRate is invalid")
	}
	left := new(big.Int).Mul(yield, new(big.Int).Mul(big.NewInt(10000), rate.Denom()))
	right := new(big.Int).Mul(principal, rate.Num())
	// The policy floor remains yield/principal; the auction ceiling is annual
	// when its API supplies a rate period. Compare both sides with integer math.
	if basis.Sign() > 0 {
		left.Mul(left, big.NewInt(31536000))
		right.Mul(right, basis)
	}
	return left.Cmp(right) <= 0, nil
}

// rateBasis rejects lossy DTO periods and bounds decimal expansion before parsing.
func rateBasis(auction threef.AuctionDto) (*big.Int, error) {
	exact, present := auction.AdditionalProperties[exactRateBasisKey]
	if !present {
		if auction.RateBasisSeconds != 0 {
			return nil, errors.New("3F auction rateBasisSeconds lacks exact decimal source")
		}
		return new(big.Int), nil
	}
	text, ok := exact.(string)
	if !ok || len(text) > 128 || !periodPattern.MatchString(text) {
		return nil, errors.New("3F auction rateBasisSeconds must be a nonnegative int64")
	}
	if i := strings.IndexAny(text, "eE"); i >= 0 {
		exponent, err := strconv.Atoi(text[i+1:])
		if err != nil || exponent < -128 || exponent > 128 {
			return nil, errors.New("3F auction rateBasisSeconds exponent exceeds bound")
		}
	}
	value, ok := new(big.Rat).SetString(text)
	if !ok || !value.IsInt() || value.Sign() < 0 || !value.Num().IsInt64() {
		return nil, errors.New("3F auction rateBasisSeconds must be a nonnegative int64")
	}
	return value.Num(), nil
}
