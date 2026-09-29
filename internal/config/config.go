// Package config loads and validates the vault-solver YAML configuration.
//
// Decoding is two-stage: the generic layer parses everything except the solver-specific block,
// which it keeps as a raw yaml.Node under Solver.Config. The selected solver decodes that node
// into its own typed struct, so solver config stays fully encapsulated in the solver package.
package config

import (
	"bytes"
	"math"
	"os"
	"time"

	"github.com/go-errors/errors"

	"gopkg.in/yaml.v3"
)

// Config is the top-level bot configuration.
type Config struct {
	Chain     ChainConfig     `yaml:"chain"`
	Signer    SignerConfig    `yaml:"signer"`
	TxManager TxManagerConfig `yaml:"txManager"`
	// Solvers is the set of solvers to run in one process — at most one entry per solver type. They
	// share the chain client and signer; transaction-sending solvers also share the single
	// nonce-serialized txManager, so they never race on nonces.
	Solvers       []SolverConfig      `yaml:"solvers"`
	Observability ObservabilityConfig `yaml:"observability"`
}

// ObservabilityConfig configures logging and the metrics/health HTTP server.
type ObservabilityConfig struct {
	// Addr is the listen address for /metrics, /healthz, /readyz.
	Addr string `yaml:"addr"`
	// Debug enables debug-level (logr V(1)) logging. The --debug CLI flag overrides this.
	Debug bool `yaml:"debug"`
}

// ChainConfig describes the EVM endpoint the bot reads from and sends to.
type ChainConfig struct {
	RPCURL string `yaml:"rpcUrl"`
	// RPCAttemptTimeoutMs bounds each HTTP(S) endpoint attempt, including reading the response body.
	// Shorter caller deadlines still apply. Zero uses the default of 20 seconds.
	RPCAttemptTimeoutMs int `yaml:"rpcAttemptTimeoutMs"`
	// RPCFallbackURLs are additional HTTP(S) RPC endpoints tried, in order, when the primary `rpcUrl`
	// is unavailable. All must be on the same chain. Optional; empty means no fallback.
	RPCFallbackURLs []string `yaml:"rpcFallbackUrls,omitempty"`
	// WriteRPCURL, when set, broadcasts signed transactions and supplies both startup nonce reads.
	// Other reads stay on `rpcUrl`. Point this at the private/MEV-protected endpoint that accepts the
	// fills so startup observes its private nonce lane. Optional; empty means `rpcUrl` serves both.
	WriteRPCURL string `yaml:"writeRpcUrl,omitempty"`
	// CancelRPCURL overrides only same-nonce self-cancellation broadcasts. Empty uses the ordinary
	// transaction broadcast route; nonce, fee and receipt reads keep their existing endpoints.
	CancelRPCURL string `yaml:"cancelRpcUrl,omitempty"`
	ChainID      uint64 `yaml:"chainId"`
	// WSURL is optional; when set it enables live log subscriptions (a latency optimization only).
	WSURL string `yaml:"wsUrl,omitempty"`
	// MulticallAddress overrides the Multicall3 contract used to batch reads. Defaults to the
	// canonical cross-chain Multicall3 deployment when unset. With gas accounting enabled,
	// it must also support getCurrentBlockTimestamp(); startup checks this selector.
	MulticallAddress string `yaml:"multicallAddress,omitempty"`
}

// SignerConfig selects how the signing key is sourced. Exactly one of the two modes must be set:
// an environment variable holding a hex private key, or a keystore file plus a passphrase env var.
type SignerConfig struct {
	KeyEnv        string `yaml:"keyEnv,omitempty"`
	KeystorePath  string `yaml:"keystorePath,omitempty"`
	PassphraseEnv string `yaml:"passphraseEnv,omitempty"`
}

