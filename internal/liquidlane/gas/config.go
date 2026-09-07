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
	native, err := parseFeed(raw.NativeUSDFeed, raw.NativeMaxAge, "gas.nativeUsdFeed", "gas.nativeMaxAge")
	if err != nil {
		return OracleConfig{}, err
	}
	cfg := OracleConfig{NativeUSDFeed: native, TokenUSDFeeds: make(map[common.Address]USDFeed, len(raw.TokenUSDFeeds))}
	for index, item := range raw.TokenUSDFeeds {
		path := "gas.tokenUsdFeeds[" + strconv.Itoa(index) + "]"
		token, err := parse.NonZeroAddress(item.Token, path+".token")
		if err != nil {
			return OracleConfig{}, err
		}
		feed, err := parseFeed(item.Feed, item.MaxAge, path+".feed", path+".maxAge")
		if err != nil {
			return OracleConfig{}, err
		}
		if _, exists := cfg.TokenUSDFeeds[token]; exists {
			return OracleConfig{}, errors.Errorf("%s.token: duplicate token %s", path, token.Hex())
		}
		cfg.TokenUSDFeeds[token] = feed
	}
	if len(cfg.TokenUSDFeeds) == 0 {
		return OracleConfig{}, errors.New("gas.tokenUsdFeeds must contain at least one token feed")
	}
	return cfg, nil
}

func parseFeed(address, age, addressField, ageField string) (USDFeed, error) {
	parsed, addressErr := parse.NonZeroAddress(address, addressField)
	maxAge, ageErr := parse.Duration(age, 0, ageField)
	if err := errors.Join(addressErr, ageErr); err != nil {
		return USDFeed{}, err
	}
	if maxAge <= 0 {
		return USDFeed{}, errors.Errorf("%s is required", ageField)
	}
	return USDFeed{Address: parsed, MaxAge: maxAge}, nil
}
