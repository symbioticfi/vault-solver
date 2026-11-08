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
	// Solvers selects the integration workers for this solver process, at most one per type.
	// Workers use the process chain client, signer and SDK transaction manager.
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
	// RPCURL and WriteRPCURL must use HTTP(S).
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
	ChainID     uint64 `yaml:"chainId"`
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

// TxManagerConfig bounds this process's transaction sender.
type TxManagerConfig struct {
	Confirmations         uint64  `yaml:"confirmations"`
	MaxFeeGwei            float64 `yaml:"maxFeeGwei"`
	MaxGas                uint64  `yaml:"maxGas"`
	PollIntervalMs        int     `yaml:"pollIntervalMs"`
	ReplacementIntervalMs int     `yaml:"replacementIntervalMs"`
	ShutdownTimeoutMs     int     `yaml:"shutdownTimeoutMs"`
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
	// Keep in sync with chain.defaultRPCAttemptTimeout.
	DefaultRPCAttemptTimeoutMs   = 20_000
	DefaultPollIntervalMs        = 2_000
	DefaultReplacementIntervalMs = 30_000
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
	if c.TxManager.PollIntervalMs == 0 {
		c.TxManager.PollIntervalMs = DefaultPollIntervalMs
	}
	if c.TxManager.ReplacementIntervalMs == 0 {
		c.TxManager.ReplacementIntervalMs = DefaultReplacementIntervalMs
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
	if c.Confirmations > 1024 {
		return errors.New("txManager.confirmations must not exceed 1024")
	}
	for _, timing := range []struct {
		name  string
		value int
	}{
		{"pollIntervalMs", c.PollIntervalMs}, {"replacementIntervalMs", c.ReplacementIntervalMs}, {"shutdownTimeoutMs", c.ShutdownTimeoutMs},
	} {
		if timing.value <= 0 || int64(timing.value) > math.MaxInt64/int64(time.Millisecond) {
			return errors.Errorf("txManager.%s must be positive and fit in a time.Duration", timing.name)
		}
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
