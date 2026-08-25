package redstoneoev

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

func allSuccess(res []chain.CallResult, expectedLen int) bool {
	if len(res) != expectedLen {
		return false
	}
	for i := range res {
		if !res[i].Success {
			return false
		}
	}
	return true
}

func cloneBig(v *big.Int) *big.Int {
	if v == nil {
		return nil
	}
	return new(big.Int).Set(v)
}

func orZero(v *big.Int) *big.Int {
	if v == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(v)
}

func cloneAdapterSnapshot(in types.AdapterSnapshot) types.AdapterSnapshot {
	out := in
	out.FreeAssets = cloneBig(in.FreeAssets)
	out.Withdrawable = cloneBig(in.Withdrawable)
	out.Redeemable = make([]types.RedeemableSnapshot, len(in.Redeemable))
	for i, r := range in.Redeemable {
		out.Redeemable[i] = r
		out.Redeemable[i].MaxRate = cloneBig(r.MaxRate)
		out.Redeemable[i].MaxAssets = cloneBig(r.MaxAssets)
		out.Redeemable[i].AcquireBalance = cloneBig(r.AcquireBalance)
	}
	return out
}
