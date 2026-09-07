package redstoneoev

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/oev/executor"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

var executorB = executor.NewRedStoneExecutor()

// reader owns RedStone Executor reads and maps shared LiquidLane facts into OEV strategy input.
type reader struct {
	chain *chain.Client
	ll    *liquidlane.Reader
	gas   *liquidlanegas.OracleReader
}

func newReader(
	c *chain.Client,
	log logr.Logger,
	gasCfg *liquidlanegas.OracleConfig,
	liquidityLens common.Address,
) (*reader, error) {
	var gasReader *liquidlanegas.OracleReader
	if gasCfg != nil {
		var err error
		gasReader, err = liquidlanegas.NewOracleReader(c, *gasCfg)
		if err != nil {
			return nil, err
		}
	}
	return &reader{chain: c, ll: liquidlane.NewReader(c, log, liquidityLens), gas: gasReader}, nil
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
	var state ExecutorState
	if state.Nonce, err = executorB.UnpackNonces(res[0].ReturnData); err != nil {
		return ExecutorState{}, errors.Errorf("decode executor nonce: %w", err)
	}
	if state.Deposit, err = executorB.UnpackDeposits(res[1].ReturnData); err != nil {
		return ExecutorState{}, errors.Errorf("decode executor deposit: %w", err)
	}
	if state.Locked, err = executorB.UnpackLocked(res[2].ReturnData); err != nil {
		return ExecutorState{}, errors.Errorf("decode executor lock: %w", err)
	}
	if state.Nonce == nil || !state.Nonce.IsUint64() || state.Deposit == nil || state.Deposit.Sign() < 0 {
		return ExecutorState{}, errors.New("executor state contains invalid accounting values")
	}
	return state, nil
}

// ReadAdapterSnapshot maps the shared LiquidLane snapshot to the stable OEV strategy contract.
func (r *reader) ReadAdapterSnapshot(
	ctx context.Context,
	adapterAddress common.Address,
	callback common.Address,
) (types.AdapterSnapshot, error) {
	snapshot, err := r.ll.ReadAdapterSnapshot(ctx, adapterAddress, callback)
	if err != nil {
		return types.AdapterSnapshot{}, err
	}
	// The shared reader returns owned decoded values; transfer them into the
	// strategy shape before publishing the new snapshot.
	redeemable := make([]types.RedeemableSnapshot, len(snapshot.Routes))
	for i, route := range snapshot.Routes {
		redeemable[i] = types.RedeemableSnapshot{
			Asset: route.TokenIn, Decimals: route.TokenInDecimals,
			MaxRate: route.MaxRate, MaxAssets: route.MaxAssets, AcquireBalance: route.AcquireBalance,
		}
	}
	return types.AdapterSnapshot{
		Address: snapshot.Adapter.Adapter, Vault: snapshot.Vault,
		Loan: snapshot.TokenOut, LoanDecimals: snapshot.TokenOutDecimals,
		Paused:     snapshot.Paused,
		FreeAssets: snapshot.FreeAssets, Withdrawable: snapshot.Withdrawable,
		Redeemable: redeemable, Filler: snapshot.Authorized,
	}, nil
}

// ReadGasPrices returns the shared token/native price snapshot when gas accounting is configured.
// A nil snapshot is the explicit gas-disabled mode; configured oracle failures are returned so the
// solver keeps its last coherent state and eventually fails closed on cache staleness.
func (r *reader) ReadGasPrices(
	ctx context.Context,
	adapter types.AdapterSnapshot,
	now time.Time,
) (*liquidlanegas.PriceSnapshot, error) {
	if r.gas == nil {
		return nil, nil
	}
	return r.gas.Read(ctx, []liquidlanegas.Token{{
		Address:  adapter.Loan,
		Decimals: adapter.LoanDecimals,
	}}, now)
}
