package liquidlane

import "github.com/go-errors/errors"

// SolverMode identifies who supplies LiquidLane execution terms.
type SolverMode string

const (
	SolverModeExternal SolverMode = "external"
	SolverModeInternal SolverMode = "internal"
)

func ParseSolverMode(raw string) (SolverMode, error) {
	switch SolverMode(raw) {
	case "", SolverModeExternal:
		return SolverModeExternal, nil
	case SolverModeInternal:
		return SolverModeInternal, nil
	default:
		return "", errors.Errorf(`solverMode: must be "external" or "internal", got %q`, raw)
	}
}
