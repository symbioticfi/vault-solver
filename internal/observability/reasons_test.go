package observability_test

import (
	"fmt"
	"maps"
	"testing"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

func TestSummarizeReasonsGroupsAndCaps(t *testing.T) {
	const maxExamples = 20
	var reasons []string
	for i := range maxExamples + 5 {
		reasons = append(reasons, fmt.Sprintf("auction %d: status %q is not open or solvable", i, "settled"))
	}
	reasons = append(reasons, "auction 99 adapter 0xab: principal 50 is below minAssets 100")

	checks, examples := observability.SummarizeReasons(reasons, maxExamples)
	wantChecks := map[string]int{
		"auction #: status # is not open or solvable":           maxExamples + 5,
		"auction # adapter #: principal # is below minAssets #": 1,
	}
	if !maps.Equal(checks, wantChecks) {
		t.Fatalf("checks = %v, want %v", checks, wantChecks)
	}
	if len(examples) != maxExamples+1 || examples[0] != reasons[0] || examples[maxExamples] != "6 more" {
		t.Fatalf("examples = %q, want the first %d reasons then \"6 more\"", examples, maxExamples)
	}
	if reasons[maxExamples] != fmt.Sprintf("auction %d: status %q is not open or solvable", maxExamples, "settled") {
		t.Fatal("summary overwrote the caller's reasons")
	}

	checks, examples = observability.SummarizeReasons(reasons[:2], maxExamples)
	if len(checks) != 1 || len(examples) != 2 {
		t.Fatalf("under the cap: checks = %v, examples = %q", checks, examples)
	}
}
