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
	"slices"
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
	// ReconcileNonces follows canonical account state after contention without shared storage.
	ReconcileNonces bool `yaml:"reconcileNonces"`
	// Confirmations to wait for before treating a transaction as final.
	Confirmations uint64 `yaml:"confirmations"`
	// MaxFeeGwei is the required absolute EIP-1559 max fee per gas.
	MaxFeeGwei float64 `yaml:"maxFeeGwei"`
	// BroadcastTimeoutMs bounds one transaction submission RPC call independently of replacement cadence.
	BroadcastTimeoutMs int `yaml:"broadcastTimeoutMs"`
	// AccountPollIntervalMs controls signer balance and nonce telemetry refresh cadence.
	AccountPollIntervalMs int `yaml:"accountPollIntervalMs"`
	// ReplacementIntervalMs paces the fallback fee bump of a pending transaction while fee history is
	// unreadable; with readable fee history, pending transactions are repriced on block evidence.
	ReplacementIntervalMs int `yaml:"replacementIntervalMs"`
	// PendingTimeoutMs switches a still-pending call to a same-nonce cancellation.
	PendingTimeoutMs int `yaml:"pendingTimeoutMs"`
	// ShutdownTimeoutMs bounds how long shutdown drains an accepted transaction lifecycle.
	ShutdownTimeoutMs int `yaml:"shutdownTimeoutMs"`
	// Horizon tunes fee pricing and block-evidence repricing.
	Horizon HorizonFeeConfig `yaml:"horizon"`
}

// HorizonFeeConfig tunes txManager fee pricing: the base-fee horizon of the fee cap, the tip rule,
// the repricing evidence, and gas-estimate headroom. Zero values select the defaults.
type HorizonFeeConfig struct {
	// MaxBlocks is how many blocks the initial fee cap keeps the full tip valid at the maximum
	// EIP-1559 base-fee increase.
	MaxBlocks int `yaml:"maxBlocks"`
	// BlockTimeMs is the chain slot time; it sets the evaluation cadence and the next-block estimate time.
	BlockTimeMs int `yaml:"blockTimeMs"`
	// TipFloorGwei is the tip while the last two blocks had room for the transaction.
	TipFloorGwei float64 `yaml:"tipFloorGwei"`
	// FullBlockTipGwei is the tip when one of the last two blocks had no room for it.
	FullBlockTipGwei float64 `yaml:"fullBlockTipGwei"`
	// CongestedTipFloorGwei and CongestedTipCapGwei clamp the reward-percentile tip used when both
	// of the last two blocks had no room.
	CongestedTipFloorGwei float64 `yaml:"congestedTipFloorGwei"`
	CongestedTipCapGwei   float64 `yaml:"congestedTipCapGwei"`
	// CongestedRewardBlocks and CongestedRewardPercentile pick that tip: the highest percentile
	// reward over this many recent blocks.
	CongestedRewardBlocks     int     `yaml:"congestedRewardBlocks"`
	CongestedRewardPercentile float64 `yaml:"congestedRewardPercentile"`
	// EscalateAfterFullBlocks is how many consecutive full blocks a valid pending transaction must
	// lose before its tip is repriced.
	EscalateAfterFullBlocks int `yaml:"escalateAfterFullBlocks"`
	// StallAfterBlocks is how many blocks with room a valid pending transaction may lose before it is
	// re-estimated and rebroadcast.
	StallAfterBlocks int `yaml:"stallAfterBlocks"`
	// GasHeadroomBps is the headroom over the next-block gas estimate; FallbackGasHeadroomBps applies
	// when the read RPC cannot estimate against the next block.
	GasHeadroomBps         int `yaml:"gasHeadroomBps"`
	FallbackGasHeadroomBps int `yaml:"fallbackGasHeadroomBps"`
}

// SolverConfig names the solver implementation and carries its opaque, deferred config.
type SolverConfig struct {
	Name string `yaml:"name"`
	// Config is decoded by the selected solver into its own typed struct (two-stage decode).
	Config yaml.Node `yaml:"config"`
}

// DefaultConfirmations is used when TxManager.Confirmations is unset.
const DefaultConfirmations = 2

// Fee pricing defaults, applied to unset txManager.horizon fields.
const (
	DefaultHorizonMaxBlocks                 = 6
	DefaultHorizonBlockTimeMs               = 12_000
	DefaultHorizonTipFloorGwei              = 0.02
	DefaultHorizonFullBlockTipGwei          = 0.1
	DefaultHorizonCongestedTipFloorGwei     = 0.2
	DefaultHorizonCongestedTipCapGwei       = 15
	DefaultHorizonCongestedRewardBlocks     = 3
	DefaultHorizonCongestedRewardPercentile = 50
	DefaultHorizonEscalateAfterFullBlocks   = 2
	DefaultHorizonStallAfterBlocks          = 3
	DefaultHorizonGasHeadroomBps            = 500
	DefaultHorizonFallbackGasHeadroomBps    = 1000

	maxHorizonBlocks       = 12
	maxHorizonWindowBlocks = 64
	maxGasHeadroomBps      = 5000
)

