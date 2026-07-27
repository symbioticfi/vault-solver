package lifi

import (
	"context"
	"math/big"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/lifi/inputsettler"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	liquidsnapshot "github.com/symbioticfi/vault-solver/internal/liquidlane/snapshot"
)

var (
	lifiInputSettler = inputsettler.NewILifiInputSettler()
)

type reader struct {
	chain     *chain.Client
	snapshots *liquidsnapshot.Reader
}

type route = liquidlane.Route

type quoteSnapshotSet = liquidsnapshot.Quote
type fillSnapshotSet = liquidsnapshot.Fill

func newReader(c *chain.Client, log logr.Logger, gasCfg liquidlanegas.OracleConfig) (*reader, error) {
	snapshots, err := liquidsnapshot.New(c, log, &gasCfg)
	if err != nil {
		return nil, err
	}
	return &reader{chain: c, snapshots: snapshots}, nil
}

func (r *reader) resolveRoutes(ctx context.Context, adapters []common.Address) ([]route, error) {
	return r.snapshots.ResolveRoutes(ctx, adapters)
}

func (r *reader) validateGasTokens(routes []route) error {
	return r.snapshots.ValidateGasTokens(routes)
}

func (r *reader) quoteSnapshots(
	ctx context.Context,
	routes []route,
	executorAddr common.Address,
	chainTime time.Time,
) (quoteSnapshotSet, error) {
	return r.snapshots.Quote(ctx, routes, executorAddr, chainTime)
}

func (r *reader) fillSnapshots(
	ctx context.Context,
	routes []route,
	executorAddr common.Address,
	tokenIn common.Address,
	amountIn *big.Int,
	chainTime time.Time,
) (fillSnapshotSet, error) {
	return r.snapshots.Fill(ctx, routes, executorAddr, tokenIn, amountIn, chainTime)
}

func (r *reader) validateExecutor(
	ctx context.Context,
	executorAddr common.Address,
	inputSettler common.Address,
	outputSettler common.Address,
	caller common.Address,
) error {
	calls := []chain.Call{
		{Target: executorAddr, Data: lifiExecutor.PackINPUTSETTLER()},
		{Target: executorAddr, Data: lifiExecutor.PackOUTPUTSETTLER()},
		{Target: executorAddr, Data: lifiExecutor.PackIsCaller(caller)},
	}
	results, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return errors.Errorf("executor configuration: %w", err)
	}
	if len(results) != len(calls) || !results[0].Success || !results[1].Success || !results[2].Success {
		return errors.New("executor configuration: unresolved")
	}
	gotInput, inputErr := lifiExecutor.UnpackINPUTSETTLER(results[0].ReturnData)
	gotOutput, outputErr := lifiExecutor.UnpackOUTPUTSETTLER(results[1].ReturnData)
	if inputErr != nil || outputErr != nil {
		return errors.New("executor immutables: malformed response")
	}
	if gotInput != inputSettler || gotOutput != outputSettler {
		return errors.Errorf("executor immutables mismatch: input=%s output=%s", gotInput.Hex(), gotOutput.Hex())
	}
	allowed, err := lifiExecutor.UnpackIsCaller(results[2].ReturnData)
	if err != nil {
		return errors.New("executor caller authorization: malformed response")
	}
	if !allowed {
		return errors.Errorf("executor caller %s is not authorized", caller.Hex())
	}
	return nil
}

func (r *reader) validateZeroGovernanceFee(ctx context.Context, inputSettler common.Address) error {
	ret, err := r.chain.CallContract(ctx, ethereum.CallMsg{
		To:   &inputSettler,
		Data: lifiInputSettler.PackGovernanceFee(),
	}, nil)
	if err != nil {
		return errors.Errorf("input settler governance fee: %w", err)
	}
	fee, err := lifiInputSettler.UnpackGovernanceFee(ret)
	if err != nil {
		return errors.Errorf("input settler governance fee: malformed response: %w", err)
	}
	if fee != 0 {
		return errors.Errorf("input settler governance fee is %d, expected zero", fee)
	}
	return nil
}

func (r *reader) validateDirectAuthorization(
	ctx context.Context,
	executorAddr common.Address,
	routes []route,
) error {
	direct, err := r.snapshots.FilterAuthorizedRoutes(ctx, routes, executorAddr)
	if err != nil {
		return err
	}
	if missing := liquidlane.UnauthorizedAdapters(routes, direct); len(missing) > 0 {
		return errors.Errorf(
			"executor %s is not authorized as direct filler for configured adapters: %v",
			executorAddr.Hex(), missing,
		)
	}
	return nil
}

func (r *reader) orderIdentifier(
	ctx context.Context,
	inputSettler common.Address,
	order inputsettler.StandardOrder,
) (common.Hash, error) {
	data, err := lifiInputSettler.TryPackOrderIdentifier(order)
	if err != nil {
		return common.Hash{}, errors.Errorf("pack orderIdentifier: %w", err)
	}
	ret, err := r.chain.CallContract(ctx, ethereum.CallMsg{To: &inputSettler, Data: data}, nil)
	if err != nil {
		return common.Hash{}, errors.Errorf("call orderIdentifier: %w", err)
	}
	orderID, err := lifiInputSettler.UnpackOrderIdentifier(ret)
	if err != nil {
		return common.Hash{}, errors.Errorf("unpack orderIdentifier: %w", err)
	}
	return common.Hash(orderID), nil
}

func (r *reader) orderStatus(ctx context.Context, inputSettler common.Address, orderID common.Hash) (uint8, error) {
	data, err := lifiInputSettler.TryPackOrderStatus(orderID)
	if err != nil {
		return 0, errors.Errorf("pack orderStatus: %w", err)
	}
	ret, err := r.chain.CallContract(ctx, ethereum.CallMsg{To: &inputSettler, Data: data}, nil)
	if err != nil {
		return 0, errors.Errorf("call orderStatus: %w", err)
	}
	status, err := lifiInputSettler.UnpackOrderStatus(ret)
	if err != nil {
		return 0, errors.Errorf("unpack orderStatus: %w", err)
	}
	return status, nil
}

func (r *reader) latestBlockNumber(ctx context.Context) (uint64, error) {
	n, err := r.chain.BlockNumber(ctx)
	if err != nil {
		return 0, errors.Errorf("block number: %w", err)
	}
	return n, nil
}

func (r *reader) latestBlockTime(ctx context.Context) (time.Time, error) {
	header, err := r.chain.HeaderByNumber(ctx, nil)
	if err != nil {
		return time.Time{}, errors.Errorf("latest block header: %w", err)
	}
	return time.Unix(int64(header.Time), 0), nil
}
