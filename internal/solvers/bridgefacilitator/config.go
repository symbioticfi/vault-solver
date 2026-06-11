package bridgefacilitator

import (
	"time"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"
)

// rawConfig mirrors the YAML shape; strings are parsed into typed values in parse(). 3F registers
// exactly one offer-address per facilitator, so the bot serves a single vault+adapter pair.
type rawConfig struct {
	APIBaseURL      string       `yaml:"apiBaseUrl"`
	APIKeyEnv       string       `yaml:"apiKeyEnv"`
	RedeemBatchSize int          `yaml:"redeemBatchSize"`
	Vault           string       `yaml:"vault"`
	Adapter         string       `yaml:"adapter"`
	Intervals       rawIntervals `yaml:"intervals"`
}

type rawIntervals struct {
	Discover   string `yaml:"discover"`
	RedeemPoll string `yaml:"redeemPoll"`
	Reconcile  string `yaml:"reconcile"`
}

// Config is the validated, typed solver configuration.
type Config struct {
	APIBaseURL string
	// APIKeyEnv is the env var holding a pre-generated 3F API key (sent as the x-api-key header).
	APIKeyEnv string
	// RedeemBatchSize caps how many Requests are redeemed in a single redeem() call (gas bound).
	RedeemBatchSize int
	// Target is the single vault+adapter pair this facilitator serves. 3F allows exactly one
	// offer-address per facilitator, so this solver is single-pair by construction.
	Target    Target
	Intervals Intervals
}

// Target is the vault+adapter the bot facilitates. The exposure/return caps are no longer config: they
// live on the adapter (setExposureLimits) and the bot reads them on-chain each poll.
type Target struct {
	Vault   common.Address
	Adapter common.Address
	// Collateral is the vault's collateral token, resolved on-chain at startup. Auctions are matched
	// to this target by their deposit asset equalling Collateral.
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
	defaultDiscover   = time.Hour
	defaultRedeemPoll = 5 * time.Minute
	defaultReconcile  = 15 * time.Minute
)

// defaultRedeemBatchSize caps the Requests redeemed per redeem() call when unset.
const defaultRedeemBatchSize = 10

// parseConfig decodes and validates the opaque solver config block.
func parseConfig(node yaml.Node) (*Config, error) {
	var raw rawConfig
	if err := node.Decode(&raw); err != nil {
		return nil, errors.Errorf("decode solver config: %w", err)
	}
	if raw.APIBaseURL == "" {
		return nil, errors.New("apiBaseUrl is required")
	}

	redeemBatch := raw.RedeemBatchSize
	if redeemBatch <= 0 {
		redeemBatch = defaultRedeemBatchSize
	}

	target, err := parseTarget(raw)
	if err != nil {
		return nil, err
	}

	return &Config{
		APIBaseURL:      raw.APIBaseURL,
		APIKeyEnv:       raw.APIKeyEnv,
		RedeemBatchSize: redeemBatch,
		Target:          target,
		Intervals: Intervals{
			Discover:   parseDurationOr(raw.Intervals.Discover, defaultDiscover),
			RedeemPoll: parseDurationOr(raw.Intervals.RedeemPoll, defaultRedeemPoll),
			Reconcile:  parseDurationOr(raw.Intervals.Reconcile, defaultReconcile),
		},
	}, nil
}

func parseTarget(raw rawConfig) (Target, error) {
	vault, err := parseAddress(raw.Vault, "vault")
	if err != nil {
		return Target{}, err
	}
	adapter, err := parseAddress(raw.Adapter, "adapter")
	if err != nil {
		return Target{}, err
	}
	return Target{Vault: vault, Adapter: adapter}, nil
}

func parseAddress(s, field string) (common.Address, error) {
	if !common.IsHexAddress(s) {
		return common.Address{}, errors.Errorf("%s: invalid address %q", field, s)
	}
	return common.HexToAddress(s), nil
}

func parseDurationOr(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}