const (
	// Keep in sync with chain.defaultRPCAttemptTimeout (internal/chain/fallback.go).
	DefaultRPCAttemptTimeoutMs   = 20_000
	DefaultBroadcastTimeoutMs    = 5_000
	DefaultAccountPollIntervalMs = 30_000
	DefaultReplacementIntervalMs = 30_000
	DefaultPendingTimeoutMs      = 300_000
	DefaultShutdownTimeoutMs     = 60_000
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

	var cfg Config
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
	c.TxManager.Horizon.applyDefaults()
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
	return c.Horizon.validate()
}

func (h *HorizonFeeConfig) applyDefaults() {
	if h.MaxBlocks == 0 {
		h.MaxBlocks = DefaultHorizonMaxBlocks
	}
	if h.BlockTimeMs == 0 {
		h.BlockTimeMs = DefaultHorizonBlockTimeMs
	}
	if h.TipFloorGwei == 0 {
		h.TipFloorGwei = DefaultHorizonTipFloorGwei
	}
	if h.FullBlockTipGwei == 0 {
		h.FullBlockTipGwei = DefaultHorizonFullBlockTipGwei
	}
	if h.CongestedTipFloorGwei == 0 {
		h.CongestedTipFloorGwei = DefaultHorizonCongestedTipFloorGwei
	}
	if h.CongestedTipCapGwei == 0 {
		h.CongestedTipCapGwei = DefaultHorizonCongestedTipCapGwei
	}
	if h.CongestedRewardBlocks == 0 {
		h.CongestedRewardBlocks = DefaultHorizonCongestedRewardBlocks
	}
	if h.CongestedRewardPercentile == 0 {
		h.CongestedRewardPercentile = DefaultHorizonCongestedRewardPercentile
	}
	if h.EscalateAfterFullBlocks == 0 {
		h.EscalateAfterFullBlocks = DefaultHorizonEscalateAfterFullBlocks
	}
	if h.StallAfterBlocks == 0 {
		h.StallAfterBlocks = DefaultHorizonStallAfterBlocks
	}
	if h.GasHeadroomBps == 0 {
		h.GasHeadroomBps = DefaultHorizonGasHeadroomBps
	}
	if h.FallbackGasHeadroomBps == 0 {
		h.FallbackGasHeadroomBps = DefaultHorizonFallbackGasHeadroomBps
	}
}

func (h HorizonFeeConfig) validate() error {
	if h.MaxBlocks < 3 || h.MaxBlocks > maxHorizonBlocks {
		return errors.Errorf("txManager.horizon.maxBlocks must be between 3 and %d", maxHorizonBlocks)
	}
	if h.BlockTimeMs <= 0 || int64(h.BlockTimeMs) > math.MaxInt64/int64(time.Millisecond) {
		return errors.New("txManager.horizon.blockTimeMs must be positive and fit in a time.Duration")
	}
	tips := []float64{h.TipFloorGwei, h.FullBlockTipGwei, h.CongestedTipFloorGwei, h.CongestedTipCapGwei}
	for _, tip := range tips {
		if tip <= 0 || math.IsNaN(tip) || math.IsInf(tip, 0) {
			return errors.New("txManager.horizon tips must be finite and positive")
		}
	}
	if !slices.IsSorted(tips) {
		return errors.New("txManager.horizon tips must satisfy " +
			"tipFloorGwei <= fullBlockTipGwei <= congestedTipFloorGwei <= congestedTipCapGwei")
	}
	if h.CongestedRewardPercentile <= 0 || h.CongestedRewardPercentile > 100 {
		return errors.New("txManager.horizon.congestedRewardPercentile must be in (0, 100]")
	}
	for _, window := range []struct {
		name   string
		blocks int
	}{
		{"congestedRewardBlocks", h.CongestedRewardBlocks},
		{"escalateAfterFullBlocks", h.EscalateAfterFullBlocks},
		{"stallAfterBlocks", h.StallAfterBlocks},
	} {
		if window.blocks < 1 || window.blocks > maxHorizonWindowBlocks {
			return errors.Errorf("txManager.horizon.%s must be between 1 and %d", window.name, maxHorizonWindowBlocks)
		}
	}
	if h.GasHeadroomBps < 0 || h.FallbackGasHeadroomBps < h.GasHeadroomBps || h.FallbackGasHeadroomBps > maxGasHeadroomBps {
		return errors.Errorf(
			"txManager.horizon gas headroom must satisfy 0 <= gasHeadroomBps <= fallbackGasHeadroomBps <= %d",
			maxGasHeadroomBps,
		)
	}
	return nil
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
