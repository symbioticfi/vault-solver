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

type routeExecutors struct {
	Swap        common.Address
	Redemptions map[common.Address]common.Address
}

func (r routeExecutors) forToken(token common.Address) (common.Address, bool) {
	if executor, ok := r.Redemptions[token]; ok {
		return executor, true
	}
	return r.Swap, r.Swap != (common.Address{})
}

type routeExecutorConfig struct {
	SwapExecutor        string            `yaml:"swapExecutor"`
	RedemptionExecutors map[string]string `yaml:"redemptionExecutors"`
}

type config struct {
	BackendURL      string
	StateDir        string
	Executor        common.Address
	Curator         curatorConfig
	RecipeExecutors map[common.Address]routeExecutors
}

func parseConfig(node yaml.Node) (config, error) {
	var raw struct {
		BackendURL      string                         `yaml:"backendUrl"`
		StateDir        string                         `yaml:"stateDir"`
		Executor        string                         `yaml:"executor"`
		Curator         curatorConfig                  `yaml:"curator"`
		RecipeExecutors map[string]routeExecutorConfig `yaml:"recipeExecutors"`
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
	cfg := config{BackendURL: raw.BackendURL, StateDir: raw.StateDir, Executor: executor, Curator: raw.Curator, RecipeExecutors: make(map[common.Address]routeExecutors)}
	for venue, value := range raw.RecipeExecutors {
		connector, err := parse.NonZeroAddress(venue, "connector")
		if err != nil {
			return config{}, err
		}
		if _, exists := cfg.RecipeExecutors[connector]; exists {
			return config{}, errors.New("duplicate connector")
		}
		routes := routeExecutors{Redemptions: map[common.Address]common.Address{}}
		used := map[common.Address]bool{}
		if value.SwapExecutor != "" {
			routes.Swap, err = parse.NonZeroAddress(value.SwapExecutor, "swap executor")
			if err != nil {
				return config{}, err
			}
			used[routes.Swap] = true
		}
		if len(value.RedemptionExecutors) > 64 {
			return config{}, errors.New("connector exceeds 64 redemption executors")
		}
		for token, executor := range value.RedemptionExecutors {
			asset, err := parse.NonZeroAddress(token, "redemption asset")
			if err != nil {
				return config{}, err
			}
			target, err := parse.NonZeroAddress(executor, "redemption executor")
			if err != nil {
				return config{}, err
			}
			if _, exists := routes.Redemptions[asset]; exists {
				return config{}, errors.New("duplicate redemption asset")
			}
			if used[target] {
				return config{}, errors.New("redemption assets require distinct executors")
			}
			routes.Redemptions[asset] = target
			used[target] = true
		}
		if len(used) == 0 {
			return config{}, errors.New("connector requires a swap or redemption executor")
		}
		cfg.RecipeExecutors[connector] = routes
	}
	return cfg, nil
}
