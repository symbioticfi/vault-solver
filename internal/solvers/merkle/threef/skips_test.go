package threef

import (
	"maps"
	"strings"
	"testing"

	"github.com/go-logr/logr/funcr"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

func TestSkipLogCapsAndFormatsOnlyAtDebug(t *testing.T) {
	for _, verbosity := range []int{0, 1} {
		var lines []string
		log := funcr.New(func(_, args string) { lines = append(lines, args) }, funcr.Options{Verbosity: verbosity})
		skips := newSkipLog(observability.WithLogger(t.Context(), log))
		for i := range maxSkipExamples + 5 {
			skips.addf("auction %d: status %q is not open or solvable", i, "settled")
		}
		skips.addf("auction %d: request %s is not eligible for this market", 99, "0xab")

		if got := skips.flush("3F auctions skipped"); got != `26 skipped, first auction 0: status "settled" is not open or solvable` {
			t.Fatalf("decline = %q", got)
		}
		if verbosity == 0 {
			if len(lines) != 0 || len(skips.reasons) != 1 {
				t.Fatalf("debug off: logged %q, formatted %d reasons, want none logged and only the first formatted", lines, len(skips.reasons))
			}
			continue
		}
		if len(lines) != 1 || !strings.Contains(lines[0], `"6 more"`) || strings.Contains(lines[0], "auction 20:") {
			t.Fatalf("debug on logged %q, want one line with 20 examples and \"6 more\"", lines)
		}
		checks, examples := summarizeSkips(skips.reasons)
		want := map[string]int{"auction #: status # is not open or solvable": maxSkipExamples + 5, "auction #: request # is not eligible for this market": 1}
		if !maps.Equal(checks, want) || len(examples) != maxSkipExamples+1 || skips.reasons[maxSkipExamples] == "6 more" {
			t.Fatalf("checks = %v, examples = %q", checks, examples)
		}
	}
	if got := newSkipLog(t.Context()).flush("nothing"); got != "" {
		t.Fatalf("empty flush = %q, want \"\"", got)
	}
}
