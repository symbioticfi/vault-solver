package bridgefacilitator

import (
	"context"
	"fmt"
	"math/big"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/symbioticfi/vault-solver/api/bindings/3f/adapter"
	"github.com/symbioticfi/vault-solver/api/bindings/3f/vaultcontroller"
	"github.com/symbioticfi/vault-solver/api/bindings/adapterfactory"
	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/api/bindings/lens"
	"github.com/symbioticfi/vault-solver/api/bindings/vaultv2"
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
	lensB     = lens.NewFrontendLiquidityLens()

	reverts = chain.RevertDecoderFor(&adapter.ThreeFAdapterMetaData, &adapterfactory.IAdapterFactoryMetaData,
		&lens.FrontendLiquidityLensMetaData, &vaultv2.IVaultV2MetaData)
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
	// lens is the FrontendLiquidityLens address. When non-zero, funding headroom is read from the lens's
	// cross-adapter deallocation-cascade estimate instead of the adapter's own getMaxAssets(); zero falls
	// back to the adapter getter.
	lens common.Address
}

func newReader(c *chain.Client, lens common.Address) *reader {
	return &reader{chain: c, lens: lens}
}

// multicall names the revert when the whole batch reverts, as it does when a required sub-call fails.
func (r *reader) multicall(ctx context.Context, calls []chain.Call) ([]chain.CallResult, error) {
	res, err := r.chain.Multicall(ctx, calls)
	return res, reverts.Explain(err)
}

