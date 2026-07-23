package bridgefacilitator

import (
	"context"
	"math/big"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/symbioticfi/vault-solver/api/bindings/3f/adapter"
	"github.com/symbioticfi/vault-solver/api/bindings/3f/vaultcontroller"
	"github.com/symbioticfi/vault-solver/api/bindings/adapterfactory"
	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/signer"
)

// Contract bindings (abigen --v2): typed Pack/Unpack helpers for the Multicall3 sub-calls below, so an
// ABI change fails at compile time (see CLAUDE.md "Code generation").
//
// The ThreeFAdapter computes its own JIT-funding headroom on-chain via getMaxAssets() (it folds in the
// delegator's per-adapter limitOf, the vault's withdrawable liquidity, and any pending sweep), so the bot
// no longer reads the delegator/vault directly for sizing. The collateral token is read during each
// adapter refresh via IERC4626(vault).asset() to match auctions.
var (
	bfAdapter = adapter.NewThreeFAdapter()
	factoryB  = adapterfactory.NewIAdapterFactory()
	vc        = vaultcontroller.NewIVaultController()
	erc4626b  = erc4626.NewIERC4626()
)

// maxRequests mirrors MAX_REQUESTS in IThreeFAdapter — the adapter rejects a new request once it tracks
// this many. It is the bot's concurrency pre-screen cap and the clamp bound for the on-chain
// requestsLength() count. (50 is a compile-time constant, immutable per deployment, so it is mirrored
// here rather than read.)
const maxRequests = 50

// maxFactoryEntities bounds the configured factory snapshot before allocating one call per entity.
// A real deployment is orders of magnitude smaller; larger reported counts are rejected so corrupt
// or malicious data cannot exhaust RAM.
const maxFactoryEntities = 2_000

// erc1271MagicValue is the ERC-1271 return value of isValidSignature(bytes32,bytes) for a valid
// signature (`bytes4(keccak256("isValidSignature(bytes32,bytes)"))`).
var erc1271MagicValue = [4]byte{0x16, 0x26, 0xba, 0x7e}

// eligibilityProbeMessage is signed once at startup to build the signerProbe. Its hash is arbitrary and
// deliberately distinct from any EIP-712 offer digest, so the resulting signature cannot be replayed as
// an offer; the adapter's isValidSignature validates the raw hash against its offerSigner regardless.
const eligibilityProbeMessage = "vault-solver:3f:offer-signer-eligibility:v1"

// signerProbe is a fixed (hash, signature) pair produced once from the solver's key. It is fed to each
// adapter's ERC-1271 isValidSignature to test whether this solver is an authorized offer signer for that
// adapter — matching the exact on-chain check 3F uses to accept offers, so it works whether the adapter's
// offerSigner is this solver's EOA (ecrecover) or an EIP-1271 contract that authorizes this key. The pair
// is reusable across every adapter and across periodic re-checks (see resolveAdapters).
type signerProbe struct {
	hash [32]byte
	sig  []byte
}

// newSignerProbe signs the fixed eligibility message with the solver's key once.
func newSignerProbe(s signer.Signer) (signerProbe, error) {
	hash := crypto.Keccak256Hash([]byte(eligibilityProbeMessage))
	sig, err := s.SignHash(hash)
	if err != nil {
		return signerProbe{}, errors.Errorf("sign offer-signer eligibility probe: %w", err)
	}
	return signerProbe{hash: hash, sig: sig}, nil
}

// reader performs the adapter- and Request-side on-chain reads the solver relies on, batching via
// Multicall3 where calls are independent.
type reader struct {
	chain *chain.Client
}

func newReader(c *chain.Client) *reader {
	return &reader{chain: c}
}

