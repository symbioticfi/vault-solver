package bridgefacilitator

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/signer"
)

// TestLiveGenerateKey exercises the real 3F generate-key flow against the live API. It is skipped
// unless SOLVER_LIVE_AUTH=1 (so it never runs in CI), and needs SOLVER_PRIVATE_KEY in the env. A
// pass means the facilitator (the signer EOA) is onboarded and a key was issued; a 403 means the
// signature is accepted but the address isn't registered with 3F yet.
func TestLiveGenerateKey(t *testing.T) {
	if os.Getenv("SOLVER_LIVE_AUTH") != "1" {
		t.Skip("set SOLVER_LIVE_AUTH=1 and SOLVER_PRIVATE_KEY to run the live 3F auth check")
	}
	pk := os.Getenv("SOLVER_PRIVATE_KEY")
	if pk == "" {
		t.Fatal("SOLVER_PRIVATE_KEY not set")
	}
	sgnr, err := signer.NewFromHexKey(pk)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}

	baseURL := os.Getenv("SOLVER_3F_BASE_URL")
	if baseURL == "" {
		baseURL = "https://bf.dev.gcp.3f.xyz"
	}

	ac, err := newAPIClient(baseURL, 30*time.Second, sgnr, sgnr.Address(), "", logr.Discard())
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	key, err := ac.generate(ctx)
	if err != nil {
		t.Fatalf("generate-key failed for facilitator %s:\n  %v", sgnr.Address().Hex(), err)
	}
	t.Logf("AUTH OK — facilitator %s issued key %s", sgnr.Address().Hex(), maskKey(key))
}

func maskKey(s string) string {
	if len(s) <= 10 {
		return "***"
	}
	return s[:10] + "…(redacted)"
}
