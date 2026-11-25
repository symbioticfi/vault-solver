package uniswapx

import (
	"strings"
	"sync"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
)

// quoteStrategyDeclines observes existing terminal planning decisions independently of log level.
// One request owns the collector. The mutex also permits a strategy to trace parallel candidates;
// DecideQuote must join its workers before returning. Different candidate failures deliberately
// retain the generic strategy metric reason (the last attempted route is not a reason for every
// route). Diagnostic detail keeps a bounded sample.
type quoteStrategyDeclines struct {
	mu       sync.Mutex
	value    quoteDeclineReason
	details  strings.Builder
	examples int
	omitted  bool
}

const (
	maxQuoteDeclineExamples = 8
	maxQuoteDeclineBytes    = 2048
	quoteDeclineOmitted     = "; more detail omitted"
)

func (d *quoteStrategyDeclines) trace(next strategies.DecisionTrace) strategies.DecisionTrace {
	return func(message string, fields ...any) {
		if message == "liquidlane quote declined" {
			reason := quoteDeclineStrategy
			if len(fields) >= 2 {
				key, _ := fields[0].(string)
				if value, ok := fields[1].(string); ok && key == "reason" {
					reason = strategyQuoteDeclineReason(value)
				}
			}
			d.mu.Lock()
			d.addDetail(fields)
			if d.value == "" {
				d.value = reason
			} else if d.value != reason {
				d.value = quoteDeclineStrategy
			}
			d.mu.Unlock()
		}
		next.Log(message, fields...)
	}
}

func (d *quoteStrategyDeclines) reason() quoteDeclineReason {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.value == "" {
		return quoteDeclineStrategy
	}
	return d.value
}

// addDetail runs under mu and reserves space for the omission marker.
func (d *quoteStrategyDeclines) addDetail(fields []any) {
	remaining := maxQuoteDeclineBytes - len(quoteDeclineOmitted) - d.details.Len()
	if d.examples > 0 {
		remaining -= len("; ")
	}
	if d.omitted || d.examples >= maxQuoteDeclineExamples || remaining <= 0 {
		d.omitted = true
		return
	}
	detail := strategies.FormatFields(fields)
	if len(detail) > remaining {
		detail = strings.ToValidUTF8(detail[:remaining], "")
		d.omitted = true
	}
	if d.examples > 0 {
		d.details.WriteString("; ")
	}
	d.details.WriteString(detail)
	d.examples++
}

func (d *quoteStrategyDeclines) detail() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.examples == 0 {
		return "strategy returned no quote without recording a decline"
	}
	if d.omitted {
		return d.details.String() + quoteDeclineOmitted
	}
	return d.details.String()
}

// Only these local planner decisions have established semantics. Never classify arbitrary
// error text, webhook responses, route identifiers or request data as metric labels.
func strategyQuoteDeclineReason(value string) quoteDeclineReason {
	switch value {
	case "no-matching-routes":
		return quoteDeclineNoMatchingRoutes
	case "no-candidates":
		return quoteDeclineNoCandidates
	case "amount-below-minimum":
		return quoteDeclineAmountBelowMinimum
	case "insufficient-capacity":
		return quoteDeclineInsufficientCapacity
	case "no-allocation":
		return quoteDeclineNoAllocation
	case "gas-exceeds-output":
		return quoteDeclineGasExceedsOutput
	case "minimum-input-cannot-cover-output":
		return quoteDeclineMinimumInput
	default:
		return quoteDeclineStrategy
	}
}

func boundedQuoteRejectionReason(value string) string {
	switch quoteDeclineReason(value) {
	case quoteDeclineBlocked, quoteDeclineMissingIDs,
		quoteDeclineUnsupportedType, quoteDeclineInvalidOutputs, quoteDeclineUnsupportedProtocol,
		quoteDeclineUnsupportedChain, quoteDeclineInvalidAddress, quoteDeclinePairOutOfScope,
		quoteDeclineInvalidAmount, quoteDeclineQuoteStateUnavailable, quoteDeclineStrategy,
		quoteDeclineNoMatchingRoutes, quoteDeclineNoCandidates, quoteDeclineAmountBelowMinimum,
		quoteDeclineInsufficientCapacity, quoteDeclineNoAllocation, quoteDeclineGasExceedsOutput,
		quoteDeclineMinimumInput, quoteDeclineStateChanged:
		return strings.ReplaceAll(value, "-", "_")
	}
	switch value {
	case "read-body", "body-too-large", "invalid-json", "trailing-json",
		"invalid-breaker-notification", "strategy_error", "internal_error", "response_write_error":
		return strings.ReplaceAll(value, "-", "_")
	default:
		return "unknown"
	}
}
