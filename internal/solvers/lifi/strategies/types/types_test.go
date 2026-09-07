package types

import (
	"encoding/json"
	"strings"
	"testing"

	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

func TestFillInputDecisionTraceIsNotSerialized(t *testing.T) {
	payload, err := json.Marshal(FillInput{
		OrderID: "order-1",
		Trace:   func(string, ...any) {},
	})
	testcheck.NoError(t, err, "Marshal: %v")
	if strings.Contains(strings.ToLower(string(payload)), "trace") {
		t.Fatalf("fill input leaked decision trace: %s", payload)
	}
}
