package merkle

import (
	"encoding/json"
	"regexp"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"gopkg.in/yaml.v3"
)

const (
	runtimePolicy                 = "recipe-derived-v2"
	recipeCacheBytes        int64 = 16 << 20
	defaultPollInterval           = 15 * time.Second
	defaultOperationTimeout       = 30 * time.Second
	defaultStrategyInterval       = time.Minute
)

type rawConfig struct {
	Mode             string                `yaml:"mode" json:"mode"`
	StateDir         string                `yaml:"stateDir" json:"stateDir"`
	Curator          protocolio.HTTPConfig `yaml:"curator" json:"curator"`
	PollInterval     string                `yaml:"pollInterval" json:"pollInterval"`
	OperationTimeout string                `yaml:"operationTimeout" json:"operationTimeout"`
	Strategies       []rawStrategy         `yaml:"strategies" json:"strategies"`
}
type rawStrategy struct {
	Name       string `yaml:"name" json:"name"`
	Executor   string `yaml:"executor" json:"executor"`
	RecipeHash string `yaml:"recipeHash" json:"recipeHash"`
	Interval   string `yaml:"interval" json:"interval"`
}
type config struct {
	mode, stateDir                 string
	curator                        protocolio.HTTPConfig
	pollInterval, operationTimeout time.Duration
	strategies                     []keeper.Strategy
}

var strategyName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func parseConfig(node yaml.Node) (config, error) {
	var raw rawConfig
	if err := solver.DecodeStrict(node, &raw); err != nil {
		return config{}, err
	}
	raw.Mode = parse.OrDefault(raw.Mode, "simulate")
	if raw.Mode != "send" && raw.Mode != "simulate" {
		return config{}, errors.New("merkle mode must be send or simulate")
	}
	if raw.StateDir == "" || len(raw.Strategies) == 0 || len(raw.Strategies) > 256 {
		return config{}, errors.New("merkle stateDir and 1..256 strategies required")
	}
	curator, err := permissions.NormalizeHostConfig(raw.Curator)
	if err != nil {
		return config{}, err
	}
	raw.Curator = curator
	poll, err := parse.Duration(raw.PollInterval, defaultPollInterval, "merkle pollInterval")
	if err != nil {
		return config{}, err
	}
	timeout, err := parse.Duration(raw.OperationTimeout, defaultOperationTimeout, "merkle operationTimeout")
	if err != nil {
		return config{}, err
	}
	if poll != defaultPollInterval || timeout != defaultOperationTimeout {
		return config{}, errors.New("merkle polling and operation timeout are fixed; remove legacy overrides")
	}
	cfg := config{mode: raw.Mode, stateDir: raw.StateDir, curator: raw.Curator, pollInterval: poll, operationTimeout: timeout}
	seen := make(map[string]bool)
	for _, s := range raw.Strategies {
		if !strategyName.MatchString(s.Name) || seen[s.Name] {
			return config{}, errors.New("merkle unique strategy name required")
		}
		seen[s.Name] = true
		executor, err := parse.NonZeroAddress(s.Executor, "merkle executor")
		if err != nil {
			return config{}, err
		}
		if s.RecipeHash != "" {
			if _, err := parse.Hash(s.RecipeHash, "merkle recipeHash"); err != nil {
				return config{}, err
			}
		}
		interval, err := parse.Duration(s.Interval, defaultStrategyInterval, "merkle strategy interval")
		if err != nil {
			return config{}, err
		}
		if interval != defaultStrategyInterval {
			return config{}, errors.New("merkle strategy interval is fixed; remove the legacy override")
		}
		cfg.strategies = append(cfg.strategies, keeper.Strategy{Config: keeper.Config{Name: s.Name, Executor: executor.Hex(), RecipeHash: s.RecipeHash, Interval: interval}, Providers: []provider.Provider{keeper.NewRecipeInputs()}})
	}
	return cfg, nil
}

// Bind derived identity and the shared source to existing recovery guards.
func strategyRevision(strategy keeper.Config, curator protocolio.HTTPConfig, caller common.Address) (string, error) {
	raw, err := json.Marshal(struct {
		Policy   string                `json:"policy"`
		Strategy keeper.Config         `json:"strategy"`
		Curator  protocolio.HTTPConfig `json:"curator"`
		Caller   common.Address        `json:"caller"`
	}{runtimePolicy, strategy, curator, caller})
	if err != nil {
		return "", errors.Errorf("merkle strategy identity: %w", err)
	}
	return crypto.Keccak256Hash(raw).Hex(), nil
}
