package lifi

import (
	"github.com/go-errors/errors"
	local "github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	remote "github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/webhook"
)

func newStrategy(spec StrategyConfig) (types.Strategy, error) {
	switch spec.Name {
	case "", local.Name:
		return local.NewFromConfig(spec.Config)
	case remote.Name:
		return remote.NewFromConfig(spec.Config)
	default:
		return nil, errors.Errorf("unknown LI.FI strategy %q (available: default, webhook)", spec.Name)
	}
}
