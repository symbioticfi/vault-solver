package planning

import (
	"strings"
	"testing"

	"github.com/go-logr/logr/funcr"
)

func TestNewDecisionTrace(t *testing.T) {
	var logs []string
	log := funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{})
	if trace := NewDecisionTrace(log, "requestId", "request-1"); trace != nil {
		t.Fatal("decision trace enabled without debug verbosity")
	}

	log = funcr.NewJSON(
		func(entry string) { logs = append(logs, entry) },
		funcr.Options{Verbosity: 1},
	)
	trace := NewDecisionTrace(log, "requestId", "request-1", "quoteId", "quote-1")
	if trace == nil {
		t.Fatal("decision trace disabled at debug verbosity")
	}
	trace.Decline("fill", "insufficient-capacity")

	if len(logs) != 1 ||
		!strings.Contains(logs[0], `"requestId":"request-1"`) ||
		!strings.Contains(logs[0], `"quoteId":"quote-1"`) ||
		!strings.Contains(logs[0], `"reason":"insufficient-capacity"`) {
		t.Fatalf("logs = %v", logs)
	}
}
