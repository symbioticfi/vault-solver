package redstoneoev

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/parse"
	webhookstrategy "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/webhook"
)

// rawConfig mirrors the YAML shape; strings/ms are parsed into typed values in parseConfig.
type rawConfig struct {
	WS               rawWS                    `yaml:"ws"`
	Executor         string                   `yaml:"executor"`
	Adapter          string                   `yaml:"adapter"`
	Callback         string                   `yaml:"callback"`
	LiquidityLens    string                   `yaml:"liquidityLens"`
	Gas              *liquidlanegas.RawConfig `yaml:"gas"`
	Strategy         rawStrategyConfig        `yaml:"strategy"`
	DryRun           bool                     `yaml:"dryRun"`
	MaxTxGasPriceWei string                   `yaml:"maxTxGasPriceWei"`
	MaxBidWei        string                   `yaml:"maxBidWei"`
	Breaker          rawBreaker               `yaml:"breaker"`
	Intervals        rawIntervals             `yaml:"intervals"`
}

type rawWS struct {
	URL       string `yaml:"url"`
	APIKeyEnv string `yaml:"apiKeyEnv"`
}

type rawStrategyConfig struct {
	Name   string    `yaml:"name"`
	Config yaml.Node `yaml:"config"`
}

type rawBreaker struct {
	MaxFailures int  `yaml:"maxFailures"`
	WindowMs    *int `yaml:"windowMs"`
}

type rawIntervals struct {
	// Pointers so an omitted field (→ default) is distinguishable from a set-but-invalid one: a present
	// non-positive interval is a misconfiguration and is rejected, never silently defaulted.
	OpsPollMs             *int `yaml:"opsPollMs"`
	ExecutorStateMaxAgeMs *int `yaml:"executorStateMaxAgeMs"`
}

// Config is the validated, typed redstone-oev configuration.
type Config struct {
	WSURL     string
	APIKeyEnv string

	Executor common.Address
	Adapter  common.Address
	Callback common.Address
	// LiquidityLens is the optional FrontendLiquidityLens address. When set, LiquidLane swappable headroom
	// is read from the lens's cross-adapter deallocation-cascade estimate instead of the adapter's own
	// getMaxAssets(tokenToRedeem); zero falls back to the adapter getter.
	LiquidityLens common.Address
	Gas           *liquidlanegas.OracleConfig

	Strategy StrategyConfig
	DryRun   bool

	MaxTxGasPrice *big.Int
	MaxBidWei     *big.Int

	BreakerMaxFailures int
	BreakerWindow      time.Duration

	OpsPoll time.Duration
	// ExecutorStateMaxAge is the maximum age of the solver-owned Executor accounting cache before
	// bidding fails closed on executor_state_stale.
	ExecutorStateMaxAge time.Duration
}

type StrategyConfig struct {
	Name   string
	Config yaml.Node
}

const (
	defaultMaxTxGasPrice       = 60_000_000_000 // 60 gwei
	defaultBreakerFails        = 3              // halt after 3 failed liquidations in the window
	defaultBreakerWindow       = time.Hour
	defaultOpsPoll             = 10 * time.Second
	defaultExecutorStateMaxAge = 30 * time.Second
	defaultStrategyName        = "default"
)

// parseConfig decodes and validates the opaque redstone-oev solver config block.
func parseConfig(node yaml.Node) (*Config, error) {
	var raw rawConfig
	if err := parse.DecodeStrict(node, &raw); err != nil { // reject unknown keys → typos fail fast
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
	adapter, err := parse.NonZeroAddress(raw.Adapter, "adapter")
	if err != nil {
		return nil, err
	}
	callback, err := parse.NonZeroAddress(raw.Callback, "callback")
	if err != nil {
		return nil, err
	}
	var liquidityLens common.Address
	if raw.LiquidityLens != "" {
		if liquidityLens, err = parse.NonZeroAddress(raw.LiquidityLens, "liquidityLens"); err != nil {
			return nil, err
		}
	}
	var gas *liquidlanegas.OracleConfig
	if raw.Gas != nil {
		parsed, gasErr := liquidlanegas.ParseConfig(*raw.Gas)
		if gasErr != nil {
			return nil, gasErr
		}
		gas = &parsed
	}

	breakerWindow, err := parse.MsDuration(raw.Breaker.WindowMs, defaultBreakerWindow, "breaker.windowMs")
	if err != nil {
		return nil, err
	}
	opsPoll, err := parse.MsDuration(raw.Intervals.OpsPollMs, defaultOpsPoll, "intervals.opsPollMs")
	if err != nil {
		return nil, err
	}
	executorStateMaxAge, err := parse.MsDuration(raw.Intervals.ExecutorStateMaxAgeMs, defaultExecutorStateMaxAge, "intervals.executorStateMaxAgeMs")
	if err != nil {
		return nil, err
	}
	if opsPoll >= executorStateMaxAge {
		return nil, errors.Errorf("intervals.opsPollMs (%s) must be < intervals.executorStateMaxAgeMs (%s)", opsPoll, executorStateMaxAge)
	}

	cfg := &Config{
		WSURL:         raw.WS.URL,
		APIKeyEnv:     raw.WS.APIKeyEnv,
		Executor:      executor,
		Adapter:       adapter,
		Callback:      callback,
		LiquidityLens: liquidityLens,
		Gas:           gas,
		Strategy: StrategyConfig{
			Name:   parse.OrDefault(raw.Strategy.Name, defaultStrategyName),
			Config: raw.Strategy.Config,
		},
		DryRun:              raw.DryRun,
		BreakerMaxFailures:  parse.OrDefault(raw.Breaker.MaxFailures, defaultBreakerFails),
		BreakerWindow:       breakerWindow,
		OpsPoll:             opsPoll,
		ExecutorStateMaxAge: executorStateMaxAge,
	}
	if cfg.MaxTxGasPrice, err = parse.Big(parse.OrDefault(raw.MaxTxGasPriceWei, big.NewInt(defaultMaxTxGasPrice).String()), "maxTxGasPriceWei"); err != nil {
		return nil, err
	}
	if cfg.MaxTxGasPrice.Sign() <= 0 { // signed into the EXECUTOR_V6 bid as the tx.gasprice ceiling; the contract requires it > 0
		return nil, errors.New("maxTxGasPriceWei must be > 0")
	}
	if raw.MaxBidWei != "" {
		if cfg.MaxBidWei, err = parse.Big(raw.MaxBidWei, "maxBidWei"); err != nil {
			return nil, err
		}
		if cfg.MaxBidWei.Sign() <= 0 {
			return nil, errors.New("maxBidWei must be > 0")
		}
	}
	if cfg.Strategy.Name == webhookstrategy.Name && cfg.MaxBidWei == nil {
		return nil, errors.Errorf("maxBidWei is required for %s strategy", cfg.Strategy.Name)
	}
	return cfg, nil
}
