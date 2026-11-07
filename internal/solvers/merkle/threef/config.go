package threef

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/vault-solver/internal/merklehost"
	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"gopkg.in/yaml.v3"
)

const pollInterval = 15 * time.Second
const operationTimeout = 30 * time.Second

type config struct {
	Mode      string             `yaml:"mode"`
	Executor  string             `yaml:"executor"`
	Policy    string             `yaml:"policy"`
	Curator   merklehost.Curator `yaml:"curator"`
	URL       string             `yaml:"url"`
	APIKeyEnv string             `yaml:"apiKeyEnv"`
	Broker    string             `yaml:"broker"`
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
	executor, err := parse.NonZeroAddress(cfg.Executor, "executor")
	if err != nil {
		return config{}, err
	}
	cfg.Executor = executor.Hex()
	policy, err := parse.NonZeroAddress(cfg.Policy, "policy")
	if err != nil {
		return config{}, err
	}
	cfg.Policy = policy.Hex()
	broker, err := parse.NonZeroAddress(cfg.Broker, "broker")
	if err != nil {
		return config{}, err
	}
	cfg.Broker = broker.Hex()
	if err := permissions.ValidateURL(cfg.Curator.URL); err != nil {
		return config{}, err
	}
	if err := validateAPIConfig(cfg.apiConfig()); err != nil {
		return config{}, err
	}
	return cfg, nil
}
