package lifi

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

type rawConfig struct {
	OrderServer        rawOrderServerConfig     `yaml:"orderServer"`
	InputSettler       string                   `yaml:"inputSettler"`
	OutputSettler      string                   `yaml:"outputSettler"`
	Executor           string                   `yaml:"executor"`
	LiquidityLens      string                   `yaml:"liquidityLens"`
	Adapters           []string                 `yaml:"adapters"`
	TokensToQuote      string                   `yaml:"tokensToQuote"`
	PermissionedTokens []string                 `yaml:"permissionedTokens"`
	QuoteIntervalMs    int                      `yaml:"quoteIntervalMs"`
	QuoteTTL           string                   `yaml:"quoteTtl"`
	QuoteRefreshMode   string                   `yaml:"quoteRefreshMode"`
	SolverMode         string                   `yaml:"solverMode"`
	DiscountsURL       string                   `yaml:"privateDiscountsUrl"`
	Gas                *liquidlanegas.RawConfig `yaml:"gas"`
	Strategy           StrategyConfig           `yaml:"strategy"`
}

type rawOrderServerConfig struct {
	BaseURL     string `yaml:"baseUrl"`
	WSURL       string `yaml:"wsUrl"`
	APIKeyEnv   string `yaml:"apiKeyEnv"`
	HTTPTimeout string `yaml:"httpTimeout"`
}

type Config struct {
	OrderServer   OrderServerConfig
	InputSettler  common.Address
	OutputSettler common.Address
	Executor      common.Address
	// LiquidityLens is the optional FrontendLiquidityLens address. When set, LiquidLane swappable headroom
	// is read from the lens's cross-adapter deallocation-cascade estimate instead of each adapter's own
	// getMaxAssets(tokenToRedeem); zero falls back to the adapter getter.
	LiquidityLens    common.Address
	Adapters         []common.Address
	TokenPolicy      tokenpolicy.Policy
	QuoteInterval    time.Duration
	QuoteTTL         time.Duration
	QuoteRefreshMode string
	SolverMode       liquidlane.SolverMode
	DiscountsURL     string
	Gas              *liquidlanegas.OracleConfig
	Strategy         StrategyConfig
}

type OrderServerConfig struct {
	BaseURL     string
	WSURL       string
	APIKeyEnv   string
	HTTPTimeout time.Duration
}

type StrategyConfig struct {
	Name   string    `yaml:"name"`
	Config yaml.Node `yaml:"config"`
}

const (
	defaultHTTPTimeout       = 10 * time.Second
	defaultQuoteInterval     = 30 * time.Second
	defaultQuoteTTL          = 36 * time.Second
	defaultBlockPollInterval = time.Second
	defaultQuoteRefreshMode  = quoteRefreshModeBlock
	defaultStrategyName      = "default"
)

const (
	quoteRefreshModeInterval = "interval"
	quoteRefreshModeBlock    = "block"
)

