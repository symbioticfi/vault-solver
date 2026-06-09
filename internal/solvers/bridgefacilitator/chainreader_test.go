package bridgefacilitator

import (
	"math/big"
	"testing"
)

// TestDeriveLiquidity covers the pure reduction of the core-mirror on-chain reads
// (delegator.limitOf, adapter.totalAssets, adapter.outstandingPrincipal, vault.withdrawable) to the
// sizer's inputs:
//
//	fundable    = max(min(limit - held, withdrawable), 0)
//	outstanding = max(outstandingPrincipal, 0)
func TestDeriveLiquidity(t *testing.T) {
	t.Parallel()

	bn := big.NewInt
	huge := bn(1_000_000) // vault liquidity not the binding constraint

	tests := []struct {
		name                                            string
		limit, held, outstandingPrincipal, withdrawable *big.Int
		wantFundable, wantOutstanding                   *big.Int
	}{
		{
			name:                 "room available (cap binds)",
			limit:                bn(1000),
			held:                 bn(400),
			outstandingPrincipal: bn(350),
			withdrawable:         huge,
			wantFundable:         bn(600),
			wantOutstanding:      bn(350),
		},
		{
			name:                 "vault liquidity binds below cap headroom",
			limit:                bn(1000),
			held:                 bn(400),
			outstandingPrincipal: bn(350),
			withdrawable:         bn(250),
			wantFundable:         bn(250),
			wantOutstanding:      bn(350),
		},
		{
			name:                 "dry vault clamps fundable to zero despite cap room",
			limit:                bn(1000),
			held:                 bn(400),
			outstandingPrincipal: bn(350),
			withdrawable:         bn(0),
			wantFundable:         bn(0),
			wantOutstanding:      bn(350),
		},
		{
			name:                 "cap fully consumed",
			limit:                bn(1000),
			held:                 bn(1000),
			outstandingPrincipal: bn(900),
			withdrawable:         huge,
			wantFundable:         bn(0),
			wantOutstanding:      bn(900),
		},
		{
			name:                 "held over cap clamps fundable to zero",
			limit:                bn(500),
			held:                 bn(800),
			outstandingPrincipal: bn(700),
			withdrawable:         huge,
			wantFundable:         bn(0),
			wantOutstanding:      bn(700),
		},
		{
			name:                 "negative outstanding clamps to zero",
			limit:                bn(1000),
			held:                 bn(100),
			outstandingPrincipal: bn(-5),
			withdrawable:         huge,
			wantFundable:         bn(900),
			wantOutstanding:      bn(0),
		},
		{
			name:                 "all zero",
			limit:                bn(0),
			held:                 bn(0),
			outstandingPrincipal: bn(0),
			withdrawable:         bn(0),
			wantFundable:         bn(0),
			wantOutstanding:      bn(0),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotFundable, gotOutstanding := deriveLiquidity(tc.limit, tc.held, tc.outstandingPrincipal, tc.withdrawable)
			if gotFundable.Cmp(tc.wantFundable) != 0 {
				t.Errorf("fundable = %s, want %s", gotFundable, tc.wantFundable)
			}
			if gotOutstanding.Cmp(tc.wantOutstanding) != 0 {
				t.Errorf("outstanding = %s, want %s", gotOutstanding, tc.wantOutstanding)
			}
		})
	}
}

// TestDeriveLiquidityDoesNotMutateInputs guards against the clamp accidentally aliasing/mutating the
// caller's *big.Int values (deriveLiquidity must allocate its own results).
func TestDeriveLiquidityDoesNotMutateInputs(t *testing.T) {
	t.Parallel()

	limit := big.NewInt(500)
	held := big.NewInt(800) // held > limit, so fundable clamps to 0
	outstandingPrincipal := big.NewInt(-1)
	withdrawable := big.NewInt(1000)

	_, _ = deriveLiquidity(limit, held, outstandingPrincipal, withdrawable)

	if limit.Cmp(big.NewInt(500)) != 0 || held.Cmp(big.NewInt(800)) != 0 ||
		outstandingPrincipal.Cmp(big.NewInt(-1)) != 0 || withdrawable.Cmp(big.NewInt(1000)) != 0 {
		t.Fatalf("deriveLiquidity mutated its inputs: limit=%s held=%s outstanding=%s withdrawable=%s",
			limit, held, outstandingPrincipal, withdrawable)
	}
}
