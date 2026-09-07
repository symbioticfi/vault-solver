// Package config owns the process YAML contract. Solver configuration remains an opaque node.
package config

import (
	"bytes"
	"math"
	"os"
	"time"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/parse"
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
	// RPCFallbackURLs are additional HTTP(S) RPC endpoints tried, in order, when the primary `rpcUrl`
	// is unavailable. All must be on the same chain. Optional; empty means no fallback.
	RPCFallbackURLs []string `yaml:"rpcFallbackUrls,omitempty"`
	// WriteRPCURL, when set, broadcasts signed transactions and supplies both startup nonce reads.
	// Other reads stay on `rpcUrl`. Point this at the private/MEV-protected endpoint that accepts the
	// fills so startup observes its private nonce lane. Optional; empty means `rpcUrl` serves both.
	WriteRPCURL string `yaml:"writeRpcUrl,omitempty"`
	ChainID     uint64 `yaml:"chainId"`
	// WSURL is optional; when set it enables live log subscriptions (a latency optimization only).
	WSURL string `yaml:"wsUrl,omitempty"`
	// MulticallAddress overrides the Multicall3 contract used to batch reads. Defaults to the
	// canonical cross-chain Multicall3 deployment when unset.
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
	// ReplacementIntervalMs is how often a pending transaction is fee-bumped.
	ReplacementIntervalMs int `yaml:"replacementIntervalMs"`
	// PendingTimeoutMs switches a still-pending call to a same-nonce cancellation.
	PendingTimeoutMs int `yaml:"pendingTimeoutMs"`
	// ShutdownTimeoutMs bounds how long shutdown drains an accepted transaction lifecycle.
	ShutdownTimeoutMs int `yaml:"shutdownTimeoutMs"`
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

// Load validates a single YAML document after expanding deployment values. Secret values belong
// behind *Env references; their environment variables are read only by the component using them.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Errorf("read config %q: %w", path, err)
	}
	var cfg Config
	if err := parse.YAML(bytes.NewBufferString(os.ExpandEnv(string(data))), &cfg); err != nil {
		return nil, errors.Errorf("parse config %q: %w", path, err)
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, errors.Errorf("invalid config %q: %w", path, err)
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	tx := &c.TxManager
	tx.Confirmations = parse.OrDefault(tx.Confirmations, DefaultConfirmations)
	tx.BroadcastTimeoutMs = parse.OrDefault(tx.BroadcastTimeoutMs, DefaultBroadcastTimeoutMs)
	tx.AccountPollIntervalMs = parse.OrDefault(tx.AccountPollIntervalMs, DefaultAccountPollIntervalMs)
	tx.ReplacementIntervalMs = parse.OrDefault(tx.ReplacementIntervalMs, DefaultReplacementIntervalMs)
	tx.PendingTimeoutMs = parse.OrDefault(tx.PendingTimeoutMs, DefaultPendingTimeoutMs)
	tx.ShutdownTimeoutMs = parse.OrDefault(tx.ShutdownTimeoutMs, DefaultShutdownTimeoutMs)
	c.Observability.Addr = parse.OrDefault(c.Observability.Addr, DefaultObservabilityAddr)
	c.Chain.MulticallAddress = parse.OrDefault(c.Chain.MulticallAddress, DefaultMulticallAddress)
}

func (c *Config) Validate() error {
	if c.Chain.RPCURL == "" {
		return errors.New("chain.rpcUrl is required")
	}
	if c.Chain.ChainID == 0 {
		return errors.New("chain.chainId is required")
	}
	for i, url := range c.Chain.RPCFallbackURLs {
		if url == "" {
			return errors.Errorf("chain.rpcFallbackUrls[%d] is empty", i)
		}
	}
	if err := c.Signer.validate(); err != nil {
		return err
	}
	if err := c.TxManager.validate(false); err != nil {
		return err
	}
	if len(c.Solvers) == 0 {
		return errors.New("at least one solver is required (set `solvers`)")
	}
	seen := make(map[string]bool, len(c.Solvers))
	for i, spec := range c.Solvers {
		if spec.Name == "" {
			return errors.Errorf("solvers[%d].name is required", i)
		}
		if seen[spec.Name] {
			return errors.Errorf("duplicate solver %q: only one entry per solver type is allowed", spec.Name)
		}
		seen[spec.Name] = true
	}
	return nil
}

func (c *Config) ValidateTxManager() error { return c.TxManager.validate(true) }

func (c TxManagerConfig) validate(required bool) error {
	if math.IsNaN(c.MaxFeeGwei) || math.IsInf(c.MaxFeeGwei, 0) || c.MaxFeeGwei < 0 || (required && c.MaxFeeGwei == 0) {
		return errors.New("txManager.maxFeeGwei must be finite and positive")
	}
	if math.IsNaN(c.TipGwei) || math.IsInf(c.TipGwei, 0) || c.TipGwei < 0 {
		return errors.New("txManager.tipGwei must be finite and non-negative")
	}
	for _, interval := range []struct {
		name string
		ms   int
	}{
		{"broadcastTimeoutMs", c.BroadcastTimeoutMs},
		{"accountPollIntervalMs", c.AccountPollIntervalMs},
		{"replacementIntervalMs", c.ReplacementIntervalMs},
		{"pendingTimeoutMs", c.PendingTimeoutMs},
		{"shutdownTimeoutMs", c.ShutdownTimeoutMs},
	} {
		if interval.ms <= 0 {
			return errors.Errorf("txManager.%s must be positive", interval.name)
		}
		if uint64(interval.ms) > uint64(math.MaxInt64/int64(time.Millisecond))/4 {
			return errors.Errorf("txManager.%s exceeds the supported duration", interval.name)
		}
	}
	if c.PendingTimeoutMs < c.ReplacementIntervalMs {
		return errors.New("txManager.pendingTimeoutMs must be at least replacementIntervalMs")
	}
	return nil
}

func (s SignerConfig) validate() error {
	switch {
	case s.KeyEnv != "" && s.KeystorePath != "":
		return errors.New("signer: set exactly one of keyEnv or keystorePath, not both")
	case s.KeyEnv != "":
		return nil
	case s.KeystorePath == "":
		return errors.New("signer: one of keyEnv or keystorePath is required")
	case s.PassphraseEnv == "":
		return errors.New("signer: keystorePath requires passphraseEnv")
	default:
		return nil
	}
}
