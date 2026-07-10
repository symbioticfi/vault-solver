package defaultstrategy

import (
	"math/big"
	"net/url"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/parse"
)

const (
	defaultAllowFullLiquidation = true
	defaultSwapHaircut          = 200
	defaultDiscoveryMaxHF       = 1.30
	defaultMaxTrackedPositions  = 10_000
	defaultCallbackAuthTTL      = time.Minute
	defaultFeedMaxAge           = time.Hour
	defaultMonitorPoll          = 10 * time.Second
	defaultMaxStateAge          = 90 * time.Second
)

type rawConfig struct {
	LoanEthFeed         *rawLoanEthFeed `yaml:"loanEthFeed"`
	MorphoAPIURL        string          `yaml:"morphoApiUrl"`
	DiscoveryMaxHF      *float64        `yaml:"discoveryMaxHealthFactor"`
	MaxTrackedPositions *int            `yaml:"maxTrackedPositions"`
	MonitorPollMs       *int            `yaml:"monitorPollMs"`
	MaxStateAgeMs       *int            `yaml:"maxStateAgeMs"`
	Bid                 rawBidPlan      `yaml:"bid"`
	Sizing              rawSizing       `yaml:"sizing"`
}

type rawLoanEthFeed struct {
	EthUsd   string `yaml:"ethUsd"`
	LoanUsd  string `yaml:"loanUsd"`
	MaxAgeMs *int   `yaml:"maxAgeMs"`
}

type loanEthFeed struct {
	LoanUsdFeed common.Address
	EthUsdFeed  common.Address
	MaxAge      time.Duration
}

type rawBidPlan struct {
	BidEth                string `yaml:"bidEth"`
	AuthTtlMs             *int   `yaml:"authTtlMs"`
	MinBundleProfitBidBps *int   `yaml:"minBundleProfitBidBps"`
	TotalBundleProfitBps  *int   `yaml:"totalBundleProfitBps"`
}

type rawSizing struct {
	AllowFullLiquidation *bool `yaml:"allowFullLiquidation"`
	SwapHaircutBps       *int  `yaml:"swapHaircutBps"`
}

func ParseConfig(node yaml.Node) (Config, error) {
	var raw rawConfig
	if err := parse.DecodeStrict(node, &raw); err != nil {
		return Config{}, err
	}
	bidWei, err := parse.EthToWei(parse.OrDefault(raw.Bid.BidEth, "0"), "strategy.config.bid.bidEth")
	if err != nil {
		return Config{}, err
	}
	if bidWei.Sign() <= 0 {
		return Config{}, errors.New("strategy.config.bid.bidEth must be > 0")
	}
	cfg := Config{
		BidWei:                   bidWei,
		CallbackAuthTTL:          defaultCallbackAuthTTL,
		MonitorPoll:              defaultMonitorPoll,
		MaxStateAge:              defaultMaxStateAge,
		DiscoveryMaxHealthFactor: defaultDiscoveryMaxHF,
		MaxTrackedPositions:      defaultMaxTrackedPositions,
		Sizing: SizingParams{
			AllowFullLiquidation: defaultAllowFullLiquidation,
			SwapHaircutBps:       defaultSwapHaircut,
		},
	}
	if cfg.LoanEthFeed, err = parseLoanEthFeed(raw.LoanEthFeed); err != nil {
		return Config{}, err
	}
	if cfg.LoanEthFeed == nil {
		return Config{}, errors.New("strategy.config.loanEthFeed is required")
	}
	if raw.Bid.AuthTtlMs != nil {
		authTTL, err := parse.MsDuration(raw.Bid.AuthTtlMs, cfg.CallbackAuthTTL, "strategy.config.bid.authTtlMs")
		if err != nil {
			return Config{}, err
		}
		cfg.CallbackAuthTTL = authTTL
	}
	if cfg.CallbackAuthTTL <= 0 {
		return Config{}, errors.New("strategy.config.bid.authTtlMs must be > 0")
	}
	if raw.Bid.MinBundleProfitBidBps != nil {
		if *raw.Bid.MinBundleProfitBidBps < 0 {
			return Config{}, errors.New("strategy.config.bid.minBundleProfitBidBps must be >= 0")
		}
		cfg.MinBundleProfitBidBps = *raw.Bid.MinBundleProfitBidBps
	}
	if raw.Bid.TotalBundleProfitBps != nil {
		if *raw.Bid.TotalBundleProfitBps < 0 || *raw.Bid.TotalBundleProfitBps > 10_000 {
			return Config{}, errors.New("strategy.config.bid.totalBundleProfitBps must be in [0, 10000]")
		}
		cfg.TotalBundleProfitBps = *raw.Bid.TotalBundleProfitBps
	}
	if raw.Sizing.SwapHaircutBps != nil {
		cfg.Sizing.SwapHaircutBps = *raw.Sizing.SwapHaircutBps
	}
	if raw.Sizing.AllowFullLiquidation != nil {
		cfg.Sizing.AllowFullLiquidation = *raw.Sizing.AllowFullLiquidation
	}
	if cfg.Sizing.SwapHaircutBps < 0 || cfg.Sizing.SwapHaircutBps >= 10_000 {
		return Config{}, errors.Errorf("strategy.config.sizing.swapHaircutBps must be in [0, 10000), got %d", cfg.Sizing.SwapHaircutBps)
	}
	if raw.MorphoAPIURL != "" {
		u, perr := url.Parse(raw.MorphoAPIURL)
		if perr != nil || !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return Config{}, errors.Errorf("strategy.config.morphoApiUrl must be an absolute http/https URL, got %q", raw.MorphoAPIURL)
		}
		cfg.MorphoAPIURL = raw.MorphoAPIURL
	}
	if raw.DiscoveryMaxHF != nil {
		if *raw.DiscoveryMaxHF <= 0 {
			return Config{}, errors.Errorf("strategy.config.discoveryMaxHealthFactor must be > 0, got %v", *raw.DiscoveryMaxHF)
		}
		cfg.DiscoveryMaxHealthFactor = *raw.DiscoveryMaxHF
	}
	if raw.MaxTrackedPositions != nil {
		if *raw.MaxTrackedPositions <= 0 {
			return Config{}, errors.Errorf("strategy.config.maxTrackedPositions must be > 0, got %d", *raw.MaxTrackedPositions)
		}
		cfg.MaxTrackedPositions = *raw.MaxTrackedPositions
	}
	if raw.MonitorPollMs != nil {
		poll, err := parse.MsDuration(raw.MonitorPollMs, cfg.MonitorPoll, "strategy.config.monitorPollMs")
		if err != nil {
			return Config{}, err
		}
		cfg.MonitorPoll = poll
	}
	if raw.MaxStateAgeMs != nil {
		maxAge, err := parse.MsDuration(raw.MaxStateAgeMs, cfg.MaxStateAge, "strategy.config.maxStateAgeMs")
		if err != nil {
			return Config{}, err
		}
		cfg.MaxStateAge = maxAge
	}
	if cfg.MonitorPoll >= cfg.MaxStateAge {
		return Config{}, errors.Errorf("strategy.config.monitorPollMs (%s) must be < strategy.config.maxStateAgeMs (%s)", cfg.MonitorPoll, cfg.MaxStateAge)
	}
	return cfg, nil
}

