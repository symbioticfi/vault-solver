package txmanager

import "time"

// FeePolicy selects how the manager prices and reprices attempts.
type FeePolicy string

const (
	// FeePolicyLegacy prices 2×base + tip and bumps pending attempts on a timer.
	FeePolicyLegacy FeePolicy = "legacy"
	// FeePolicyHorizon prices an exact EIP-1559 validity horizon and reprices from block evidence.
	FeePolicyHorizon FeePolicy = "horizon"
)

// FeeConfig tunes fee selection. A zero field takes its default, except MaxHeadLagBlocks, where
// zero is a valid setting and nil takes the default. Gwei amounts are per unit of gas.
type FeeConfig struct {
	Policy                    FeePolicy     // "" => legacy
	BlockTime                 time.Duration // slot time for head lag, next-block estimates and cadence; 0 => 12s
	MinHorizonBlocks          uint64        // fewest guaranteed valid blocks before a send is refused; 0 => 2
	MaxHorizonBlocks          uint64        // horizon a horizon-policy send targets; 0 => 6
	PricingHorizonBlocks      uint64        // horizon for quote pricing and the funding gate; 0 => 5
	MaxHeadLagBlocks          *uint64       // snapshot lag tolerated before waiting for a new head; nil => 2
	TipFloorGwei              float64       // tip in blocks with room and the refusal-floor tip; 0 => 0.02
	SingleFullBlockTipGwei    float64       // tip after one of the last two blocks had no room; 0 => 0.1
	CongestedTipFloorGwei     float64       // lower clamp of the demand-run tip; 0 => 0.2
	CongestedTipCapGwei       float64       // upper clamp of the demand-run tip; 0 => 15
	CongestedRewardBlocks     uint64        // latest blocks whose reward percentile a demand run follows; 0 => 3
	CongestedRewardPercentile float64       // reward percentile a demand run follows; 0 => 50
	EscalateAfterFullMisses   uint64        // consecutive missed blocks without room before a reprice; 0 => 2
	StallAfterRoomyMisses     uint64        // missed blocks with room before a stall response; 0 => 3
}

// GasConfig tunes gas-limit selection. A nil basis-point field takes its default; zero basis points
// is a valid setting.
type GasConfig struct {
	HeadroomBps               *uint64       // headroom over the estimate; nil => 500 (5%)
	NextBlockEstimateDisabled bool          // false => estimate in next-block context under the horizon policy
	FallbackHeadroomBps       *uint64       // headroom over a plain fallback estimate; nil => 1000
	EstimateTimeout           time.Duration // bound on one gas estimate; 0 => 5s
}

// BalanceConfig configures the signer balance checks. Its zero value keeps the balance guard on.
type BalanceConfig struct {
	GuardDisabled        bool    // false => cap every attempt at what the balance can fund
	ReferenceGasUnits    uint64  // fill gas limit assumed by the funding gate; 0 => gate off
	FundingHysteresisBps *uint64 // extra balance needed to become fundable again; nil => 2000
	TargetEth            float64 // operator funding target exported for alerts; 0 => unset
}

// ShadowConfig configures the metrics-only fee policy evaluator. Its zero value keeps it on.
type ShadowConfig struct {
	Disabled bool
}

const (
	defaultBlockTime                 = 12 * time.Second
	defaultMinHorizonBlocks          = 2
	defaultMaxHorizonBlocks          = 6
	defaultPricingHorizonBlocks      = 5
	defaultMaxHeadLagBlocks          = 2
	defaultTipFloorGwei              = 0.02
	defaultSingleFullBlockTipGwei    = 0.1
	defaultCongestedTipFloorGwei     = 0.2
	defaultCongestedTipCapGwei       = 15
	defaultCongestedRewardBlocks     = 3
	defaultCongestedRewardPercentile = 50
	defaultEscalateAfterFullMisses   = 2
	defaultStallAfterRoomyMisses     = 3
	// defaultGasHeadroomBps is the 5% the manager has always added to its estimate.
	defaultGasHeadroomBps         = 500
	defaultFallbackGasHeadroomBps = 1000
	defaultGasEstimateTimeout     = 5 * time.Second
	defaultFundingHysteresisBps   = 2000
)

