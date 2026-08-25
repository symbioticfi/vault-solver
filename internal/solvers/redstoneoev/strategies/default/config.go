package defaultstrategy

import (
	"net/url"
	"strconv"
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
	defaultMonitorPoll          = 10 * time.Second
	defaultMaxStateAge          = 90 * time.Second
)

type rawConfig struct {
	MorphoAPIURL        string          `yaml:"morphoApiUrl"`
	DiscoveryMaxHF      *float64        `yaml:"discoveryMaxHealthFactor"`
	MaxTrackedPositions *int            `yaml:"maxTrackedPositions"`
	MonitorPollMs       *int            `yaml:"monitorPollMs"`
	MaxStateAgeMs       *int            `yaml:"maxStateAgeMs"`
	TestMonitor         *rawTestMonitor `yaml:"testMonitor"`
	Bid                 rawBidPlan      `yaml:"bid"`
	Sizing              rawSizing       `yaml:"sizing"`
}

type rawTestMonitor struct {
	Markets   []string `yaml:"markets"`
	Positions []string `yaml:"positions"`
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

func defaultConfig() Config {
	return Config{
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
	cfg := defaultConfig()
	cfg.BidWei = bidWei
	authTTL, err := parse.MsDuration(raw.Bid.AuthTtlMs, cfg.CallbackAuthTTL, "strategy.config.bid.authTtlMs")
	if err != nil {
		return Config{}, err
	}
	cfg.CallbackAuthTTL = authTTL
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
	if raw.TestMonitor != nil {
		if len(raw.TestMonitor.Markets) == 0 {
			return Config{}, errors.New("strategy.config.testMonitor.markets must not be empty")
		}
		if len(raw.TestMonitor.Positions) == 0 {
			return Config{}, errors.New("strategy.config.testMonitor.positions must not be empty")
		}
		cfg.TestMonitor = &TestMonitorConfig{
			Markets:   make([]common.Hash, len(raw.TestMonitor.Markets)),
			Positions: make([]common.Address, len(raw.TestMonitor.Positions)),
		}
		for i, value := range raw.TestMonitor.Markets {
			market, parseErr := parse.Hash(value, "strategy.config.testMonitor.markets["+strconv.Itoa(i)+"]")
			if parseErr != nil {
				return Config{}, parseErr
			}
			cfg.TestMonitor.Markets[i] = market
		}
		for i, value := range raw.TestMonitor.Positions {
			position, parseErr := parse.Address(value, "strategy.config.testMonitor.positions["+strconv.Itoa(i)+"]")
			if parseErr != nil {
				return Config{}, parseErr
			}
			cfg.TestMonitor.Positions[i] = position
		}
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
	poll, err := parse.MsDuration(raw.MonitorPollMs, cfg.MonitorPoll, "strategy.config.monitorPollMs")
	if err != nil {
		return Config{}, err
	}
	cfg.MonitorPoll = poll
	maxAge, err := parse.MsDuration(raw.MaxStateAgeMs, cfg.MaxStateAge, "strategy.config.maxStateAgeMs")
	if err != nil {
		return Config{}, err
	}
	cfg.MaxStateAge = maxAge
	if cfg.MonitorPoll >= cfg.MaxStateAge {
		return Config{}, errors.Errorf("strategy.config.monitorPollMs (%s) must be < strategy.config.maxStateAgeMs (%s)", cfg.MonitorPoll, cfg.MaxStateAge)
	}
	return cfg, nil
}
