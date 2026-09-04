package liquidlane

import "testing"

func TestParseSolverMode(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want SolverMode
	}{
		{want: SolverModeExternal},
		{raw: "external", want: SolverModeExternal},
		{raw: "internal", want: SolverModeInternal},
	} {
		got, err := ParseSolverMode(test.raw)
		if err != nil || got != test.want {
			t.Fatalf("ParseSolverMode(%q) = %q, %v", test.raw, got, err)
		}
	}
	if _, err := ParseSolverMode("unknown"); err == nil ||
		err.Error() != `solverMode: must be "external" or "internal", got "unknown"` {
		t.Fatalf("unknown mode error = %v", err)
	}
}
