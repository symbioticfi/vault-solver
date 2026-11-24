// Package uniswapx is the uniswapx-merkle solver: UniswapX quoting and order intake settled
// through caller-bound Merkle recipes. It owns the solver's registration and config; the
// UniswapX engine it runs is shared with the adapter-backed uniswapx-filler.
package uniswapx

import (
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"
	engine "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx"
)

const Name = "uniswapx-merkle"

//nolint:gochecknoinits // Self-registration is the solver plugin contract.
func init() { solver.Register(Name, factory) }

// rawConfig is the uniswapx-merkle configuration: the UniswapX engine settings and the recipes that
// fund its fills. It has no adapter, solver mode or gas accounting settings.
type rawConfig = filler.RawSolverConfig[engine.RawEngineConfig]

func parseConfig(node yaml.Node) (*engine.Config, filler.Config, error) {
	var raw rawConfig
	if err := solver.DecodeStrict(node, &raw); err != nil {
		return nil, filler.Config{}, err
	}
	lane, err := raw.Lane(raw.Engine.Executor)
	if err != nil {
		return nil, filler.Config{}, err
	}
	cfg, err := engine.ParseRecipeConfig(raw.Engine)
	return cfg, lane, err
}

func factory(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
	cfg, lane, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	return engine.NewRecipeSolver(Name, cfg, lane, deps)
}
