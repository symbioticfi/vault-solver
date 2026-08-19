package liquidlane

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

const (
	FillAmountInput          = "input"
	FillAmountOutput         = "output"
	FillAmountPlannedSurplus = "planned_surplus"
	fillWorkflowEvent        = "fill"
	fillWorkflowOutcome      = "success"
)

// FillWorkflowSpec declares the shared successful-fill signals used by LiquidLane solvers.
func FillWorkflowSpec() observability.WorkflowSpec {
	return observability.WorkflowSpec{
		Events: []observability.WorkflowEventSpec{{
			Event: fillWorkflowEvent, Outcomes: []string{fillWorkflowOutcome},
		}},
		Amounts: []observability.WorkflowAmountSpec{{
			Event: fillWorkflowEvent,
			Kinds: []string{FillAmountInput, FillAmountOutput, FillAmountPlannedSurplus},
		}},
	}
}

// FillMetrics records successful LiquidLane fill count, freshness, and amounts without additional
// chain reads.
type FillMetrics struct {
	workflow *observability.WorkflowMetrics
	now      func() time.Time
}

// NewFillMetrics binds the shared fill path to its owning solver's workflow metrics.
func NewFillMetrics(workflow *observability.WorkflowMetrics) *FillMetrics {
	return &FillMetrics{workflow: workflow, now: time.Now}
}

func (m *FillMetrics) Observe(
	receipt *types.Receipt,
	tokenIn common.Address,
	amountIn *big.Int,
	tokenOut common.Address,
	amountOut *big.Int,
	plannedSurplus *big.Int,
) {
	if m == nil || receipt == nil || receipt.Status != types.ReceiptStatusSuccessful {
		return
	}
	m.workflow.ObserveEventAt(fillWorkflowEvent, fillWorkflowOutcome, 1, m.now())
	m.add(tokenIn, FillAmountInput, amountIn)
	m.add(tokenOut, FillAmountOutput, amountOut)
	m.add(tokenOut, FillAmountPlannedSurplus, plannedSurplus)
}

func (m *FillMetrics) add(token common.Address, kind string, amount *big.Int) {
	if token != (common.Address{}) {
		m.workflow.AddAmount(fillWorkflowEvent, token.Hex(), kind, amount)
	}
}

// PlannedSurplus returns the positive difference between planned routed and required output.
func PlannedSurplus(routed, required *big.Int) *big.Int {
	if routed == nil || required == nil || routed.Cmp(required) <= 0 {
		return new(big.Int)
	}
	return new(big.Int).Sub(routed, required)
}
