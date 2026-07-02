package redstoneoev

import (
	"context"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

// ReadFillerStatus checks the adapter's caller predicate:
// callback == marketMaker || callback == owner || isFiller(marketMaker, callback).
func (r *reader) ReadFillerStatus(ctx context.Context, callback, adapter common.Address) (bool, error) {
	res, err := r.chain.Multicall(ctx, []chain.Call{
		{Target: adapter, AllowFailure: true, Data: llAdapter.PackMarketMaker()},
		{Target: adapter, AllowFailure: true, Data: llAdapter.PackOwner()},
	})
	if err != nil {
		return false, err
	}
	authorized, mm, needFiller := resolveFillerAuth(callback, res)
	if !needFiller {
		return authorized, nil
	}
	fRes, err := r.chain.Multicall(ctx, []chain.Call{
		{Target: adapter, AllowFailure: true, Data: llAdapter.PackIsFiller(mm, callback)},
	})
	if err != nil {
		return false, err
	}
	if len(fRes) == 1 && fRes[0].Success {
		if ok, e := llAdapter.UnpackIsFiller(fRes[0].ReturnData); e == nil {
			return ok, nil
		}
	}
	return false, nil // isFiller unreadable → fail closed
}

func resolveFillerAuth(callback common.Address, res []chain.CallResult) (authorized bool, marketMaker common.Address, needFiller bool) {
	var mm common.Address
	hasMM, direct := false, false
	if len(res) > 0 && res[0].Success {
		if v, e := llAdapter.UnpackMarketMaker(res[0].ReturnData); e == nil {
			mm, hasMM = v, v != (common.Address{})
			direct = mm == callback
		}
	}
	if len(res) > 1 && res[1].Success {
		if owner, e := llAdapter.UnpackOwner(res[1].ReturnData); e == nil && owner == callback {
			direct = true
		}
	}
	switch {
	case direct:
		return true, mm, false // marketMaker/owner == callback
	case hasMM:
		return false, mm, true // need isFiller(marketMaker, callback)
	default:
		return false, mm, false // no marketMaker + not owned → fail closed
	}
}
