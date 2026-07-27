package uniswapx

import (
	"strings"
	"testing"

	"github.com/go-logr/logr/funcr"
)

func TestDecisionTraceRequiresDebugVerbosityAndKeepsCorrelation(t *testing.T) {
	var logs []string
	solver := &Solver{
		log: funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{}),
	}
	if trace := solver.decisionTrace("requestId", "request-1"); trace != nil {
		t.Fatal("decision trace enabled without debug verbosity")
	}

	solver.log = funcr.NewJSON(
		func(entry string) { logs = append(logs, entry) },
		funcr.Options{Verbosity: 1},
	)
	trace := solver.decisionTrace("requestId", "request-1", "quoteId", "quote-1")
	if trace == nil {
		t.Fatal("decision trace disabled at debug verbosity")
	}
	trace.Log("liquidlane quote declined", "reason", "gas-exceeds-output")

	if len(logs) != 1 ||
		!strings.Contains(logs[0], `"requestId":"request-1"`) ||
		!strings.Contains(logs[0], `"quoteId":"quote-1"`) ||
		!strings.Contains(logs[0], `"reason":"gas-exceeds-output"`) {
		t.Fatalf("logs = %v", logs)
	}
}
