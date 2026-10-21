package rfq

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"gopkg.in/yaml.v3"
)

type curatorConfig struct {
	URL       string `yaml:"url"`
	APIKeyEnv string `yaml:"apiKeyEnv"`
}

func (c curatorConfig) httpConfig() protocolio.HTTPConfig {
	return protocolio.HTTPConfig{URL: c.URL, APIKeyEnv: c.APIKeyEnv, TimeoutSeconds: permissions.HostTimeoutSeconds, MaxResponseBytes: permissions.HostMaxResponseBytes}
}

type config struct {
	BackendURL      string
	StateDir        string
	Executor        common.Address
	Curator         curatorConfig
	RecipeExecutors map[common.Address]map[common.Address]common.Address
}

func parseConfig(node yaml.Node) (config, error) {
	var raw struct {
		BackendURL      string                       `yaml:"backendUrl"`
		StateDir        string                       `yaml:"stateDir"`
		Executor        string                       `yaml:"executor"`
		Curator         curatorConfig                `yaml:"curator"`
		RecipeExecutors map[string]map[string]string `yaml:"recipeExecutors"`
	}
	if err := solver.DecodeStrict(node, &raw); err != nil {
		return config{}, err
	}
	if err := permissions.ValidateConfig(protocolio.HTTPConfig{URL: raw.BackendURL, TimeoutSeconds: permissions.HostTimeoutSeconds, MaxResponseBytes: permissions.HostMaxResponseBytes}); err != nil {
		return config{}, errors.Errorf("backendUrl: %w", err)
	}
	if err := permissions.ValidateConfig(raw.Curator.httpConfig()); err != nil {
		return config{}, errors.Errorf("curator: %w", err)
	}
	if raw.StateDir == "" || len(raw.RecipeExecutors) == 0 || len(raw.RecipeExecutors) > 256 {
		return config{}, errors.New("rfq-merkle requires stateDir and 1..256 recipeExecutors")
	}
	executor, err := parse.NonZeroAddress(raw.Executor, "executor")
	if err != nil {
		return config{}, err
	}
	cfg := config{BackendURL: raw.BackendURL, StateDir: raw.StateDir, Executor: executor, Curator: raw.Curator, RecipeExecutors: make(map[common.Address]map[common.Address]common.Address)}
	used := map[common.Address]bool{}
	for venue, assets := range raw.RecipeExecutors {
		connector, err := parse.NonZeroAddress(venue, "connector")
		if err != nil {
			return config{}, err
		}
		if _, exists := cfg.RecipeExecutors[connector]; exists {
			return config{}, errors.New("duplicate connector")
		}
		if len(assets) == 0 || len(assets) > 64 {
			return config{}, errors.New("connector requires 1..64 asset recipe executors")
		}
		routes := make(map[common.Address]common.Address)
		for token, executor := range assets {
			asset, err := parse.NonZeroAddress(token, "asset")
			if err != nil {
				return config{}, err
			}
			target, err := parse.NonZeroAddress(executor, "recipe executor")
			if err != nil {
				return config{}, err
			}
			if _, exists := routes[asset]; exists {
				return config{}, errors.New("duplicate asset")
			}
			if used[target] {
				return config{}, errors.New("Liquid Lane assets require distinct recipe executors")
			}
			routes[asset] = target
			used[target] = true
		}
		cfg.RecipeExecutors[connector] = routes
	}
	return cfg, nil
}
