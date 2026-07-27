package uniswapx

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	uxexecutor "github.com/symbioticfi/vault-solver/api/bindings/uniswapx/executor"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	liquidsnapshot "github.com/symbioticfi/vault-solver/internal/liquidlane/snapshot"
)

var uniswapXExecutor = uxexecutor.NewLiquidLaneUniswapXExecutor()

const maxExecutorCallers = 256

type reader struct {
	chain     *chain.Client
	snapshots *liquidsnapshot.Reader
}

type snapshot = liquidsnapshot.Quote
type fillSnapshot = liquidsnapshot.Fill

func newReader(c *chain.Client, log logr.Logger, cfg liquidlanegas.OracleConfig, liquidityLens common.Address) (*reader, error) {
	snapshots, err := liquidsnapshot.New(c, log, cfg, liquidityLens)
	if err != nil {
		return nil, err
	}
	return &reader{chain: c, snapshots: snapshots}, nil
}

func (r *reader) resolveRoutes(ctx context.Context, adapters []common.Address) ([]liquidlane.Route, error) {
	return r.snapshots.ResolveRoutes(ctx, adapters)
}

func (r *reader) validateExecutorCode(
	ctx context.Context,
	executor common.Address,
) error {
	code, err := r.chain.CodeAt(ctx, executor, nil)
	if err != nil {
		return errors.Errorf("read executor bytecode: %w", err)
	}
	return requireExecutorCode(executor, code)
}

func requireExecutorCode(executor common.Address, code []byte) error {
	if len(code) == 0 {
		return errors.Errorf("executor %s has no bytecode", executor.Hex())
	}
	return nil
}

func (r *reader) validateExecutorCaller(
	ctx context.Context,
	executor, caller common.Address,
) error {
	calls := make([]chain.Call, maxExecutorCallers)
	for i := range calls {
		calls[i] = chain.Call{
			Target:       executor,
			AllowFailure: true,
			Data:         uniswapXExecutor.PackCallers(big.NewInt(int64(i))),
		}
	}
	results, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return errors.Errorf("read executor callers: %w", err)
	}
	if len(results) != len(calls) {
		return errors.Errorf("read executor callers: got %d results, want %d", len(results), len(calls))
	}
	return requireExecutorCaller(caller, results)
}

func requireExecutorCaller(caller common.Address, results []chain.CallResult) error {
	for i, result := range results {
		if !result.Success {
			return errors.Errorf("executor caller %s is not authorized", caller.Hex())
		}
		got, err := uniswapXExecutor.UnpackCallers(result.ReturnData)
		if err != nil {
			return errors.Errorf("decode executor caller %d: %w", i, err)
		}
		if got == caller {
			return nil
		}
	}
	return errors.Errorf(
		"executor caller scan reached safety limit %d before finding %s",
		len(results),
		caller.Hex(),
	)
}

func (r *reader) unauthorizedAdapters(
	ctx context.Context,
	executor common.Address,
	routes []liquidlane.Route,
) ([]common.Address, error) {
	authorized, err := r.snapshots.FilterAuthorizedRoutes(ctx, routes, executor)
	if err != nil {
		return nil, err
	}
	return liquidlane.UnauthorizedAdapters(routes, authorized), nil
}

func (r *reader) validateGasTokens(routes []liquidlane.Route) error {
	return r.snapshots.ValidateGasTokens(routes)
}

func (r *reader) quoteSnapshot(ctx context.Context, routes []liquidlane.Route, executor common.Address, now time.Time) (snapshot, error) {
	return r.snapshots.Quote(ctx, routes, executor, now)
}

func (r *reader) fillSnapshot(
	ctx context.Context,
	routes []liquidlane.Route,
	executor, tokenIn common.Address,
	amountIn *big.Int,
	now time.Time,
) (fillSnapshot, error) {
	return r.snapshots.Fill(ctx, routes, executor, tokenIn, amountIn, now)
}

func (r *reader) physicalFillQuotes(
	ctx context.Context,
	routes []liquidlane.Route,
	tokenIn common.Address,
	amountIn *big.Int,
) ([]liquidlane.FillQuote, error) {
	return r.snapshots.ReadFillQuotes(ctx, routes, tokenIn, amountIn)
}

func (r *reader) latestBlockTime(ctx context.Context) (time.Time, error) {
	header, err := r.chain.HeaderByNumber(ctx, nil)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(int64(header.Time), 0), nil
}

func (r *reader) transactionBlockTime(ctx context.Context, txHash common.Hash) (time.Time, error) {
	receipt, err := r.chain.TransactionReceipt(ctx, txHash)
	if err != nil {
		return time.Time{}, errors.Errorf("read transaction receipt %s: %w", txHash.Hex(), err)
	}
	if receipt == nil || receipt.BlockNumber == nil || receipt.Status != types.ReceiptStatusSuccessful {
		return time.Time{}, errors.Errorf("transaction %s has no successful canonical receipt", txHash.Hex())
	}
	header, err := r.chain.HeaderByNumber(ctx, receipt.BlockNumber)
	if err != nil {
		return time.Time{}, errors.Errorf("read transaction block %s: %w", txHash.Hex(), err)
	}
	if header == nil || receipt.BlockHash != (common.Hash{}) && header.Hash() != receipt.BlockHash {
		return time.Time{}, errors.Errorf("transaction %s receipt is not canonical", txHash.Hex())
	}
	return time.Unix(int64(header.Time), 0), nil
}
