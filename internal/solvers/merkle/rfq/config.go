package rfq

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/vault-solver/internal/merklehost"
	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"gopkg.in/yaml.v3"
)

type Config struct {
	RecipeExecutor common.Address
	QuoteOnly      bool
	BackendURL     string
	StateDir       string
	Executor       common.Address
	Curator        merklehost.Curator
	Connectors     map[common.Address]bool
}

func parseConfig(node yaml.Node) (Config, error) {
	var raw struct {
		BackendURL     string             `yaml:"backendUrl"`
		StateDir       string             `yaml:"stateDir"`
		Executor       string             `yaml:"executor"`
		RecipeExecutor string             `yaml:"recipeExecutor"`
		Curator        merklehost.Curator `yaml:"curator"`
		Connectors     []string           `yaml:"connectors"`
	}
	if err := solver.DecodeStrict(node, &raw); err != nil {
		return Config{}, err
	}
	if len(raw.Connectors) > 256 {
		return Config{}, errors.New("at most 256 connectors")
	}
	executor, err := parse.NonZeroAddress(raw.Executor, "executor")
	if err != nil {
		return Config{}, err
	}
	recipeExecutor, err := parse.NonZeroAddress(raw.RecipeExecutor, "recipeExecutor")
	if err != nil {
		return Config{}, err
	}
	cfg := Config{RecipeExecutor: recipeExecutor, BackendURL: raw.BackendURL, StateDir: raw.StateDir, Executor: executor, Curator: raw.Curator, Connectors: make(map[common.Address]bool)}
	for _, venue := range raw.Connectors {
		connector, err := parse.NonZeroAddress(venue, "connector")
		if err != nil {
			return Config{}, err
		}
		if cfg.Connectors[connector] {
			return Config{}, errors.New("duplicate connector")
		}
		cfg.Connectors[connector] = true
	}
	return cfg, cfg.Validate()
}

// Validate checks the reusable Merkle execution component's startup settings.
func (c Config) Validate() error {
	if c.RecipeExecutor == (common.Address{}) || c.Executor == (common.Address{}) {
		return errors.New("executor and recipeExecutor must be nonzero")
	}
	if err := permissions.ValidateConfig(protocolio.HTTPConfig{URL: c.BackendURL, TimeoutSeconds: permissions.HostTimeoutSeconds, MaxResponseBytes: permissions.HostMaxResponseBytes}); err != nil {
		return errors.Errorf("backendUrl: %w", err)
	}
	if !c.QuoteOnly {
		if c.StateDir == "" {
			return errors.New("recipeExecutor requires stateDir")
		}
		if err := permissions.ValidateConfig(c.Curator.HTTPConfig()); err != nil {
			return errors.Errorf("curator: %w", err)
		}
	}
	return nil
}
