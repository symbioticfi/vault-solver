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

	FillOutcomeSuccess     = "success"
	FillOutcomeFailure     = "failure"
	FillOutcomeNotAdmitted = "not_admitted"
)

// FillWorkflowSpec declares the shared successful-fill signals used by LiquidLane solvers.
func FillWorkflowSpec() observability.WorkflowSpec {
	return observability.WorkflowSpec{
		Events: []observability.WorkflowEventSpec{{
			Event: fillWorkflowEvent, Outcomes: []string{FillOutcomeSuccess},
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

func (m *FillMetrics) Observe(receipt *types.Receipt, tokenIn common.Address, amountIn *big.Int,
	tokenOut common.Address, amountOut, surplus *big.Int) {
	if m == nil || receipt == nil || receipt.Status != types.ReceiptStatusSuccessful {
		return
	}
	m.ObserveOutcome(FillOutcomeSuccess)
	for _, entry := range []struct {
		token  common.Address
		kind   string
		amount *big.Int
	}{
		{tokenIn, FillAmountInput, amountIn}, {tokenOut, FillAmountOutput, amountOut}, {tokenOut, FillAmountPlannedSurplus, surplus},
	} {
		if entry.token != (common.Address{}) {
			m.workflow.AddAmount(fillWorkflowEvent, entry.token.Hex(), entry.kind, entry.amount)
		}
	}
}

// ObserveOutcome records one bounded fill outcome without adding successful receipt amounts.
func (m *FillMetrics) ObserveOutcome(outcome string) {
	if m != nil {
		m.workflow.ObserveEventAt(fillWorkflowEvent, outcome, 1, m.now())
	}
}

// PlannedSurplus returns the positive difference between planned routed and required output.
func PlannedSurplus(routed, required *big.Int) *big.Int {
	surplus := new(big.Int)
	if routed != nil && required != nil {
		surplus.Sub(routed, required)
	}
	if surplus.Sign() < 0 {
		surplus.SetInt64(0)
	}
	return surplus
}
