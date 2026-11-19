// Package filler runs protocol fills through caller-bound Merkle recipes. It owns what every
// protocol filler shares: the connector workflow and route cache, the signer lane with its
// single in-flight fill, and the spend accounting quotes subtract. A protocol package adds
// only its order source and the settlement envelope around the recipe call.
package filler

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/merklehost"
	"github.com/symbioticfi/vault-solver/internal/parse"
)

// RawConfig is the YAML shape shared by Merkle fillers; protocol configs inline it.
type RawConfig struct {
	Executor        string             `yaml:"executor"`
	RecipeExecutors []string           `yaml:"recipeExecutors"`
	Curator         merklehost.Curator `yaml:"curator"`
}

// RawSolverConfig is the YAML shape of a Merkle solver that runs a protocol engine: the engine's
// own settings inline, plus the recipes that fund its fills. The engine names the executor.
type RawSolverConfig[Engine any] struct {
	Engine Engine `yaml:",inline"`

	RecipeExecutors []string           `yaml:"recipeExecutors"`
	Curator         merklehost.Curator `yaml:"curator"`
}

// Lane validates the Merkle settings for the engine's executor.
func (r RawSolverConfig[Engine]) Lane(executor string) (Config, error) {
	return RawConfig{Executor: executor, RecipeExecutors: r.RecipeExecutors, Curator: r.Curator}.Parse()
}

// Config identifies one protocol settlement contract and the recipes funding its fills.
type Config struct {
	// Executor is the protocol settlement contract: it calls the recipe and receives the payment.
	Executor        common.Address
	RecipeExecutors []common.Address
	// Curator provides the Merkle permission tree for route discovery and execution.
	Curator merklehost.Curator
	// QuoteOnly discovers routes without a signer or transaction lane.
	QuoteOnly bool
}

// Parse validates the shared YAML fields.
func (r RawConfig) Parse() (Config, error) {
	executor, err := parse.NonZeroAddress(r.Executor, "executor")
	if err != nil {
		return Config{}, err
	}
	recipeExecutors, err := connector.ParseRecipeExecutors(r.RecipeExecutors)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{Executor: executor, RecipeExecutors: recipeExecutors, Curator: r.Curator}
	return cfg, cfg.Validate()
}

// Validate checks the startup settings of a Merkle filler.
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
	if err := permissions.ValidateURL(c.Curator.URL); err != nil {
		return errors.Errorf("curator: %w", err)
	}
	return nil
}
