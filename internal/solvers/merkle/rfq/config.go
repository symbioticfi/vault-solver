package rfq

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/vault-solver/internal/merklehost"
	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"gopkg.in/yaml.v3"
)

type Config struct {
	RecipeExecutors []common.Address
	QuoteOnly       bool
	BackendURL      string
	Executor        common.Address
	Curator         merklehost.Curator
}

func parseConfig(node yaml.Node) (Config, error) {
	var raw struct {
		BackendURL      string             `yaml:"backendUrl"`
		Executor        string             `yaml:"executor"`
		RecipeExecutors []string           `yaml:"recipeExecutors"`
		Curator         merklehost.Curator `yaml:"curator"`
	}
	if err := solver.DecodeStrict(node, &raw); err != nil {
		return Config{}, err
	}
	executor, err := parse.NonZeroAddress(raw.Executor, "executor")
	if err != nil {
		return Config{}, err
	}
	recipeExecutors, err := ParseRecipeExecutors(raw.RecipeExecutors)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{RecipeExecutors: recipeExecutors, BackendURL: raw.BackendURL, Executor: executor, Curator: raw.Curator}
	return cfg, cfg.Validate()
}

// Validate checks the reusable Merkle execution component's startup settings.
func (c Config) Validate() error {
	if len(c.RecipeExecutors) == 0 || c.Executor == (common.Address{}) {
		return errors.New("executor and recipeExecutors must be nonzero")
	}
	seen := make(map[common.Address]bool, len(c.RecipeExecutors))
	for _, executor := range c.RecipeExecutors {
		if executor == (common.Address{}) || seen[executor] {
			return errors.New("recipeExecutors must contain distinct nonzero addresses")
		}
		seen[executor] = true
	}
	if err := permissions.ValidateURL(c.BackendURL); err != nil {
		return errors.Errorf("backendUrl: %w", err)
	}
	if !c.QuoteOnly {
		if err := permissions.ValidateURL(c.Curator.URL); err != nil {
			return errors.Errorf("curator: %w", err)
		}
	}
	return nil
}

// ParseRecipeExecutors parses the shared RFQ host and plugin configuration.
func ParseRecipeExecutors(values []string) ([]common.Address, error) {
	out := make([]common.Address, 0, len(values))
	for _, value := range values {
		address, err := parse.NonZeroAddress(value, "recipeExecutors")
		if err != nil {
			return nil, err
		}
		out = append(out, address)
	}
	return out, nil
}
