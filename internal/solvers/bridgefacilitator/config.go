package bridgefacilitator

import (
	"time"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solver"
)

// rawConfig mirrors the YAML shape; strings are parsed into typed values in parse(). 3F registers
// exactly one offer-address per facilitator, so the bot serves a single vault+adapter pair.
type rawConfig struct {
	APIBaseURL      string       `yaml:"apiBaseUrl"`
	APIKeyEnv       string       `yaml:"apiKeyEnv"`
	RedeemBatchSize int          `yaml:"redeemBatchSize"`
	Adapter         string       `yaml:"adapter"`
	HTTPTimeout     string       `yaml:"httpTimeout"`
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
	// HTTPTimeout bounds every 3F API call so a hung request can't stall the single solver loop
	// (including redemption scans). Applied as the 3F http.Client timeout.
	HTTPTimeout time.Duration
	// Target is the single vault+adapter pair this facilitator serves. 3F allows exactly one
	// offer-address per facilitator, so this solver is single-pair by construction.
	Target    Target
	Intervals Intervals
}

// Target is the adapter the bot facilitates. Only the adapter is config: Vault (adapter.vault()) and
// Collateral (vault.asset()) are resolved on-chain at startup (see Solver.resolveTarget) and fixed for
// the adapter's lifetime. Exposure/return caps also live on-chain (setExposureLimits), read each poll.
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
	defaultDiscover   = time.Hour
	defaultRedeemPoll = 5 * time.Minute
	defaultReconcile  = 15 * time.Minute
)

// defaultRedeemBatchSize caps the Requests redeemed per redeem() call when unset.
const defaultRedeemBatchSize = 10

// defaultHTTPTimeout bounds each 3F API call when httpTimeout is unset.
const defaultHTTPTimeout = 30 * time.Second

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

	target, err := parseTarget(raw)
	if err != nil {
		return nil, err
	}

	discover, err := parseDuration(raw.Intervals.Discover, defaultDiscover, "intervals.discover")
	if err != nil {
		return nil, err
	}
	redeemPoll, err := parseDuration(raw.Intervals.RedeemPoll, defaultRedeemPoll, "intervals.redeemPoll")
	if err != nil {
		return nil, err
	}
	reconcile, err := parseDuration(raw.Intervals.Reconcile, defaultReconcile, "intervals.reconcile")
	if err != nil {
		return nil, err
	}

	httpTimeout, err := parseDuration(raw.HTTPTimeout, defaultHTTPTimeout, "httpTimeout")
	if err != nil {
		return nil, err
	}

	return &Config{
		APIBaseURL:      raw.APIBaseURL,
		APIKeyEnv:       raw.APIKeyEnv,
		RedeemBatchSize: redeemBatch,
		HTTPTimeout:     httpTimeout,
		Target:          target,
		Intervals:       Intervals{Discover: discover, RedeemPoll: redeemPoll, Reconcile: reconcile},
	}, nil
}

func parseTarget(raw rawConfig) (Target, error) {
	// The zero address is rejected so an unreplaced placeholder fails at startup rather than being
	// registered as the 3F offer-address.
	adapter, err := parseNonZeroAddress(raw.Adapter, "adapter")
	if err != nil {
		return Target{}, err
	}
	return Target{Adapter: adapter}, nil
}

func parseAddress(s, field string) (common.Address, error) {
	if !common.IsHexAddress(s) {
		return common.Address{}, errors.Errorf("%s: invalid address %q", field, s)
	}
	return common.HexToAddress(s), nil
}

func parseNonZeroAddress(s, field string) (common.Address, error) {
	addr, err := parseAddress(s, field)
	if err != nil {
		return common.Address{}, err
	}
	if addr == (common.Address{}) {
		return common.Address{}, errors.Errorf("%s: zero address (placeholder not replaced?)", field)
	}
	return addr, nil
}

// parseDuration returns fallback when s is empty, but a present-but-invalid or non-positive value is
// an error rather than a silent fall back to the default — a typo'd interval should fail, not run at
// some surprising cadence.
func parseDuration(s string, fallback time.Duration, field string) (time.Duration, error) {
	if s == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, errors.Errorf("%s: invalid duration %q: %w", field, s, err)
	}
	if d <= 0 {
		return 0, errors.Errorf("%s: duration must be positive, got %q", field, s)
	}
	return d, nil
}
