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
	"github.com/symbioticfi/vault-solver/api/bindings/lens"
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

// factoryAdapters returns a bounded factory entity snapshot in registry order. The registry is
// append-only, so totalEntities followed by a batched entity(i) read is a consistent enumeration.
func (r *reader) factoryAdapters(ctx context.Context, factory common.Address) ([]common.Address, error) {
	total, err := chain.ReadOne(ctx, r.chain, chain.Call{Target: factory, Data: factoryB.PackTotalEntities()}, factoryB.UnpackTotalEntities)
	if err != nil {
		return nil, errors.Errorf("adapter factory totalEntities(): %w", err)
	}
	if total == nil || total.Sign() < 0 || !total.IsInt64() || total.Int64() > maxFactoryEntities {
		return nil, errors.Errorf("adapter factory entity count %v exceeds safety limit %d", total, maxFactoryEntities)
	}
	if total.Sign() == 0 {
		return nil, nil
	}
	calls := make([]chain.Call, int(total.Int64()))
	for index := range calls {
		calls[index] = chain.Call{Target: factory, Data: factoryB.PackEntity(big.NewInt(int64(index)))}
	}
	results, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, errors.Errorf("adapter factory returned %d entities, want %d", len(results), len(calls))
	}
	addresses := make([]common.Address, len(results))
	for index, result := range results {
		address, err := chain.Decode(result, factoryB.UnpackEntity)
		if err != nil {
			return nil, errors.Errorf("adapter factory entity(%d): %w", index, err)
		}
		if address == (common.Address{}) {
			return nil, errors.Errorf("adapter factory entity(%d) is zero", index)
		}
		addresses[index] = address
	}
	return addresses, nil
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
	magic, err := chain.Decode(res, bfAdapter.UnpackIsValidSignature)
	return err == nil && magic == erc1271MagicValue
}

// errAdapterUnconfigured marks an adapter whose on-chain wiring is incomplete (a zero offerSigner,
// vault or asset). That is the normal state of a freshly deployed adapter, not a read failure, so
// callers skip it quietly instead of alerting.
var errAdapterUnconfigured = errors.New("adapter not configured")

// decodeAddr returns the non-zero address a Multicall sub-call returned, or an error tagged with
// `what` if it reverted, failed to decode, or returned zero.
func decodeAddr(res chain.CallResult, unpack func([]byte) (common.Address, error), what string) (common.Address, error) {
	addr, err := chain.Decode(res, unpack)
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
	if len(adapters) == 0 {
		return nil, nil
	}
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

	// Resolve each distinct backing vault once; several adapters may share it.
	assetCalls := make([]chain.Call, 0, len(adapters))
	assetIndex := make(map[common.Address]int)
	for i := range out {
		row := &out[i]
		row.vault, row.err = decodeAddr(res[3*i], bfAdapter.UnpackVault, "adapter.vault()")
		if row.err != nil {
			continue
		}
		row.signer, row.err = decodeAddr(res[3*i+1], bfAdapter.UnpackOfferSigner, "adapter.offerSigner()")
		if row.err != nil {
			continue
		}
		row.authorized = authorizedByProbe(res[3*i+2])
		if !row.authorized {
			continue
		}
		if _, exists := assetIndex[row.vault]; !exists {
			assetIndex[row.vault] = len(assetCalls)
			assetCalls = append(assetCalls, chain.Call{Target: row.vault, Data: erc4626b.PackAsset(), AllowFailure: true})
		}
	}
	if len(assetCalls) == 0 {
		return out, nil
	}
	assets, err := r.chain.Multicall(ctx, assetCalls)
	if err != nil {
		return nil, err
	}
	if len(assets) != len(assetCalls) {
		return nil, errors.Errorf("asset resolution returned %d results, want %d", len(assets), len(assetCalls))
	}
	for i := range out {
		row := &out[i]
		if row.err == nil && row.authorized {
			row.collateral, row.err = decodeAddr(assets[assetIndex[row.vault]], erc4626b.UnpackAsset, "vault.asset()")
		}
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
	var state exposureState
	var count *big.Int
	// Keep the requested getter, decoder and destination together. This prevents
	// positional drift when adapter limits change.
	reads := []struct {
		call        chain.Call
		decode      func([]byte) (*big.Int, error)
		destination **big.Int
	}{
		{chain.Call{Target: adapterAddr, Data: bfAdapter.PackGetMaxAssets()}, bfAdapter.UnpackGetMaxAssets, &state.fundable},
		{chain.Call{Target: adapterAddr, Data: bfAdapter.PackMinYieldPerRequest()}, bfAdapter.UnpackMinYieldPerRequest, &state.minYieldPpm},
		{chain.Call{Target: adapterAddr, Data: bfAdapter.PackMinAssetsPerRequest()}, bfAdapter.UnpackMinAssetsPerRequest, &state.minAssets},
		{chain.Call{Target: adapterAddr, Data: bfAdapter.PackMaxAssetsPerRequest()}, bfAdapter.UnpackMaxAssetsPerRequest, &state.maxAssets},
		{chain.Call{Target: adapterAddr, Data: bfAdapter.PackRequestsLength()}, bfAdapter.UnpackRequestsLength, &count},
	}
	if r.lens != (common.Address{}) {
		reads[0].call = chain.Call{Target: r.lens, Data: lensB.PackGetMaxAssets(adapterAddr)}
		reads[0].decode = lensB.UnpackGetMaxAssets
	}
	calls := make([]chain.Call, len(reads))
	for i, read := range reads {
		calls[i] = read.call
	}
	results, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return exposureState{}, err
	}
	if len(results) != len(reads) {
		return exposureState{}, errors.Errorf("multicall returned %d results, want %d", len(results), len(reads))
	}
	for i, result := range results {
		value, err := chain.Decode(result, reads[i].decode)
		if err != nil {
			return exposureState{}, errors.Errorf("liquidity multicall: decode sub-call %d: %w", i, err)
		}
		*reads[i].destination = value
	}
	state.openCount = clampCount(count)
	return state, nil
}

