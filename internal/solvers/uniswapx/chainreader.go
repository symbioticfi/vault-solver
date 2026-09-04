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
)

var uniswapXExecutor = uxexecutor.NewLiquidLaneUniswapXExecutor()

const maxExecutorCallers = 256

type reader struct {
	*liquidlane.Reader

	chain *chain.Client
}

type snapshot = liquidlane.QuoteSnapshot
type fillSnapshot = liquidlane.FillSnapshot

func newReader(c *chain.Client, log logr.Logger, cfg *liquidlanegas.OracleConfig, liquidityLens common.Address) (*reader, error) {
	snapshots, err := liquidlane.NewReader(c, log, liquidityLens, cfg)
	if err != nil {
		return nil, err
	}
	return &reader{chain: c, Reader: snapshots}, nil
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
	calls := executorCallerCalls(executor)
	results, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return errors.Errorf("read executor callers: %w", err)
	}
	if len(results) != len(calls) {
		return errors.Errorf("read executor callers: got %d results, want %d", len(results), len(calls))
	}
	return requireExecutorCaller(caller, results)
}

func executorCallerCalls(executor common.Address) []chain.Call {
	calls := make([]chain.Call, maxExecutorCallers)
	for index := range calls {
		calls[index] = chain.Call{
			Target: executor, AllowFailure: true,
			Data: uniswapXExecutor.PackCallers(big.NewInt(int64(index))),
		}
	}
	return calls
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
	authorized, err := r.FilterAuthorizedRoutes(ctx, routes, executor)
	if err != nil {
		return nil, err
	}
	return liquidlane.UnauthorizedAdapters(routes, authorized), nil
}

func (r *reader) latestBlockTime(ctx context.Context) (time.Time, error) {
	header, err := r.chain.HeaderByNumber(ctx, nil)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(int64(header.Time), 0), nil
}

func (r *reader) transactionBlockTimeConfirmed(
	ctx context.Context,
	txHash common.Hash,
	confirmations uint64,
) (time.Time, error) {
	receipt, err := r.chain.TransactionReceipt(ctx, txHash)
	if err != nil {
		return time.Time{}, errors.Errorf("read transaction receipt %s: %w", txHash.Hex(), err)
	}
	if err := validateSuccessfulReceipt(txHash, receipt); err != nil {
		return time.Time{}, err
	}
	header, err := r.chain.HeaderByNumber(ctx, receipt.BlockNumber)
	if err != nil {
		return time.Time{}, errors.Errorf("read transaction block %s: %w", txHash.Hex(), err)
	}
	if header == nil || receipt.BlockHash != (common.Hash{}) && header.Hash() != receipt.BlockHash {
		return time.Time{}, errors.Errorf("transaction %s receipt is not canonical", txHash.Hex())
	}
	head, err := r.chain.HeaderByNumber(ctx, nil)
	if err != nil {
		return time.Time{}, errors.Errorf("read latest block for transaction %s: %w", txHash.Hex(), err)
	}
	if head == nil || head.Number == nil {
		return time.Time{}, errors.Errorf("latest block for transaction %s has no number", txHash.Hex())
	}
	if err := requireConfirmationDepth(receipt.BlockNumber, head.Number, confirmations); err != nil {
		return time.Time{}, errors.Errorf("transaction %s: %w", txHash.Hex(), err)
	}
	return time.Unix(int64(header.Time), 0), nil
}

func validateSuccessfulReceipt(txHash common.Hash, receipt *types.Receipt) error {
	if receipt == nil || receipt.BlockNumber == nil || receipt.Status != types.ReceiptStatusSuccessful {
		return errors.Errorf("transaction %s has no successful canonical receipt", txHash.Hex())
	}
	return nil
}

func requireConfirmationDepth(receiptBlock, head *big.Int, confirmations uint64) error {
	if receiptBlock == nil || head == nil {
		return errors.New("receipt and head block numbers are required")
	}
	confirmedAt := new(big.Int).Add(receiptBlock, new(big.Int).SetUint64(confirmations))
	if head.Cmp(confirmedAt) < 0 {
		return errors.Errorf(
			"%s confirmations pending at head %s",
			new(big.Int).Sub(confirmedAt, head),
			head,
		)
	}
	return nil
}
