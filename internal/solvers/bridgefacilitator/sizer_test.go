package bridgefacilitator

import (
	"math/big"
	"testing"
)

func bi(n int64) *big.Int { return big.NewInt(n) }

func TestSizeOffer(t *testing.T) {
	base := func() sizeInputs {
		return sizeInputs{
			fundable:      bi(500_000),
			maxAssets:     bi(250_000),
			minAssets:     bi(0),
			openCount:     0,
			maxConcurrent: maxRequests,
		}
	}

	tests := []struct {
		name   string
		mutate func(*sizeInputs)
		wantOK bool
		want   *big.Int
	}{
		{
			name:   "maxAssets binds",
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
			mutate: func(in *sizeInputs) { in.openCount = maxRequests },
			wantOK: false,
		},
		{
			name:   "maxAssets 0 rejects all: cannot bid",
			mutate: func(in *sizeInputs) { in.maxAssets = bi(0) },
			wantOK: false,
		},
		{
			name:   "maxAssets above fundable: fundable binds",
			mutate: func(in *sizeInputs) { in.maxAssets = bi(1_000_000) },
			wantOK: true,
			want:   bi(500_000),
		},
		{
			name:   "capacity below minAssets floor: cannot bid",
			mutate: func(in *sizeInputs) { in.fundable = bi(1_000); in.minAssets = bi(5_000) },
			wantOK: false,
		},
		{
			name:   "capacity at minAssets floor: can bid",
			mutate: func(in *sizeInputs) { in.fundable = bi(5_000); in.minAssets = bi(5_000) },
			wantOK: true,
			want:   bi(5_000),
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
