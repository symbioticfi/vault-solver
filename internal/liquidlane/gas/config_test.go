package gas

import (
	"testing"

	"github.com/go-errors/errors"
)

func TestRequireReferenceGasUnits(t *testing.T) {
	for _, tc := range []struct {
		name  string
		units uint64
		want  error
	}{
		{name: "unset reference gas is refused", want: ErrReferenceGasUnitsRequired},
		{name: "one unit", units: 1},
		{name: "mainnet fill", units: 4_400_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := RequireReferenceGasUnits(tc.units)
			if tc.want == nil && err != nil || tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("RequireReferenceGasUnits(%d) = %v, want %v", tc.units, err, tc.want)
			}
		})
	}
}
