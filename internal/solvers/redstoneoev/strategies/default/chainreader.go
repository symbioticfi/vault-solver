package defaultstrategy

import (
	"context"
	"maps"
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/oev/callback"
	morphobinding "github.com/symbioticfi/vault-solver/api/bindings/oev/morpho"
	"github.com/symbioticfi/vault-solver/api/bindings/oev/oracle"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/morpho"
)

var (
	callbackABI = callback.NewSymbioticOevSolver()
	morphoABI   = morphobinding.NewMorpho()
	oracleABI   = oracle.NewMorphoOracle()
)

type chainReader struct {
	chain *chain.Client
	log   logr.Logger
}

func newChainReader(c *chain.Client, log logr.Logger) *chainReader {
	return &chainReader{
		chain: c,
		log:   log,
	}
}

func (r *chainReader) ReadNativeBalance(ctx context.Context, account common.Address) (*big.Int, error) {
	return r.chain.BalanceAt(ctx, account, nil)
}

func (r *chainReader) ResolveParams(ctx context.Context, morphoAddr common.Address, ids []common.Hash) (map[common.Hash]MarketParams, error) {
	if len(ids) == 0 {
		return map[common.Hash]MarketParams{}, nil
	}
	calls := make([]chain.Call, len(ids))
	for i, id := range ids {
		calls[i] = chain.Call{Target: morphoAddr, AllowFailure: true, Data: morphoABI.PackIdToMarketParams(id)}
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(res) != len(calls) {
		return nil, errors.Errorf("resolveParams: got %d results, want %d", len(res), len(calls))
	}
	out := make(map[common.Hash]MarketParams, len(ids))
	for i, id := range ids {
		if !res[i].Success {
			continue
		}
		decoded, decodeErr := morphoABI.UnpackIdToMarketParams(res[i].ReturnData)
		if decodeErr != nil || decoded.Lltv == nil {
			continue
		}
		mp := MarketParams(decoded)
		if derived, verr := deriveMarketID(mp); verr != nil || derived != id {
			r.log.V(1).Info("market id mismatch; dropping", "id", id.Hex())
			continue
		}
		out[id] = mp
	}
	return out, nil
}

func (r *chainReader) ReadHead(ctx context.Context) (number uint64, timestamp uint64, err error) {
	header, err := r.chain.HeaderByNumber(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	if header == nil || header.Number == nil || !header.Number.IsUint64() {
		return 0, 0, errors.New("header unavailable")
	}
	return header.Number.Uint64(), header.Time, nil
}

func (r *chainReader) ReadCallbackMorpho(ctx context.Context, callback common.Address) (common.Address, error) {
	address, err := chain.ReadOne(ctx, r.chain, chain.Call{Target: callback, AllowFailure: true, Data: callbackABI.PackMORPHO()}, callbackABI.UnpackMORPHO)
	if err != nil {
		return common.Address{}, errors.Errorf("read callback MORPHO: %w", err)
	}
	return address, nil
}

func (r *chainReader) ReadTestMarketStates(ctx context.Context, morphoAddr common.Address, params map[common.Hash]MarketParams) (map[common.Hash]MarketInfo, map[common.Hash]*big.Int, error) {
	ids := slices.SortedFunc(maps.Keys(params), common.Hash.Cmp)
	calls := make([]chain.Call, 0, len(ids)*2)
	for _, id := range ids {
		p := params[id]
		calls = append(calls,
			chain.Call{Target: morphoAddr, AllowFailure: true, Data: morphoABI.PackMarket(id)},
			chain.Call{Target: p.Oracle, AllowFailure: true, Data: oracleABI.PackPrice()},
		)
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, nil, err
	}
	if len(res) != len(calls) {
		return nil, nil, errors.Errorf("testMonitor markets: got %d results, want %d", len(res), len(calls))
	}
	markets := make(map[common.Hash]MarketInfo, len(ids))
	prices := make(map[common.Hash]*big.Int, len(ids))
	for i, id := range ids {
		marketRes := res[i*2]
		priceRes := res[i*2+1]
		if !marketRes.Success || !priceRes.Success {
			continue
		}
		state, ok := decodeTestMarketState(marketRes.ReturnData, params[id])
		if !ok {
			continue
		}
		price, err := oracleABI.UnpackPrice(priceRes.ReturnData)
		if err != nil || price == nil || price.Sign() <= 0 {
			continue
		}
		markets[id] = MarketInfo{Params: params[id], State: state}
		prices[id] = price
	}
	return markets, prices, nil
}

func (r *chainReader) ReadTestPositions(ctx context.Context, morphoAddr common.Address, markets map[common.Hash]MarketInfo, borrowers []common.Address) (map[common.Hash]map[common.Address]morpho.PositionState, error) {
	ids := slices.SortedFunc(maps.Keys(markets), common.Hash.Cmp)
	calls := make([]chain.Call, 0, len(ids)*len(borrowers))
	for _, id := range ids {
		for _, borrower := range borrowers {
			calls = append(calls, chain.Call{Target: morphoAddr, AllowFailure: true, Data: morphoABI.PackPosition(id, borrower)})
		}
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(res) != len(calls) {
		return nil, errors.Errorf("testMonitor positions: got %d results, want %d", len(res), len(calls))
	}
	out := make(map[common.Hash]map[common.Address]morpho.PositionState, len(ids))
	for i := range res {
		if !res[i].Success {
			continue
		}
		pos, err := morphoABI.UnpackPosition(res[i].ReturnData)
		if err != nil {
			continue
		}
		if pos.BorrowShares == nil || pos.Collateral == nil || (pos.BorrowShares.Sign() == 0 && pos.Collateral.Sign() == 0) {
			continue
		}
		id, borrower := ids[i/len(borrowers)], borrowers[i%len(borrowers)]
		if out[id] == nil {
			out[id] = map[common.Address]morpho.PositionState{}
		}
		out[id][borrower] = morpho.PositionState{BorrowShares: pos.BorrowShares, Collateral: pos.Collateral}
	}
	return out, nil
}

func decodeTestMarketState(data []byte, params MarketParams) (morpho.MarketState, bool) {
	out, err := morphoABI.UnpackMarket(data)
	if err != nil || out.TotalSupplyAssets == nil || out.TotalSupplyShares == nil ||
		out.TotalBorrowAssets == nil || out.TotalBorrowShares == nil || out.LastUpdate == nil ||
		out.Fee == nil || params.Lltv == nil || !out.LastUpdate.IsUint64() {
		return morpho.MarketState{}, false
	}
	return morpho.MarketState{
		TotalSupplyAssets: out.TotalSupplyAssets,
		TotalSupplyShares: out.TotalSupplyShares,
		TotalBorrowAssets: out.TotalBorrowAssets,
		TotalBorrowShares: out.TotalBorrowShares,
		LastUpdate:        out.LastUpdate.Uint64(),
		Fee:               out.Fee,
		Lltv:              params.Lltv,
		BorrowRatePerSec:  big.NewInt(0),
	}, true
}
