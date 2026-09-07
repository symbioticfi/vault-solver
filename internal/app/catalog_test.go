package app

import (
	"strings"
	"testing"

	"github.com/symbioticfi/vault-solver/internal/solver"
	"gopkg.in/yaml.v3"
)

func TestConstructEveryIntegration(t *testing.T) {
	for _, name := range []string{"3f-bridge-facilitator", "lifi-samechain", "redstone-oev", "rfq-filler", "uniswapx-filler"} {
		if _, err := newSolver(name, yaml.Node{}, solver.Deps{}); err == nil || strings.Contains(err.Error(), "unknown solver") {
			t.Fatalf("%s must reach its configuration validation: %v", name, err)
		}
	}
	if _, err := newSolver("absent", yaml.Node{}, solver.Deps{}); err == nil || !strings.Contains(err.Error(), "unknown solver") {
		t.Fatalf("unknown solver: %v", err)
	}
}