// factoryAdapters returns a bounded factory entity snapshot in registry order. The registry is
// append-only, so totalEntities followed by a batched entity(i) read is a consistent enumeration.
func (r *reader) factoryAdapters(ctx context.Context, factory common.Address) ([]common.Address, error) {
	res, err := r.chain.Multicall(ctx, []chain.Call{{Target: factory, Data: factoryB.PackTotalEntities()}})
	if err != nil {
		return nil, err
	}
	if len(res) != 1 || !res[0].Success {
		return nil, errors.New("adapter factory totalEntities() reverted")
	}
	total, err := factoryB.UnpackTotalEntities(res[0].ReturnData)
	if err != nil {
		return nil, errors.Errorf("adapter factory totalEntities(): %w", err)
	}
	if total.Cmp(big.NewInt(maxFactoryEntities)) > 0 {
		return nil, errors.Errorf("adapter factory entity count %s exceeds safety limit %d", total.String(), maxFactoryEntities)
	}
	count := int(total.Int64())
	if count == 0 {
		return nil, nil
	}

	calls := make([]chain.Call, count)
	for i := range calls {
		calls[i] = chain.Call{Target: factory, Data: factoryB.PackEntity(big.NewInt(int64(i)))}
	}
	res, err = r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(res) != count {
		return nil, errors.Errorf("adapter factory returned %d entities, want %d", len(res), count)
	}

	adapters := make([]common.Address, count)
	for i := range res {
		if !res[i].Success {
			return nil, errors.Errorf("adapter factory entity(%d) reverted", i)
		}
		adapterAddr, unpackErr := factoryB.UnpackEntity(res[i].ReturnData)
		if unpackErr != nil {
			return nil, errors.Errorf("adapter factory entity(%d): %w", i, unpackErr)
		}
		if adapterAddr == (common.Address{}) {
			return nil, errors.Errorf("adapter factory entity(%d) is zero", i)
		}
		adapters[i] = adapterAddr
	}
	return adapters, nil
}

// resolvedAdapter is one adapter's refresh resolution: its vault, that vault's collateral (the
// ERC-4626 asset, used to match auctions), its offer-signer (diagnostic only), and whether this solver
// is an authorized offer signer for it (adapter.isValidSignature accepted the probe). err is set (other
// fields zero) if a required read reverted, so the caller can drop just that adapter.
type resolvedAdapter struct {
	vault      common.Address
	collateral common.Address
	signer     common.Address
	authorized bool
	err        error
}

// authorizedByProbe reports whether the adapter's ERC-1271 isValidSignature accepted the probe
// signature. A revert or any non-magic return means not authorized (drop the adapter), not a hard error.
func authorizedByProbe(res chain.CallResult) bool {
	if !res.Success {
		return false
	}
	magic, err := bfAdapter.UnpackIsValidSignature(res.ReturnData)
	if err != nil {
		return false
	}
	return magic == erc1271MagicValue
}

// decodeAddr returns the non-zero address a Multicall sub-call returned, or an error tagged with
// `what` if it reverted, failed to decode, or returned zero.
func decodeAddr(res chain.CallResult, unpack func([]byte) (common.Address, error), what string) (common.Address, error) {
	if !res.Success {
		return common.Address{}, errors.Errorf("%s reverted", what)
	}
	addr, err := unpack(res.ReturnData)
	if err != nil {
		return common.Address{}, errors.Errorf("decode %s: %w", what, err)
	}
	if addr == (common.Address{}) {
		return common.Address{}, errors.Errorf("%s returned zero address", what)
	}
	return addr, nil
}