// TxManagerConfig tunes the shared transaction sender.
type TxManagerConfig struct {
	// Confirmations to wait for before treating a transaction as final.
	Confirmations uint64 `yaml:"confirmations"`
	// MaxFeeGwei is the required absolute EIP-1559 max fee per gas.
	MaxFeeGwei float64 `yaml:"maxFeeGwei"`
	// TipGwei is the minimum EIP-1559 priority fee; 0 derives it from recent fee history.
	TipGwei float64 `yaml:"tipGwei"`
	// BroadcastTimeoutMs bounds one transaction submission RPC call independently of replacement cadence.
	BroadcastTimeoutMs int `yaml:"broadcastTimeoutMs"`
	// AccountPollIntervalMs controls signer balance and nonce telemetry refresh cadence.
	AccountPollIntervalMs int `yaml:"accountPollIntervalMs"`
	// ReplacementIntervalMs is how often a pending transaction is fee-bumped under the legacy fee policy.
	// Under the horizon policy pending attempts follow blocks, and it only paces the fallback bump while
	// fee reads fail and sizes the shutdown budget.
	ReplacementIntervalMs int `yaml:"replacementIntervalMs"`
	// PendingTimeoutMs switches a still-pending call to a same-nonce cancellation.
	PendingTimeoutMs int `yaml:"pendingTimeoutMs"`
	// ShutdownTimeoutMs bounds how long shutdown drains an accepted transaction lifecycle.
	ShutdownTimeoutMs int `yaml:"shutdownTimeoutMs"`

	// The nested blocks below start from their documented defaults before decoding, so an omitted
	// key keeps its default while an explicit zero or false is honoured and validated. The flat
	// fields above keep their historical "zero means unset" behaviour.

	// Fees selects the fee policy and tunes its validity horizon and priority-fee ladder.
	Fees TxFeesConfig `yaml:"fees"`
	// Gas tunes the gas-limit headroom and the next-block gas estimate.
	Gas TxGasConfig `yaml:"gas"`
	// Balance configures the per-attempt balance guard and the lane funding gate.
	Balance TxBalanceConfig `yaml:"balance"`
	// Shadow configures the metrics-only evaluator that scores both fee policies on every head.
	Shadow TxShadowConfig `yaml:"shadow"`
}

// TxFeesConfig tunes fee selection. Gwei amounts are per unit of gas.
type TxFeesConfig struct {
	// Policy is "legacy" (2×base + tip with timer-driven bumps) or "horizon" (an exact EIP-1559
	// validity horizon with block-driven repricing).
	Policy string `yaml:"policy"`
	// BlockTimeMs is the slot time used for head lag, the next-block estimate and evaluation cadence.
	BlockTimeMs int `yaml:"blockTimeMs"`
	// MinHorizonBlocks is the fewest blocks, from the real next block, a signed fee must stay valid
	// for; a send the balance cannot keep valid that long is refused.
	MinHorizonBlocks int `yaml:"minHorizonBlocks"`
	// MaxHorizonBlocks is the validity horizon a horizon-policy send targets when the balance allows.
	MaxHorizonBlocks int `yaml:"maxHorizonBlocks"`
	// PricingHorizonBlocks is the horizon quotes are priced and the funding gate is measured at.
	PricingHorizonBlocks int `yaml:"pricingHorizonBlocks"`
	// MaxHeadLagBlocks is how many blocks a fee snapshot may trail before sends wait for a newer head.
	MaxHeadLagBlocks int `yaml:"maxHeadLagBlocks"`
	// TipFloorGwei is the priority fee in blocks with room, and the tip the refusal floor assumes.
	TipFloorGwei float64 `yaml:"tipFloorGwei"`
	// SingleFullBlockTipGwei is the tip when one of the last two blocks had no room for the gas limit.
	SingleFullBlockTipGwei float64 `yaml:"singleFullBlockTipGwei"`
	// CongestedTipFloorGwei and CongestedTipCapGwei clamp the observed reward in a demand run (both
	// of the last two blocks without room).
	CongestedTipFloorGwei float64 `yaml:"congestedTipFloorGwei"`
	CongestedTipCapGwei   float64 `yaml:"congestedTipCapGwei"`
	// CongestedRewardBlocks and CongestedRewardPercentile select the reward a demand run follows:
	// the maximum of that percentile over that many latest blocks.
	CongestedRewardBlocks     int     `yaml:"congestedRewardBlocks"`
	CongestedRewardPercentile float64 `yaml:"congestedRewardPercentile"`
	// EscalateAfterFullMisses is how many consecutive missed blocks without room trigger a reprice.
	EscalateAfterFullMisses int `yaml:"escalateAfterFullMisses"`
	// StallAfterRoomyMisses is how many missed blocks with room trigger a re-estimate and rebroadcast.
	StallAfterRoomyMisses int `yaml:"stallAfterRoomyMisses"`
}

