package strategies

import "testing"

func TestRecordDeclinesKeepsLatestDeclineAndForwards(t *testing.T) {
	var forwarded int
	trace, last := DecisionTrace(func(string, ...any) { forwarded++ }).RecordDeclines()
	if got := last(); got != "no decline recorded" {
		t.Fatalf("empty = %q", got)
	}
	trace.Decline("fill", "no-quotes", "quotes", 0)
	trace.Log("liquidlane fill selected", "routes", 1)
	if got, want := last(), "liquidlane fill declined reason=no-quotes quotes=0"; got != want {
		t.Fatalf("last = %q, want %q", got, want)
	}
	if forwarded != 2 {
		t.Fatalf("forwarded %d events, want 2", forwarded)
	}
	// A nil inner trace still records.
	nilTrace, nilLast := DecisionTrace(nil).RecordDeclines()
	nilTrace.Decline("quote", "no-candidates")
	if got := nilLast(); got != "liquidlane quote declined reason=no-candidates" {
		t.Fatalf("nil trace last = %q", got)
	}
}
