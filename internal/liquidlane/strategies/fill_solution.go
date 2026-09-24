package strategies

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// FillSolution holds selected routes before the protocol output requirement is applied.
// It owns its amounts; callers can finalize it repeatedly without mutating the allocation.
type FillSolution struct {
	routes       []FillRoute
	gasAmount    *big.Int
	maxAmountOut *big.Int
}

// NewFillSolution snapshots a strategy's selected routes and gas cost. Route outputs
// must be positive and gas non-negative. MinAmountOut is assigned by Finalize;
// source identity and capacity remain subject to the solver's ValidateFillRoutes.
func NewFillSolution(routes []FillRoute, gasAmount *big.Int) *FillSolution {
	if len(routes) == 0 || gasAmount != nil && gasAmount.Sign() < 0 {
		return nil
	}
	gas := new(big.Int)
	if gasAmount != nil {
		gas.Set(gasAmount)
	}
	total := new(big.Int)
	selected := make([]FillRoute, len(routes))
	for index, route := range routes {
		if route.ExpectedAmountOut == nil || route.ExpectedAmountOut.Sign() <= 0 {
			return nil
		}
		total.Add(total, route.ExpectedAmountOut)
		selected[index] = cloneFillRoute(route)
		selected[index].MinAmountOut = nil
	}
	maximum := new(big.Int).Sub(total, gas)
	if maximum.Sign() <= 0 {
		return nil
	}
	return &FillSolution{routes: selected, gasAmount: gas, maxAmountOut: maximum}
}

// MaxAmountOut is the largest protocol output requirement this allocation can satisfy.
func (a *FillSolution) MaxAmountOut() *big.Int {
	if a == nil {
		return new(big.Int)
	}
	return liquidlane.CloneBig(a.maxAmountOut)
}

// Finalize distributes the required protocol output and gas across selected routes.
func (a *FillSolution) Finalize(requiredAmountOut *big.Int) []FillRoute {
	if a == nil || requiredAmountOut == nil || requiredAmountOut.Sign() <= 0 ||
		requiredAmountOut.Cmp(a.maxAmountOut) > 0 {
		return nil
	}
	minimumTotal := new(big.Int).Add(requiredAmountOut, a.gasAmount)
	targets := make([]*big.Int, len(a.routes))
	for index, route := range a.routes {
		targets[index] = route.ExpectedAmountOut
	}
	minimums := distributeMinimums(targets, minimumTotal)
	if minimums == nil {
		return nil
	}
	routes := make([]FillRoute, len(a.routes))
	for index, route := range a.routes {
		routes[index] = cloneFillRoute(route)
		routes[index].MinAmountOut = minimums[index]
	}
	return routes
}

func distributeMinimums(targets []*big.Int, total *big.Int) []*big.Int {
	if len(targets) == 0 || total == nil || total.Sign() <= 0 ||
		total.Cmp(big.NewInt(int64(len(targets)))) < 0 {
		return nil
	}
	capacity := new(big.Int)
	for _, target := range targets {
		if target == nil || target.Sign() <= 0 {
			return nil
		}
		capacity.Add(capacity, target)
	}
	if total.Cmp(capacity) > 0 {
		return nil
	}
	remaining := new(big.Int).Sub(total, big.NewInt(int64(len(targets))))
	remainingCapacity := new(big.Int).Sub(capacity, big.NewInt(int64(len(targets))))
	minimums := make([]*big.Int, len(targets))
	for index, target := range targets {
		available := new(big.Int).Sub(target, big.NewInt(1))
		allocation := new(big.Int)
		if index == len(targets)-1 {
			allocation.Set(remaining)
		} else if remainingCapacity.Sign() > 0 {
			allocation.Mul(remaining, available)
			allocation.Div(allocation, remainingCapacity)
		}
		if allocation.Cmp(available) > 0 {
			return nil
		}
		remaining.Sub(remaining, allocation)
		minimums[index] = allocation.Add(allocation, big.NewInt(1))
		remainingCapacity.Sub(remainingCapacity, available)
	}
	if remaining.Sign() != 0 {
		return nil
	}
	return minimums
}