// TxGasConfig tunes gas-limit selection. Basis points are relative to the gas estimate.
type TxGasConfig struct {
	// HeadroomBps is added to the gas estimate to form the gas limit.
	HeadroomBps int `yaml:"headroomBps"`
	// NextBlockEstimate estimates gas in the next block's context (blockOverrides) under the horizon
	// policy; an upstream that rejects or ignores the overrides falls back to a plain estimate.
	NextBlockEstimate bool `yaml:"nextBlockEstimate"`
	// FallbackHeadroomBps replaces HeadroomBps when the plain fallback estimate is used.
	FallbackHeadroomBps int `yaml:"fallbackHeadroomBps"`
	// EstimateTimeoutMs bounds one gas estimate, separately from the fee-read budget.
	EstimateTimeoutMs int `yaml:"estimateTimeoutMs"`
}

// TxBalanceConfig configures the signer balance checks.
type TxBalanceConfig struct {
	// Guard caps every attempt's max fee at what the signer balance can fund and refuses a send it
	// cannot keep valid for minHorizonBlocks. It applies under both fee policies.
	Guard bool `yaml:"guard"`
	// ReferenceGasUnits is the gas limit the funding gate and the shadow evaluator assume for a fill.
	// Zero turns the funding gate off.
	ReferenceGasUnits int64 `yaml:"referenceGasUnits"`
	// FundingHysteresisBps is the extra balance, over the gate threshold, needed to become fundable
	// again after the lane went unfundable.
	FundingHysteresisBps int `yaml:"fundingHysteresisBps"`
	// TargetEth is the operator's funding target, exported for alerts only. Zero leaves it unset.
	TargetEth float64 `yaml:"targetEth"`
}

// TxShadowConfig configures the metrics-only fee policy evaluator.
type TxShadowConfig struct {
	// Enabled scores virtual fills under both fee policies on every head without sending anything.
	Enabled bool `yaml:"enabled"`
}

// SolverConfig names the solver implementation and carries its opaque, deferred config.
type SolverConfig struct {
	Name string `yaml:"name"`
	// Config is decoded by the selected solver into its own typed struct (two-stage decode).
	Config yaml.Node `yaml:"config"`
}

// DefaultConfirmations is used when TxManager.Confirmations is unset.
const DefaultConfirmations = 2

const (
	// Keep in sync with chain.defaultRPCAttemptTimeout (internal/chain/fallback.go).
	DefaultRPCAttemptTimeoutMs   = 20_000
	DefaultBroadcastTimeoutMs    = 5_000
	DefaultAccountPollIntervalMs = 30_000
	DefaultReplacementIntervalMs = 30_000
	DefaultPendingTimeoutMs      = 300_000
	DefaultShutdownTimeoutMs     = 60_000
)

// Fee policies accepted by txManager.fees.policy.
const (
	FeePolicyLegacy  = "legacy"
	FeePolicyHorizon = "horizon"
)

// Defaults for the nested txManager blocks. Keep in sync with the zero-value defaults of
// txmanager.Config (internal/txmanager); cmd/vault-solver tests pin the two sets together.
const (
	DefaultFeePolicy                 = FeePolicyLegacy
	DefaultBlockTimeMs               = 12_000
	DefaultMinHorizonBlocks          = 2
	DefaultMaxHorizonBlocks          = 6
	DefaultPricingHorizonBlocks      = 5
	DefaultMaxHeadLagBlocks          = 2
	DefaultTipFloorGwei              = 0.02
	DefaultSingleFullBlockTipGwei    = 0.1
	DefaultCongestedTipFloorGwei     = 0.2
	DefaultCongestedTipCapGwei       = 15
	DefaultCongestedRewardBlocks     = 3
	DefaultCongestedRewardPercentile = 50
	DefaultEscalateAfterFullMisses   = 2
	DefaultStallAfterRoomyMisses     = 3
	DefaultGasHeadroomBps            = 500
	DefaultFallbackGasHeadroomBps    = 1000
	DefaultGasEstimateTimeoutMs      = 5000
	DefaultFundingHysteresisBps      = 2000
)

// Bounds on the nested txManager knobs.
const (
	// MaxHorizonBlocksLimit caps maxHorizonBlocks: a 12-block horizon already prices ×3.65 growth
	// of the next base fee.
	MaxHorizonBlocksLimit = 12
	// MaxGasHeadroomBps caps gas.headroomBps at 50% over the estimate.
	MaxGasHeadroomBps = 5000
	// maxBasisPoints caps the other basis-point knobs at 100%.
	maxBasisPoints = 10_000
	// maxFeeHistoryBlocks is the largest block count eth_feeHistory serves (go-ethereum's limit).
	maxFeeHistoryBlocks = 1024
	// minHorizonPricingGap is how many blocks of base-fee growth the pricing horizon must tolerate
	// between a quote and its fill, over the minimum horizon.
	minHorizonPricingGap = 2
)

