package threef

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"gopkg.in/yaml.v3"
)

const pollInterval = 15 * time.Second
const operationTimeout = 30 * time.Second

type curatorConfig struct {
	URL       string `yaml:"url"`
	APIKeyEnv string `yaml:"apiKeyEnv"`
}

func (c curatorConfig) httpConfig() protocolio.HTTPConfig {
	return protocolio.HTTPConfig{URL: c.URL, APIKeyEnv: c.APIKeyEnv, TimeoutSeconds: permissions.HostTimeoutSeconds, MaxResponseBytes: permissions.HostMaxResponseBytes}
}

type config struct {
	Mode      string        `yaml:"mode"`
	StateDir  string        `yaml:"stateDir"`
	Executors []string      `yaml:"executors"`
	Curator   curatorConfig `yaml:"curator"`
	URL       string        `yaml:"url"`
	APIKeyEnv string        `yaml:"apiKeyEnv"`
	Broker    string        `yaml:"broker"`
}

func (c config) apiConfig() apiConfig {
	return apiConfig{URL: c.URL, APIKeyEnv: c.APIKeyEnv, Broker: common.HexToAddress(c.Broker)}
}
func parseConfig(node yaml.Node) (config, error) {
	var cfg config
	if err := solver.DecodeStrict(node, &cfg); err != nil {
		return config{}, err
	}
	cfg.Mode = parse.OrDefault(cfg.Mode, "simulate")
	if cfg.Mode != "simulate" && cfg.Mode != "send" {
		return config{}, errors.New("threef-merkle mode must be send or simulate")
	}
	if cfg.StateDir == "" {
		return config{}, errors.New("threef-merkle stateDir is required")
	}
	if len(cfg.Executors) == 0 || len(cfg.Executors) > 256 {
		return config{}, errors.New("threef-merkle requires 1..256 executors")
	}
	seen := make(map[common.Address]bool, len(cfg.Executors))
	for i, text := range cfg.Executors {
		value, err := parse.NonZeroAddress(text, "executors["+strconv.Itoa(i)+"]")
		if err != nil {
			return config{}, err
		}
		if seen[value] {
			return config{}, errors.New("duplicate threef-merkle executor")
		}
		seen[value] = true
		cfg.Executors[i] = value.Hex()
	}
	broker, err := parse.NonZeroAddress(cfg.Broker, "broker")
	if err != nil {
		return config{}, err
	}
	cfg.Broker = broker.Hex()
	if err := permissions.ValidateConfig(cfg.Curator.httpConfig()); err != nil {
		return config{}, err
	}
	if err := validateAPIConfig(cfg.apiConfig()); err != nil {
		return config{}, err
	}
	return cfg, nil
}
func strategyRevision(strategy keeper.Config, cfg config, caller common.Address) (string, error) {
	raw, err := json.Marshal(struct {
		Policy   string                `json:"policy"`
		Strategy keeper.Config         `json:"strategy"`
		Curator  protocolio.HTTPConfig `json:"curator"`
		API      apiConfig             `json:"api"`
		Caller   common.Address        `json:"caller"`
	}{"1", strategy, cfg.Curator.httpConfig(), cfg.apiConfig(), caller})
	if err != nil {
		return "", errors.Errorf("threef-merkle strategy identity: %w", err)
	}
	return crypto.Keccak256Hash(raw).Hex(), nil
}
