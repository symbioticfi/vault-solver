package bridgefacilitator

import (
	"context"
	"fmt"
	"math/big"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/api/bindings/3f/adapter"
	"github.com/symbioticfi/vault-solver/api/bindings/3f/vaultcontroller"
	"github.com/symbioticfi/vault-solver/api/bindings/vaultv2"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

// Parsed ABIs for packing/decoding Multicall3 sub-calls. adapterABI is defined in redeemer.go.
var (
	vaultABI = mustABI(vaultv2.IVaultV2MetaData)
	vcABI    = mustABI(vaultcontroller.IVaultControllerMetaData)
)

func mustABI(md *bind.MetaData) abi.ABI {
	parsed, err := md.GetAbi()
	if err != nil {
		panic(fmt.Sprintf("bridgefacilitator: parse ABI: %v", err))
	}
	return *parsed
}

// reader performs the adapter- and Request-side on-chain reads the solver relies on, batching via
// Multicall3 where calls are independent.
type reader struct {
	chain *chain.Client
}

func newReader(c *chain.Client) *reader { return &reader{chain: c} }

func (r *reader) adapterCaller(addr common.Address) (*adapter.BridgeFacilitatorAdapterCaller, error) {
	caller, err := adapter.NewBridgeFacilitatorAdapterCaller(addr, r.chain.Client)
	if err != nil {
		return nil, errors.Errorf("bind adapter %s: %w", addr, err)
	}
	return caller, nil
}

// vaultCollateral returns the vault's collateral token, used to match auctions (by deposit asset)
// to this funding vault.
func (r *reader) vaultCollateral(ctx context.Context, vault common.Address) (common.Address, error) {
	res, err := r.chain.Multicall(ctx, []chain.Call{{Target: vault, Data: mustPack(vaultABI, "collateral")}})
	if err != nil {
		return common.Address{}, err
	}
	if len(res) != 1 || !res[0].Success {
		return common.Address{}, errors.New("vault.collateral() reverted")
	}
	return unpackAddress(vaultABI, "collateral", res[0].ReturnData)
}

// liquidityAndExposure reads, in a SINGLE multicall, everything the offer sizer needs:
//
//	fundable    = min(vault.allocatable(), adapterLimit(adapter) - adapterAllocated(adapter))
//	outstanding = adapterAllocated(adapter) - adapter.realizedPrincipal()   // principal in live loans
//	openCount   = len(adapter.activeRequests())
//
// Deriving `outstanding` from the two running totals avoids iterating positions(request) entirely.
// (In a loss scenario `outstanding` is conservatively overstated by the unreconciled shortfall
// until deallocate, which only makes the bot bid less — safe.)
//
//nolint:revive // function-result-limit: (fundable, outstanding, openCount, err) reads clearer than a result struct.
func (r *reader) liquidityAndExposure(
	ctx context.Context, vault, adapterAddr common.Address,
) (fundable, outstanding *big.Int, openCount int, err error) {
	calls := []chain.Call{
		{Target: vault, Data: mustPack(vaultABI, "allocatable")},
		{Target: vault, Data: mustPack(vaultABI, "adapterLimit", adapterAddr)},
		{Target: vault, Data: mustPack(vaultABI, "adapterAllocated", adapterAddr)},
		{Target: adapterAddr, Data: mustPack(adapterABI, "realizedPrincipal")},
		{Target: adapterAddr, Data: mustPack(adapterABI, "activeRequests")},
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, nil, 0, err
	}
	if len(res) != len(calls) {
		return nil, nil, 0, errors.Errorf("multicall returned %d results, want %d", len(res), len(calls))
	}
	for i, rr := range res {
		if !rr.Success {
			return nil, nil, 0, errors.Errorf("liquidity multicall: sub-call %d reverted", i)
		}
	}

	allocatable, err := unpackBig(vaultABI, "allocatable", res[0].ReturnData)
	if err != nil {
		return nil, nil, 0, err
	}
	adapterLimit, err := unpackBig(vaultABI, "adapterLimit", res[1].ReturnData)
	if err != nil {
		return nil, nil, 0, err
	}
	adapterAllocated, err := unpackBig(vaultABI, "adapterAllocated", res[2].ReturnData)
	if err != nil {
		return nil, nil, 0, err
	}
	realized, err := unpackBig(adapterABI, "realizedPrincipal", res[3].ReturnData)
	if err != nil {
		return nil, nil, 0, err
	}
	reqs, err := unpackAddresses(adapterABI, "activeRequests", res[4].ReturnData)
	if err != nil {
		return nil, nil, 0, err
	}

	room := new(big.Int).Sub(adapterLimit, adapterAllocated)
	if room.Sign() < 0 {
		room.SetInt64(0)
	}
	fundable = minBig(allocatable, room)

	outstanding = new(big.Int).Sub(adapterAllocated, realized)
	if outstanding.Sign() < 0 {
		outstanding.SetInt64(0)
	}
	return fundable, outstanding, len(reqs), nil
}

// readyToRedeem returns the adapter's active Requests that are currently redeemable. It reads the
// active set (one call) then batches every canWithdraw() into a single multicall.
func (r *reader) readyToRedeem(ctx context.Context, adapterAddr common.Address) ([]common.Address, error) {
	caller, err := r.adapterCaller(adapterAddr)
	if err != nil {
		return nil, err
	}
	reqs, err := caller.ActiveRequests(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, errors.Errorf("adapter.activeRequests(): %w", err)
	}
	if len(reqs) == 0 {
		return nil, nil
	}

	calls := make([]chain.Call, len(reqs))
	for i, req := range reqs {
		// AllowFailure: a single malformed Request must not break the whole batch.
		calls[i] = chain.Call{Target: req, AllowFailure: true, Data: mustPack(vcABI, "canWithdraw")}
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}

	ready := make([]common.Address, 0, len(reqs))
	for i, rr := range res {
		if !rr.Success {
			continue
		}
		ok, derr := unpackBool(vcABI, "canWithdraw", rr.ReturnData)
		if derr != nil {
			continue
		}
		if ok {
			ready = append(ready, reqs[i])
		}
	}
	return ready, nil
}

/* ───────── ABI pack/unpack helpers for multicall ───────── */

// mustPack packs a static method call; a failure is a programming error (wrong method/args).
func mustPack(a abi.ABI, method string, args ...interface{}) []byte {
	data, err := a.Pack(method, args...)
	if err != nil {
		panic(fmt.Sprintf("bridgefacilitator: pack %s: %v", method, err))
	}
	return data
}

func unpackBig(a abi.ABI, method string, data []byte) (*big.Int, error) {
	vals, err := a.Unpack(method, data)
	if err != nil {
		return nil, errors.Errorf("unpack %s: %w", method, err)
	}
	n, ok := vals[0].(*big.Int)
	if !ok {
		return nil, errors.Errorf("unpack %s: unexpected type %T", method, vals[0])
	}
	return n, nil
}

func unpackBool(a abi.ABI, method string, data []byte) (bool, error) {
	vals, err := a.Unpack(method, data)
	if err != nil {
		return false, errors.Errorf("unpack %s: %w", method, err)
	}
	b, ok := vals[0].(bool)
	if !ok {
		return false, errors.Errorf("unpack %s: unexpected type %T", method, vals[0])
	}
	return b, nil
}

func unpackAddress(a abi.ABI, method string, data []byte) (common.Address, error) {
	vals, err := a.Unpack(method, data)
	if err != nil {
		return common.Address{}, errors.Errorf("unpack %s: %w", method, err)
	}
	addr, ok := vals[0].(common.Address)
	if !ok {
		return common.Address{}, errors.Errorf("unpack %s: unexpected type %T", method, vals[0])
	}
	return addr, nil
}

func unpackAddresses(a abi.ABI, method string, data []byte) ([]common.Address, error) {
	vals, err := a.Unpack(method, data)
	if err != nil {
		return nil, errors.Errorf("unpack %s: %w", method, err)
	}
	addrs, ok := vals[0].([]common.Address)
	if !ok {
		return nil, errors.Errorf("unpack %s: unexpected type %T", method, vals[0])
	}
	return addrs, nil
}