// DefaultObservabilityAddr is used when Observability.Addr is unset.
const DefaultObservabilityAddr = ":9090"

// DefaultMulticallAddress is the canonical Multicall3 deployment (same address on most chains,
// including Ethereum mainnet and Sepolia). Used when Chain.MulticallAddress is unset.
const DefaultMulticallAddress = "0xcA11bde05977b3631167028862bE2a173976CA11"

// Load reads, parses, defaults, and validates the config at path.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Errorf("read config %q: %w", path, err)
	}

	// Expand ${VAR}/$VAR from the environment so non-secret, deploy-injected fields (e.g. rpcUrl)
	// can come from the environment. Secrets must NOT use this: they belong in the *Env name fields
	// (keyEnv, passphraseEnv, backendSharedSecretEnv, …), which os.Getenv at point of use and never place the secret
	// into this Config struct (so dumping/logging the config can't leak it). An undefined var
	// expands to "", which surfaces via Validate for required fields.
	raw = []byte(os.ExpandEnv(string(raw)))

	cfg := defaultConfig()
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true) // reject unknown keys to catch typos early
	if err := dec.Decode(&cfg); err != nil {
		return nil, errors.Errorf("parse config %q: %w", path, err)
	}

	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, errors.Errorf("invalid config %q: %w", path, err)
	}
	return &cfg, nil
}

// defaultConfig is the decode target: the nested txManager blocks start from their defaults, so
// yaml.v3 overwrites only the keys a config sets (an empty or null block keeps every default).
func defaultConfig() Config {
	return Config{TxManager: TxManagerConfig{
		Fees: TxFeesConfig{
			Policy:                    DefaultFeePolicy,
			BlockTimeMs:               DefaultBlockTimeMs,
			MinHorizonBlocks:          DefaultMinHorizonBlocks,
			MaxHorizonBlocks:          DefaultMaxHorizonBlocks,
			PricingHorizonBlocks:      DefaultPricingHorizonBlocks,
			MaxHeadLagBlocks:          DefaultMaxHeadLagBlocks,
			TipFloorGwei:              DefaultTipFloorGwei,
			SingleFullBlockTipGwei:    DefaultSingleFullBlockTipGwei,
			CongestedTipFloorGwei:     DefaultCongestedTipFloorGwei,
			CongestedTipCapGwei:       DefaultCongestedTipCapGwei,
			CongestedRewardBlocks:     DefaultCongestedRewardBlocks,
			CongestedRewardPercentile: DefaultCongestedRewardPercentile,
			EscalateAfterFullMisses:   DefaultEscalateAfterFullMisses,
			StallAfterRoomyMisses:     DefaultStallAfterRoomyMisses,
		},
		Gas: TxGasConfig{
			HeadroomBps:         DefaultGasHeadroomBps,
			NextBlockEstimate:   true,
			FallbackHeadroomBps: DefaultFallbackGasHeadroomBps,
			EstimateTimeoutMs:   DefaultGasEstimateTimeoutMs,
		},
		Balance: TxBalanceConfig{
			Guard:                true,
			FundingHysteresisBps: DefaultFundingHysteresisBps,
		},
		Shadow: TxShadowConfig{Enabled: true},
	}}
}

func (c *Config) applyDefaults() {
	if c.Chain.RPCAttemptTimeoutMs == 0 {
		c.Chain.RPCAttemptTimeoutMs = DefaultRPCAttemptTimeoutMs
	}
	if c.TxManager.Confirmations == 0 {
		c.TxManager.Confirmations = DefaultConfirmations
	}
	if c.TxManager.BroadcastTimeoutMs == 0 {
		c.TxManager.BroadcastTimeoutMs = DefaultBroadcastTimeoutMs
	}
	if c.TxManager.AccountPollIntervalMs == 0 {
		c.TxManager.AccountPollIntervalMs = DefaultAccountPollIntervalMs
	}
	if c.TxManager.ReplacementIntervalMs == 0 {
		c.TxManager.ReplacementIntervalMs = DefaultReplacementIntervalMs
	}
	if c.TxManager.PendingTimeoutMs == 0 {
		c.TxManager.PendingTimeoutMs = DefaultPendingTimeoutMs
	}
	if c.TxManager.ShutdownTimeoutMs == 0 {
		c.TxManager.ShutdownTimeoutMs = DefaultShutdownTimeoutMs
	}
	if c.Observability.Addr == "" {
		c.Observability.Addr = DefaultObservabilityAddr
	}
	if c.Chain.MulticallAddress == "" {
		c.Chain.MulticallAddress = DefaultMulticallAddress
	}
}

