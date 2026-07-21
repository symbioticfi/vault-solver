package lifi

import (
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/webhook"
)

func newStrategy(spec StrategyConfig, chainClient *chain.Client, log logr.Logger) (types.Strategy, error) {
	name := spec.Name
	if name == "" {
		name = defaultStrategyName
	}
	return strategies.New(name, spec.Config, strategies.Deps{Chain: chainClient, Log: log})
}