// factoryAdapters returns a bounded factory entity snapshot in registry order. The registry is
// append-only, so totalEntities followed by a batched entity(i) read is a consistent enumeration.
func (r *reader) factoryAdapters(ctx context.Context, factory common.Address) ([]common.Address, error) {
	res, err := r.multicall(ctx, []chain.Call{{Target: factory, Data: factoryB.PackTotalEntities()}})
	if err != nil {
		return nil, err
	}
	if len(res) != 1 {
		return nil, errors.Errorf("adapter factory totalEntities() returned %d results, want 1", len(res))
	}
	if !res[0].Success {
		return nil, errors.Errorf("adapter factory totalEntities() reverted: %s", reverts.Describe(res[0].ReturnData))
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
	res, err = r.multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(res) != count {
		return nil, errors.Errorf("adapter factory returned %d entities, want %d", len(res), count)
	}

	adapters := make([]common.Address, count)
	for i := range res {
		if !res[i].Success {
			return nil, errors.Errorf("adapter factory entity(%d) reverted: %s", i, reverts.Describe(res[i].ReturnData))
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
// fields zero) if a required read reverted, so the caller can drop just that adapter. unauthorized
// says why the probe was rejected.
type resolvedAdapter struct {
	vault        common.Address
	collateral   common.Address
	signer       common.Address
	authorized   bool
	unauthorized string
	err          error
}

// probeRejection says why the adapter's ERC-1271 isValidSignature rejected the probe signature, or
// returns "" when it accepted it. A revert or any non-magic return means not authorized (drop the
// adapter), not a hard error.
func probeRejection(res chain.CallResult) string {
	if !res.Success {
		return "isValidSignature reverted: " + reverts.Describe(res.ReturnData)
	}
	magic, err := bfAdapter.UnpackIsValidSignature(res.ReturnData)
	if err != nil {
		return fmt.Sprintf("isValidSignature malformed: %v", err)
	}
	if magic != erc1271MagicValue {
		return fmt.Sprintf("isValidSignature returned %x, want magic value %x", magic, erc1271MagicValue)
	}
	return ""
}

// errAdapterUnconfigured marks an adapter whose on-chain wiring is incomplete (a zero offerSigner,
// vault or asset). That is the normal state of a freshly deployed adapter, not a read failure, so
// callers skip it quietly instead of alerting.
var errAdapterUnconfigured = errors.New("adapter not configured")

// decodeAddr returns the non-zero address a Multicall sub-call returned, or an error tagged with
// `what` if it reverted, failed to decode, or returned zero.
func decodeAddr(res chain.CallResult, unpack func([]byte) (common.Address, error), what string) (common.Address, error) {
	if !res.Success {
		return common.Address{}, errors.Errorf("%s reverted: %s", what, reverts.Describe(res.ReturnData))
	}
	addr, err := unpack(res.ReturnData)
	if err != nil {
		return common.Address{}, errors.Errorf("decode %s: %w", what, err)
	}
	if addr == (common.Address{}) {
		return common.Address{}, errors.Errorf("%s returned zero address: %w", what, errAdapterUnconfigured)
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
	res, err := r.multicall(ctx, calls)
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
		out[i].unauthorized = probeRejection(res[base+2])
		out[i].authorized = out[i].unauthorized == ""
		if !out[i].authorized {
			continue // not an authorized offer signer; the caller drops it (no collateral read needed)
		}
		assetCalls = append(assetCalls, chain.Call{Target: vault, Data: erc4626b.PackAsset(), AllowFailure: true})
		assetIdx = append(assetIdx, i)
	}
	if len(assetCalls) == 0 {
		return out, nil
	}

	ares, err := r.multicall(ctx, assetCalls)
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
	fundable    *big.Int         // getMaxAssets(): min(limitOf - totalAssets, vault.withdrawable), 0 if a sweep is pending
	openCount   int              // active request count (requests[] length)
	maxAssets   *big.Int         // maxAssetsPerRequest — always-active ceiling (0 = reject-all)
	minAssets   *big.Int         // minAssetsPerRequest (0 = no floor)
	minYieldPpm *big.Int         // minYieldPerRequest (ppm) — exact on-chain floor (0 = no floor)
	requests    []common.Address // requests[] — the loans this adapter has funded and not yet finalized
}

// liquidityAndExposure reads the adapter's JIT-funding headroom (getMaxAssets), its per-request caps, and
// its active-request count in a single multicall. getMaxAssets() is authoritative for funding: it already
// bounds the headroom by both the delegator's per-adapter cap AND the vault's withdrawable liquidity, so
// the bot can't sign an offer the JIT pull at consume time can't satisfy. openCount is the adapter's own
// requestsLength() (a single read) feeding the concurrency pre-screen.
func (r *reader) liquidityAndExposure(ctx context.Context, adapterAddr common.Address) (exposureState, error) {
	// getMaxAssets headroom comes from the lens when configured (it models the delegator's cross-adapter
	// deallocation cascade, which the adapter's own getter overstates); otherwise from the adapter itself.
	maxAssetsCall := chain.Call{Target: adapterAddr, Data: bfAdapter.PackGetMaxAssets()}
	if r.lens != (common.Address{}) {
		maxAssetsCall = chain.Call{Target: r.lens, Data: lensB.PackGetMaxAssets(adapterAddr)}
	}
	calls := []chain.Call{
		maxAssetsCall,
		{Target: adapterAddr, Data: bfAdapter.PackMinYieldPerRequest()},
		{Target: adapterAddr, Data: bfAdapter.PackMinAssetsPerRequest()},
		{Target: adapterAddr, Data: bfAdapter.PackMaxAssetsPerRequest()},
		{Target: adapterAddr, Data: bfAdapter.PackRequestsLength()},
	}
	res, err := r.multicall(ctx, calls)
	if err != nil {
		return exposureState{}, err
	}
	if len(res) != len(calls) {
		return exposureState{}, errors.Errorf("multicall returned %d results, want %d", len(res), len(calls))
	}
	for i, rr := range res {
		if !rr.Success {
			return exposureState{}, errors.Errorf("liquidity multicall: sub-call %d reverted: %s", i, reverts.Describe(rr.ReturnData))
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
	n := clampCount(openCount)
	// The funded request addresses let the offer cache drop offers the chain confirms as funded. A
	// partial read only leaves some funded offers live, so completeness is not required here.
	var requests []common.Address
	if n > 0 {
		rres, rerr := r.multicall(ctx, requestSlotCalls(adapterAddr, n))
		if rerr != nil {
			return exposureState{}, rerr
		}
		requests, _ = collectRequests(rres, n)
	}

	return exposureState{
		fundable:    fundable,
		openCount:   n,
		maxAssets:   maxAssets,
		minAssets:   minAssets,
		minYieldPpm: minYield,
		requests:    requests,
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
// a concurrent finalize can shrink the array between the length read and these, so individual slots may
// revert without discarding the other valid results.
func requestSlotCalls(adapterAddr common.Address, n int) []chain.Call {
	calls := make([]chain.Call, n)
	for i := range calls {
		calls[i] = chain.Call{Target: adapterAddr, AllowFailure: true, Data: bfAdapter.PackRequests(big.NewInt(int64(i)))}
	}
	return calls
}

// collectRequests decodes every valid requests(i) result, with a gap naming each slot that keeps the
// response from being a complete snapshot: a failed, malformed, or zero result, or a missing or extra one.
// Gaps do not prevent safe work on the valid subset.
func collectRequests(res []chain.CallResult, expected int) ([]common.Address, []string) {
	var gaps []string
	if len(res) != expected {
		gaps = append(gaps, fmt.Sprintf("requests(i) returned %d results, want %d", len(res), expected))
	}
	if len(res) > expected {
		res = res[:expected]
	}
	out := make([]common.Address, 0, len(res))
	for i, rr := range res {
		if !rr.Success {
			gaps = append(gaps, fmt.Sprintf("requests(%d) call failed: %s", i, reverts.Describe(rr.ReturnData)))
			continue
		}
		addr, err := bfAdapter.UnpackRequests(rr.ReturnData)
		switch {
		case err != nil:
			gaps = append(gaps, fmt.Sprintf("requests(%d) malformed: %v", i, err))
		case addr == (common.Address{}):
			gaps = append(gaps, fmt.Sprintf("requests(%d) returned the zero address", i))
		default:
			out = append(out, addr)
		}
	}
	return out, gaps
}

// readyToRedeem returns the adapter's active Requests that are currently redeemable. It reads
// requestsLength(), enumerates exactly that many requests(i), then batches every canWithdraw() into a
// single multicall. The gaps name every requested slot or canWithdraw result that was missing or
// undecodable; valid results are returned even when there are gaps.
func (r *reader) readyToRedeem(ctx context.Context, adapterAddr common.Address) ([]common.Address, []string, error) {
	lres, err := r.multicall(ctx, []chain.Call{{Target: adapterAddr, Data: bfAdapter.PackRequestsLength()}})
	if err != nil {
		return nil, nil, err
	}
	if len(lres) != 1 {
		return nil, nil, errors.Errorf("adapter.requestsLength() returned %d results, want 1", len(lres))
	}
	if !lres[0].Success {
		return nil, nil, errors.Errorf("adapter.requestsLength() reverted: %s", reverts.Describe(lres[0].ReturnData))
	}
	n, err := bfAdapter.UnpackRequestsLength(lres[0].ReturnData)
	if err != nil {
		return nil, nil, errors.Errorf("adapter.requestsLength(): %w", err)
	}
	var gaps []string
	if !n.IsInt64() || n.Sign() < 0 || n.Int64() > int64(maxRequests) {
		gaps = append(gaps, fmt.Sprintf("requestsLength %s exceeds maxRequests %d", n, maxRequests))
	}
	count := clampCount(n)
	if count == 0 {
		return nil, gaps, nil
	}

	res, err := r.multicall(ctx, requestSlotCalls(adapterAddr, count))
	if err != nil {
		return nil, nil, err
	}
	reqs, slotGaps := collectRequests(res, count)
	gaps = append(gaps, slotGaps...)
	if len(reqs) == 0 {
		return nil, gaps, nil
	}

	calls := make([]chain.Call, len(reqs))
	for i, req := range reqs {
		// AllowFailure: a single malformed Request must not break the whole batch.
		calls[i] = chain.Call{Target: req, AllowFailure: true, Data: vc.PackCanWithdraw()}
	}
	res, err = r.multicall(ctx, calls)
	if err != nil {
		return nil, nil, err
	}

	ready := make([]common.Address, 0, len(reqs))
	if len(res) != len(reqs) {
		gaps = append(gaps, fmt.Sprintf("canWithdraw returned %d results, want %d", len(res), len(reqs)))
	}
	resultCount := min(len(res), len(reqs))
	for i, rr := range res[:resultCount] {
		if !rr.Success {
			gaps = append(gaps, fmt.Sprintf("canWithdraw on %s call failed: %s", reqs[i].Hex(), reverts.Describe(rr.ReturnData)))
			continue
		}
		ok, derr := vc.UnpackCanWithdraw(rr.ReturnData)
		if derr != nil {
			gaps = append(gaps, fmt.Sprintf("canWithdraw on %s malformed: %v", reqs[i].Hex(), derr))
			continue
		}
		if ok {
			ready = append(ready, reqs[i])
		}
	}
	return ready, gaps, nil
}
