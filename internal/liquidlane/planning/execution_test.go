package planning

import (
	"math/big"
	"testing"
)

func TestFillInputCheckAmounts(t *testing.T) {
	for _, tt := range []struct {
		name           string
		input, output  *big.Int
		ready, invalid bool
	}{
		{name: "missing input", output: big.NewInt(1), invalid: true},
		{name: "zero input", input: big.NewInt(0), output: big.NewInt(1), invalid: true},
		{name: "negative input", input: big.NewInt(-1), output: big.NewInt(1), invalid: true},
		{name: "missing output", input: big.NewInt(10), invalid: true},
		{name: "zero output", input: big.NewInt(10), output: big.NewInt(0), invalid: true},
		{name: "negative output", input: big.NewInt(10), output: big.NewInt(-1), invalid: true},
		{name: "below minimum", input: big.NewInt(9), output: big.NewInt(1)},
		{name: "at minimum", input: big.NewInt(10), output: big.NewInt(1), ready: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			declined := false
			in := FillInput{AmountIn: tt.input, OutputAmount: tt.output,
				Trace: func(string, ...any) { declined = true },
			}
			ready, err := in.CheckAmounts(big.NewInt(10))
			if ready != tt.ready || (err != nil) != tt.invalid || declined != (!tt.ready && !tt.invalid) {
				t.Fatalf("ready=%v err=%v declined=%v", ready, err, declined)
			}
		})
	}
}
