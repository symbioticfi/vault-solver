package defaultstrategy

import (
	"math"
	"net/url"
	"time"

	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/bigmath"
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
	MorphoAPIURL        string     `yaml:"morphoApiUrl"`
	DiscoveryMaxHF      *float64   `yaml:"discoveryMaxHealthFactor"`
	MaxTrackedPositions *int       `yaml:"maxTrackedPositions"`
	MonitorPollMs       *int       `yaml:"monitorPollMs"`
	MaxStateAgeMs       *int       `yaml:"maxStateAgeMs"`
	Bid                 rawBidPlan `yaml:"bid"`
	Sizing              rawSizing  `yaml:"sizing"`
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
		DiscoveryMaxHealthFactor: defaultDiscoveryMaxHF,
		MaxTrackedPositions:      defaultMaxTrackedPositions,
		Sizing: SizingParams{
			AllowFullLiquidation: defaultAllowFullLiquidation,
			SwapHaircutBps:       defaultSwapHaircut,
		},
	}
	for _, duration := range []struct {
		field    string
		raw      *int
		fallback time.Duration
		out      *time.Duration
	}{
		{"strategy.config.bid.authTtlMs", raw.Bid.AuthTtlMs, defaultCallbackAuthTTL, &cfg.CallbackAuthTTL},
		{"strategy.config.monitorPollMs", raw.MonitorPollMs, defaultMonitorPoll, &cfg.MonitorPoll},
		{"strategy.config.maxStateAgeMs", raw.MaxStateAgeMs, defaultMaxStateAge, &cfg.MaxStateAge},
	} {
		if *duration.out, err = parse.MsDuration(duration.raw, duration.fallback, duration.field); err != nil {
			return Config{}, err
		}
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
		if *raw.DiscoveryMaxHF <= 0 || math.IsNaN(*raw.DiscoveryMaxHF) || math.IsInf(*raw.DiscoveryMaxHF, 0) {
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
	if cfg.MonitorPoll >= cfg.MaxStateAge {
		return Config{}, errors.Errorf("strategy.config.monitorPollMs (%s) must be < strategy.config.maxStateAgeMs (%s)", cfg.MonitorPoll, cfg.MaxStateAge)
	}
	return cfg, nil
}

func ConfigForTest(cfg Config) Config {
	cfg.BidWei = bigmath.Clone(cfg.BidWei)
	cfg.DiscoveryMaxHealthFactor = parse.OrDefault(cfg.DiscoveryMaxHealthFactor, defaultDiscoveryMaxHF)
	cfg.MaxTrackedPositions = parse.OrDefault(cfg.MaxTrackedPositions, defaultMaxTrackedPositions)
	cfg.CallbackAuthTTL = parse.OrDefault(cfg.CallbackAuthTTL, defaultCallbackAuthTTL)
	cfg.MonitorPoll = parse.OrDefault(cfg.MonitorPoll, defaultMonitorPoll)
	cfg.MaxStateAge = parse.OrDefault(cfg.MaxStateAge, defaultMaxStateAge)
	cfg.Sizing = parse.OrDefault(cfg.Sizing, SizingParams{AllowFullLiquidation: defaultAllowFullLiquidation, SwapHaircutBps: defaultSwapHaircut})
	return cfg
}
