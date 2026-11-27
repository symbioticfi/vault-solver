package observability

import (
	"fmt"
	"regexp"
)

// MaxReasonExamples is the usual cap on the reasons one summary line carries verbatim.
const MaxReasonExamples = 20

// reasonValues matches the values inside a reason (quoted text, hex, numbers), masked to group reasons
// by the check that produced them.
var reasonValues = regexp.MustCompile(`"[^"]*"|0x[0-9a-fA-F]+|[0-9]+`)

// SummarizeReasons bounds a list of decline reasons for one log line: checks counts the reasons per
// check (the reason with its values masked to "#"), and examples keeps the first maxExamples verbatim,
// followed by "N more" when some were left out. The caller's slice is not modified.
func SummarizeReasons(reasons []string, maxExamples int) (checks map[string]int, examples []string) {
	checks = make(map[string]int)
	for _, reason := range reasons {
		checks[reasonValues.ReplaceAllString(reason, "#")]++
	}
	n := min(len(reasons), maxExamples)
	examples = reasons[:n:n] // full slice: appending "N more" must not overwrite reasons[n]
	if more := len(reasons) - n; more > 0 {
		examples = append(examples, fmt.Sprintf("%d more", more))
	}
	return checks, examples
}
