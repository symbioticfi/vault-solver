package bridgefacilitator

import (
	"slices"
	"testing"

	"github.com/symbioticfi/vault-solver/internal/solver"
)

// TestSelfRegistered verifies the package's init() registered the solver under Name, so a blank
// import in main is enough to make it selectable from config.
func TestSelfRegistered(t *testing.T) {
	if !slices.Contains(solver.Registered(), Name) {
		t.Fatalf("solver %q not registered; have %v", Name, solver.Registered())
	}
}
