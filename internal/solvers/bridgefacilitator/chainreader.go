package bridgefacilitator

import (
	"context"
	"math/big"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/api/bindings/3f/adapter"
	"github.com/symbioticfi/vault-solver/api/bindings/3f/vaultcontroller"
	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

// Contract bindings (abigen --v2): typed Pack/Unpack helpers for the Multicall3 sub-calls below, so an
// ABI change fails at compile time (see CLAUDE.md "Code generation").
//
// The ThreeFAdapter computes its own JIT-funding headroom on-chain via getMaxAssets() (it folds in the
// delegator's per-adapter limitOf, the vault's withdrawable liquidity, and any pending sweep), so the bot
// no longer reads the delegator/vault directly for sizing. The collateral token is still read once at
// startup via IERC4626(vault).asset() to match auctions.
var (
	bfAdapter = adapter.NewThreeFAdapter()
	vc        = vaultcontroller.NewIVaultController()
	erc4626b  = erc4626.NewIERC4626()
)

// maxRequests mirrors MAX_REQUESTS in IThreeFAdapter — the adapter rejects a new request once it tracks
// this many. requests[] exposes only an element getter (no length/list), so the active set is enumerated
// requests(0..maxRequests-1); it also bounds the bot's pre-screen concurrency check.
const maxRequests = 50

// reader performs the adapter- and Request-side on-chain reads the solver relies on, batching via
// Multicall3 where calls are independent.
type reader struct {
	chain *chain.Client
}

func newReader(c *chain.Client) *reader {
	return &reader{chain: c}
}

// resolvedAdapter is one adapter's startup resolution: its vault, that vault's collateral (the
// ERC-4626 asset, used to match auctions), and its EIP-1271 offer-signer. err is set (other fields
// zero) if any read reverted, so the caller can drop just that adapter.
type resolvedAdapter struct {
	vault      common.Address
	collateral common.Address
	signer     common.Address
	err        error
}

// decodeAddr returns the address a Multicall sub-call returned, or an error tagged with `what` if it
// reverted or failed to decode.
func decodeAddr(res chain.CallResult, unpack func([]byte) (common.Address, error), what string) (common.Address, error) {
	if !res.Success {
		return common.Address{}, errors.Errorf("%s reverted", what)
	}
	addr, err := unpack(res.ReturnData)
	if err != nil {
		return common.Address{}, errors.Errorf("decode %s: %w", what, err)
	}
	return addr, nil
}

// resolveAdapters resolves every adapter's vault, collateral, and offer-signer in two Multicalls
// regardless of adapter count: round 1 batches each adapter's vault()+offerSigner(); round 2 batches
// asset() on the vaults round 1 returned. Per-call AllowFailure isolates a bad adapter to its own err;
// a returned error is a whole-batch RPC failure.
func (r *reader) resolveAdapters(ctx context.Context, adapters []common.Address) ([]resolvedAdapter, error) {
	out := make([]resolvedAdapter, len(adapters))

	calls := make([]chain.Call, 0, 2*len(adapters))
	for _, a := range adapters {
		calls = append(calls,
			chain.Call{Target: a, Data: bfAdapter.PackVault(), AllowFailure: true},
			chain.Call{Target: a, Data: bfAdapter.PackOfferSigner(), AllowFailure: true},
		)
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}

	// Decode round 1; queue an asset() call for each adapter whose vault and signer both resolved.
	assetCalls := make([]chain.Call, 0, len(adapters))
	assetIdx := make([]int, 0, len(adapters)) // assetIdx[k] = out index of assetCalls[k]
	for i := range adapters {
		vault, derr := decodeAddr(res[2*i], bfAdapter.UnpackVault, "adapter.vault()")
		if derr != nil {
			out[i].err = derr
			continue
		}
		signer, derr := decodeAddr(res[2*i+1], bfAdapter.UnpackOfferSigner, "adapter.offerSigner()")
		if derr != nil {
			out[i].err = derr
			continue
		}
		out[i].vault, out[i].signer = vault, signer
		assetCalls = append(assetCalls, chain.Call{Target: vault, Data: erc4626b.PackAsset(), AllowFailure: true})
		assetIdx = append(assetIdx, i)
	}
	if len(assetCalls) == 0 {
		return out, nil
	}

	ares, err := r.chain.Multicall(ctx, assetCalls)
	if err != nil {
		return nil, err
	}
	for k, idx := range assetIdx {
		collateral, derr := decodeAddr(ares[k], erc4626b.UnpackAsset, "vault.asset()")
		if derr != nil {
			out[idx].err = derr
			continue
		}
		out[idx].collateral = collateral
	}
	return out, nil
}

// exposureState bundles the per-target funding headroom and the adapter's per-request caps the offer
// sizer needs. Caps are the adapter's on-chain risk limits (setLimitsPerRequest, each 0 = disabled); the
// bot pre-screens offers before the contract enforces them at consume time.
type exposureState struct {
	fundable    *big.Int // getMaxAssets(): min(limitOf - totalAssets, vault.withdrawable), 0 if a sweep is pending
	openCount   int      // number of active requests (requests[] length, enumerated)
	maxAssets   *big.Int // maxAssetsPerRequest (0 = no per-request ceiling)
	minAssets   *big.Int // minAssetsPerRequest (0 = no per-request floor)
	minYieldBps *big.Int // minYieldPerRequest (ppm) converted to bps (0 = no yield floor)
}

// liquidityAndExposure reads the adapter's JIT-funding headroom (getMaxAssets), its per-request caps, and
// its active-request count in a single multicall. getMaxAssets() is authoritative for funding: it already
// bounds the headroom by both the delegator's per-adapter cap AND the vault's withdrawable liquidity, so
// the bot can't sign an offer the JIT pull at consume time can't satisfy. The active set is enumerated via
// requests(0..maxRequests-1) (AllowFailure; the dense array's first gap ends the set) so openCount feeds
// the concurrency pre-screen.
func (r *reader) liquidityAndExposure(ctx context.Context, adapterAddr common.Address) (exposureState, error) {
	calls := append([]chain.Call{
		{Target: adapterAddr, Data: bfAdapter.PackGetMaxAssets()},
		{Target: adapterAddr, Data: bfAdapter.PackMinYieldPerRequest()},
		{Target: adapterAddr, Data: bfAdapter.PackMinAssetsPerRequest()},
		{Target: adapterAddr, Data: bfAdapter.PackMaxAssetsPerRequest()},
	}, requestSlotCalls(adapterAddr)...)
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return exposureState{}, err
	}
	if len(res) != len(calls) {
		return exposureState{}, errors.Errorf("multicall returned %d results, want %d", len(res), len(calls))
	}
	// The four limit reads must succeed; the requests(i) slots are AllowFailure (past-end reverts mark the end).
	const limitReads = 4
	for i := range limitReads {
		if !res[i].Success {
			return exposureState{}, errors.Errorf("liquidity multicall: sub-call %d reverted", i)
		}
	}

	fundable, err := bfAdapter.UnpackGetMaxAssets(res[0].ReturnData)
	if err != nil {
		return exposureState{}, err
	}
	minYield, err := bfAdapter.UnpackMinYieldPerRequest(res[1].ReturnData)
	if err != nil {
		return exposureState{}, err
	}
	minAssets, err := bfAdapter.UnpackMinAssetsPerRequest(res[2].ReturnData)
	if err != nil {
		return exposureState{}, err
	}
	maxAssets, err := bfAdapter.UnpackMaxAssetsPerRequest(res[3].ReturnData)
	if err != nil {
		return exposureState{}, err
	}

	return exposureState{
		fundable:    fundable,
		openCount:   len(collectRequests(res[limitReads:])),
		maxAssets:   maxAssets,
		minAssets:   minAssets,
		minYieldBps: ppmToBps(minYield),
	}, nil
}

