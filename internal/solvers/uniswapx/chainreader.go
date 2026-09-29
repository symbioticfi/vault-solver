package uniswapx

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	uxexecutor "github.com/symbioticfi/vault-solver/api/bindings/uniswapx/executor"
	uxpermit2 "github.com/symbioticfi/vault-solver/api/bindings/uniswapx/permit2"
	uxreactor "github.com/symbioticfi/vault-solver/api/bindings/uniswapx/reactor"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	liquidsnapshot "github.com/symbioticfi/vault-solver/internal/liquidlane/snapshot"
)

var (
	uniswapXExecutor = uxexecutor.NewLiquidLaneUniswapXExecutor()
	uniswapXReactor  = uxreactor.NewV2DutchOrderReactor()
	permit2Binding   = uxpermit2.NewPermit2()
)

const maxExecutorCallers = 256

type reader struct {
	chain     *chain.Client
	snapshots *liquidsnapshot.Reader
}

type snapshot = liquidsnapshot.Quote
type fillSnapshot = liquidsnapshot.Fill

func newReader(c *chain.Client, log logr.Logger, cfg *liquidlanegas.OracleConfig, liquidityLens common.Address) (*reader, error) {
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

func (r *reader) validateGasOracles(ctx context.Context, routes []liquidlane.Route) error {
	return r.snapshots.ValidateGasOracles(ctx, routes)
}

func (r *reader) validateGasTokens(routes []liquidlane.Route) error {
	return r.snapshots.ValidateGasTokens(routes)
}

func (r *reader) quoteSnapshot(ctx context.Context, routes []liquidlane.Route, executor common.Address) (snapshot, error) {
	return r.snapshots.Quote(ctx, routes, executor)
}

func (r *reader) fillSnapshot(
	ctx context.Context,
	routes []liquidlane.Route,
	executor, tokenIn common.Address,
	amountIn *big.Int,
) (fillSnapshot, error) {
	return r.snapshots.Fill(ctx, routes, executor, tokenIn, amountIn)
}

func (r *reader) physicalFillQuotes(
	ctx context.Context,
	routes []liquidlane.Route,
	tokenIn common.Address,
	amountIn *big.Int,
) ([]liquidlane.FillQuote, error) {
	return r.snapshots.ReadFillQuotes(ctx, routes, tokenIn, amountIn)
}

// reactorPermit2 reads the Permit2 contract the reactor spends order nonces through.
func (r *reader) reactorPermit2(ctx context.Context, reactor common.Address) (common.Address, error) {
	ret, err := r.chain.CallContract(ctx, ethereum.CallMsg{To: &reactor, Data: uniswapXReactor.PackPermit2()}, nil)
	if err != nil {
		return common.Address{}, errors.Errorf("call reactor permit2: %w", err)
	}
	permit2, err := uniswapXReactor.UnpackPermit2(ret)
	if err != nil {
		return common.Address{}, errors.Errorf("unpack reactor permit2: %w", err)
	}
	if permit2 == (common.Address{}) {
		return common.Address{}, errors.Errorf("reactor %s reports a zero permit2", reactor.Hex())
	}
	return permit2, nil
}

// orderNonceUsed reports whether Permit2 has spent the swapper's unordered order nonce. The reactor
// spends it on every fill and the swapper can spend it to cancel, so a set bit means no further fill
// of that order can succeed.
func (r *reader) orderNonceUsed(
	ctx context.Context, permit2, swapper common.Address, nonce *big.Int,
) (bool, error) {
	if nonce == nil || nonce.Sign() < 0 {
		return false, errors.New("order nonce must be a non-negative integer")
	}
	word, bit := permit2NonceBit(nonce)
	data, err := permit2Binding.TryPackNonceBitmap(swapper, word)
	if err != nil {
		return false, errors.Errorf("pack nonceBitmap: %w", err)
	}
	ret, err := r.chain.CallContract(ctx, ethereum.CallMsg{To: &permit2, Data: data}, nil)
	if err != nil {
		return false, errors.Errorf("call nonceBitmap: %w", err)
	}
	bitmap, err := permit2Binding.UnpackNonceBitmap(ret)
	if err != nil {
		return false, errors.Errorf("unpack nonceBitmap: %w", err)
	}
	return bitmap.Bit(bit) == 1, nil
}

// permit2NonceBit locates an unordered nonce in Permit2's bitmap the way SignatureTransfer's
// bitmapPositions does: word nonce >> 8, bit nonce & 0xff.
func permit2NonceBit(nonce *big.Int) (word *big.Int, bit int) {
	return new(big.Int).Rsh(nonce, 8), int(new(big.Int).And(nonce, big.NewInt(0xff)).Int64())
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
