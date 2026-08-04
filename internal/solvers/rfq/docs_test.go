package rfq

import (
	"bytes"
	"os"
	"testing"
)

func TestSwapDocumentationPinsConfigurationAndSecurityContract(t *testing.T) {
	checks := map[string][]string{
		"../../../config/rfq.example.yaml": {
			"swapEnabled", "router", "swapQuoteTtlMs", "30000",
		},
		"../../../README.md": {
			"POST /swap", "DISCOVERY", "CONFIRM", "BUILD", "does not broadcast",
			"x-rfq-shared-secret",
		},
		"../../../docs/RFQ-PLAN.md": {
			"in-memory", "0x9a4568b6", "0x8fa5c671", "restart", "transfer-before-call",
			"zero native value", "aggregate output floor",
		},
	}
	for path, required := range checks {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, needle := range required {
			if !bytes.Contains(body, []byte(needle)) {
				t.Errorf("%s missing %q", path, needle)
			}
		}
	}
}