// Validate checks required fields and mutually-exclusive options.
func (c *Config) Validate() error {
	if c.Chain.RPCAttemptTimeoutMs <= 0 || int64(c.Chain.RPCAttemptTimeoutMs) > math.MaxInt64/int64(time.Millisecond) {
		return errors.New("chain.rpcAttemptTimeoutMs must be positive and fit in a time.Duration")
	}
	if c.Chain.RPCURL == "" {
		return errors.New("chain.rpcUrl is required")
	}
	for i, u := range c.Chain.RPCFallbackURLs {
		if u == "" {
			return errors.Errorf("chain.rpcFallbackUrls[%d] is empty", i)
		}
	}
	if c.Chain.ChainID == 0 {
		return errors.New("chain.chainId is required")
	}
	if err := c.TxManager.validate(false); err != nil {
		return err
	}
	if err := c.Signer.validate(); err != nil {
		return err
	}
	if len(c.Solvers) == 0 {
		return errors.New("at least one solver is required (set `solvers`)")
	}
	seen := make(map[string]bool, len(c.Solvers))
	for i, s := range c.Solvers {
		if s.Name == "" {
			return errors.Errorf("solvers[%d].name is required", i)
		}
		if seen[s.Name] {
			return errors.Errorf("duplicate solver %q: only one entry per solver type is allowed", s.Name)
		}
		seen[s.Name] = true
	}
	return nil
}

// ValidateTxManager checks fields required only when at least one solver sends transactions.
func (c *Config) ValidateTxManager() error {
	return c.TxManager.validate(true)
}

func (c TxManagerConfig) validate(required bool) error {
	if c.MaxFeeGwei < 0 || required && c.MaxFeeGwei == 0 ||
		math.IsNaN(c.MaxFeeGwei) || math.IsInf(c.MaxFeeGwei, 0) {
		return errors.New("txManager.maxFeeGwei must be finite and positive")
	}
	if c.TipGwei < 0 || math.IsNaN(c.TipGwei) || math.IsInf(c.TipGwei, 0) {
		return errors.New("txManager.tipGwei must be finite and non-negative")
	}
	if c.BroadcastTimeoutMs <= 0 {
		return errors.New("txManager.broadcastTimeoutMs must be positive")
	}
	if c.AccountPollIntervalMs <= 0 {
		return errors.New("txManager.accountPollIntervalMs must be positive")
	}
	if c.ReplacementIntervalMs <= 0 {
		return errors.New("txManager.replacementIntervalMs must be positive")
	}
	if c.PendingTimeoutMs < c.ReplacementIntervalMs {
		return errors.New("txManager.pendingTimeoutMs must be at least replacementIntervalMs")
	}
	if c.ShutdownTimeoutMs <= 0 {
		return errors.New("txManager.shutdownTimeoutMs must be positive")
	}
	if err := c.Fees.validate(); err != nil {
		return err
	}
	if c.Fees.Policy == FeePolicyHorizon && c.TipGwei != 0 {
		return errors.New("txManager.tipGwei must be 0 under fees.policy horizon, whose tip comes from the fees.*TipGwei ladder")
	}
	if err := c.Gas.validate(); err != nil {
		return err
	}
	return c.Balance.validate()
}

