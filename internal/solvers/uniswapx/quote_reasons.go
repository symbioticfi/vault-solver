package uniswapx

import (
	"fmt"
	"strings"
	"sync"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
)

// quoteStrategyDeclines observes existing terminal planning decisions independently of log level.
// One request owns the collector. The mutex also permits a strategy to trace parallel candidates;
// DecideQuote must join its workers before returning. Different candidate failures deliberately
// retain the generic strategy metric reason (the last attempted route is not a reason for every
// route); the logged detail lists every decline instead.
type quoteStrategyDeclines struct {
	mu      sync.Mutex
	value   quoteDeclineReason
	details []string
}

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
			d.details = append(d.details, formatDecline(fields))
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

// detail lists every recorded decline for the quote log line.
func (d *quoteStrategyDeclines) detail() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.details) == 0 {
		return "strategy returned no quote without recording a decline"
	}
	return strings.Join(d.details, "; ")
}

func formatDecline(fields []any) string {
	var b strings.Builder
	for i := 0; i+1 < len(fields); i += 2 {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%v=%v", fields[i], fields[i+1])
	}
	return b.String()
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
