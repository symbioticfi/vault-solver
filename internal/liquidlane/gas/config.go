package gas

import (
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

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
