package bridgefacilitator

import (
	"context"
	"math"
	"math/big"
	"sync"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/api/bindings/3f/adapter"
	"github.com/symbioticfi/vault-solver/api/bindings/3f/vaultcontroller"
	"github.com/symbioticfi/vault-solver/api/bindings/delegator"
	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/api/bindings/vaultv2"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

// Contract bindings (abigen --v2): typed Pack/Unpack helpers for the Multicall3 sub-calls below, so an
// ABI change fails at compile time (see CLAUDE.md "Code generation").
//
// In the redesigned core-mirror model the funding cap and asset live on different contracts than the
// vault: the collateral token is read via IERC4626(vault).asset(), and the per-adapter cap via
// UniversalDelegator(vault.delegator()).limitOf(adapter). vaultV2 is therefore only used for the
// vault.delegator() and vault.withdrawable() lookups.
var (
	bfAdapter = adapter.NewBridgeFacilitatorAdapter()
	vaultV2   = vaultv2.NewIVaultV2()
	vc        = vaultcontroller.NewIVaultController()
	erc4626b  = erc4626.NewIERC4626()
	deleg     = delegator.NewUniversalDelegator()
)

// reader performs the adapter- and Request-side on-chain reads the solver relies on, batching via
// Multicall3 where calls are independent.
type reader struct {
	chain *chain.Client

	// delegatorMu guards the per-vault delegator-address cache. Reader methods are invoked serially
	// from the solver's single ticker loop, but the cache is guarded anyway so it stays correct if
	// that ever changes.
	delegatorMu    sync.Mutex
	delegatorCache map[common.Address]common.Address
}

func newReader(c *chain.Client) *reader {
	return &reader{chain: c, delegatorCache: make(map[common.Address]common.Address)}
}

// adapterVault returns the vault the adapter funds (bound once at adapter initialize), so config
// carries only the adapter address and the bot derives the vault at startup.
func (r *reader) adapterVault(ctx context.Context, adapterAddr common.Address) (common.Address, error) {
	res, err := r.chain.Multicall(ctx, []chain.Call{{Target: adapterAddr, Data: bfAdapter.PackVault()}})
	if err != nil {
		return common.Address{}, err
	}
	if len(res) != 1 || !res[0].Success {
		return common.Address{}, errors.New("adapter.vault() reverted")
	}
	return bfAdapter.UnpackVault(res[0].ReturnData)
}

// vaultAsset returns the vault's collateral token, used to match auctions (by deposit asset) to this
// funding vault. In the core-mirror VaultV2 the deposit/collateral token is the ERC-4626 asset, so
// this reads IERC4626(vault).asset() (the old vault.collateral() no longer exists).
func (r *reader) vaultAsset(ctx context.Context, vault common.Address) (common.Address, error) {
	res, err := r.chain.Multicall(ctx, []chain.Call{{Target: vault, Data: erc4626b.PackAsset()}})
	if err != nil {
		return common.Address{}, err
	}
	if len(res) != 1 || !res[0].Success {
		return common.Address{}, errors.New("vault.asset() reverted")
	}
	return erc4626b.UnpackAsset(res[0].ReturnData)
}

// vaultDelegator resolves the vault's delegator address (the contract that holds the per-adapter
// allocation caps via limitOf). It is read once per vault and cached: a Multicall can't feed one
// call's result into another, so liquidityAndExposure needs the delegator address up front, and the
// vault's delegator is effectively fixed for the bot's lifetime. A cache miss falls through to a
// single eth_call.
func (r *reader) vaultDelegator(ctx context.Context, vault common.Address) (common.Address, error) {
	r.delegatorMu.Lock()
	if d, ok := r.delegatorCache[vault]; ok {
		r.delegatorMu.Unlock()
		return d, nil
	}
	r.delegatorMu.Unlock()

	res, err := r.chain.Multicall(ctx, []chain.Call{{Target: vault, Data: vaultV2.PackDelegator()}})
	if err != nil {
		return common.Address{}, err
	}
	if len(res) != 1 || !res[0].Success {
		return common.Address{}, errors.New("vault.delegator() reverted")
	}
	d, err := vaultV2.UnpackDelegator(res[0].ReturnData)
	if err != nil {
		return common.Address{}, err
	}

	r.delegatorMu.Lock()
	r.delegatorCache[vault] = d
	r.delegatorMu.Unlock()
	return d, nil
}

// exposureState bundles the per-target liquidity and the adapter's exposure caps the offer sizer needs.
// The four caps are the adapter's authoritative on-chain risk limits (setExposureLimits, each 0 =
// disabled); the bot reads them to pre-screen offers before the contract enforces them at consume time.
type exposureState struct {
	fundable      *big.Int // max(min(limitOf - totalAssets, vault.withdrawable), 0)
	outstanding   *big.Int // outstandingPrincipal (live sleeve exposure), clamped >= 0
	openCount     int      // len(activeRequests)
	perRequestMax *big.Int // perRequestMaxCollateral (0 = no limit)
	totalMax      *big.Int // totalMaxCollateral (0 = no limit)
	minYieldBps   *big.Int // minRequestYieldBps (0 = no floor)
	maxConcurrent int      // maxConcurrentLoans (0 = no limit; an unrepresentable value is treated as none)
}

// liquidityAndExposure reads the JIT-funding headroom plus the adapter's exposure caps in one multicall.
// Funding is just-in-time: at consume time the adapter pulls the principal via the delegator's
// allocateExact, which can raise at most the vault's withdrawable liquidity. So fundable is bounded by
// BOTH the per-adapter cap headroom AND vault.withdrawable() — otherwise the bot could sign an offer the
// JIT pull can't satisfy and the consume would revert (mirrors LiquidLane's getMaxAssets). A Multicall
// can't chain one call's result into another, so the delegator address is resolved first (cached; see
// vaultDelegator), then everything is read in a single batched multicall.
func (r *reader) liquidityAndExposure(
	ctx context.Context, vault, adapterAddr common.Address,
) (exposureState, error) {
	delegatorAddr, err := r.vaultDelegator(ctx, vault)
	if err != nil {
		return exposureState{}, errors.Errorf("resolve vault delegator: %w", err)
	}

	calls := []chain.Call{
		{Target: delegatorAddr, Data: deleg.PackLimitOf(adapterAddr)},
		{Target: adapterAddr, Data: bfAdapter.PackTotalAssets()},
		{Target: adapterAddr, Data: bfAdapter.PackOutstandingPrincipal()},
		{Target: adapterAddr, Data: bfAdapter.PackActiveRequests()},
		{Target: vault, Data: vaultV2.PackWithdrawable()},
		{Target: adapterAddr, Data: bfAdapter.PackPerRequestMaxCollateral()},
		{Target: adapterAddr, Data: bfAdapter.PackTotalMaxCollateral()},
		{Target: adapterAddr, Data: bfAdapter.PackMinRequestYieldBps()},
		{Target: adapterAddr, Data: bfAdapter.PackMaxConcurrentLoans()},
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

	limit, err := deleg.UnpackLimitOf(res[0].ReturnData)
	if err != nil {
		return exposureState{}, err
	}
	held, err := bfAdapter.UnpackTotalAssets(res[1].ReturnData)
	if err != nil {
		return exposureState{}, err
	}
	outstandingPrincipal, err := bfAdapter.UnpackOutstandingPrincipal(res[2].ReturnData)
	if err != nil {
		return exposureState{}, err
	}
	reqs, err := bfAdapter.UnpackActiveRequests(res[3].ReturnData)
	if err != nil {
		return exposureState{}, err
	}
	withdrawable, err := vaultV2.UnpackWithdrawable(res[4].ReturnData)
	if err != nil {
		return exposureState{}, err
	}
	perRequestMax, err := bfAdapter.UnpackPerRequestMaxCollateral(res[5].ReturnData)
	if err != nil {
		return exposureState{}, err
	}
	totalMax, err := bfAdapter.UnpackTotalMaxCollateral(res[6].ReturnData)
	if err != nil {
		return exposureState{}, err
	}
	minYieldBps, err := bfAdapter.UnpackMinRequestYieldBps(res[7].ReturnData)
	if err != nil {
		return exposureState{}, err
	}
	maxConcurrent, err := bfAdapter.UnpackMaxConcurrentLoans(res[8].ReturnData)
	if err != nil {
		return exposureState{}, err
	}

	fundable, outstanding := deriveLiquidity(limit, held, outstandingPrincipal, withdrawable)
	return exposureState{
		fundable:      fundable,
		outstanding:   outstanding,
		openCount:     len(reqs),
		perRequestMax: perRequestMax,
		totalMax:      totalMax,
		minYieldBps:   minYieldBps,
		maxConcurrent: loanCount(maxConcurrent),
	}, nil
}

// loanCount converts the on-chain maxConcurrentLoans uint256 to the sizer's int. 0 (disabled) and any
// value too large to represent both mean "no concurrency limit".
func loanCount(n *big.Int) int {
	if n.IsInt64() {
		if v := n.Int64(); v > 0 && v <= math.MaxInt32 {
			return int(v)
		}
	}
	return 0
}

// deriveLiquidity reduces the raw on-chain reads to the sizer's inputs. Split out as a pure helper so
// the clamping is unit-testable without a chain backend:
//
//	fundable    = max(min(limit - held, withdrawable), 0)   // cap headroom AND vault JIT-pull liquidity
//	outstanding = max(outstandingPrincipal, 0)
func deriveLiquidity(limit, held, outstandingPrincipal, withdrawable *big.Int) (fundable, outstanding *big.Int) {
	fundable = new(big.Int).Sub(limit, held)
	if fundable.Cmp(withdrawable) > 0 {
		fundable = new(big.Int).Set(withdrawable)
	}
	if fundable.Sign() < 0 {
		fundable.SetInt64(0)
	}
	outstanding = new(big.Int).Set(outstandingPrincipal)
	if outstanding.Sign() < 0 {
		outstanding.SetInt64(0)
	}
	return fundable, outstanding
}

// readyToRedeem returns the adapter's active Requests that are currently redeemable. It reads the
// active set (one call) then batches every canWithdraw() into a single multicall.
func (r *reader) readyToRedeem(ctx context.Context, adapterAddr common.Address) ([]common.Address, error) {
	areq, err := r.chain.Multicall(ctx, []chain.Call{{Target: adapterAddr, Data: bfAdapter.PackActiveRequests()}})
	if err != nil {
		return nil, err
	}
	if len(areq) != 1 || !areq[0].Success {
		return nil, errors.New("adapter.activeRequests() reverted")
	}
	reqs, err := bfAdapter.UnpackActiveRequests(areq[0].ReturnData)
	if err != nil {
		return nil, errors.Errorf("adapter.activeRequests(): %w", err)
	}
	if len(reqs) == 0 {
		return nil, nil
	}

	calls := make([]chain.Call, len(reqs))
	for i, req := range reqs {
		// AllowFailure: a single malformed Request must not break the whole batch.
		calls[i] = chain.Call{Target: req, AllowFailure: true, Data: vc.PackCanWithdraw()}
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
