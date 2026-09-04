package lifi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFillInputDecisionTraceIsNotSerialized(t *testing.T) {
	payload, err := json.Marshal(FillInput{
		OrderID: "order-1",
		Trace:   func(string, ...any) {},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(strings.ToLower(string(payload)), "trace") {
		t.Fatalf("fill input leaked decision trace: %s", payload)
	}
}
