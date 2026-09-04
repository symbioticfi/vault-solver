package uniswapx

import "testing"

func TestQuoteDeclineMetricLabelsAreBounded(t *testing.T) {
	tests := map[string]string{
		"blocked":                 "declined_blocked",
		"invalid-request":         "declined_invalid_request",
		"quote-state-unavailable": "declined_quote_state_unavailable",
		"request-derived":         "declined_unknown",
	}
	for reason, want := range tests {
		if got := quoteMetricOutcome(reason); got != want {
			t.Errorf("quoteMetricOutcome(%q) = %q, want %q", reason, got, want)
		}
	}
}
