package threef

import (
	"fmt"
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
		return nil, errors.Errorf("%.80q is not a canonical uint256 decimal", text)
	}
	value, ok := new(big.Int).SetString(text, 10)
	if !ok || value.BitLen() > 256 {
		return nil, errors.Errorf("%s exceeds uint256", text)
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
		return common.Address{}, errors.Errorf("3F address %.66q is not a nonzero Ethereum address", text)
	}
	return common.HexToAddress(text), nil
}

func depositAsset(auction threef.AuctionDto) (common.Address, error) {
	asset, ok := auction.GetDepositAssetOk()
	if !ok || asset == nil {
		return common.Address{}, errors.Errorf("3F auction %v has no depositAsset", auction.Id)
	}
	a, err := address(asset.Address)
	if err != nil {
		return common.Address{}, errors.Errorf("3F auction %v depositAsset: %w", auction.Id, err)
	}
	return a, nil
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
	return threefrates.MinYieldReturn(principal, rate, 100_000_000)
}

// termsMismatch names the native authorization or auction rate bound the offer breaks, "" when it fits.
func termsMismatch(m offerMarket, principal, yield *big.Int, auction threef.AuctionDto) (string, error) {
	switch {
	case principal.Sign() <= 0 || yield.Sign() <= 0:
		return fmt.Sprintf("principal %s and return %s must be positive", principal, yield), nil
	case principal.BitLen() > 128 || yield.BitLen() > 128:
		return fmt.Sprintf("principal %s or return %s exceeds uint128", principal, yield), nil
	case principal.Cmp(m.MaxPrincipal) > 0:
		return fmt.Sprintf("principal %s exceeds maxPrincipal %s", principal, m.MaxPrincipal), nil
	}
	if floor := minimumReturn(principal, m.MinimumYield1e8); yield.Cmp(floor) < 0 {
		return fmt.Sprintf("return %s is below policy minimum %s", yield, floor), nil
	}
	minimum, maximum, err := auctionReturnBounds(principal, auction)
	if err != nil {
		return "", err
	}
	switch {
	case yield.Cmp(minimum) < 0:
		return fmt.Sprintf("return %s is below auction minimum %s", yield, minimum), nil
	case yield.Cmp(maximum) > 0:
		return fmt.Sprintf("return %s exceeds auction maximum %s", yield, maximum), nil
	}
	return "", nil
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
		return nil, nil, errors.Errorf("3F auction %v maxRate lacks its exact decimal source", auction.Id)
	}
	text, ok := exact.(string)
	if !ok || len(text) > 128 || !ratePattern.MatchString(text) {
		return nil, nil, errors.Errorf("3F auction %v maxRate %.40v is not an exact nonnegative decimal", auction.Id, exact)
	}
	rate, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, nil, errors.Errorf("3F auction %v maxRate %s does not parse as a rational", auction.Id, text)
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
			return nil, errors.Errorf("3F auction %v rateBasisSeconds %.40v is not a number", auction.Id, exact)
		}
	} else {
		value = new(big.Rat).SetFloat64(float64(auction.GetRateBasisSeconds()))
	}
	if value == nil {
		return nil, errors.Errorf("3F auction %v rateBasisSeconds %v is not finite", auction.Id, auction.GetRateBasisSeconds())
	}
	if !value.IsInt() || value.Sign() < 0 || !value.Num().IsInt64() {
		return nil, errors.Errorf("3F auction %v rateBasisSeconds %.40s is not a nonnegative int64", auction.Id, value.RatString())
	}
	return value.Num(), nil
}
