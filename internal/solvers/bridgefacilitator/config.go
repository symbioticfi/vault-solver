package bridgefacilitator

import (
	"strconv"
	"time"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"

	cfgparse "github.com/symbioticfi/vault-solver/internal/parse"
)

// rawConfig mirrors the YAML shape; strings are parsed into typed values in parse().
type rawConfig struct {
	APIBaseURL        string         `yaml:"apiBaseUrl"`
	RedeemBatchSize   int            `yaml:"redeemBatchSize"`
	Adapters          *[]string      `yaml:"adapters"`
	AdapterFactory    string         `yaml:"adapterFactory"`
	LiquidityLens     string         `yaml:"liquidityLens"`
	HTTPTimeout       string         `yaml:"httpTimeout"`
	OfferExpiryBuffer string         `yaml:"offerExpiryBuffer"`
	Intervals         rawIntervals   `yaml:"intervals"`
	Strategy          StrategyConfig `yaml:"strategy"`
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
	// OfferExpiryBuffer is added to an auction's solve_start_time to set a signed offer's expiration, so
	// the offer stays valid through the whole solve window regardless of when it is signed.
	OfferExpiryBuffer time.Duration
	// Targets is the configured static adapter set. Nil means adapters was omitted and the factory
	// should be discovered; a non-nil slice is authoritative.
	Targets        []Target
	AdapterFactory common.Address
	// LiquidityLens is the optional FrontendLiquidityLens address. When set, adapter funding headroom is
	// read from the lens's cross-adapter deallocation-cascade estimate instead of each adapter's own
	// getMaxAssets(); zero-value falls back to the adapter getter.
	LiquidityLens common.Address
	Intervals     Intervals
	Strategy      StrategyConfig
}

type StrategyConfig struct {
	Name   string    `yaml:"name"`
	Config yaml.Node `yaml:"config"`
}

// Target is one adapter the bot facilitates. Only static adapter addresses are config: Vault
// (adapter.vault()) and Collateral (vault.asset()) are resolved on-chain on every adapter refresh;
// per-request caps also live on-chain (setLimitsPerRequest), read each poll.
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

// defaultOfferExpiryBuffer is the solve_start_time margin applied to a signed offer's expiration when
// offerExpiryBuffer is unset — long enough to cover a full auction solve window plus slack.
const defaultOfferExpiryBuffer = 2 * time.Hour

const defaultStrategyName = "default"

// parseConfig decodes and validates the opaque solver config block.
func parseConfig(node yaml.Node) (*Config, error) {
	var raw rawConfig
	if err := cfgparse.DecodeStrict(node, &raw); err != nil {
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
	adapterFactory, err := cfgparse.OptionalNonZeroAddress(raw.AdapterFactory, "adapterFactory")
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 && adapterFactory == (common.Address{}) {
		return nil, errors.New("at least one adapters entry or adapterFactory is required")
	}
	liquidityLens, err := cfgparse.OptionalNonZeroAddress(raw.LiquidityLens, "liquidityLens")
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

	offerExpiryBuffer, err := cfgparse.Duration(raw.OfferExpiryBuffer, defaultOfferExpiryBuffer, "offerExpiryBuffer")
	if err != nil {
		return nil, err
	}
	if raw.Strategy.Name == "" {
		raw.Strategy.Name = defaultStrategyName
	}

	return &Config{
		APIBaseURL:        raw.APIBaseURL,
		RedeemBatchSize:   redeemBatch,
		HTTPTimeout:       httpTimeout,
		OfferExpiryBuffer: offerExpiryBuffer,
		Targets:           targets,
		AdapterFactory:    adapterFactory,
		LiquidityLens:     liquidityLens,
		Intervals:         Intervals{Discover: discover, RedeemPoll: redeemPoll, Reconcile: reconcile},
		Strategy:          raw.Strategy,
	}, nil
}

func parseTargets(raw rawConfig) ([]Target, error) {
	if raw.Adapters == nil {
		return nil, nil
	}
	targets := make([]Target, 0, len(*raw.Adapters))
	for i, a := range *raw.Adapters {
		adapter, err := cfgparse.NonZeroAddress(a, "adapters["+strconv.Itoa(i)+"]")
		if err != nil {
			return nil, err
		}
		targets = append(targets, Target{Adapter: adapter})
	}
	return targets, nil
}
