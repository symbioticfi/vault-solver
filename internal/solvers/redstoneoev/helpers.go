package redstoneoev

import (
	"math/big"
	"slices"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/decision"
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

func cloneAdapterSnapshot(in decision.AdapterSnapshot) decision.AdapterSnapshot {
	out := in
	out.FreeAssets = cloneBig(in.FreeAssets)
	out.Withdrawable = cloneBig(in.Withdrawable)
	out.Redeemable = slices.Clone(in.Redeemable)
	for index := range out.Redeemable {
		out.Redeemable[index].MaxRate = cloneBig(out.Redeemable[index].MaxRate)
		out.Redeemable[index].MaxAssets = cloneBig(out.Redeemable[index].MaxAssets)
		out.Redeemable[index].AcquireBalance = cloneBig(out.Redeemable[index].AcquireBalance)
	}
	return out
}
