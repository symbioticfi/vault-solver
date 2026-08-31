package bridgefacilitator

import (
	"context"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/symbioticfi/vault-solver/internal/signer"
)

// TestLiveListOffers exercises the signed per-adapter GET /v1/offer flow against the live API. It is
// skipped unless SOLVER_LIVE_AUTH=1 (so it never runs in CI), and needs SOLVER_PRIVATE_KEY in the
// env. A pass (200 or 403) means the EIP-712 signature was accepted by the 3F API.
func TestLiveListOffers(t *testing.T) {
	if os.Getenv("SOLVER_LIVE_AUTH") != "1" {
		t.Skip("set SOLVER_LIVE_AUTH=1 and SOLVER_PRIVATE_KEY to run the live 3F listOffers auth check")
	}
	pk := os.Getenv("SOLVER_PRIVATE_KEY")
	if pk == "" {
		t.Fatal("SOLVER_PRIVATE_KEY not set")
	}
	sgnr, err := signer.NewFromHexKey(strings.TrimPrefix(pk, "0x"))
	if err != nil {
		t.Fatalf("signer: %v", err)
	}

	baseURL := os.Getenv("SOLVER_3F_BASE_URL")
	if baseURL == "" {
		baseURL = "https://bf.dev.gcp.3f.xyz"
	}

	chainID := big.NewInt(11155111) // Sepolia; override for another chain
	if v := os.Getenv("SOLVER_CHAIN_ID"); v != "" {
		chainID, _ = new(big.Int).SetString(v, 10)
	}
	ac := newAPIClient(baseURL, sgnr, chainID, 30*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use the signer's own address as the adapter for the live auth check.
	offers, err := ac.listOffers(ctx, sgnr.Address())
	if err != nil {
		t.Fatalf("listOffers failed for adapter %s:\n  %v", sgnr.Address().Hex(), err)
	}
	t.Logf("AUTH OK — adapter %s returned %d offers", sgnr.Address().Hex(), len(offers))
}
