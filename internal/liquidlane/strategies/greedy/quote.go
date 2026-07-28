package greedy

import (
	"math/big"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidstrategies "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
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

	Candidates []liquidlane.QuoteCandidate
	MaxRoutes  int
	MinInput   *big.Int

	OutputBufferBps int
	InputPolicy     UncoveredInputPolicy
	GasPricing      *liquidstrategies.GasPricing
	Trace           liquidstrategies.DecisionTrace
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

// SolveQuote prices one exact-input or exact-output LiquidLane task.
func SolveQuote(task QuoteTask) (*QuoteSolution, error) {
	mode := "exact-output"
	if task.ExactInput != nil {
		mode = "exact-input"
	}
	if (task.ExactInput == nil) == (task.ExactOutput == nil) {
		return nil, errors.New("exactly one quote amount must be set")
	}
	if task.MaxRoutes <= 0 || len(task.Candidates) == 0 {
		task.Trace.Decline(
			"quote", "no-candidates",
			"candidates", len(task.Candidates),
			"maxRoutes", task.MaxRoutes,
		)
		return nil, nil
	}
	if task.OutputBufferBps < 0 || task.OutputBufferBps >= bpsDenominator {
		return nil, errors.Errorf("outputBufferBps: must be in [0,%d)", bpsDenominator)
	}
	if task.InputPolicy != RejectUncoveredInput && task.InputPolicy != AbsorbUncoveredInput {
		return nil, errors.New("invalid uncovered input policy")
	}
	if task.ExactOutput != nil && task.InputPolicy == AbsorbUncoveredInput {
		return nil, errors.New("exact-output quote cannot absorb uncovered input")
	}
	if task.MinInput != nil && task.MinInput.Sign() < 0 {
		return nil, errors.New("minInput: must be non-negative")
	}
	routes := newAllocator(task.Candidates)
	var solution *QuoteSolution
	if task.ExactInput != nil {
		solution = solveExactInputQuote(task, routes, task.ExactInput, task.MaxRoutes)
	} else {
		solution = solveExactOutputQuote(task, routes)
	}
	traceQuoteSolution(task.Trace, mode, solution)
	return solution, nil
}

func solveExactInputQuote(
	task QuoteTask,
	allocator allocator,
	amountIn *big.Int,
	maxRoutes int,
) *QuoteSolution {
	if amountIn == nil || amountIn.Sign() <= 0 ||
		(task.MinInput != nil && amountIn.Cmp(task.MinInput) < 0) {
		task.Trace.Decline(
			"quote", "amount-below-minimum",
			"amountIn", bigString(amountIn),
			"minInput", bigString(task.MinInput),
		)
		return nil
	}
	allocation := allocator.allocateExactInputWithPolicy(
		amountIn,
		maxRoutes,
		task.InputPolicy == RejectUncoveredInput,
	)
	if len(allocation.Allocations) == 0 {
		reason := "no-allocation"
		if len(allocator.sources) > 0 {
			reason = insufficientCapacityReason
		}
		task.Trace.Decline(
			"quote", reason,
			"amountIn", amountIn.String(),
			"routes", len(allocator.sources),
		)
		return nil
	}
	if allocation.Remaining.Sign() != 0 {
		if task.InputPolicy == RejectUncoveredInput {
			task.Trace.Decline(
				"quote", insufficientCapacityReason,
				"amountIn", amountIn.String(),
				"allocatedAmountIn", allocation.TotalAmountIn.String(),
				"remainingAmountIn", allocation.Remaining.String(),
				"routes", len(allocation.Allocations),
			)
			return nil
		}
		last := &allocation.Allocations[len(allocation.Allocations)-1]
		last.AmountIn.Add(last.AmountIn, allocation.Remaining)
		allocation.Remaining.SetInt64(0)
	}

	grossAmountOut := liquidlane.CloneBig(allocation.TotalAmountOut)
	gasCost := new(big.Int)
	amountOut := applyBpsDown(grossAmountOut, bpsDenominator-task.OutputBufferBps)
	if task.GasPricing != nil {
		gasCost = task.GasPricing.Cost(quoteGasLegs(allocation.Allocations))
		amountOut.Sub(amountOut, gasCost)
	}
	if amountOut.Sign() <= 0 {
		task.Trace.Decline(
			"quote", "gas-exceeds-output",
			"amountIn", amountIn.String(),
			"grossAmountOut", grossAmountOut.String(),
			"bufferedAmountOut", applyBpsDown(grossAmountOut, bpsDenominator-task.OutputBufferBps).String(),
			"gasCost", gasCost.String(),
			"netAmountOut", amountOut.String(),
		)
		return nil
	}
	return &QuoteSolution{
		AmountIn: liquidlane.CloneBig(amountIn), GrossAmountOut: grossAmountOut,
		GasCost: gasCost, AmountOut: amountOut,
		Allocations: cloneAllocations(allocation.Allocations),
	}
}

func solveExactOutputQuote(task QuoteTask, allocator allocator) *QuoteSolution {
	if task.ExactOutput == nil || task.ExactOutput.Sign() <= 0 {
		task.Trace.Decline("quote", "invalid-exact-output")
		return nil
	}
	solution := solveExactOutputQuoteGreedy(task, allocator)
	if solution == nil {
		return nil
	}
	if task.MinInput != nil && solution.AmountIn.Cmp(task.MinInput) < 0 {
		task.Trace.Log(
			"liquidlane exact-output minimum input applied",
			"calculatedAmountIn", solution.AmountIn.String(),
			"minInput", task.MinInput.String(),
		)
		minimumQuote := solveExactInputQuote(task, allocator, task.MinInput, task.MaxRoutes)
		if minimumQuote == nil || minimumQuote.AmountOut.Cmp(task.ExactOutput) < 0 {
			task.Trace.Decline(
				"quote", "minimum-input-cannot-cover-output",
				"exactOutput", task.ExactOutput.String(),
				"minInput", task.MinInput.String(),
			)
			return nil
		}
		minimumQuote.AmountOut = liquidlane.CloneBig(task.ExactOutput)
		return minimumQuote
	}
	return solution
}

func solveExactOutputQuoteGreedy(task QuoteTask, allocator allocator) *QuoteSolution {
	targetGross := grossOutputForNet(task.ExactOutput, new(big.Int), task.OutputBufferBps)
	for targetGross.Sign() > 0 {
		allocation := allocator.allocateExactOutput(targetGross, task.MaxRoutes)
		if len(allocation.Allocations) == 0 || allocation.Remaining.Sign() != 0 {
			task.Trace.Decline(
				"quote", insufficientCapacityReason,
				"mode", "exact-output",
				"targetGrossAmountOut", targetGross.String(),
				"allocatedAmountOut", allocation.TotalAmountOut.String(),
				"remainingAmountOut", allocation.Remaining.String(),
				"routes", len(allocation.Allocations),
			)
			return nil
		}
		gasCost := new(big.Int)
		if task.GasPricing != nil {
			gasCost = task.GasPricing.Cost(quoteGasLegs(allocation.Allocations))
		}
		netOutput := applyBpsDown(allocation.TotalAmountOut, bpsDenominator-task.OutputBufferBps)
		netOutput.Sub(netOutput, gasCost)
		if netOutput.Cmp(task.ExactOutput) >= 0 {
			return &QuoteSolution{
				AmountIn: allocation.TotalAmountIn, GrossAmountOut: liquidlane.CloneBig(allocation.TotalAmountOut),
				GasCost: gasCost, AmountOut: liquidlane.CloneBig(task.ExactOutput),
				Allocations: cloneAllocations(allocation.Allocations),
			}
		}
		requiredGross := grossOutputForNet(task.ExactOutput, gasCost, task.OutputBufferBps)
		if requiredGross.Cmp(targetGross) <= 0 {
			task.Trace.Decline(
				"quote", "gas-exceeds-output",
				"mode", "exact-output",
				"exactOutput", task.ExactOutput.String(),
				"grossAmountOut", allocation.TotalAmountOut.String(),
				"gasCost", gasCost.String(),
				"netAmountOut", netOutput.String(),
			)
			return nil
		}
		targetGross = requiredGross
	}
	return nil
}

func grossOutputForNet(netOutput, gasCost *big.Int, outputBufferBps int) *big.Int {
	target := new(big.Int).Add(netOutput, gasCost)
	return liquidlane.MulDivUp(
		target,
		big.NewInt(bpsDenominator),
		big.NewInt(int64(bpsDenominator-outputBufferBps)),
	)
}

func quoteGasLegs(allocations []Allocation) []liquidstrategies.GasLeg {
	legs := make([]liquidstrategies.GasLeg, len(allocations))
	for index, allocation := range allocations {
		legs[index] = liquidstrategies.GasLeg{
			Route: allocation.Candidate.Route, AmountOut: allocation.AmountOut,
			Private: allocation.Candidate.DiscountID != nil,
		}
	}
	return legs
}

func cloneAllocations(allocations []Allocation) []Allocation {
	out := make([]Allocation, len(allocations))
	for index, allocation := range allocations {
		out[index] = Allocation{
			Candidate: allocation.Candidate,
			AmountIn:  liquidlane.CloneBig(allocation.AmountIn),
			AmountOut: liquidlane.CloneBig(allocation.AmountOut),
		}
	}
	return out
}

func traceQuoteSolution(trace liquidstrategies.DecisionTrace, mode string, solution *QuoteSolution) {
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
