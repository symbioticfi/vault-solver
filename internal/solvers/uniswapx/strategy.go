package uniswapx

import (
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/webhook"
)

func newStrategy(spec StrategyConfig) (types.Strategy, error) {
	return strategies.New(spec.Name, spec.Config)
}

func validateStrategyConfig(spec StrategyConfig) error {
	return strategies.Validate(spec.Name, spec.Config)
}
