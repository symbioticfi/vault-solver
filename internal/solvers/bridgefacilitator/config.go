package bridgefacilitator

import (
	"math/big"
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
	MinReturnBps    float64      `yaml:"minReturnBps"`
	RedeemBatchSize int          `yaml:"redeemBatchSize"`
	Vault           string       `yaml:"vault"`
	Adapter         string       `yaml:"adapter"`
	Exposure        rawExposure  `yaml:"exposure"`
	Intervals       rawIntervals `yaml:"intervals"`
}

type rawExposure struct {
	PerRequestMaxUsdc  string `yaml:"perRequestMaxUsdc"`
	TotalSleeveMaxUsdc string `yaml:"totalSleeveMaxUsdc"`
	MaxConcurrentLoans int    `yaml:"maxConcurrentLoans"`
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
	// MinReturnBps is the minimum acceptable yield rate (basis points) for the bot to bid.
	MinReturnBps float64
	// RedeemBatchSize caps how many Requests are redeemed in a single redeem() call (gas bound).
	RedeemBatchSize int
	// Target is the single vault+adapter pair this facilitator serves. 3F allows exactly one
	// offer-address per facilitator, so this solver is single-pair by construction.
	Target    Target
	Intervals Intervals
}

// Target is the vault+adapter the bot facilitates, with its exposure caps.
type Target struct {
	Vault   common.Address
	Adapter common.Address
	// Collateral is the vault's collateral token, resolved on-chain at startup. Auctions are matched
	// to this target by their deposit asset equalling Collateral.
	Collateral common.Address
	Exposure   Exposure
}

// Exposure bounds risk per Request, per sleeve, and by concurrent open loans.
type Exposure struct {
	PerRequestMax      *big.Int
	TotalSleeveMax     *big.Int
	MaxConcurrentLoans int
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
		MinReturnBps:    raw.MinReturnBps,
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
	perReq, err := parseBigInt(raw.Exposure.PerRequestMaxUsdc, "exposure.perRequestMaxUsdc")
	if err != nil {
		return Target{}, err
	}
	sleeve, err := parseBigInt(raw.Exposure.TotalSleeveMaxUsdc, "exposure.totalSleeveMaxUsdc")
	if err != nil {
		return Target{}, err
	}
	if raw.Exposure.MaxConcurrentLoans <= 0 {
		return Target{}, errors.New("exposure.maxConcurrentLoans must be > 0")
	}
	return Target{
		Vault:   vault,
		Adapter: adapter,
		Exposure: Exposure{
			PerRequestMax:      perReq,
			TotalSleeveMax:     sleeve,
			MaxConcurrentLoans: raw.Exposure.MaxConcurrentLoans,
		},
	}, nil
}

func parseAddress(s, field string) (common.Address, error) {
	if !common.IsHexAddress(s) {
		return common.Address{}, errors.Errorf("%s: invalid address %q", field, s)
	}
	return common.HexToAddress(s), nil
}

func parseBigInt(s, field string) (*big.Int, error) {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Sign() < 0 {
		return nil, errors.Errorf("%s: invalid non-negative integer %q", field, s)
	}
	return n, nil
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
