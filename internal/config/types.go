// Package config owns the generic, file-backed process configuration.
package config

import "gopkg.in/yaml.v3"

// Config contains only process-wide mechanisms. SolverConfig.Config remains opaque until an
// integration selected by the composition root decodes it.
type Config struct {
	Chain         ChainConfig         `yaml:"chain"`
	Signer        SignerConfig        `yaml:"signer"`
	TxManager     TxManagerConfig     `yaml:"txManager"`
	Solvers       []SolverConfig      `yaml:"solvers"`
	Observability ObservabilityConfig `yaml:"observability"`
}

type ChainConfig struct {
	RPCURL           string   `yaml:"rpcUrl"`
	RPCFallbackURLs  []string `yaml:"rpcFallbackUrls,omitempty"`
	WriteRPCURL      string   `yaml:"writeRpcUrl,omitempty"`
	ChainID          uint64   `yaml:"chainId"`
	MulticallAddress string   `yaml:"multicallAddress,omitempty"`
}

type SignerConfig struct {
	KeyEnv        string `yaml:"keyEnv,omitempty"`
	KeystorePath  string `yaml:"keystorePath,omitempty"`
	PassphraseEnv string `yaml:"passphraseEnv,omitempty"`
}

type TxManagerConfig struct {
	Confirmations         uint64  `yaml:"confirmations"`
	MaxFeeGwei            float64 `yaml:"maxFeeGwei"`
	TipGwei               float64 `yaml:"tipGwei"`
	BroadcastTimeoutMs    int     `yaml:"broadcastTimeoutMs"`
	AccountPollIntervalMs int     `yaml:"accountPollIntervalMs"`
	ReplacementIntervalMs int     `yaml:"replacementIntervalMs"`
	PendingTimeoutMs      int     `yaml:"pendingTimeoutMs"`
	ShutdownTimeoutMs     int     `yaml:"shutdownTimeoutMs"`
}

type ObservabilityConfig struct {
	Addr  string `yaml:"addr"`
	Debug bool   `yaml:"debug"`
}

type SolverConfig struct {
	Name   string    `yaml:"name"`
	Config yaml.Node `yaml:"config"`
}
