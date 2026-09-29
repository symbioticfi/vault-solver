package gas

import (
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/parse"
)

// RawConfig is the shared YAML representation of LiquidLane gas oracle configuration.
type RawConfig struct {
	NativeUSDFeed string         `yaml:"nativeUsdFeed"`
	NativeMaxAge  string         `yaml:"nativeMaxAge"`
	TokenUSDFeeds []RawTokenFeed `yaml:"tokenUsdFeeds"`
}

type RawTokenFeed struct {
	Token  string `yaml:"token"`
	Feed   string `yaml:"feed"`
	MaxAge string `yaml:"maxAge"`
}

// ParseConfig validates the shared gas YAML without changing its gas.* field paths.
func ParseConfig(raw RawConfig) (OracleConfig, error) {
	nativeFeed, err := parse.NonZeroAddress(raw.NativeUSDFeed, "gas.nativeUsdFeed")
	if err != nil {
		return OracleConfig{}, err
	}
	nativeMaxAge, err := parse.Duration(raw.NativeMaxAge, 0, "gas.nativeMaxAge")
	if err != nil {
		return OracleConfig{}, err
	}
	if nativeMaxAge <= 0 {
		return OracleConfig{}, errors.New("gas.nativeMaxAge is required")
	}
	feeds := make(map[common.Address]USDFeed, len(raw.TokenUSDFeeds))
	for index, item := range raw.TokenUSDFeeds {
		field := "gas.tokenUsdFeeds[" + strconv.Itoa(index) + "]"
		token, tokenErr := parse.NonZeroAddress(item.Token, field+".token")
		if tokenErr != nil {
			return OracleConfig{}, tokenErr
		}
		feed, feedErr := parse.NonZeroAddress(item.Feed, field+".feed")
		if feedErr != nil {
			return OracleConfig{}, feedErr
		}
		maxAge, ageErr := parse.Duration(item.MaxAge, 0, field+".maxAge")
		if ageErr != nil {
			return OracleConfig{}, ageErr
		}
		if maxAge <= 0 {
			return OracleConfig{}, errors.Errorf("%s.maxAge is required", field)
		}
		if _, duplicate := feeds[token]; duplicate {
			return OracleConfig{}, errors.Errorf("%s.token: duplicate token %s", field, token.Hex())
		}
		feeds[token] = USDFeed{Address: feed, MaxAge: maxAge}
	}
	if len(feeds) == 0 {
		return OracleConfig{}, errors.New("gas.tokenUsdFeeds must contain at least one token feed")
	}
	return OracleConfig{
		NativeUSDFeed: USDFeed{Address: nativeFeed, MaxAge: nativeMaxAge},
		TokenUSDFeeds: feeds,
	}, nil
}

// ErrReferenceGasUnitsRequired reports gas accounting configured on a lane whose transaction manager
// prices quotes for a reference fill but assumes no reference fill gas limit.
var ErrReferenceGasUnitsRequired = errors.New(
	"gas accounting under txManager.fees.policy horizon requires txManager.balance.referenceGasUnits > 0")

// ReferenceGas is the part of the transaction manager that gas-accounted quote pricing depends on
// (txmanager.Manager).
type ReferenceGas interface {
	// ReferenceGasUnits is the fill gas limit the funding gate and quote pricing assume; 0 is unset.
	ReferenceGasUnits() uint64
	// QuotePricingUsesReferenceGas reports whether MaxFeePerGas prices a fill of ReferenceGasUnits gas.
	QuotePricingUsesReferenceGas() bool
}

// RequireReferenceGasUnits checks gas accounting (a configured gas: block) against the transaction
// manager's reference fill gas limit. Where quote pricing picks its tip for that limit (the horizon fee
// policy) an unset one is refused: every block would look roomy, so quotes would keep the floor tip through
// runs of full blocks. Elsewhere (the legacy policy) the limit only drives the lane funding gate, which stays
// off while it is unset, so that is logged once and accepted: a legacy lane with gas: keeps starting without
// the key, and the key must be set before the lane moves to the horizon policy.
func RequireReferenceGasUnits(log logr.Logger, txm ReferenceGas) error {
	if txm.ReferenceGasUnits() > 0 {
		return nil
	}
	if txm.QuotePricingUsesReferenceGas() {
		return ErrReferenceGasUnitsRequired
	}
	log.Info("gas accounting runs with the lane funding gate off: txManager.balance.referenceGasUnits is unset; " +
		"set it before moving this lane to txManager.fees.policy horizon")
	return nil
}
