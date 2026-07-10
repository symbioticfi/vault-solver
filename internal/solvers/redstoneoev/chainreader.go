package redstoneoev

import (
	"context"
	"math/big"
	"slices"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/api/bindings/oev/executor"
	"github.com/symbioticfi/vault-solver/api/bindings/vaultv2"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

var (
	executorB      = executor.NewRedStoneExecutor()
	liquidLaneRead = adapter.NewLiquidLaneAdapter()
	erc4626Read    = erc4626.NewIERC4626()
	vaultV2Read    = vaultv2.NewIVaultV2()
)

// reader performs solver-owned on-chain reads. Strategy-owned reads live in the strategy package.
type reader struct {
	chain      *chain.Client
	log        logr.Logger
	decimals   *chain.Decimals
	mu         sync.Mutex
	redeemColl map[common.Address][]common.Address
}

func newReader(c *chain.Client, log logr.Logger) *reader {
	return &reader{
		chain:      c,
		log:        log,
		decimals:   chain.NewDecimals(c),
		redeemColl: map[common.Address][]common.Address{},
	}
}

// ExecutorState is the signer's accounting on the RedStone Executor.
type ExecutorState struct {
	Nonce   *big.Int
	Deposit *big.Int
	Locked  bool
}

// ReadExecutorState reads nonces/deposits/locked for the signer in one multicall.
func (r *reader) ReadExecutorState(ctx context.Context, executorAddr, signer common.Address) (ExecutorState, error) {
	res, err := r.chain.Multicall(ctx, []chain.Call{
		{Target: executorAddr, AllowFailure: true, Data: executorB.PackNonces(signer)},
		{Target: executorAddr, AllowFailure: true, Data: executorB.PackDeposits(signer)},
		{Target: executorAddr, AllowFailure: true, Data: executorB.PackLocked(signer)},
	})
	if err != nil {
		return ExecutorState{}, err
	}
	if !allSuccess(res, 3) {
		return ExecutorState{}, errors.New("executor state read reverted")
	}
	nonce, e1 := executorB.UnpackNonces(res[0].ReturnData)
	deposit, e2 := executorB.UnpackDeposits(res[1].ReturnData)
	locked, e3 := executorB.UnpackLocked(res[2].ReturnData)
	if e1 != nil || e2 != nil || e3 != nil {
		return ExecutorState{}, errors.New("executor state decode failed")
	}
	return ExecutorState{Nonce: nonce, Deposit: deposit, Locked: locked}, nil
}

// ReadAdapterSnapshot reads the configured LiquidLane adapter context passed to every strategy.
func (r *reader) ReadAdapterSnapshot(ctx context.Context, adapterAddr, callback common.Address) (types.AdapterSnapshot, error) {
	head, err := r.readAdapterHead(ctx, adapterAddr)
	if err != nil {
		return types.AdapterSnapshot{}, err
	}
	state, err := r.readAdapterVaultState(ctx, head.Vault)
	if err != nil {
		return types.AdapterSnapshot{}, err
	}
	if state.Loan == (common.Address{}) {
		return types.AdapterSnapshot{}, errors.New("adapter loan token unresolved")
	}
	redeemable, err := r.readRedeemable(ctx, adapterAddr, head.Owner, head.MarketMaker)
	if err != nil {
		return types.AdapterSnapshot{}, err
	}
	if len(redeemable) == 0 {
		return types.AdapterSnapshot{}, errors.New("adapter redeemable collateral unresolved")
	}
	loanDecimals, err := r.fillRedeemableDecimals(ctx, state.Loan, redeemable)
	if err != nil {
		return types.AdapterSnapshot{}, err
	}
	filler, err := r.readCallbackAuthorization(ctx, adapterAddr, head, callback)
	if err != nil {
		return types.AdapterSnapshot{}, err
	}
	return types.AdapterSnapshot{
		Address:      adapterAddr,
		Vault:        head.Vault,
		Loan:         state.Loan,
		LoanDecimals: loanDecimals,
		Paused:       head.Paused,
		FreeAssets:   state.FreeAssets,
		Withdrawable: state.Withdrawable,
		Redeemable:   redeemable,
		Filler:       filler,
	}, nil
}

type adapterHead struct {
	Vault       common.Address
	Owner       common.Address
	MarketMaker common.Address
	Paused      bool
}

type adapterVaultState struct {
	Loan         common.Address
	FreeAssets   *big.Int
	Withdrawable *big.Int
}

func (r *reader) readAdapterHead(ctx context.Context, adapterAddr common.Address) (adapterHead, error) {
	res, err := r.chain.Multicall(ctx, []chain.Call{
		{Target: adapterAddr, AllowFailure: true, Data: liquidLaneRead.PackVault()},
		{Target: adapterAddr, AllowFailure: true, Data: liquidLaneRead.PackOwner()},
		{Target: adapterAddr, AllowFailure: true, Data: liquidLaneRead.PackMarketMaker()},
		{Target: adapterAddr, AllowFailure: true, Data: liquidLaneRead.PackPaused()},
	})
	if err != nil {
		return adapterHead{}, err
	}
	if !allSuccess(res, 4) {
		return adapterHead{}, errors.New("adapter head read reverted")
	}
	vault, e1 := liquidLaneRead.UnpackVault(res[0].ReturnData)
	owner, e2 := liquidLaneRead.UnpackOwner(res[1].ReturnData)
	marketMaker, e3 := liquidLaneRead.UnpackMarketMaker(res[2].ReturnData)
	paused, e4 := liquidLaneRead.UnpackPaused(res[3].ReturnData)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || vault == (common.Address{}) {
		return adapterHead{}, errors.New("adapter head decode failed")
	}
	return adapterHead{
		Vault:       vault,
		Owner:       owner,
		MarketMaker: marketMaker,
		Paused:      paused,
	}, nil
}

func (r *reader) readAdapterVaultState(ctx context.Context, vault common.Address) (adapterVaultState, error) {
	res, err := r.chain.Multicall(ctx, []chain.Call{
		{Target: vault, AllowFailure: true, Data: erc4626Read.PackAsset()},
		{Target: vault, AllowFailure: true, Data: vaultV2Read.PackFreeAssets()},
		{Target: vault, AllowFailure: true, Data: vaultV2Read.PackWithdrawable()},
	})
	if err != nil {
		return adapterVaultState{}, err
	}
	if !allSuccess(res, 3) {
		return adapterVaultState{}, errors.New("adapter vault state read reverted")
	}
	loan, e1 := erc4626Read.UnpackAsset(res[0].ReturnData)
	free, e2 := vaultV2Read.UnpackFreeAssets(res[1].ReturnData)
	withdrawable, e3 := vaultV2Read.UnpackWithdrawable(res[2].ReturnData)
	if e1 != nil || e2 != nil || e3 != nil || loan == (common.Address{}) || free == nil || withdrawable == nil {
		return adapterVaultState{}, errors.New("adapter vault state decode failed")
	}
	return adapterVaultState{
		Loan:         loan,
		FreeAssets:   free,
		Withdrawable: withdrawable,
	}, nil
}

func (r *reader) readRedeemable(ctx context.Context, adapterAddr, owner, marketMaker common.Address) ([]types.RedeemableSnapshot, error) {
	collaterals, err := r.readRedeemableCollaterals(ctx, adapterAddr)
	if err != nil {
		return nil, err
	}
	collaterals = dedupeNonZeroAddresses(collaterals)
	out := make([]types.RedeemableSnapshot, 0, len(collaterals))
	for _, coll := range collaterals {
		snap, err := r.readRedeemableSnapshot(ctx, adapterAddr, coll, owner, marketMaker)
		if err != nil {
			return nil, err
		}
		out = append(out, snap)
	}
	return out, nil
}

func (r *reader) readRedeemableSnapshot(ctx context.Context, adapterAddr, coll, owner, marketMaker common.Address) (types.RedeemableSnapshot, error) {
	calls := []chain.Call{
		{Target: adapterAddr, AllowFailure: true, Data: liquidLaneRead.PackGetMaxRate(coll)},
		{Target: adapterAddr, AllowFailure: true, Data: liquidLaneRead.PackGetMaxAssets(coll)},
		{Target: adapterAddr, AllowFailure: true, Data: liquidLaneRead.PackAcquireBalance(coll, owner)},
	}
	readMarketMakerAcquire := marketMaker != (common.Address{}) && marketMaker != owner
	if readMarketMakerAcquire {
		calls = append(calls, chain.Call{Target: adapterAddr, AllowFailure: true, Data: liquidLaneRead.PackAcquireBalance(coll, marketMaker)})
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return types.RedeemableSnapshot{}, err
	}
	if !allSuccess(res, len(calls)) {
		return types.RedeemableSnapshot{}, errors.Errorf("redeemable %s read reverted", coll.Hex())
	}
	maxRate, e1 := liquidLaneRead.UnpackGetMaxRate(res[0].ReturnData)
	maxAssets, e2 := liquidLaneRead.UnpackGetMaxAssets(res[1].ReturnData)
	acquire, e3 := liquidLaneRead.UnpackAcquireBalance(res[2].ReturnData)
	if e1 != nil || e2 != nil || e3 != nil || maxRate == nil || maxAssets == nil || acquire == nil {
		return types.RedeemableSnapshot{}, errors.Errorf("redeemable %s decode failed", coll.Hex())
	}
	if readMarketMakerAcquire {
		mmAcquire, merr := liquidLaneRead.UnpackAcquireBalance(res[3].ReturnData)
		if merr != nil || mmAcquire == nil {
			return types.RedeemableSnapshot{}, errors.Errorf("redeemable %s market-maker acquire decode failed", coll.Hex())
		}
		acquire = new(big.Int).Add(acquire, mmAcquire)
	}
	return types.RedeemableSnapshot{
		Asset:          coll,
		MaxRate:        maxRate,
		MaxAssets:      maxAssets,
		AcquireBalance: acquire,
	}, nil
}

func (r *reader) fillRedeemableDecimals(ctx context.Context, loan common.Address, redeemable []types.RedeemableSnapshot) (int, error) {
	tokens := make([]common.Address, 0, 1+len(redeemable))
	tokens = append(tokens, loan)
	for _, item := range redeemable {
		tokens = append(tokens, item.Asset)
	}
	decimals, err := r.decimals.GetMany(ctx, tokens)
	if err != nil {
		return 0, err
	}
	loanDecimals, ok := decimals[loan]
	if !ok {
		return 0, errors.Errorf("erc20.decimals() missing for loan token %s", loan.Hex())
	}
	for i := range redeemable {
		dec, hasDecimals := decimals[redeemable[i].Asset]
		if !hasDecimals {
			return 0, errors.Errorf("erc20.decimals() missing for redeemable token %s", redeemable[i].Asset.Hex())
		}
		redeemable[i].Decimals = dec
	}
	return loanDecimals, nil
}

func (r *reader) readRedeemableCollaterals(ctx context.Context, adapterAddr common.Address) ([]common.Address, error) {
	r.mu.Lock()
	c, ok := r.redeemColl[adapterAddr]
	r.mu.Unlock()
	if ok {
		return slices.Clone(c), nil
	}
	lenRes, err := r.chain.Multicall(ctx, []chain.Call{
		{Target: adapterAddr, AllowFailure: true, Data: liquidLaneRead.PackGetTokensToRedeemLength()},
	})
	if err != nil {
		return nil, err
	}
	count, ok := decodeRedeemCount(lenRes)
	if !ok {
		return nil, nil
	}
	if count == 0 {
		r.mu.Lock()
		r.redeemColl[adapterAddr] = nil
		r.mu.Unlock()
		return nil, nil
	}
	calls := make([]chain.Call, count)
	for i := range count {
		calls[i] = chain.Call{Target: adapterAddr, AllowFailure: true, Data: liquidLaneRead.PackTokensToRedeem(big.NewInt(int64(i)))}
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	toks, ok := decodeRedeemTokens(res, count)
	if !ok {
		return nil, nil
	}
	r.mu.Lock()
	r.redeemColl[adapterAddr] = slices.Clone(toks)
	r.mu.Unlock()
	return toks, nil
}

func (r *reader) readCallbackAuthorization(ctx context.Context, adapterAddr common.Address, head adapterHead, callback common.Address) (bool, error) {
	if callback == (common.Address{}) {
		return false, nil
	}
	if callback == head.Owner || callback == head.MarketMaker {
		return true, nil
	}
	if head.MarketMaker == (common.Address{}) {
		return false, nil
	}
	res, err := r.chain.Multicall(ctx, []chain.Call{
		{Target: adapterAddr, AllowFailure: true, Data: liquidLaneRead.PackIsFiller(head.MarketMaker, callback)},
	})
	if err != nil {
		return false, err
	}
	if len(res) != 1 || !res[0].Success {
		return false, nil
	}
	filler, err := liquidLaneRead.UnpackIsFiller(res[0].ReturnData)
	if err != nil {
		return false, errors.Errorf("adapter filler status decode failed: %w", err)
	}
	return filler, nil
}

func decodeRedeemCount(res []chain.CallResult) (int, bool) {
	if len(res) != 1 || !res[0].Success {
		return 0, false
	}
	n, err := liquidLaneRead.UnpackGetTokensToRedeemLength(res[0].ReturnData)
	if err != nil || n == nil || n.Sign() < 0 || !n.IsInt64() {
		return 0, false
	}
	return int(n.Int64()), true
}

func decodeRedeemTokens(res []chain.CallResult, count int) ([]common.Address, bool) {
	if len(res) != count {
		return nil, false
	}
	out := make([]common.Address, 0, count)
	for i := range res {
		if !res[i].Success {
			return nil, false
		}
		tok, err := liquidLaneRead.UnpackTokensToRedeem(res[i].ReturnData)
		if err != nil || tok == (common.Address{}) {
			return nil, false
		}
		out = append(out, tok)
	}
	return out, true
}

func dedupeNonZeroAddresses(in []common.Address) []common.Address {
	seen := make(map[common.Address]struct{}, len(in))
	out := make([]common.Address, 0, len(in))
	for _, addr := range in {
		if addr == (common.Address{}) {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	return out
}