// resolveAdapters resolves every adapter's vault, collateral, and offer-signer, and validates offer-signer
// authorization via the adapter's ERC-1271 isValidSignature(probe), in two Multicalls regardless of adapter
// count: round 1 batches each adapter's vault()+offerSigner()+isValidSignature(probe); round 2 batches
// asset() on the vaults of adapters that resolved and are authorized. Per-call AllowFailure isolates a bad
// adapter to its own err; a returned error is a whole-batch RPC failure. The probe is reusable — the same
// call drives startup validation and periodic re-validation.
func (r *reader) resolveAdapters(ctx context.Context, adapters []common.Address, probe signerProbe) ([]resolvedAdapter, error) {
	out := make([]resolvedAdapter, len(adapters))

	calls := make([]chain.Call, 0, 3*len(adapters))
	for _, a := range adapters {
		calls = append(calls,
			chain.Call{Target: a, Data: bfAdapter.PackVault(), AllowFailure: true},
			chain.Call{Target: a, Data: bfAdapter.PackOfferSigner(), AllowFailure: true},
			chain.Call{Target: a, Data: bfAdapter.PackIsValidSignature(probe.hash, probe.sig), AllowFailure: true},
		)
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(res) != len(calls) {
		return nil, errors.Errorf("adapter resolution returned %d results, want %d", len(res), len(calls))
	}

	// Decode round 1; queue an asset() call for each adapter that resolved and is an authorized signer.
	assetCalls := make([]chain.Call, 0, len(adapters))
	assetIdx := make([]int, 0, len(adapters)) // assetIdx[k] = out index of assetCalls[k]
	for i := range adapters {
		base := 3 * i
		vault, derr := decodeAddr(res[base], bfAdapter.UnpackVault, "adapter.vault()")
		if derr != nil {
			out[i].err = derr
			continue
		}
		offerSigner, derr := decodeAddr(res[base+1], bfAdapter.UnpackOfferSigner, "adapter.offerSigner()")
		if derr != nil {
			out[i].err = derr
			continue
		}
		out[i].vault, out[i].signer = vault, offerSigner
		out[i].authorized = authorizedByProbe(res[base+2])
		if !out[i].authorized {
			continue // not an authorized offer signer; the caller drops it (no collateral read needed)
		}
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
	if len(ares) != len(assetCalls) {
		return nil, errors.Errorf("asset resolution returned %d results, want %d", len(ares), len(assetCalls))
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

// exposureState is the per-target funding headroom and per-request caps (setLimitsPerRequest) the sizer
// pre-screens against before the contract enforces them at consume time.
type exposureState struct {
	fundable    *big.Int // getMaxAssets(): min(limitOf - totalAssets, vault.withdrawable), 0 if a sweep is pending
	openCount   int      // active request count (requests[] length)
	maxAssets   *big.Int // maxAssetsPerRequest — always-active ceiling (0 = reject-all)
	minAssets   *big.Int // minAssetsPerRequest (0 = no floor)
	minYieldPpm *big.Int // minYieldPerRequest (ppm) — exact on-chain floor (0 = no floor)
}

// liquidityAndExposure reads the adapter's JIT-funding headroom (getMaxAssets), its per-request caps, and
// its active-request count in a single multicall. getMaxAssets() is authoritative for funding: it already
// bounds the headroom by both the delegator's per-adapter cap AND the vault's withdrawable liquidity, so
// the bot can't sign an offer the JIT pull at consume time can't satisfy. openCount is the adapter's own
// requestsLength() (a single read) feeding the concurrency pre-screen.
func (r *reader) liquidityAndExposure(ctx context.Context, adapterAddr common.Address) (exposureState, error) {
	calls := []chain.Call{
		{Target: adapterAddr, Data: bfAdapter.PackGetMaxAssets()},
		{Target: adapterAddr, Data: bfAdapter.PackMinYieldPerRequest()},
		{Target: adapterAddr, Data: bfAdapter.PackMinAssetsPerRequest()},
		{Target: adapterAddr, Data: bfAdapter.PackMaxAssetsPerRequest()},
		{Target: adapterAddr, Data: bfAdapter.PackRequestsLength()},
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return exposureState{}, err
	}
	if len(res) != len(calls) {
		return exposureState{}, errors.Errorf("multicall returned %d results, want %d", len(res), len(calls))
	}
	for i, rr := range res {
		if !rr.Success {
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
	openCount, err := bfAdapter.UnpackRequestsLength(res[4].ReturnData)
	if err != nil {
		return exposureState{}, err
	}

	return exposureState{
		fundable:    fundable,
		openCount:   clampCount(openCount),
		maxAssets:   maxAssets,
		minAssets:   minAssets,
		minYieldPpm: minYield,
	}, nil
}

// clampCount converts the on-chain requestsLength (uint256, bounded by MAX_REQUESTS) to an int. A value
// that doesn't fit is clamped to maxRequests so the concurrency pre-screen fails closed.
func clampCount(n *big.Int) int {
	if n.IsInt64() {
		if v := n.Int64(); v >= 0 && v <= int64(maxRequests) {
			return int(v)
		}
	}
	return maxRequests
}

// requestSlotCalls builds the requests(i) reads for i in [0, n) — n from requestsLength(). AllowFailure:
// a concurrent finalize can shrink the array between the length read and these, so a tail index may
// revert; collectRequests stops at that gap.
func requestSlotCalls(adapterAddr common.Address, n int) []chain.Call {
	calls := make([]chain.Call, n)
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

// readyToRedeem returns the adapter's active Requests that are currently redeemable. It reads
// requestsLength(), enumerates exactly that many requests(i), then batches every canWithdraw() into a
// single multicall.
func (r *reader) readyToRedeem(ctx context.Context, adapterAddr common.Address) ([]common.Address, error) {
	lres, err := r.chain.Multicall(ctx, []chain.Call{{Target: adapterAddr, Data: bfAdapter.PackRequestsLength()}})
	if err != nil {
		return nil, err
	}
	if len(lres) != 1 || !lres[0].Success {
		return nil, errors.New("adapter.requestsLength() reverted")
	}
	n, err := bfAdapter.UnpackRequestsLength(lres[0].ReturnData)
	if err != nil {
		return nil, errors.Errorf("adapter.requestsLength(): %w", err)
	}
	count := clampCount(n)
	if count == 0 {
		return nil, nil
	}

	res, err := r.chain.Multicall(ctx, requestSlotCalls(adapterAddr, count))
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
