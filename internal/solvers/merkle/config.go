package merkle

import (
	"context"
	"encoding/json"
	"maps"
	"regexp"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"gopkg.in/yaml.v3"
)

type rawConfig struct {
	Mode             string        `yaml:"mode" json:"mode"`
	StateDir         string        `yaml:"stateDir" json:"stateDir"`
	PollInterval     string        `yaml:"pollInterval" json:"pollInterval"`
	OperationTimeout string        `yaml:"operationTimeout" json:"operationTimeout"`
	Strategies       []rawStrategy `yaml:"strategies" json:"strategies"`
}
type rawStrategy struct {
	Name       string          `yaml:"name" json:"name"`
	Executor   string          `yaml:"executor" json:"executor"`
	Account    string          `yaml:"account" json:"account"`
	RecipeHash string          `yaml:"recipeHash" json:"recipeHash"`
	Tree       string          `yaml:"tree" json:"tree"`
	Interval   string          `yaml:"interval" json:"interval"`
	Inputs     map[uint]string `yaml:"inputs" json:"inputs"`
}
type config struct {
	mode, stateDir                 string
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
	if raw.StateDir == "" || len(raw.Strategies) == 0 {
		return config{}, errors.New("merkle stateDir and strategies required")
	}
	poll, err := parse.Duration(raw.PollInterval, 15*time.Second, "merkle pollInterval")
	if err != nil {
		return config{}, err
	}
	timeout, err := parse.Duration(raw.OperationTimeout, 30*time.Second, "merkle operationTimeout")
	if err != nil {
		return config{}, err
	}
	cfg := config{mode: raw.Mode, stateDir: raw.StateDir, pollInterval: poll, operationTimeout: timeout}
	seen := make(map[string]bool)
	for _, s := range raw.Strategies {
		if !strategyName.MatchString(s.Name) || seen[s.Name] || s.Tree == "" {
			return config{}, errors.New("merkle unique strategy name and tree required")
		}
		seen[s.Name] = true
		executor, err := parse.NonZeroAddress(s.Executor, "merkle executor")
		if err != nil {
			return config{}, err
		}
		account, err := parse.NonZeroAddress(s.Account, "merkle account")
		if err != nil {
			return config{}, err
		}
		if s.RecipeHash != "" {
			if _, err := parse.Hash(s.RecipeHash, "merkle recipeHash"); err != nil {
				return config{}, err
			}
		}
		interval, err := parse.Duration(s.Interval, time.Minute, "merkle strategy interval")
		if err != nil {
			return config{}, err
		}
		for index, value := range s.Inputs {
			n, err := parse.Big(value, "merkle input")
			if err != nil {
				return config{}, err
			}
			if index > 4095 || n.Sign() < 0 || n.BitLen() > 256 {
				return config{}, errors.New("merkle input exceeds uint256 or input index bounds")
			}
		}
		s.Executor = executor.Hex()
		s.Account = account.Hex()
		semantic, err := json.Marshal(s)
		if err != nil {
			return config{}, errors.Errorf("fingerprint merkle workflow: %w", err)
		}
		cfg.strategies = append(cfg.strategies, keeper.Strategy{Config: keeper.Config{Name: s.Name, Executor: s.Executor, Account: s.Account, RecipeHash: s.RecipeHash, Tree: s.Tree, Interval: interval, Revision: crypto.Keccak256Hash(semantic).Hex()}, Providers: []provider.Provider{fixedInputs{inputs: s.Inputs}}})
	}
	return cfg, nil
}

// Fixed inputs supply configured amounts only; recipe instructions perform live
// capacity reads and bound protocol calls. Auction providers remain host code.
type fixedInputs struct{ inputs map[uint]string }

func (p fixedInputs) Prepare(ctx context.Context, _ provider.Snapshot) (provider.Output, error) {
	if err := ctx.Err(); err != nil {
		return provider.Output{}, err
	}
	return provider.Output{Inputs: maps.Clone(p.inputs)}, nil
}
func (fixedInputs) Advance(ctx context.Context, snapshot provider.Snapshot, _ provider.Output, _ json.RawMessage) (provider.Progress, error) {
	if err := ctx.Err(); err != nil {
		return provider.Progress{}, err
	}
	if err := snapshot.Execution.Validate(); err != nil {
		return provider.Progress{}, err
	}
	return provider.Progress{Done: true, Outcome: "confirmed"}, nil
}