// WithDefaults returns a copy of c with every unset field replaced by its default; New applies it.
// The zero Config is therefore the safe default: the legacy fee policy, the balance guard on and
// 500 bps of gas headroom. Pointer fields are always set, to fresh copies, in the result.
func (c Config) WithDefaults() Config {
	if c.PollInterval <= 0 {
		c.PollInterval = defaultPollInterval
	}
	if c.BroadcastTimeout <= 0 {
		c.BroadcastTimeout = defaultBroadcastTimeout
	}
	if c.AccountPollInterval <= 0 {
		c.AccountPollInterval = defaultAccountPollInterval
	}
	if c.ReplacementInterval <= 0 {
		c.ReplacementInterval = defaultReplacementInterval
	}
	if c.PendingTimeout <= 0 {
		c.PendingTimeout = defaultPendingTimeout
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = defaultShutdownTimeout
	}
	c.Fees = c.Fees.withDefaults()
	c.Gas = c.Gas.withDefaults()
	c.Balance = c.Balance.withDefaults()
	return c
}

func (f FeeConfig) withDefaults() FeeConfig {
	if f.Policy == "" {
		f.Policy = FeePolicyLegacy
	}
	f.BlockTime = orDefault(f.BlockTime, defaultBlockTime)
	f.MinHorizonBlocks = orDefault(f.MinHorizonBlocks, defaultMinHorizonBlocks)
	f.MaxHorizonBlocks = orDefault(f.MaxHorizonBlocks, defaultMaxHorizonBlocks)
	f.PricingHorizonBlocks = orDefault(f.PricingHorizonBlocks, defaultPricingHorizonBlocks)
	f.MaxHeadLagBlocks = copyOrDefault(f.MaxHeadLagBlocks, defaultMaxHeadLagBlocks)
	f.TipFloorGwei = orDefault(f.TipFloorGwei, defaultTipFloorGwei)
	f.SingleFullBlockTipGwei = orDefault(f.SingleFullBlockTipGwei, defaultSingleFullBlockTipGwei)
	f.CongestedTipFloorGwei = orDefault(f.CongestedTipFloorGwei, defaultCongestedTipFloorGwei)
	f.CongestedTipCapGwei = orDefault(f.CongestedTipCapGwei, defaultCongestedTipCapGwei)
	f.CongestedRewardBlocks = orDefault(f.CongestedRewardBlocks, defaultCongestedRewardBlocks)
	f.CongestedRewardPercentile = orDefault(f.CongestedRewardPercentile, defaultCongestedRewardPercentile)
	f.EscalateAfterFullMisses = orDefault(f.EscalateAfterFullMisses, defaultEscalateAfterFullMisses)
	f.StallAfterRoomyMisses = orDefault(f.StallAfterRoomyMisses, defaultStallAfterRoomyMisses)
	return f
}

func (g GasConfig) withDefaults() GasConfig {
	g.HeadroomBps = copyOrDefault(g.HeadroomBps, defaultGasHeadroomBps)
	g.FallbackHeadroomBps = copyOrDefault(g.FallbackHeadroomBps, defaultFallbackGasHeadroomBps)
	g.EstimateTimeout = orDefault(g.EstimateTimeout, defaultGasEstimateTimeout)
	return g
}

func (b BalanceConfig) withDefaults() BalanceConfig {
	b.FundingHysteresisBps = copyOrDefault(b.FundingHysteresisBps, defaultFundingHysteresisBps)
	return b
}

// orDefault replaces a non-positive value with fallback.
func orDefault[T time.Duration | uint64 | float64](value, fallback T) T {
	if value <= 0 {
		return fallback
	}
	return value
}

// copyOrDefault returns a fresh pointer to *value, or to fallback when value is nil, so a normalized
// Config never aliases its caller's memory.
func copyOrDefault(value *uint64, fallback uint64) *uint64 {
	if value != nil {
		return new(*value)
	}
	return new(fallback)
}
