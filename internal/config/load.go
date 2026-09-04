package config

import (
	"bytes"
	"io"
	"os"

	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"
)

const (
	defaultConfirmations         = 2
	defaultBroadcastTimeoutMs    = 5_000
	defaultAccountPollIntervalMs = 30_000
	defaultReplacementIntervalMs = 30_000
	defaultPendingTimeoutMs      = 300_000
	defaultShutdownTimeoutMs     = 60_000
	defaultObservabilityAddr     = ":9090"
	defaultMulticallAddress      = "0xcA11bde05977b3631167028862bE2a173976CA11"
)

// Load expands non-secret environment references, strictly decodes one YAML document, applies
// process defaults, and validates the generic contract. Secret-bearing fields store environment
// variable names and are resolved only by their owning runtime adapter.
func Load(path string) (*Config, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Errorf("read config %q: %w", path, err)
	}
	cfg, err := decode([]byte(os.ExpandEnv(string(contents))))
	if err != nil {
		return nil, errors.Errorf("parse config %q: %w", path, err)
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, errors.Errorf("invalid config %q: %w", path, err)
	}
	return cfg, nil
}

func decode(contents []byte) (*Config, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}
	var trailing yaml.Node
	err := decoder.Decode(&trailing)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if err == nil && len(trailing.Content) != 0 {
		return nil, errors.New("multiple YAML documents are not supported")
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.TxManager.Confirmations == 0 {
		c.TxManager.Confirmations = defaultConfirmations
	}
	if c.TxManager.BroadcastTimeoutMs == 0 {
		c.TxManager.BroadcastTimeoutMs = defaultBroadcastTimeoutMs
	}
	if c.TxManager.AccountPollIntervalMs == 0 {
		c.TxManager.AccountPollIntervalMs = defaultAccountPollIntervalMs
	}
	if c.TxManager.ReplacementIntervalMs == 0 {
		c.TxManager.ReplacementIntervalMs = defaultReplacementIntervalMs
	}
	if c.TxManager.PendingTimeoutMs == 0 {
		c.TxManager.PendingTimeoutMs = defaultPendingTimeoutMs
	}
	if c.TxManager.ShutdownTimeoutMs == 0 {
		c.TxManager.ShutdownTimeoutMs = defaultShutdownTimeoutMs
	}
	if c.Observability.Addr == "" {
		c.Observability.Addr = defaultObservabilityAddr
	}
	if c.Chain.MulticallAddress == "" {
		c.Chain.MulticallAddress = defaultMulticallAddress
	}
}