// ppmToBps converts minYieldPerRequest (ppm, YIELD_PRECISION=1e6) to bps, rounding up so the bot never
// bids below the on-chain floor.
func ppmToBps(ppm *big.Int) *big.Int {
	return new(big.Int).Div(new(big.Int).Add(ppm, big.NewInt(99)), big.NewInt(100)) // ceil(ppm/100); 1 bps = 100 ppm
}

// requestSlotCalls builds the requests(i) reads for i in [0, maxRequests). AllowFailure: indices past the
// dense array revert (out-of-bounds), which marks the end of the active set.
func requestSlotCalls(adapterAddr common.Address) []chain.Call {
	calls := make([]chain.Call, maxRequests)
	for i := range calls {
		calls[i] = chain.Call{Target: adapterAddr, AllowFailure: true, Data: bfAdapter.PackRequests(big.NewInt(int64(i)))}
	}
	return calls
}

// collectRequests decodes the leading run of successful requests(i) results into request addresses.
// finalizeRequest keeps the array dense (swap-pop), so the first reverted/undecodable slot ends the set.
func collectRequests(res []chain.CallResult) []common.Address {
	out := make([]common.Address, 0, len(res))
	for _, rr := range res {
		if !rr.Success {
			break
		}
		addr, err := bfAdapter.UnpackRequests(rr.ReturnData)
		if err != nil {
			break
		}
		out = append(out, addr)
	}
	return out
}

// readyToRedeem returns the adapter's active Requests that are currently redeemable. It enumerates the
// active set (requests(0..maxRequests-1)) then batches every canWithdraw() into a single multicall.
func (r *reader) readyToRedeem(ctx context.Context, adapterAddr common.Address) ([]common.Address, error) {
	res, err := r.chain.Multicall(ctx, requestSlotCalls(adapterAddr))
	if err != nil {
		return nil, err
	}
	reqs := collectRequests(res)
	if len(reqs) == 0 {
		return nil, nil
	}

	calls := make([]chain.Call, len(reqs))
	for i, req := range reqs {
		// AllowFailure: a single malformed Request must not break the whole batch.
		calls[i] = chain.Call{Target: req, AllowFailure: true, Data: vc.PackCanWithdraw()}
	}
	res, err = r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}

	ready := make([]common.Address, 0, len(reqs))
	for i, rr := range res {
		if !rr.Success {
			continue
		}
		ok, derr := vc.UnpackCanWithdraw(rr.ReturnData)
		if derr != nil {
			continue
		}
		if ok {
			ready = append(ready, reqs[i])
		}
	}
	return ready, nil
}
