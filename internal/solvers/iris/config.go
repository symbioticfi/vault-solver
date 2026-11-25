package iris

import (
	"math/big"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/merklehost"
	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solver"
)

const (
	defaultListenAddress   = ":42090"
	defaultRefreshInterval = time.Minute
	defaultBlockTime       = 12 * time.Second
	defaultSubmitLead      = 2 * time.Second
	defaultQuoteTTL        = 2 * time.Minute
	defaultDeadlineWindow  = 5 * time.Second
	defaultResponseBudget  = 25 * time.Second
	defaultPollInterval    = 500 * time.Millisecond
	// rateStep is the Iris fixed-rate granularity (BP in Iris.sol).
	rateStep = 100_000_000_000_000
)

var wad = big.NewInt(1e18)

type rawConfig struct {
	Iris            string             `yaml:"iris"`
	Account         string             `yaml:"account"`
	Morpho          string             `yaml:"morpho"`
	RecipeExecutors []string           `yaml:"recipeExecutors"`
	Curator         merklehost.Curator `yaml:"curator"`
	RefreshInterval string             `yaml:"refreshInterval"`
	Server          rawServer          `yaml:"server"`
	Pricing         rawPricing         `yaml:"pricing"`
	Timing          rawTiming          `yaml:"timing"`
}

type rawServer struct {
	ListenAddress string `yaml:"listenAddress"`
	AuthHeader    string `yaml:"authHeader"`
	AuthValueEnv  string `yaml:"authValueEnv"`
}

type rawPricing struct {
	DefaultFixedRate string    `yaml:"defaultFixedRate"`
	Rates            []rawRate `yaml:"rates"`
	MaxBond          string    `yaml:"maxBond"`
}

type rawTiming struct {
	BlockTime      string `yaml:"blockTime"`
	SubmitLead     string `yaml:"submitLead"`
	ResponseSlack  string `yaml:"responseSlack"`
	QuoteTTL       string `yaml:"quoteTtl"`
	DeadlineWindow string `yaml:"deadlineWindow"`
	ResponseBudget string `yaml:"responseBudget"`
	PollInterval   string `yaml:"pollInterval"`
}

type rawRate struct {
	CollateralToken string  `yaml:"collateralToken"`
	DebtToken       string  `yaml:"debtToken"`
	VenueID         *uint64 `yaml:"venueId"`
	FixedRate       string  `yaml:"fixedRate"`
}

// Config is the validated iris solver configuration.
type Config struct {
	Iris    common.Address
	Account common.Address
	// Morpho recovers Morpho market payloads; markets with another payload need none.
	Morpho          common.Address
	RecipeExecutors []common.Address
	Curator         merklehost.Curator
	RefreshInterval time.Duration
	Server          ServerConfig
	Pricing         Pricing
	Timing          Timing
}

// ServerConfig is the coordinator-facing HTTP endpoint. Requests must carry AuthHeader with
// the value of the AuthValueEnv environment variable, read at startup and never logged.
type ServerConfig struct {
	ListenAddress string
	AuthHeader    string
	AuthValueEnv  string
}

// Timing places the on-chain signature and the quote deadline (docs/IRIS-PLAN.md §6).
type Timing struct {
	BlockTime, SubmitLead, ResponseSlack, QuoteTTL, DeadlineWindow, ResponseBudget, PollInterval time.Duration
}

