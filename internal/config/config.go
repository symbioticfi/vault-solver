// Package config loads and validates the vault-solver YAML configuration.
//
// Decoding is two-stage: the generic layer parses everything except the solver-specific block,
// which it keeps as a raw yaml.Node under Solver.Config. The selected solver decodes that node
// into its own typed struct, so solver config stays fully encapsulated in the solver package.
package config

import (
	"bytes"
	"os"

	"github.com/go-errors/errors"

	"gopkg.in/yaml.v3"
)

// Config is the top-level bot configuration.
type Config struct {
	Chain     ChainConfig     `yaml:"chain"`
	Signer    SignerConfig    `yaml:"signer"`
	TxManager TxManagerConfig `yaml:"txManager"`
	// Solvers is the set of solvers to run in one process — at most one entry per solver type. They
	// share the chain client, signer, and (crucially) the single nonce-serialized txManager, so they
	// never race on nonces.
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
	// WriteRPCURL, when set, is used ONLY to broadcast signed transactions (eth_sendRawTransaction).
	// Every read — nonce, gas, fee, receipts, block number — stays on `rpcUrl`. Point this at a
	// private/MEV-protected endpoint (e.g. mevblocker) to submit fills privately while reading from a
	// normal RPC. Optional; empty means broadcasts also use `rpcUrl`. Expand from the environment
	// with ${WRITE_RPC_URL}.
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
	// MaxFeeGwei caps the EIP-1559 max fee per gas; 0 means "derive from base fee".
	MaxFeeGwei float64 `yaml:"maxFeeGwei"`
	// TipGwei is the EIP-1559 priority fee; 0 means "use the node's suggestion".
	TipGwei float64 `yaml:"tipGwei"`
}

// SolverConfig names the solver implementation and carries its opaque, deferred config.
type SolverConfig struct {
	Name string `yaml:"name"`
	// Config is decoded by the selected solver into its own typed struct (two-stage decode).
	Config yaml.Node `yaml:"config"`
}

// DefaultConfirmations is used when TxManager.Confirmations is unset.
const DefaultConfirmations = 2

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
	// (keyEnv, passphraseEnv, apiKeyEnv), which os.Getenv at point of use and never place the secret
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
	if c.TxManager.Confirmations == 0 {
		c.TxManager.Confirmations = DefaultConfirmations
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
	if err := c.Signer.validate(); err != nil {
		return err
	}
	if len(c.Solvers) == 0 {
		return errors.New("at least one solver is required (set `solver` or `solvers`)")
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
