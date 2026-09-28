// Package singlestrategy registers the single-source LiquidLane policy for UniswapX.
package singlestrategy

import (
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

//nolint:gochecknoinits // solver-local strategy registration.
func init() { strategies.Register(types.SingleName, defaultstrategy.NewSingleFromConfig) }
