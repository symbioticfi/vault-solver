package lifi

import (
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

// RawEngineConfig is the configuration every LI.FI solver shares, whatever settles its
// orders: the order server, the settlement contracts, the token scope, the standing-quote
// cadence, the discount service and the strategy. Each solver inlines it into its own config.
type RawEngineConfig struct {
	OrderServer        rawOrderServerConfig `yaml:"orderServer"`
	InputSettler       string               `yaml:"inputSettler"`
	OutputSettler      string               `yaml:"outputSettler"`
	Executor           string               `yaml:"executor"`
	TokensToQuote      string               `yaml:"tokensToQuote"`
	PermissionedTokens []string             `yaml:"permissionedTokens"`
	QuoteIntervalMs    int                  `yaml:"quoteIntervalMs"`
	QuoteTTL           string               `yaml:"quoteTtl"`
	QuoteRefreshMode   string               `yaml:"quoteRefreshMode"`
	DiscountsURL       string               `yaml:"privateDiscountsUrl"`
	Strategy           rawStrategyConfig    `yaml:"strategy"`
}

// rawConfig is the lifi-samechain config: the shared engine settling through native adapters.
type rawConfig struct {
	RawEngineConfig `yaml:",inline"`

	LiquidityLens string                   `yaml:"liquidityLens"`
	Adapters      []string                 `yaml:"adapters"`
	SolverMode    string                   `yaml:"solverMode"`
	Gas           *liquidlanegas.RawConfig `yaml:"gas"`
}

type rawOrderServerConfig struct {
	BaseURL        string `yaml:"baseUrl"`
	WSURL          string `yaml:"wsUrl"`
	APIKeyEnv      string `yaml:"apiKeyEnv"`
	HTTPTimeout    string `yaml:"httpTimeout"`
	WSReadTimeout  string `yaml:"wsReadTimeout"`
	WSWriteTimeout string `yaml:"wsWriteTimeout"`
}

type rawStrategyConfig struct {
	Name   string    `yaml:"name"`
	Config yaml.Node `yaml:"config"`
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
	SolverMode       string
	DiscountsURL     string
	Gas              *liquidlanegas.OracleConfig
	Strategy         StrategyConfig
}

type OrderServerConfig struct {
	BaseURL        string
	WSURL          string
	APIKeyEnv      string
	HTTPTimeout    time.Duration
	WSReadTimeout  time.Duration
	WSWriteTimeout time.Duration
}

type StrategyConfig struct {
	Name   string
	Config yaml.Node
}

const (
	defaultHTTPTimeout       = 10 * time.Second
	defaultWSReadTimeout     = 60 * time.Second
	defaultWSWriteTimeout    = 10 * time.Second
	defaultQuoteInterval     = 30 * time.Second
	defaultQuoteTTL          = 36 * time.Second
	defaultBlockPollInterval = time.Second
	defaultQuoteRefreshMode  = quoteRefreshModeBlock
	defaultStrategyName      = "default"
	webhookStrategyName      = "webhook"
	defaultSolverMode        = solverModeExternal
)

const (
	quoteRefreshModeInterval = "interval"
	quoteRefreshModeBlock    = "block"
	solverModeExternal       = "external"
	solverModeInternal       = "internal"
)

func parseConfig(node yaml.Node) (*Config, error) {
	var raw rawConfig
	if err := solver.DecodeStrict(node, &raw); err != nil {
		return nil, err
	}
	cfg, err := raw.parse()
	if err != nil {
		return nil, err
	}
	if raw.LiquidityLens != "" {
		if cfg.LiquidityLens, err = parse.NonZeroAddress(raw.LiquidityLens, "liquidityLens"); err != nil {
			return nil, err
		}
	}
	if cfg.Adapters, err = parseAdapters(raw.Adapters); err != nil {
		return nil, err
	}
	cfg.SolverMode = parse.OrDefault(raw.SolverMode, defaultSolverMode)
	switch {
	case cfg.SolverMode != solverModeExternal && cfg.SolverMode != solverModeInternal:
		return nil, errors.Errorf("solverMode: must be %q or %q, got %q", solverModeExternal, solverModeInternal, cfg.SolverMode)
	case cfg.SolverMode == solverModeInternal && cfg.DiscountsURL == "":
		return nil, errors.New("privateDiscountsUrl is required in internal solverMode")
	case cfg.SolverMode == solverModeExternal && cfg.DiscountsURL != "":
		return nil, errors.New("privateDiscountsUrl requires internal solverMode")
	}
	if raw.Gas != nil {
		gas, err := liquidlanegas.ParseConfig(*raw.Gas)
		if err != nil {
			return nil, err
		}
		cfg.Gas = &gas
	}
	return cfg, nil
}

// ParseRecipeConfig resolves the engine config of a solver that settles through connector
// recipes. Such a solver is discount-backed, its sender pays gas, and its quotes are priced
// from connector inventory, which the webhook wire shape does not carry.
func ParseRecipeConfig(raw RawEngineConfig) (*Config, error) {
	cfg, err := raw.parse()
	if err != nil {
		return nil, err
	}
	if cfg.DiscountsURL == "" {
		return nil, errors.New("privateDiscountsUrl is required")
	}
	if cfg.Strategy.Name == webhookStrategyName {
		return nil, errors.New("the webhook strategy does not support connector recipes")
	}
	cfg.SolverMode = solverModeInternal
	return cfg, nil
}

// parse resolves the settings every LI.FI solver shares.
func (raw RawEngineConfig) parse() (*Config, error) {
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
	tokenPolicy, err := tokenpolicy.Parse(raw.TokensToQuote, raw.PermissionedTokens)
	if err != nil {
		return nil, err
	}
	httpTimeout, err := parse.Duration(raw.OrderServer.HTTPTimeout, defaultHTTPTimeout, "orderServer.httpTimeout")
	if err != nil {
		return nil, err
	}
	wsReadTimeout, err := parse.Duration(raw.OrderServer.WSReadTimeout, defaultWSReadTimeout, "orderServer.wsReadTimeout")
	if err != nil {
		return nil, err
	}
	wsWriteTimeout, err := parse.Duration(raw.OrderServer.WSWriteTimeout, defaultWSWriteTimeout, "orderServer.wsWriteTimeout")
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
	if raw.OrderServer.APIKeyEnv == "" {
		return nil, errors.New("orderServer.apiKeyEnv is required")
	}
	if raw.OrderServer.BaseURL == "" {
		return nil, errors.New("orderServer.baseUrl is required")
	}
	if raw.OrderServer.WSURL == "" {
		return nil, errors.New("orderServer.wsUrl is required")
	}
	return &Config{
		OrderServer: OrderServerConfig{
			BaseURL:        raw.OrderServer.BaseURL,
			WSURL:          raw.OrderServer.WSURL,
			APIKeyEnv:      raw.OrderServer.APIKeyEnv,
			HTTPTimeout:    httpTimeout,
			WSReadTimeout:  wsReadTimeout,
			WSWriteTimeout: wsWriteTimeout,
		},
		InputSettler:     inputSettler,
		OutputSettler:    outputSettler,
		Executor:         executor,
		TokenPolicy:      tokenPolicy,
		QuoteInterval:    quoteInterval,
		QuoteTTL:         quoteTTL,
		QuoteRefreshMode: quoteRefreshMode,
		DiscountsURL:     raw.DiscountsURL,
		Strategy: StrategyConfig{
			Name:   parse.OrDefault(raw.Strategy.Name, defaultStrategyName),
			Config: raw.Strategy.Config,
		},
	}, nil
}

func (c *Config) usesDiscounts() bool { return c.SolverMode == solverModeInternal }

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
	out := make([]common.Address, 0, len(raw))
	seen := make(map[common.Address]bool, len(raw))
	for i, a := range raw {
		addr, err := parse.NonZeroAddress(a, "adapters["+strconv.Itoa(i)+"]")
		if err != nil {
			return nil, err
		}
		if seen[addr] {
			return nil, errors.Errorf("adapters[%d]: duplicate adapter %s", i, addr.Hex())
		}
		seen[addr] = true
		out = append(out, addr)
	}
	return out, nil
}
