package lifi

import (
	"strings"
	"testing"

	"github.com/go-logr/logr/funcr"
)

func TestDecisionTraceRequiresDebugVerbosityAndKeepsOrderCorrelation(t *testing.T) {
	var logs []string
	solver := &Solver{
		log: funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{}),
	}
	if trace := solver.decisionTrace("orderId", "order-1"); trace != nil {
		t.Fatal("decision trace enabled without debug verbosity")
	}

	solver.log = funcr.NewJSON(
		func(entry string) { logs = append(logs, entry) },
		funcr.Options{Verbosity: 1},
	)
	trace := solver.decisionTrace(
		"orderId", "order-1",
		"onChainOrderId", "0x1234",
		"quoteId", "quote-1",
	)
	if trace == nil {
		t.Fatal("decision trace disabled at debug verbosity")
	}
	trace.Decline("fill", "insufficient-capacity")

	if len(logs) != 1 ||
		!strings.Contains(logs[0], `"orderId":"order-1"`) ||
		!strings.Contains(logs[0], `"onChainOrderId":"0x1234"`) ||
		!strings.Contains(logs[0], `"quoteId":"quote-1"`) ||
		!strings.Contains(logs[0], `"reason":"insufficient-capacity"`) {
		t.Fatalf("logs = %v", logs)
	}
}
