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
)

var (
	lifiInputSettler = inputsettler.NewILifiInputSettler()
)

type reader struct {
	chain     *chain.Client
	ll        *liquidlane.Reader
	gasOracle *liquidlanegas.OracleReader
}

type route = liquidlane.Route

type quoteSnapshotSet struct {
	Direct        []liquidlane.Inventory
	DiscountBases []liquidlane.Inventory
	GasSnapshot   *liquidlanegas.Snapshot
	GasPrices     *liquidlanegas.PriceSnapshot
}

type fillSnapshotSet struct {
	Direct        []liquidlane.FillQuote
	DiscountBases []liquidlane.FillQuote
	GasSnapshot   *liquidlanegas.Snapshot
	GasPrices     *liquidlanegas.PriceSnapshot
}

func newReader(c *chain.Client, log logr.Logger, gasCfg liquidlanegas.OracleConfig) (*reader, error) {
	gasOracle, err := liquidlanegas.NewOracleReader(c, gasCfg)
	if err != nil {
		return nil, err
	}
	return &reader{chain: c, ll: liquidlane.NewReader(c, log), gasOracle: gasOracle}, nil
}

func (r *reader) resolveRoutes(ctx context.Context, adapters []common.Address) ([]route, error) {
	return r.ll.ResolveRoutes(ctx, adapters)
}

func (r *reader) validateGasTokens(routes []route) error {
	return r.gasOracle.ValidateTokens(gasTokens(routes))
}

func (r *reader) quoteSnapshots(
	ctx context.Context,
	routes []route,
	executorAddr common.Address,
	chainTime time.Time,
) (quoteSnapshotSet, error) {
	all, err := r.ll.ReadInventory(ctx, routes)
	if err != nil {
		return quoteSnapshotSet{}, err
	}
	direct, err := r.ll.FilterAuthorized(ctx, all, executorAddr)
	if err != nil {
		return quoteSnapshotSet{}, err
	}
	gasSnapshot, err := r.ll.ReadGasSnapshot(ctx, routes)
	if err != nil {
		return quoteSnapshotSet{}, err
	}
	gasPrices, err := r.gasOracle.Read(ctx, gasTokens(routes), chainTime)
	if err != nil {
		return quoteSnapshotSet{}, err
	}
	return quoteSnapshotSet{Direct: direct, DiscountBases: all, GasSnapshot: gasSnapshot, GasPrices: gasPrices}, nil
}

func (r *reader) fillSnapshots(
	ctx context.Context,
	routes []route,
	executorAddr common.Address,
	tokenIn common.Address,
	amountIn *big.Int,
	chainTime time.Time,
) (fillSnapshotSet, error) {
	all, err := r.ll.ReadFillQuotes(ctx, routes, tokenIn, amountIn)
	if err != nil {
		return fillSnapshotSet{}, err
	}
	authorized, err := r.ll.FilterAuthorizedRoutes(ctx, routes, executorAddr)
	if err != nil {
		return fillSnapshotSet{}, err
	}
	directAdapters := make(map[common.Address]bool, len(authorized))
	for _, item := range authorized {
		directAdapters[item.Adapter] = true
	}
	direct := make([]liquidlane.FillQuote, 0, len(all))
	for _, quote := range all {
		if directAdapters[quote.Adapter] {
			direct = append(direct, quote)
		}
	}
	gasSnapshot, err := r.ll.ReadGasSnapshot(ctx, routes)
	if err != nil {
		return fillSnapshotSet{}, err
	}
	gasPrices, err := r.gasOracle.Read(ctx, gasTokens(routes), chainTime)
	if err != nil {
		return fillSnapshotSet{}, err
	}
	return fillSnapshotSet{Direct: direct, DiscountBases: all, GasSnapshot: gasSnapshot, GasPrices: gasPrices}, nil
}

func gasTokens(routes []route) []liquidlanegas.Token {
	tokens := make([]liquidlanegas.Token, 0, len(routes))
	for _, route := range routes {
		tokens = append(tokens, liquidlanegas.Token{Address: route.TokenOut, Decimals: route.TokenOutDecimals})
	}
	return tokens
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
	direct, err := r.ll.FilterAuthorizedRoutes(ctx, routes, executorAddr)
	if err != nil {
		return err
	}
	if len(direct) != len(routes) {
		return errors.Errorf("executor has direct filler authorization for %d of %d configured routes", len(direct), len(routes))
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
