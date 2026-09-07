package bridgefacilitator

import (
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

type StrategyConfig = cfgparse.NamedConfig

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

	cfg := &Config{
		APIBaseURL:      raw.APIBaseURL,
		RedeemBatchSize: cfgparse.OrDefault(raw.RedeemBatchSize, defaultRedeemBatchSize),
		Strategy:        StrategyConfig{Name: cfgparse.OrDefault(raw.Strategy.Name, defaultStrategyName), Config: raw.Strategy.Config},
	}
	if cfg.RedeemBatchSize < 1 {
		return nil, errors.New("redeemBatchSize must be positive")
	}
	var err error
	if cfg.Targets, err = parseTargets(raw); err != nil {
		return nil, err
	}
	if cfg.AdapterFactory, err = cfgparse.OptionalAddress(raw.AdapterFactory, "adapterFactory"); err != nil {
		return nil, err
	}
	if cfg.LiquidityLens, err = cfgparse.OptionalAddress(raw.LiquidityLens, "liquidityLens"); err != nil {
		return nil, err
	}
	if len(cfg.Targets) == 0 && cfg.AdapterFactory == (common.Address{}) {
		return nil, errors.New("at least one adapters entry or adapterFactory is required")
	}
	for _, duration := range []struct {
		field, raw string
		fallback   time.Duration
		out        *time.Duration
	}{
		{"intervals.discover", raw.Intervals.Discover, defaultDiscover, &cfg.Intervals.Discover},
		{"intervals.redeemPoll", raw.Intervals.RedeemPoll, defaultRedeemPoll, &cfg.Intervals.RedeemPoll},
		{"intervals.reconcile", raw.Intervals.Reconcile, defaultReconcile, &cfg.Intervals.Reconcile},
		{"httpTimeout", raw.HTTPTimeout, defaultHTTPTimeout, &cfg.HTTPTimeout},
		{"offerExpiryBuffer", raw.OfferExpiryBuffer, defaultOfferExpiryBuffer, &cfg.OfferExpiryBuffer},
	} {
		if *duration.out, err = cfgparse.Duration(duration.raw, duration.fallback, duration.field); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func parseTargets(raw rawConfig) ([]Target, error) {
	if raw.Adapters == nil {
		return nil, nil
	}
	adapters, err := cfgparse.Addresses(*raw.Adapters, "adapters")
	if err != nil {
		return nil, err
	}
	targets := make([]Target, 0, len(adapters))
	for _, adapter := range adapters {
		targets = append(targets, Target{Adapter: adapter})
	}
	return targets, nil
}
