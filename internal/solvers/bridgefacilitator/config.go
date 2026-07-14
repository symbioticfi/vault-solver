package bridgefacilitator

import (
	"strconv"
	"time"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"

	cfgparse "github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solver"
)

// rawConfig mirrors the YAML shape; strings are parsed into typed values in parse().
type rawConfig struct {
	APIBaseURL      string            `yaml:"apiBaseUrl"`
	RedeemBatchSize int               `yaml:"redeemBatchSize"`
	Adapters        []string          `yaml:"adapters"`
	HTTPTimeout     string            `yaml:"httpTimeout"`
	Intervals       rawIntervals      `yaml:"intervals"`
	Strategy        rawStrategyConfig `yaml:"strategy"`
}

type rawStrategyConfig struct {
	Name   string    `yaml:"name"`
	Config yaml.Node `yaml:"config"`
}

type rawIntervals struct {
	Discover   string `yaml:"discover"`
	RedeemPoll string `yaml:"redeemPoll"`
	Reconcile  string `yaml:"reconcile"`
}

// Config is the validated, typed solver configuration.
type Config struct {
	APIBaseURL string
	// RedeemBatchSize caps how many Requests are redeemed in a single redeem() call (gas bound).
	RedeemBatchSize int
	// HTTPTimeout bounds every 3F API call so a hung request can't stall the single solver loop
	// (including redemption scans). Applied as the 3F http.Client timeout.
	HTTPTimeout time.Duration
	// Targets is the list of vault+adapter pairs this facilitator serves.
	Targets   []Target
	Intervals Intervals
	Strategy  StrategyConfig
}

type StrategyConfig struct {
	Name   string
	Config yaml.Node
}

// Target is one adapter the bot facilitates. Only the adapter is config: Vault (adapter.vault()) and
// Collateral (vault.asset()) are resolved on-chain at startup (resolveTargets); per-request caps also
// live on-chain (setLimitsPerRequest), read each poll.
type Target struct {
	Adapter common.Address
	// Auctions are matched to this target by their deposit asset equalling Collateral.
	Vault      common.Address
	Collateral common.Address
}

// Intervals controls the solver's loop cadences.
type Intervals struct {
	Discover   time.Duration
	RedeemPoll time.Duration
	Reconcile  time.Duration
}

// Default loop cadences (used when a field is unset).
const (
	defaultDiscover   = 5 * time.Minute
	defaultRedeemPoll = 5 * time.Minute
	defaultReconcile  = 15 * time.Minute
)

// defaultRedeemBatchSize caps the Requests redeemed per redeem() call when unset.
const defaultRedeemBatchSize = 10

// defaultHTTPTimeout bounds each 3F API call when httpTimeout is unset.
const defaultHTTPTimeout = 30 * time.Second

const defaultStrategyName = "default"

// parseConfig decodes and validates the opaque solver config block.
func parseConfig(node yaml.Node) (*Config, error) {
	var raw rawConfig
	if err := solver.DecodeStrict(node, &raw); err != nil {
		return nil, err
	}
	if raw.APIBaseURL == "" {
		return nil, errors.New("apiBaseUrl is required")
	}

	redeemBatch := raw.RedeemBatchSize
	if redeemBatch <= 0 {
		redeemBatch = defaultRedeemBatchSize
	}

	targets, err := parseTargets(raw)
	if err != nil {
		return nil, err
	}

	discover, err := cfgparse.Duration(raw.Intervals.Discover, defaultDiscover, "intervals.discover")
	if err != nil {
		return nil, err
	}
	redeemPoll, err := cfgparse.Duration(raw.Intervals.RedeemPoll, defaultRedeemPoll, "intervals.redeemPoll")
	if err != nil {
		return nil, err
	}
	reconcile, err := cfgparse.Duration(raw.Intervals.Reconcile, defaultReconcile, "intervals.reconcile")
	if err != nil {
		return nil, err
	}

	httpTimeout, err := cfgparse.Duration(raw.HTTPTimeout, defaultHTTPTimeout, "httpTimeout")
	if err != nil {
		return nil, err
	}

	strategy := StrategyConfig{Name: raw.Strategy.Name, Config: raw.Strategy.Config}
	if strategy.Name == "" {
		strategy.Name = defaultStrategyName
	}

	return &Config{
		APIBaseURL:      raw.APIBaseURL,
		RedeemBatchSize: redeemBatch,
		HTTPTimeout:     httpTimeout,
		Targets:         targets,
		Intervals:       Intervals{Discover: discover, RedeemPoll: redeemPoll, Reconcile: reconcile},
		Strategy:        strategy,
	}, nil
}

func parseTargets(raw rawConfig) ([]Target, error) {
	if len(raw.Adapters) == 0 {
		return nil, errors.New("at least one adapters entry is required")
	}
	targets := make([]Target, 0, len(raw.Adapters))
	for i, a := range raw.Adapters {
		adapter, err := cfgparse.NonZeroAddress(a, "adapters["+strconv.Itoa(i)+"]")
		if err != nil {
			return nil, err
		}
		targets = append(targets, Target{Adapter: adapter})
	}
	return targets, nil
}
