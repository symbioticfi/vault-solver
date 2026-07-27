package uniswapx

import (
	"net"
	"net/url"
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

const (
	defaultListenAddress    = ":42080"
	defaultHTTPTimeout      = 450 * time.Millisecond
	defaultPollInterval     = time.Second
	defaultRefreshInterval  = 12 * time.Second
	defaultQuoteTTL         = 30 * time.Second
	defaultDiscountTimeout  = 2 * time.Second
	defaultDiscountValidity = 15 * time.Second
	defaultStrategyName     = "default"
	defaultSolverMode       = solverModeExternal
	solverModeExternal      = "external"
	solverModeInternal      = "internal"
)

type rawConfig struct {
	Reactor       string                   `yaml:"reactor"`
	Executor      string                   `yaml:"executor"`
	LiquidityLens string                   `yaml:"liquidityLens"`
	Adapters      []string                 `yaml:"adapters"`
	SolverMode    string                   `yaml:"solverMode"`
	TokensToQuote string                   `yaml:"tokensToQuote"`
	Permissioned  []string                 `yaml:"permissionedTokens"`
	QuoteServer   rawQuoteServerConfig     `yaml:"quoteServer"`
	OrderServer   rawOrderServerConfig     `yaml:"orderServer"`
	Discounts     *rawDiscountConfig       `yaml:"discounts"`
	Gas           *liquidlanegas.RawConfig `yaml:"gas"`
	Breaker       rawBreakerConfig         `yaml:"breaker"`
	Strategy      rawStrategyConfig        `yaml:"strategy"`
}

type rawDiscountConfig struct {
	BaseURL         string `yaml:"baseUrl"`
	HTTPTimeout     string `yaml:"httpTimeout"`
	MinimumValidity string `yaml:"minimumValidity"`
}

type rawQuoteServerConfig struct {
	ListenAddress   string `yaml:"listenAddress"`
	HTTPTimeout     string `yaml:"httpTimeout"`
	RefreshInterval string `yaml:"refreshInterval"`
	QuoteTTL        string `yaml:"quoteTtl"`
}

type rawOrderServerConfig struct {
	BaseURL      string                `yaml:"baseUrl"`
	APIKeyEnv    string                `yaml:"apiKeyEnv"`
	PollInterval string                `yaml:"pollInterval"`
	HTTPTimeout  string                `yaml:"httpTimeout"`
	Beta         bool                  `yaml:"beta"`
	Sources      rawOrderSourcesConfig `yaml:"sources"`
}

type rawOrderSourcesConfig struct {
	ExclusiveV2 *bool `yaml:"exclusiveV2"`
	PublicV2    bool  `yaml:"publicV2"`
}

type rawStrategyConfig struct {
	Name   string    `yaml:"name"`
	Config yaml.Node `yaml:"config"`
}

type rawBreakerConfig struct {
	MaxFailures int    `yaml:"maxFailures"`
	Window      string `yaml:"window"`
}

type Config struct {
	Reactor  common.Address
	Executor common.Address
	// LiquidityLens is the optional FrontendLiquidityLens address. When set, LiquidLane swappable headroom
	// is read from the lens's cross-adapter deallocation-cascade estimate instead of each adapter's own
	// getMaxAssets(tokenToRedeem); zero falls back to the adapter getter.
	LiquidityLens common.Address
	Adapters      []common.Address
	SolverMode    string
	TokenPolicy   tokenpolicy.Policy
	QuoteServer   QuoteServerConfig
	OrderServer   OrderServerConfig
	Discounts     *DiscountConfig
	Gas           *liquidlanegas.OracleConfig
	Breaker       BreakerConfig
	Strategy      StrategyConfig
}

type DiscountConfig struct {
	BaseURL         string
	HTTPTimeout     time.Duration
	MinimumValidity time.Duration
}

type QuoteServerConfig struct {
	ListenAddress   string
	HTTPTimeout     time.Duration
	RefreshInterval time.Duration
	QuoteTTL        time.Duration
}

type OrderServerConfig struct {
	BaseURL      string
	APIKeyEnv    string
	PollInterval time.Duration
	HTTPTimeout  time.Duration
	Beta         bool
	Sources      OrderSourcesConfig
}

type OrderSourcesConfig struct {
	ExclusiveV2 bool
	PublicV2    bool
}

type StrategyConfig struct {
	Name   string
	Config yaml.Node
}

type BreakerConfig struct {
	MaxFailures int
	Window      time.Duration
}

func parseConfig(node yaml.Node) (*Config, error) {
	var raw rawConfig
	if err := solver.DecodeStrict(node, &raw); err != nil {
		return nil, err
	}
	reactor, err := parse.NonZeroAddress(raw.Reactor, "reactor")
	if err != nil {
		return nil, err
	}
	executor, err := parse.NonZeroAddress(raw.Executor, "executor")
	if err != nil {
		return nil, err
	}
	var liquidityLens common.Address
	if raw.LiquidityLens != "" {
		if liquidityLens, err = parse.NonZeroAddress(raw.LiquidityLens, "liquidityLens"); err != nil {
			return nil, err
		}
	}
	adapters, err := parseAddressList(raw.Adapters, "adapters")
	if err != nil {
		return nil, err
	}
	solverMode := parse.OrDefault(raw.SolverMode, defaultSolverMode)
	if solverMode != solverModeExternal && solverMode != solverModeInternal {
		return nil, errors.Errorf(
			"solverMode: must be %q or %q, got %q",
			solverModeExternal,
			solverModeInternal,
			solverMode,
		)
	}
	if solverMode == solverModeExternal && len(adapters) == 0 {
		return nil, errors.New(`solverMode "external" requires at least one adapters entry`)
	}
	policy, err := tokenpolicy.Parse(raw.TokensToQuote, raw.Permissioned)
	if err != nil {
		return nil, err
	}
	quoteHTTPTimeout, err := parse.Duration(raw.QuoteServer.HTTPTimeout, defaultHTTPTimeout, "quoteServer.httpTimeout")
	if err != nil {
		return nil, err
	}
	quoteTTL, err := parse.Duration(raw.QuoteServer.QuoteTTL, defaultQuoteTTL, "quoteServer.quoteTtl")
	if err != nil {
		return nil, err
	}
	refreshInterval, err := parse.Duration(
		raw.QuoteServer.RefreshInterval,
		defaultRefreshInterval,
		"quoteServer.refreshInterval",
	)
	if err != nil {
		return nil, err
	}
	if quoteTTL/2 < refreshInterval {
		return nil, errors.Errorf(
			"quoteServer.quoteTtl must be at least twice refresh interval %s, got %s",
			refreshInterval,
			quoteTTL,
		)
	}
	pollInterval, err := parse.Duration(raw.OrderServer.PollInterval, defaultPollInterval, "orderServer.pollInterval")
	if err != nil {
		return nil, err
	}
	if pollInterval < 167*time.Millisecond {
		return nil, errors.New("orderServer.pollInterval must be at least 167ms")
	}
	orderHTTPTimeout, err := parse.Duration(raw.OrderServer.HTTPTimeout, 5*time.Second, "orderServer.httpTimeout")
	if err != nil {
		return nil, err
	}
	if raw.OrderServer.APIKeyEnv == "" {
		return nil, errors.New("orderServer.apiKeyEnv is required")
	}
	if raw.OrderServer.BaseURL == "" {
		return nil, errors.New("orderServer.baseUrl is required")
	}
	if err := validateServiceURL(raw.OrderServer.BaseURL, "orderServer.baseUrl"); err != nil {
		return nil, err
	}
	exclusiveV2 := true
	if raw.OrderServer.Sources.ExclusiveV2 != nil {
		exclusiveV2 = *raw.OrderServer.Sources.ExclusiveV2
	}
	sources := OrderSourcesConfig{ExclusiveV2: exclusiveV2, PublicV2: raw.OrderServer.Sources.PublicV2}
	if !sources.ExclusiveV2 {
		return nil, errors.New("orderServer.sources.exclusiveV2 must be enabled while quote server is enabled")
	}
	if solverMode == solverModeInternal && raw.Discounts == nil {
		return nil, errors.New("discounts is required in internal solverMode")
	}
	if solverMode == solverModeExternal && raw.Discounts != nil {
		return nil, errors.New("discounts requires internal solverMode")
	}
	discountConfig, err := parseDiscountConfig(raw.Discounts)
	if err != nil {
		return nil, err
	}
	var gas *liquidlanegas.OracleConfig
	if raw.Gas != nil {
		parsed, gasErr := liquidlanegas.ParseConfig(*raw.Gas)
		if gasErr != nil {
			return nil, gasErr
		}
		gas = &parsed
	}
	breaker, err := parseBreakerConfig(raw.Breaker)
	if err != nil {
		return nil, err
	}
	return &Config{
		Reactor: reactor, Executor: executor, LiquidityLens: liquidityLens,
		Adapters: adapters, SolverMode: solverMode, TokenPolicy: policy,
		QuoteServer: QuoteServerConfig{
			ListenAddress:   parse.OrDefault(raw.QuoteServer.ListenAddress, defaultListenAddress),
			HTTPTimeout:     quoteHTTPTimeout,
			RefreshInterval: refreshInterval, QuoteTTL: quoteTTL,
		},
		OrderServer: OrderServerConfig{
			BaseURL: raw.OrderServer.BaseURL, APIKeyEnv: raw.OrderServer.APIKeyEnv,
			PollInterval: pollInterval, HTTPTimeout: orderHTTPTimeout, Beta: raw.OrderServer.Beta, Sources: sources,
		},
		Discounts: discountConfig,
		Gas:       gas,
		Breaker:   breaker,
		Strategy:  StrategyConfig{Name: parse.OrDefault(raw.Strategy.Name, defaultStrategyName), Config: raw.Strategy.Config},
	}, nil
}

func (c *Config) usesDiscounts() bool { return c.SolverMode == solverModeInternal }

func (c *Config) restrictsToAdapters() bool {
	return c.SolverMode == solverModeExternal && len(c.Adapters) > 0
}

func (c *Config) quoteScopesToAdapters() bool {
	return len(c.Adapters) > 0
}

func parseDiscountConfig(raw *rawDiscountConfig) (*DiscountConfig, error) {
	if raw == nil {
		return nil, nil
	}
	if raw.BaseURL == "" {
		return nil, errors.New("discounts.baseUrl is required")
	}
	if err := validateServiceURL(raw.BaseURL, "discounts.baseUrl"); err != nil {
		return nil, err
	}
	timeout, err := parse.Duration(raw.HTTPTimeout, defaultDiscountTimeout, "discounts.httpTimeout")
	if err != nil {
		return nil, err
	}
	validity, err := parse.Duration(raw.MinimumValidity, defaultDiscountValidity, "discounts.minimumValidity")
	if err != nil {
		return nil, err
	}
	return &DiscountConfig{BaseURL: raw.BaseURL, HTTPTimeout: timeout, MinimumValidity: validity}, nil
}

func parseBreakerConfig(raw rawBreakerConfig) (BreakerConfig, error) {
	maxFailures := raw.MaxFailures
	if maxFailures == 0 {
		maxFailures = 3
	}
	if maxFailures < 1 {
		return BreakerConfig{}, errors.New("breaker.maxFailures must be positive")
	}
	window, err := parse.Duration(raw.Window, 5*time.Minute, "breaker.window")
	if err != nil {
		return BreakerConfig{}, err
	}
	return BreakerConfig{MaxFailures: maxFailures, Window: window}, nil
}

func validateServiceURL(raw, field string) error {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return errors.Errorf("%s must be an absolute URL, got %q", field, raw)
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" {
		host := u.Hostname()
		ip := net.ParseIP(host)
		if host == "localhost" || ip != nil && ip.IsLoopback() {
			return nil
		}
	}
	return errors.Errorf("%s must use https, except loopback http for local development", field)
}

func parseAddressList(values []string, field string) ([]common.Address, error) {
	out := make([]common.Address, 0, len(values))
	seen := make(map[common.Address]bool, len(values))
	for i, value := range values {
		address, err := parse.NonZeroAddress(value, field+"["+strconv.Itoa(i)+"]")
		if err != nil {
			return nil, err
		}
		if seen[address] {
			return nil, errors.Errorf("%s[%d]: duplicate address %s", field, i, address.Hex())
		}
		seen[address] = true
		out = append(out, address)
	}
	return out, nil
}
