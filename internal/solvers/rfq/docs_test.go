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
			"POST /swap", "DISCOVERY", "CONFIRM", "BUILD", "never broadcasts",
			"x-rfq-shared-secret", "transport-only", "64 adapter calls",
		},
		"../../../docs/RFQ-PLAN.md": {
			"in-memory", "0x9a4568b6", "Private discount calldata is never", "restart", "transfer-before-call",
			"zero native value", "aggregate output floor", "transport-only request ID",
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
