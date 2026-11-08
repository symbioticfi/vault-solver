package liquidlane

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/core/types"

	txmanager "github.com/symbioticfi/merkle-executor/sdk/transaction"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

const (
	fillWorkflowEvent = "fill"

	FillOutcomeSuccess     = "success"
	FillOutcomeFailure     = "failure"
	FillOutcomeNotAdmitted = "not_admitted"
	// FillOutcomeObsolete is a fill the solver retired because the order was settled elsewhere
	// (filled or cancelled on-chain) before its transaction landed.
	FillOutcomeObsolete = "obsolete"
)

// FillWorkflowSpec declares the shared successful-fill signals used by LiquidLane solvers.
func FillWorkflowSpec() observability.WorkflowSpec {
	return observability.WorkflowSpec{
		Events: []observability.WorkflowEventSpec{{
			Event: fillWorkflowEvent, Outcomes: []string{FillOutcomeSuccess},
		}},
	}
}

// FillMetrics records successful LiquidLane fill count and freshness without additional chain reads.
type FillMetrics struct {
	workflow *observability.WorkflowMetrics
	now      func() time.Time
}

// NewFillMetrics binds the shared fill path to its owning solver's workflow metrics.
func NewFillMetrics(workflow *observability.WorkflowMetrics) *FillMetrics {
	return &FillMetrics{workflow: workflow, now: time.Now}
}

// ReceiptObserver records the manager's normal and late successful receipt telemetry.
// The callback only updates metrics; it retains no order or solver state and performs no I/O.
func (m *FillMetrics) ReceiptObserver(observers ...func(context.Context, txmanager.Result)) func(context.Context, txmanager.Result) {
	if m == nil {
		return nil
	}
	return func(ctx context.Context, result txmanager.Result) {
		m.Observe(result.Receipt)
		for _, observe := range observers {
			if observe != nil {
				observe(ctx, result)
			}
		}
	}
}

func (m *FillMetrics) Observe(receipt *types.Receipt) {
	if m == nil || receipt == nil || receipt.Status != types.ReceiptStatusSuccessful {
		return
	}
	m.ObserveOutcome(FillOutcomeSuccess)
}

// ObserveOutcome records one bounded fill outcome.
func (m *FillMetrics) ObserveOutcome(outcome string) {
	if m != nil {
		m.workflow.ObserveEventAt(fillWorkflowEvent, outcome, 1, m.now())
	}
}

// PlannedSurplus returns the positive difference between planned routed and required output.
func PlannedSurplus(routed, required *big.Int) *big.Int {
	if routed == nil || required == nil || routed.Cmp(required) <= 0 {
		return new(big.Int)
	}
	return new(big.Int).Sub(routed, required)
}
