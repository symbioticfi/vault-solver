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

	Candidates []liquidlane.QuoteCandidate
	MaxRoutes  int
	MinInput   *big.Int

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
	routes := newRouteBook(task.Candidates)
	var solution *QuoteSolution
	if task.ExactInput != nil {
		solution = solveExactInputQuote(task, routes, task.ExactInput)
	} else {
		solution = solveExactOutputQuote(task, routes)
	}
	traceQuoteSolution(task.Trace, mode, solution)
	return solution, nil
}

func solveExactInputQuote(
	task QuoteTask,
	book routeBook,
	amountIn *big.Int,
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
	plan := book.allocateInput(
		amountIn,
		task.MaxRoutes,
		task.InputPolicy == RejectUncoveredInput,
	)
	if len(plan.Allocations) == 0 {
		reason := "no-allocation"
		if len(book.routes) > 0 {
			reason = insufficientCapacityReason
		}
		task.Trace.Decline(
			"quote", reason,
			"amountIn", amountIn.String(),
			"routes", len(book.routes),
		)
		return nil
	}
	if plan.Remaining.Sign() != 0 {
		if task.InputPolicy == RejectUncoveredInput {
			task.Trace.Decline(
				"quote", insufficientCapacityReason,
				"amountIn", amountIn.String(),
				"allocatedAmountIn", plan.TotalAmountIn.String(),
				"remainingAmountIn", plan.Remaining.String(),
				"routes", len(plan.Allocations),
			)
			return nil
		}
		last := &plan.Allocations[len(plan.Allocations)-1]
		last.AmountIn.Add(last.AmountIn, plan.Remaining)
		plan.Remaining.SetInt64(0)
	}

	grossAmountOut := liquidlane.CloneBig(plan.TotalAmountOut)
	gasCost := new(big.Int)
	amountOut := scaleDown(grossAmountOut, bpsDenominator-task.OutputBufferBps)
	if task.GasPricing != nil {
		gasCost = task.GasPricing.Cost(quoteGasLegs(plan.Allocations))
		amountOut.Sub(amountOut, gasCost)
	}
	if amountOut.Sign() <= 0 {
		task.Trace.Decline(
			"quote", "gas-exceeds-output",
			"amountIn", amountIn.String(),
			"grossAmountOut", grossAmountOut.String(),
			"bufferedAmountOut", scaleDown(grossAmountOut, bpsDenominator-task.OutputBufferBps).String(),
			"gasCost", gasCost.String(),
			"netAmountOut", amountOut.String(),
		)
		return nil
	}
	return &QuoteSolution{
		AmountIn: liquidlane.CloneBig(amountIn), GrossAmountOut: grossAmountOut,
		GasCost: gasCost, AmountOut: amountOut,
		Allocations: cloneAllocations(plan.Allocations),
	}
}

func solveExactOutputQuote(task QuoteTask, book routeBook) *QuoteSolution {
	if task.ExactOutput == nil || task.ExactOutput.Sign() <= 0 {
		task.Trace.Decline("quote", "invalid-exact-output")
		return nil
	}
	solution := solveExactOutputQuoteGreedy(task, book)
	if solution == nil {
		return nil
	}
	if task.MinInput != nil && solution.AmountIn.Cmp(task.MinInput) < 0 {
		task.Trace.Log(
			"liquidlane exact-output minimum input applied",
			"calculatedAmountIn", solution.AmountIn.String(),
			"minInput", task.MinInput.String(),
		)
		minimumQuote := solveExactInputQuote(task, book, task.MinInput)
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

func solveExactOutputQuoteGreedy(task QuoteTask, book routeBook) *QuoteSolution {
	targetGross := grossOutputForNet(task.ExactOutput, new(big.Int), task.OutputBufferBps)
	for {
		plan := book.allocateOutput(targetGross, task.MaxRoutes)
		if len(plan.Allocations) == 0 || plan.Remaining.Sign() != 0 {
			task.Trace.Decline(
				"quote", insufficientCapacityReason,
				"mode", "exact-output",
				"targetGrossAmountOut", targetGross.String(),
				"allocatedAmountOut", plan.TotalAmountOut.String(),
				"remainingAmountOut", plan.Remaining.String(),
				"routes", len(plan.Allocations),
			)
			return nil
		}
		gasCost := new(big.Int)
		if task.GasPricing != nil {
			gasCost = task.GasPricing.Cost(quoteGasLegs(plan.Allocations))
		}
		netOutput := scaleDown(plan.TotalAmountOut, bpsDenominator-task.OutputBufferBps)
		netOutput.Sub(netOutput, gasCost)
		if netOutput.Cmp(task.ExactOutput) >= 0 {
			return &QuoteSolution{
				AmountIn: plan.TotalAmountIn, GrossAmountOut: liquidlane.CloneBig(plan.TotalAmountOut),
				GasCost: gasCost, AmountOut: liquidlane.CloneBig(task.ExactOutput),
				Allocations: cloneAllocations(plan.Allocations),
			}
		}
		requiredGross := grossOutputForNet(task.ExactOutput, gasCost, task.OutputBufferBps)
		if requiredGross.Cmp(targetGross) <= 0 {
			task.Trace.Decline(
				"quote", "gas-exceeds-output",
				"mode", "exact-output",
				"exactOutput", task.ExactOutput.String(),
				"grossAmountOut", plan.TotalAmountOut.String(),
				"gasCost", gasCost.String(),
				"netAmountOut", netOutput.String(),
			)
			return nil
		}
		targetGross = requiredGross
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