func parseLoanEthFeed(in *rawLoanEthFeed) (*loanEthFeed, error) {
	if in == nil {
		return nil, nil
	}
	loanFeed, err := parse.NonZeroAddress(in.LoanUsd, "strategy.config.loanEthFeed.loanUsd")
	if err != nil {
		return nil, err
	}
	ethFeed, err := parse.NonZeroAddress(in.EthUsd, "strategy.config.loanEthFeed.ethUsd")
	if err != nil {
		return nil, err
	}
	maxAge := defaultFeedMaxAge
	if in.MaxAgeMs != nil {
		if *in.MaxAgeMs <= 0 {
			return nil, errors.New("strategy.config.loanEthFeed.maxAgeMs must be > 0")
		}
		maxAge = time.Duration(*in.MaxAgeMs) * time.Millisecond
	}
	return &loanEthFeed{LoanUsdFeed: loanFeed, EthUsdFeed: ethFeed, MaxAge: maxAge}, nil
}

func ConfigForTest(overrides Config) Config {
	cfg := Config{
		DiscoveryMaxHealthFactor: defaultDiscoveryMaxHF,
		MaxTrackedPositions:      defaultMaxTrackedPositions,
		CallbackAuthTTL:          defaultCallbackAuthTTL,
		MonitorPoll:              defaultMonitorPoll,
		MaxStateAge:              defaultMaxStateAge,
		Sizing: SizingParams{
			AllowFullLiquidation: defaultAllowFullLiquidation,
			SwapHaircutBps:       defaultSwapHaircut,
		},
	}
	if overrides.MorphoAPIURL != "" {
		cfg.MorphoAPIURL = overrides.MorphoAPIURL
	}
	if overrides.DiscoveryMaxHealthFactor != 0 {
		cfg.DiscoveryMaxHealthFactor = overrides.DiscoveryMaxHealthFactor
	}
	if overrides.MaxTrackedPositions != 0 {
		cfg.MaxTrackedPositions = overrides.MaxTrackedPositions
	}
	if overrides.BidWei != nil {
		cfg.BidWei = new(big.Int).Set(overrides.BidWei)
	}
	if overrides.LoanEthFeed != nil {
		cfg.LoanEthFeed = &loanEthFeed{
			LoanUsdFeed: overrides.LoanEthFeed.LoanUsdFeed,
			EthUsdFeed:  overrides.LoanEthFeed.EthUsdFeed,
			MaxAge:      overrides.LoanEthFeed.MaxAge,
		}
	}
	if overrides.MinBundleProfitBidBps != 0 {
		cfg.MinBundleProfitBidBps = overrides.MinBundleProfitBidBps
	}
	if overrides.TotalBundleProfitBps != 0 {
		cfg.TotalBundleProfitBps = overrides.TotalBundleProfitBps
	}
	if overrides.Sizing != (SizingParams{}) {
		cfg.Sizing = overrides.Sizing
	}
	cfg.Adapter = overrides.Adapter
	if overrides.CallbackAuthTTL != 0 {
		cfg.CallbackAuthTTL = overrides.CallbackAuthTTL
	}
	if overrides.MonitorPoll != 0 {
		cfg.MonitorPoll = overrides.MonitorPoll
	}
	if overrides.MaxStateAge != 0 {
		cfg.MaxStateAge = overrides.MaxStateAge
	}
	return cfg
}
