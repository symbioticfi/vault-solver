package merkle

import "testing"

func TestSimulationCannotHideRecoveredSignerOwnership(t *testing.T) {
	if err := validateRecoveredMode("simulate", "signed-order"); err == nil {
		t.Fatal("simulation hid unresolved shared signer work")
	}
	if err := validateRecoveredMode("send", "signed-order"); err != nil {
		t.Fatal(err)
	}
	if err := validateRecoveredMode("simulate", ""); err != nil {
		t.Fatal(err)
	}
}
