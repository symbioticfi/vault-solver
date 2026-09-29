package gas

import (
	"strings"
	"testing"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"
)

type referenceGasStub struct {
	units   uint64
	horizon bool
}

func (s referenceGasStub) ReferenceGasUnits() uint64          { return s.units }
func (s referenceGasStub) QuotePricingUsesReferenceGas() bool { return s.horizon }

// The reference gas is required only where quote pricing sizes its tip for it (the horizon policy); under the
// legacy policy an unset one only leaves the funding gate off, so a legacy lane with gas: keeps starting and
// says so once (fee-gas strategy §2.12 PR2, §2.14).
func TestRequireReferenceGasUnits(t *testing.T) {
	for _, tc := range []struct {
		name    string
		txm     referenceGasStub
		want    error
		wantLog bool
	}{
		{name: "horizon pricing without reference gas is refused", txm: referenceGasStub{horizon: true}, want: ErrReferenceGasUnitsRequired},
		{name: "horizon pricing with one unit", txm: referenceGasStub{units: 1, horizon: true}},
		{name: "horizon pricing with a mainnet fill", txm: referenceGasStub{units: 4_400_000, horizon: true}},
		{name: "legacy pricing without reference gas starts with the gate off", txm: referenceGasStub{}, wantLog: true},
		{name: "legacy pricing with reference gas", txm: referenceGasStub{units: 4_400_000}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs []string
			log := funcr.NewJSON(func(line string) { logs = append(logs, line) }, funcr.Options{})
			err := RequireReferenceGasUnits(log, tc.txm)
			if tc.want == nil && err != nil || tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("RequireReferenceGasUnits(%+v) = %v, want %v", tc.txm, err, tc.want)
			}
			logged := len(logs) == 1 && strings.Contains(logs[0], "funding gate off")
			if logged != tc.wantLog || len(logs) > 1 {
				t.Fatalf("logs = %q, want the funding-gate-off line %t", logs, tc.wantLog)
			}
		})
	}
}