// validate enforces the fee knobs' ranges and orderings. The ceiling of the tip ladder depends on
// maxFeeGwei and is checked by txmanager.Manager.ValidateFeeHeadroom.
func (f TxFeesConfig) validate() error {
	if f.Policy != FeePolicyLegacy && f.Policy != FeePolicyHorizon {
		return errors.Errorf("txManager.fees.policy must be %q or %q, got %q", FeePolicyLegacy, FeePolicyHorizon, f.Policy)
	}
	if !validDurationMs(f.BlockTimeMs) {
		return errors.New("txManager.fees.blockTimeMs must be positive and fit in a time.Duration")
	}
	// Bounding the minimum first keeps minHorizonBlocks + minHorizonPricingGap from overflowing.
	if f.MinHorizonBlocks < 1 || f.MinHorizonBlocks > MaxHorizonBlocksLimit {
		return errors.Errorf("txManager.fees.minHorizonBlocks must be between 1 and %d", MaxHorizonBlocksLimit)
	}
	if f.PricingHorizonBlocks < f.MinHorizonBlocks+minHorizonPricingGap {
		return errors.Errorf("txManager.fees.pricingHorizonBlocks must be at least minHorizonBlocks + %d", minHorizonPricingGap)
	}
	if f.MaxHorizonBlocks < f.PricingHorizonBlocks || f.MaxHorizonBlocks > MaxHorizonBlocksLimit {
		return errors.Errorf("txManager.fees.maxHorizonBlocks must be between pricingHorizonBlocks and %d", MaxHorizonBlocksLimit)
	}
	if f.MaxHeadLagBlocks < 0 {
		return errors.New("txManager.fees.maxHeadLagBlocks must not be negative")
	}
	if !finite(f.TipFloorGwei) || f.TipFloorGwei <= 0 {
		return errors.New("txManager.fees.tipFloorGwei must be finite and positive")
	}
	ladder := []struct {
		name  string
		value float64
	}{
		{"tipFloorGwei", f.TipFloorGwei},
		{"singleFullBlockTipGwei", f.SingleFullBlockTipGwei},
		{"congestedTipFloorGwei", f.CongestedTipFloorGwei},
		{"congestedTipCapGwei", f.CongestedTipCapGwei},
	}
	for i := 1; i < len(ladder); i++ {
		if !finite(ladder[i].value) || ladder[i].value < ladder[i-1].value {
			return errors.Errorf("txManager.fees.%s must be finite and at least %s", ladder[i].name, ladder[i-1].name)
		}
	}
	if f.CongestedRewardBlocks < 1 || f.CongestedRewardBlocks > maxFeeHistoryBlocks {
		return errors.Errorf("txManager.fees.congestedRewardBlocks must be between 1 and %d", maxFeeHistoryBlocks)
	}
	if !finite(f.CongestedRewardPercentile) || f.CongestedRewardPercentile <= 0 || f.CongestedRewardPercentile > 100 {
		return errors.New("txManager.fees.congestedRewardPercentile must be above 0 and at most 100")
	}
	if f.EscalateAfterFullMisses < 1 {
		return errors.New("txManager.fees.escalateAfterFullMisses must be at least 1")
	}
	if f.StallAfterRoomyMisses < 1 {
		return errors.New("txManager.fees.stallAfterRoomyMisses must be at least 1")
	}
	return nil
}

func (g TxGasConfig) validate() error {
	if g.HeadroomBps < 0 || g.HeadroomBps > MaxGasHeadroomBps {
		return errors.Errorf("txManager.gas.headroomBps must be between 0 and %d", MaxGasHeadroomBps)
	}
	if g.FallbackHeadroomBps < g.HeadroomBps || g.FallbackHeadroomBps > maxBasisPoints {
		return errors.Errorf("txManager.gas.fallbackHeadroomBps must be between headroomBps and %d", maxBasisPoints)
	}
	if !validDurationMs(g.EstimateTimeoutMs) {
		return errors.New("txManager.gas.estimateTimeoutMs must be positive and fit in a time.Duration")
	}
	return nil
}

func (b TxBalanceConfig) validate() error {
	if b.ReferenceGasUnits < 0 {
		return errors.New("txManager.balance.referenceGasUnits must not be negative")
	}
	if b.FundingHysteresisBps < 0 || b.FundingHysteresisBps > maxBasisPoints {
		return errors.Errorf("txManager.balance.fundingHysteresisBps must be between 0 and %d", maxBasisPoints)
	}
	if !finite(b.TargetEth) || b.TargetEth < 0 {
		return errors.New("txManager.balance.targetEth must be finite and non-negative")
	}
	return nil
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// validDurationMs reports whether ms is a positive millisecond count that fits in a time.Duration.
func validDurationMs(ms int) bool {
	return ms > 0 && int64(ms) <= math.MaxInt64/int64(time.Millisecond)
}

func (s SignerConfig) validate() error {
	hasEnv := s.KeyEnv != ""
	hasKeystore := s.KeystorePath != ""
	switch {
	case hasEnv && hasKeystore:
		return errors.New("signer: set exactly one of keyEnv or keystorePath, not both")
	case hasEnv:
		return nil
	case hasKeystore:
		if s.PassphraseEnv == "" {
			return errors.New("signer: keystorePath requires passphraseEnv")
		}
		return nil
	default:
		return errors.New("signer: one of keyEnv or keystorePath is required")
	}
}