// clampCount converts the on-chain requestsLength (uint256, bounded by MAX_REQUESTS) to an int. A value
// that doesn't fit is clamped to maxRequests so the concurrency pre-screen fails closed.
func clampCount(n *big.Int) int {
	if n != nil && n.IsInt64() {
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

// collectRequests decodes every valid requests(i) result while reporting whether the response was a
// complete snapshot. A failed, malformed, zero, missing, or extra result makes the snapshot incomplete,
// but does not prevent safe work on the valid subset.
func collectRequests(res []chain.CallResult, expected int) ([]common.Address, bool) {
	complete := len(res) == expected
	if len(res) > expected {
		res = res[:expected]
	}
	out := make([]common.Address, 0, len(res))
	for _, rr := range res {
		addr, err := chain.Decode(rr, bfAdapter.UnpackRequests)
		if err != nil || addr == (common.Address{}) {
			complete = false
			continue
		}
		out = append(out, addr)
	}
	return out, complete
}

// readyToRedeem returns the adapter's active Requests that are currently redeemable. It reads
// requestsLength(), enumerates exactly that many requests(i), then batches every canWithdraw() into a
// single multicall. The boolean reports whether every requested slot and canWithdraw result was present
// and decodable; valid results are returned even when the snapshot is incomplete.
func (r *reader) readyToRedeem(ctx context.Context, address common.Address) ([]common.Address, bool, error) {
	count, err := chain.ReadOne(ctx, r.chain, chain.Call{Target: address, Data: bfAdapter.PackRequestsLength()}, bfAdapter.UnpackRequestsLength)
	if err != nil {
		return nil, false, errors.Errorf("adapter.requestsLength(): %w", err)
	}
	if count == nil {
		return nil, false, errors.New("adapter.requestsLength(): nil count")
	}
	complete := count.Sign() >= 0 && count.IsInt64() && count.Int64() <= maxRequests
	bounded := clampCount(count)
	if bounded == 0 {
		return nil, complete, nil
	}
	slots, err := r.chain.Multicall(ctx, requestSlotCalls(address, bounded))
	if err != nil {
		return nil, false, err
	}
	requests, allSlots := collectRequests(slots, bounded)
	complete = complete && allSlots
	if len(requests) == 0 {
		return nil, complete, nil
	}
	calls := make([]chain.Call, len(requests))
	for index, request := range requests {
		calls[index] = chain.Call{Target: request, Data: vc.PackCanWithdraw(), AllowFailure: true}
	}
	results, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, false, err
	}
	complete = complete && len(results) == len(requests)
	var ready []common.Address
	for index := range min(len(requests), len(results)) {
		result := results[index]
		withdrawable, err := chain.Decode(result, vc.UnpackCanWithdraw)
		if err != nil {
			complete = false
			continue
		}
		if withdrawable {
			ready = append(ready, requests[index])
		}
	}
	return ready, complete, nil
}
