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

type stateReader interface {
	ReadExecutorState(ctx context.Context, executor, signer common.Address) (ExecutorState, error)
	ReadAdapterSnapshot(ctx context.Context, adapter, callback common.Address) (types.AdapterSnapshot, error)
	ReadGasPrices(ctx context.Context, adapter types.AdapterSnapshot, now time.Time) (*liquidlanegas.PriceSnapshot, error)
}

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
	nonce, nonceErr := executorB.UnpackNonces(res[0].ReturnData)
	deposit, depositErr := executorB.UnpackDeposits(res[1].ReturnData)
	locked, lockedErr := executorB.UnpackLocked(res[2].ReturnData)
	if nonceErr != nil || depositErr != nil || lockedErr != nil {
		return ExecutorState{}, errors.New("executor state decode failed")
	}
	return ExecutorState{Nonce: nonce, Deposit: deposit, Locked: locked}, nil
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
	redeemable := make([]types.RedeemableSnapshot, 0, len(snapshot.Routes))
	for _, route := range snapshot.Routes {
		redeemable = append(redeemable, types.RedeemableSnapshot{
			Asset: route.TokenIn, Decimals: route.TokenInDecimals,
			MaxRate: liquidlane.CloneBig(route.MaxRate), MaxAssets: liquidlane.CloneBig(route.MaxAssets),
			AcquireBalance: liquidlane.CloneBig(route.AcquireBalance),
		})
	}
	return types.AdapterSnapshot{
		Address: snapshot.Adapter.Adapter, Vault: snapshot.Vault,
		Loan: snapshot.TokenOut, LoanDecimals: snapshot.TokenOutDecimals,
		Paused:     snapshot.Paused,
		FreeAssets: liquidlane.CloneBig(snapshot.FreeAssets), Withdrawable: liquidlane.CloneBig(snapshot.Withdrawable),
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
