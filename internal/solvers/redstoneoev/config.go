package redstoneoev

import (
	"math/big"
	"net/url"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solver"
)

// rawConfig mirrors the YAML shape; strings/ms are parsed into typed values in parseConfig.
type rawConfig struct {
	WS                  rawWS           `yaml:"ws"`
	Executor            string          `yaml:"executor"`
	Callback            string          `yaml:"callback"`
	Adapter             string          `yaml:"adapter"`
	MorphoAPIURL        string          `yaml:"morphoApiUrl"`
	DiscoveryMaxHF      *float64        `yaml:"discoveryMaxHealthFactor"`
	MaxTrackedPositions *int            `yaml:"maxTrackedPositions"`
	LoanEthFeed         *rawLoanEthFeed `yaml:"loanEthFeed"`
	Bid                 rawBid          `yaml:"bid"`
	Sizing              rawSizing       `yaml:"sizing"`
	Breaker             rawBreaker      `yaml:"breaker"`
	Intervals           rawIntervals    `yaml:"intervals"`
}

type rawWS struct {
	URL       string `yaml:"url"`
	APIKeyEnv string `yaml:"apiKeyEnv"`
}

type rawBid struct {
	BidEth                string `yaml:"bidEth"`
	MinBundleProfitBidBps *int   `yaml:"minBundleProfitBidBps"`
	TotalBundleProfitBps  *int   `yaml:"totalBundleProfitBps"`
	MaxTxGasPriceWei      string `yaml:"maxTxGasPriceWei"`
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

type rawBreaker struct {
	MaxFailures int  `yaml:"maxFailures"`
	WindowMs    *int `yaml:"windowMs"`
}

type rawSizing struct {
	AllowFullLiquidation *bool `yaml:"allowFullLiquidation"`
	SwapHaircutBps       *int  `yaml:"swapHaircutBps"`
}

type rawIntervals struct {
	// Pointers so an omitted field (→ default) is distinguishable from a set-but-invalid one: a present
	// non-positive interval is a misconfiguration and is rejected, never silently defaulted.
	OpsPollMs     *int `yaml:"opsPollMs"`
	MonitorPollMs *int `yaml:"monitorPollMs"`
	MaxStateAgeMs *int `yaml:"maxStateAgeMs"`
}

// Config is the validated, typed redstone-oev configuration.
type Config struct {
	WSURL     string
	APIKeyEnv string

	Executor common.Address
	Callback common.Address
	Adapter  common.Address

	// MorphoAPIURL is the Morpho GraphQL endpoint the solver polls for Morpho market state and at-risk
	// positions. It is required by the production monitor factory.
	MorphoAPIURL string
	// DiscoveryMaxHealthFactor is the API at-risk band ceiling: positions with healthFactor ≤ this are
	// snapshotted, then local Morpho math decides actual liquidatability at the auction price.
	DiscoveryMaxHealthFactor float64
	// MaxTrackedPositions is the Morpho API `first` window and hard in-memory position cap.
	MaxTrackedPositions int

	BidWei                *big.Int
	LoanEthFeed           *loanEthFeed
	MinBundleProfitBidBps int
	TotalBundleProfitBps  int
	MaxTxGasPrice         *big.Int
	Sizing                SizingParams

	BreakerMaxFailures int
	BreakerWindow      time.Duration

	OpsPoll     time.Duration
	MonitorPoll time.Duration
	// MaxStateAge is the maximum age of any background cache (monitor snapshot, ops state) before
	// bidding fails closed on stale_state. Must exceed every background poll interval.
	MaxStateAge time.Duration
}

const (
	defaultAllowFullLiquidation = true           // target 100% collateral unless explicitly disabled
	defaultSwapHaircut          = 200            // 2%
	defaultMaxTxGasPrice        = 60_000_000_000 // 60 gwei
	defaultFeedMaxAge           = time.Hour      // generous Chainlink-style heartbeat bound
	defaultBreakerFails         = 3              // halt after 3 failed liquidations in the window
	defaultBreakerWindow        = time.Hour
	defaultOpsPoll              = 30 * time.Second
	defaultMonitorPoll          = 10 * time.Second // cadence of the monitor snapshot poll
	defaultMaxStateAge          = 90 * time.Second // 3× the slowest default poll; bidding halts past this
	defaultDiscoveryMaxHF       = 1.30             // API at-risk band ceiling (spec §3.2: within 30% of liquidation)
	defaultMaxTrackedPositions  = 10_000           // API `first` window + in-memory at-risk cap
)

// parseConfig decodes and validates the opaque redstone-oev solver config block.
func parseConfig(node yaml.Node) (*Config, error) {
	var raw rawConfig
	if err := solver.DecodeStrict(node, &raw); err != nil { // reject unknown keys → typos fail fast
		return nil, err
	}
	if raw.WS.URL == "" {
		return nil, errors.New("ws.url is required")
	}
	if raw.WS.APIKeyEnv == "" {
		return nil, errors.New("ws.apiKeyEnv is required")
	}
	executor, err := parse.NonZeroAddress(raw.Executor, "executor")
	if err != nil {
		return nil, err
	}
	callback, err := parse.NonZeroAddress(raw.Callback, "callback")
	if err != nil {
		return nil, err
	}

	breakerWindow, err := parse.MsDuration(raw.Breaker.WindowMs, defaultBreakerWindow, "breaker.windowMs")
	if err != nil {
		return nil, err
	}
	opsPoll, err := parse.MsDuration(raw.Intervals.OpsPollMs, defaultOpsPoll, "intervals.opsPollMs")
	if err != nil {
		return nil, err
	}
	monitorPoll, err := parse.MsDuration(raw.Intervals.MonitorPollMs, defaultMonitorPoll, "intervals.monitorPollMs")
	if err != nil {
		return nil, err
	}
	maxStateAge, err := parse.MsDuration(raw.Intervals.MaxStateAgeMs, defaultMaxStateAge, "intervals.maxStateAgeMs")
	if err != nil {
		return nil, err
	}
	// Every background loop must refresh strictly faster than the staleness cutoff, or steady-state
	// bidding would flap between fresh and stale on ordinary poll cadence.
	if opsPoll >= maxStateAge {
		return nil, errors.Errorf("intervals.opsPollMs (%s) must be < intervals.maxStateAgeMs (%s)", opsPoll, maxStateAge)
	}
	if monitorPoll >= maxStateAge {
		return nil, errors.Errorf("intervals.monitorPollMs (%s) must be < intervals.maxStateAgeMs (%s)", monitorPoll, maxStateAge)
	}

	// SwapHaircutBps can't use OrDefault: an explicit 0 (no extra haircut) must be distinguishable from
	// unset (→ defaultSwapHaircut), so the YAML field is a pointer and only nil falls back to the default.
	swapHaircut := defaultSwapHaircut
	if raw.Sizing.SwapHaircutBps != nil {
		swapHaircut = *raw.Sizing.SwapHaircutBps
	}
	allowFullLiquidation := defaultAllowFullLiquidation
	if raw.Sizing.AllowFullLiquidation != nil {
		allowFullLiquidation = *raw.Sizing.AllowFullLiquidation
	}

	cfg := &Config{
		WSURL:     raw.WS.URL,
		APIKeyEnv: raw.WS.APIKeyEnv,
		Executor:  executor,
		Callback:  callback,
		Sizing: SizingParams{
			AllowFullLiquidation: allowFullLiquidation,
			SwapHaircutBps:       swapHaircut,
		},
		BreakerMaxFailures: parse.OrDefault(raw.Breaker.MaxFailures, defaultBreakerFails),
		BreakerWindow:      breakerWindow,
		OpsPoll:            opsPoll,
		MonitorPoll:        monitorPoll,
		MaxStateAge:        maxStateAge,
	}
	if cfg.Adapter, err = parse.NonZeroAddress(raw.Adapter, "adapter"); err != nil {
		return nil, err // required: sizing needs the adapter's redemption rate; the callback pins it as LiquidLaneAdapter
	}
	if cfg.BidWei, err = parse.EthToWei(parse.OrDefault(raw.Bid.BidEth, "0"), "bid.bidEth"); err != nil {
		return nil, err
	}
	if cfg.BidWei.Sign() <= 0 {
		return nil, errors.New("bid.bidEth must be > 0")
	}
	if cfg.LoanEthFeed, err = parseLoanEthFeed(raw.LoanEthFeed); err != nil {
		return nil, err
	}
	if cfg.LoanEthFeed == nil {
		return nil, errors.New("loanEthFeed is required")
	}
	if cfg.MaxTxGasPrice, err = parse.Big(parse.OrDefault(raw.Bid.MaxTxGasPriceWei, big.NewInt(defaultMaxTxGasPrice).String()), "bid.maxTxGasPriceWei"); err != nil {
		return nil, err
	}
	if cfg.MaxTxGasPrice.Sign() <= 0 { // signed into the EXECUTOR_V6 bid as the tx.gasprice ceiling; the contract requires it > 0
		return nil, errors.New("bid.maxTxGasPriceWei must be > 0")
	}
	if raw.Bid.MinBundleProfitBidBps != nil {
		if *raw.Bid.MinBundleProfitBidBps < 0 {
			return nil, errors.New("bid.minBundleProfitBidBps must be >= 0")
		}
		cfg.MinBundleProfitBidBps = *raw.Bid.MinBundleProfitBidBps
	}
	if raw.Bid.TotalBundleProfitBps != nil {
		if *raw.Bid.TotalBundleProfitBps < 0 || *raw.Bid.TotalBundleProfitBps > 10_000 {
			return nil, errors.New("bid.totalBundleProfitBps must be in [0, 10000]")
		}
		cfg.TotalBundleProfitBps = *raw.Bid.TotalBundleProfitBps
	}
	if cfg.Sizing.SwapHaircutBps < 0 || cfg.Sizing.SwapHaircutBps >= 10_000 {
		return nil, errors.Errorf("sizing.swapHaircutBps must be in [0, 10000), got %d", cfg.Sizing.SwapHaircutBps)
	}
	if raw.MorphoAPIURL != "" {
		u, perr := url.Parse(raw.MorphoAPIURL)
		if perr != nil || !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, errors.Errorf("morphoApiUrl must be an absolute http/https URL, got %q", raw.MorphoAPIURL)
		}
		cfg.MorphoAPIURL = raw.MorphoAPIURL
	}
	// At-risk band ceiling for API position snapshots (healthFactor_lte). Default 1.30 (spec §3.2).
	cfg.DiscoveryMaxHealthFactor = defaultDiscoveryMaxHF
	if raw.DiscoveryMaxHF != nil {
		if *raw.DiscoveryMaxHF <= 0 {
			return nil, errors.Errorf("discoveryMaxHealthFactor must be > 0, got %v", *raw.DiscoveryMaxHF)
		}
		cfg.DiscoveryMaxHealthFactor = *raw.DiscoveryMaxHF
	}
	// Bounds the in-memory tracked at-risk set AND doubles as the GraphQL `first` arg; >0 required (0/neg
	// would track nothing).
	cfg.MaxTrackedPositions = defaultMaxTrackedPositions
	if raw.MaxTrackedPositions != nil {
		if *raw.MaxTrackedPositions <= 0 {
			return nil, errors.Errorf("maxTrackedPositions must be > 0, got %d", *raw.MaxTrackedPositions)
		}
		cfg.MaxTrackedPositions = *raw.MaxTrackedPositions
	}
	return cfg, nil
}

// parseLoanEthFeed validates the single loan token's oracle feed config (nil -> no feed). Both feed
// addresses are required; maxAgeMs defaults to defaultFeedMaxAge when unset and must be positive when set.
func parseLoanEthFeed(in *rawLoanEthFeed) (*loanEthFeed, error) {
	if in == nil {
		return nil, nil
	}
	loanFeed, err := parse.Address(in.LoanUsd, "loanEthFeed.loanUsd")
	if err != nil {
		return nil, err
	}
	ethFeed, err := parse.Address(in.EthUsd, "loanEthFeed.ethUsd")
	if err != nil {
		return nil, err
	}
	maxAge := defaultFeedMaxAge
	if in.MaxAgeMs != nil {
		if *in.MaxAgeMs <= 0 {
			return nil, errors.New("loanEthFeed.maxAgeMs must be > 0")
		}
		maxAge = time.Duration(*in.MaxAgeMs) * time.Millisecond
	}
	return &loanEthFeed{LoanUsdFeed: loanFeed, EthUsdFeed: ethFeed, MaxAge: maxAge}, nil
}