func parseConfig(node yaml.Node) (Config, error) {
	var raw rawConfig
	if err := solver.DecodeStrict(node, &raw); err != nil {
		return Config{}, err
	}
	var cfg Config
	var err error
	if cfg.Iris, err = parse.NonZeroAddress(raw.Iris, "iris"); err != nil {
		return Config{}, err
	}
	if cfg.Account, err = parse.NonZeroAddress(raw.Account, "account"); err != nil {
		return Config{}, err
	}
	if raw.Morpho != "" {
		if cfg.Morpho, err = parse.NonZeroAddress(raw.Morpho, "morpho"); err != nil {
			return Config{}, err
		}
	}
	seen := make(map[common.Address]bool, len(raw.RecipeExecutors))
	for _, value := range raw.RecipeExecutors {
		executor, err := parse.NonZeroAddress(value, "recipeExecutors")
		if err != nil {
			return Config{}, err
		}
		if seen[executor] {
			return Config{}, errors.Errorf("recipeExecutors: duplicate %s", executor.Hex())
		}
		seen[executor] = true
		cfg.RecipeExecutors = append(cfg.RecipeExecutors, executor)
	}
	if len(cfg.RecipeExecutors) == 0 {
		return Config{}, errors.New("recipeExecutors: at least one executor required")
	}
	if err := permissions.ValidateURL(raw.Curator.URL); err != nil {
		return Config{}, errors.Errorf("curator: %w", err)
	}
	cfg.Curator = raw.Curator
	if cfg.RefreshInterval, err = parse.Duration(raw.RefreshInterval, defaultRefreshInterval, "refreshInterval"); err != nil {
		return Config{}, err
	}
	cfg.Server = ServerConfig{
		ListenAddress: parse.OrDefault(raw.Server.ListenAddress, defaultListenAddress),
		AuthHeader:    raw.Server.AuthHeader, AuthValueEnv: raw.Server.AuthValueEnv,
	}
	// Every firm request spends gas on-chain, so the endpoint never runs unauthenticated.
	if cfg.Server.AuthHeader == "" || cfg.Server.AuthValueEnv == "" {
		return Config{}, errors.New("server: authHeader and authValueEnv are required")
	}
	if cfg.Pricing, err = parsePricing(raw); err != nil {
		return Config{}, err
	}
	if cfg.Timing, err = parseTiming(raw); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func parsePricing(raw rawConfig) (Pricing, error) {
	var pricing Pricing
	var err error
	if raw.Pricing.DefaultFixedRate != "" {
		if pricing.Default, err = parseRate(raw.Pricing.DefaultFixedRate, "pricing.defaultFixedRate"); err != nil {
			return Pricing{}, err
		}
	}
	for i, r := range raw.Pricing.Rates {
		rule := RateRule{VenueID: r.VenueID}
		field := "pricing.rates[" + strconv.Itoa(i) + "]"
		if rule.FixedRate, err = parseRate(r.FixedRate, field+".fixedRate"); err != nil {
			return Pricing{}, err
		}
		for value, target := range map[string]**common.Address{r.CollateralToken: &rule.CollateralToken, r.DebtToken: &rule.DebtToken} {
			if value == "" {
				continue
			}
			address, err := parse.NonZeroAddress(value, field)
			if err != nil {
				return Pricing{}, err
			}
			*target = &address
		}
		if rule.VenueID != nil && *rule.VenueID > maxVenueID {
			return Pricing{}, errors.Errorf("%s.venueId: must be at most %d", field, maxVenueID)
		}
		pricing.Rules = append(pricing.Rules, rule)
	}
	if pricing.Default == nil && len(pricing.Rules) == 0 {
		return Pricing{}, errors.New("pricing: defaultFixedRate or rates required")
	}
	if raw.Pricing.MaxBond != "" {
		if pricing.MaxBond, err = parse.Big(raw.Pricing.MaxBond, "pricing.maxBond"); err != nil {
			return Pricing{}, err
		}
		if pricing.MaxBond.Sign() <= 0 {
			return Pricing{}, errors.New("pricing.maxBond: must be positive")
		}
	}
	return pricing, nil
}

// parseRate accepts a WAD rate in (0, 1e18] that Iris can represent.
func parseRate(value, field string) (*big.Int, error) {
	rate, err := parse.Big(value, field)
	if err != nil {
		return nil, err
	}
	if rate.Sign() <= 0 || rate.Cmp(wad) > 0 || new(big.Int).Mod(rate, big.NewInt(rateStep)).Sign() != 0 {
		return nil, errors.Errorf("%s: must be a WAD rate in (0, 1e18] and a multiple of 1e14", field)
	}
	return rate, nil
}

func parseTiming(raw rawConfig) (Timing, error) {
	t := raw.Timing
	var timing Timing
	for _, field := range []struct {
		value    string
		fallback time.Duration
		name     string
		target   *time.Duration
	}{
		{t.BlockTime, defaultBlockTime, "timing.blockTime", &timing.BlockTime},
		{t.SubmitLead, defaultSubmitLead, "timing.submitLead", &timing.SubmitLead},
		{t.QuoteTTL, defaultQuoteTTL, "timing.quoteTtl", &timing.QuoteTTL},
		{t.DeadlineWindow, defaultDeadlineWindow, "timing.deadlineWindow", &timing.DeadlineWindow},
		{t.ResponseBudget, defaultResponseBudget, "timing.responseBudget", &timing.ResponseBudget},
		{t.PollInterval, defaultPollInterval, "timing.pollInterval", &timing.PollInterval},
	} {
		d, err := parse.Duration(field.value, field.fallback, field.name)
		if err != nil {
			return Timing{}, err
		}
		*field.target = d
	}
	// Zero slack is the default and must be expressible, so it does not use parse.Duration.
	if t.ResponseSlack != "" {
		d, err := time.ParseDuration(t.ResponseSlack)
		if err != nil || d < 0 {
			return Timing{}, errors.Errorf("timing.responseSlack: invalid non-negative duration %q", t.ResponseSlack)
		}
		timing.ResponseSlack = d
	}
	if timing.BlockTime%time.Second != 0 {
		return Timing{}, errors.New("timing.blockTime: must be whole seconds")
	}
	if timing.DeadlineWindow >= timing.QuoteTTL {
		return Timing{}, errors.New("timing.deadlineWindow: must be shorter than quoteTtl")
	}
	return timing, nil
}
