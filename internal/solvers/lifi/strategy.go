package lifi

import (
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/webhook"
)

func newStrategy(spec StrategyConfig) (types.Strategy, error) {
	return strategies.New(spec.Name, spec.Config)
}