func parseConfig(node yaml.Node) (*Config, error) {
	var raw rawConfig
	if err := parse.DecodeStrict(node, &raw); err != nil {
		return nil, err
	}

	inputSettler, err := parse.NonZeroAddress(raw.InputSettler, "inputSettler")
	if err != nil {
		return nil, err
	}
	outputSettler, err := parse.NonZeroAddress(raw.OutputSettler, "outputSettler")
	if err != nil {
		return nil, err
	}
	executor, err := parse.NonZeroAddress(raw.Executor, "executor")
	if err != nil {
		return nil, err
	}
	liquidityLens, err := parse.OptionalNonZeroAddress(raw.LiquidityLens, "liquidityLens")
	if err != nil {
		return nil, err
	}
	adapters, err := parseAdapters(raw.Adapters)
	if err != nil {
		return nil, err
	}
	tokenPolicy, err := tokenpolicy.Parse(raw.TokensToQuote, raw.PermissionedTokens)
	if err != nil {
		return nil, err
	}
	httpTimeout, err := parse.Duration(raw.OrderServer.HTTPTimeout, defaultHTTPTimeout, "orderServer.httpTimeout")
	if err != nil {
		return nil, err
	}
	quoteRefreshMode := parse.OrDefault(raw.QuoteRefreshMode, defaultQuoteRefreshMode)
	if quoteRefreshMode != quoteRefreshModeInterval && quoteRefreshMode != quoteRefreshModeBlock {
		return nil, errors.Errorf("quoteRefreshMode: must be %q or %q, got %q",
			quoteRefreshModeInterval, quoteRefreshModeBlock, quoteRefreshMode)
	}
	quoteInterval, err := parseQuoteInterval(raw.QuoteIntervalMs, quoteRefreshMode)
	if err != nil {
		return nil, err
	}
	quoteTTL, err := parse.Duration(raw.QuoteTTL, defaultQuoteTTL, "quoteTtl")
	if err != nil {
		return nil, err
	}
	if quoteTTL/2 < quoteInterval {
		return nil, errors.Errorf("quoteTtl must be at least twice quote interval %s, got %s", quoteInterval, quoteTTL)
	}
	apiKeyEnv := raw.OrderServer.APIKeyEnv
	if apiKeyEnv == "" {
		return nil, errors.New("orderServer.apiKeyEnv is required")
	}
	if raw.OrderServer.BaseURL == "" {
		return nil, errors.New("orderServer.baseUrl is required")
	}
	if raw.OrderServer.WSURL == "" {
		return nil, errors.New("orderServer.wsUrl is required")
	}
	solverMode, err := liquidlane.ParseSolverMode(raw.SolverMode)
	if err != nil {
		return nil, err
	}
	if solverMode == liquidlane.SolverModeInternal && raw.DiscountsURL == "" {
		return nil, errors.New("privateDiscountsUrl is required in internal solverMode")
	}
	if solverMode == liquidlane.SolverModeExternal && raw.DiscountsURL != "" {
		return nil, errors.New("privateDiscountsUrl requires internal solverMode")
	}
	gas, err := liquidlanegas.ParseOptionalConfig(raw.Gas)
	if err != nil {
		return nil, err
	}
	if raw.Strategy.Name == "" {
		raw.Strategy.Name = defaultStrategyName
	}
	return &Config{
		OrderServer: OrderServerConfig{
			BaseURL:     raw.OrderServer.BaseURL,
			WSURL:       raw.OrderServer.WSURL,
			APIKeyEnv:   apiKeyEnv,
			HTTPTimeout: httpTimeout,
		},
		InputSettler:     inputSettler,
		OutputSettler:    outputSettler,
		Executor:         executor,
		LiquidityLens:    liquidityLens,
		Adapters:         adapters,
		TokenPolicy:      tokenPolicy,
		QuoteInterval:    quoteInterval,
		QuoteTTL:         quoteTTL,
		QuoteRefreshMode: quoteRefreshMode,
		SolverMode:       solverMode,
		DiscountsURL:     raw.DiscountsURL,
		Gas:              gas,
		Strategy:         raw.Strategy,
	}, nil
}

func (c *Config) usesDiscounts() bool { return c.SolverMode == liquidlane.SolverModeInternal }

func parseQuoteInterval(ms int, mode string) (time.Duration, error) {
	if ms == 0 {
		if mode == quoteRefreshModeBlock {
			return defaultBlockPollInterval, nil
		}
		return defaultQuoteInterval, nil
	}
	if ms < 0 {
		return 0, errors.Errorf("quoteIntervalMs: must be positive, got %d", ms)
	}
	return time.Duration(ms) * time.Millisecond, nil
}

func parseAdapters(raw []string) ([]common.Address, error) {
	if len(raw) == 0 {
		return nil, errors.New("at least one adapters entry is required")
	}
	return parse.NonZeroAddresses(raw, "adapters")
}
