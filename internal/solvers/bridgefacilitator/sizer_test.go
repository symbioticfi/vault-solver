package bridgefacilitator

import (
	"math/big"
	"testing"
)

func bi(n int64) *big.Int { return big.NewInt(n) }

func TestSizeOffer(t *testing.T) {
	base := func() sizeInputs {
		return sizeInputs{
			perRequestMax: bi(250_000),
			fundable:      bi(500_000),
			openCount:     0,
			maxConcurrent: 10,
		}
	}

	tests := []struct {
		name   string
		mutate func(*sizeInputs)
		wantOK bool
		want   *big.Int
	}{
		{
			name:   "perRequestMax binds",
			mutate: func(in *sizeInputs) {},
			wantOK: true,
			want:   bi(250_000),
		},
		{
			name:   "fundable binds",
			mutate: func(in *sizeInputs) { in.fundable = bi(100_000) },
			wantOK: true,
			want:   bi(100_000),
		},
		{
			name:   "concurrency cap reached",
			mutate: func(in *sizeInputs) { in.openCount = 10 },
			wantOK: false,
		},
		{
			name:   "perRequestMax disabled (0): fundable binds",
			mutate: func(in *sizeInputs) { in.perRequestMax = bi(0) },
			wantOK: true,
			want:   bi(500_000),
		},
		{
			name:   "maxConcurrent disabled (0): no concurrency limit",
			mutate: func(in *sizeInputs) { in.maxConcurrent = 0; in.openCount = 100 },
			wantOK: true,
			want:   bi(250_000),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := base()
			tc.mutate(&in)
			got, ok := sizeOffer(in)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if tc.wantOK && got.Cmp(tc.want) != 0 {
				t.Fatalf("amount = %s, want %s", got, tc.want)
			}
		})
	}
}
