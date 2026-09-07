package planning

import (
	"math/big"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// UncoveredInputPolicy describes whether a LiquidLane quote must source output
// for every input unit or may absorb excess input as price impact.
type UncoveredInputPolicy uint8

const (
	RejectUncoveredInput UncoveredInputPolicy = iota
	AbsorbUncoveredInput
)

const insufficientCapacityReason = "insufficient-capacity"

// QuoteTask is a protocol-neutral LiquidLane pricing problem. Exactly one of
// ExactInput and ExactOutput must be set.
type QuoteTask struct {
	ExactInput  *big.Int
	ExactOutput *big.Int

	MaxRoutes int
	MinInput  *big.Int

	OutputBufferBps int
	InputPolicy     UncoveredInputPolicy
	GasPricing      *GasPricing
	Trace           DecisionTrace
}

// QuoteSolution is the priced amount pair and the LiquidLane allocation that
// produced it. Protocol adapters may omit the allocation from their wire reply.
type QuoteSolution struct {
	AmountIn       *big.Int
	GrossAmountOut *big.Int
	GasCost        *big.Int
	AmountOut      *big.Int
	Allocations    []Allocation
}

// Solve uses the same allocation and net-output calculation for both quote sides.
// Exact output increases the gross target until the selected routes also pay for their gas.
func (a QuotePool) Solve(task QuoteTask) (*QuoteSolution, error) {
	if err := task.validate(); err != nil {
		return nil, err
	}
	if task.MaxRoutes <= 0 || len(a.sources) == 0 {
		task.Trace.Decline("quote", "no-candidates", "candidates", len(a.sources), "maxRoutes", task.MaxRoutes)
		return nil, nil
	}
	mode := "exact-input"
	var result *QuoteSolution
	if task.ExactInput != nil {
		result = task.priceInput(a, task.ExactInput)
	} else {
		mode = "exact-output"
		result = task.priceOutput(a)
	}
	traceQuoteSolution(task.Trace, mode, result)
	return result, nil
}

func (task QuoteTask) validate() error {
	switch {
	case (task.ExactInput == nil) == (task.ExactOutput == nil):
		return errors.New("exactly one quote amount must be set")
	case task.OutputBufferBps < 0 || task.OutputBufferBps >= bpsDenominator:
		return errors.Errorf("outputBufferBps: must be in [0,%d)", bpsDenominator)
	case task.InputPolicy != RejectUncoveredInput && task.InputPolicy != AbsorbUncoveredInput:
		return errors.New("invalid uncovered input policy")
	case task.ExactOutput != nil && task.InputPolicy == AbsorbUncoveredInput:
		return errors.New("exact-output quote cannot absorb uncovered input")
	case task.MinInput != nil && task.MinInput.Sign() < 0:
		return errors.New("minInput: must be non-negative")
	default:
		return nil
	}
}

func (task QuoteTask) priceInput(a QuotePool, amount *big.Int) *QuoteSolution {
	if amount.Sign() <= 0 || (task.MinInput != nil && amount.Cmp(task.MinInput) < 0) {
		task.Trace.Decline("quote", "amount-below-minimum", "amountIn", bigString(amount), "minInput", bigString(task.MinInput))
		return nil
	}
	allocation := a.allocateExactInputWithPolicy(amount, task.MaxRoutes, task.InputPolicy == RejectUncoveredInput)
	if len(allocation.Allocations) == 0 || (allocation.Remaining.Sign() > 0 && task.InputPolicy == RejectUncoveredInput) {
		task.Trace.Decline("quote", insufficientCapacityReason, "amountIn", amount.String(), "remainingAmountIn", allocation.Remaining.String())
		return nil
	}
	// Absorbed input belongs to the last selected adapter but creates no extra output or capacity.
	last := &allocation.Allocations[len(allocation.Allocations)-1]
	last.AmountIn.Add(last.AmountIn, allocation.Remaining)
	allocation.TotalAmountIn.Set(amount)
	result := task.price(allocation)
	if result.AmountOut.Sign() <= 0 {
		task.Trace.Decline("quote", "gas-exceeds-output", "grossAmountOut", result.GrossAmountOut.String(), "gasCost", result.GasCost.String())
		return nil
	}
	return result
}

func (task QuoteTask) priceOutput(a QuotePool) *QuoteSolution {
	if task.ExactOutput.Sign() <= 0 {
		task.Trace.Decline("quote", "invalid-exact-output")
		return nil
	}
	target := grossOutputForNet(task.ExactOutput, new(big.Int), task.OutputBufferBps)
	for {
		allocation := a.allocateExactOutput(target, task.MaxRoutes)
		if len(allocation.Allocations) == 0 || allocation.Remaining.Sign() != 0 {
			task.Trace.Decline("quote", insufficientCapacityReason, "targetGrossAmountOut", target.String(), "remainingAmountOut", allocation.Remaining.String())
			return nil
		}
		result := task.price(allocation)
		if result.AmountOut.Cmp(task.ExactOutput) >= 0 {
			if task.MinInput != nil && result.AmountIn.Cmp(task.MinInput) < 0 {
				result = task.priceInput(a, task.MinInput)
				if result == nil || result.AmountOut.Cmp(task.ExactOutput) < 0 {
					task.Trace.Decline("quote", "minimum-input-cannot-cover-output")
					return nil
				}
			}
			result.AmountOut.Set(task.ExactOutput)
			return result
		}
		next := grossOutputForNet(task.ExactOutput, result.GasCost, task.OutputBufferBps)
		if next.Cmp(target) <= 0 {
			task.Trace.Decline("quote", "gas-exceeds-output", "gasCost", result.GasCost.String())
			return nil
		}
		target = next
	}
}

func (task QuoteTask) price(allocation allocationResult) *QuoteSolution {
	gas := new(big.Int)
	if task.GasPricing != nil {
		gas = task.GasPricing.Cost(quoteGasLegs(allocation.Allocations))
	}
	net := applyBpsDown(allocation.TotalAmountOut, bpsDenominator-task.OutputBufferBps)
	net.Sub(net, gas)
	return &QuoteSolution{
		AmountIn: allocation.TotalAmountIn, GrossAmountOut: allocation.TotalAmountOut,
		GasCost: gas, AmountOut: net, Allocations: allocation.Allocations,
	}
}

func grossOutputForNet(netOutput, gasCost *big.Int, outputBufferBps int) *big.Int {
	target := new(big.Int).Add(netOutput, gasCost)
	return liquidlane.MulDivUp(
		target,
		big.NewInt(bpsDenominator),
		big.NewInt(int64(bpsDenominator-outputBufferBps)),
	)
}

func quoteGasLegs(allocations []Allocation) []GasLeg {
	legs := make([]GasLeg, len(allocations))
	for index, allocation := range allocations {
		legs[index] = GasLeg{
			Route: allocation.Candidate.Route, AmountOut: allocation.AmountOut,
			Private: allocation.Candidate.DiscountID != nil,
		}
	}
	return legs
}

func traceQuoteSolution(trace DecisionTrace, mode string, solution *QuoteSolution) {
	if trace == nil || solution == nil {
		return
	}
	trace.Log(
		"liquidlane quote selected",
		"mode", mode,
		"amountIn", solution.AmountIn.String(),
		"grossAmountOut", solution.GrossAmountOut.String(),
		"gasCost", solution.GasCost.String(),
		"amountOut", solution.AmountOut.String(),
		"routes", len(solution.Allocations),
	)
	for index, allocation := range solution.Allocations {
		trace.Log(
			"liquidlane quote leg selected",
			"leg", index,
			"candidateId", allocation.Candidate.ID,
			"routeId", allocation.Candidate.Route.ID,
			"capacityId", liquidlane.RouteCapacityID(allocation.Candidate.Route),
			"adapter", allocation.Candidate.Route.Adapter.Hex(),
			"amountIn", bigString(allocation.AmountIn),
			"amountOut", bigString(allocation.AmountOut),
			"private", allocation.Candidate.DiscountID != nil,
		)
	}
}

func bigString(value *big.Int) string {
	if value == nil {
		return "0"
	}
	return value.String()
}
