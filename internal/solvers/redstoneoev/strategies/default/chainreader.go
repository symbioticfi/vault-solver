package defaultstrategy

import (
	"context"
	"maps"
	"math/big"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/oev/aggregator"
	"github.com/symbioticfi/vault-solver/api/bindings/oev/callback"
	irmbinding "github.com/symbioticfi/vault-solver/api/bindings/oev/irm"
	morphobinding "github.com/symbioticfi/vault-solver/api/bindings/oev/morpho"
	"github.com/symbioticfi/vault-solver/api/bindings/oev/oracle"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/morpho"
)

var (
	callbackABI = callback.NewSymbioticOevSolver()
	feedABI     = aggregator.NewAggregatorV3()
	irmABI      = irmbinding.NewAdaptiveCurveIrm()
	morphoABI   = morphobinding.NewMorpho()
	oracleABI   = oracle.NewMorphoOracle()
)

type multicaller interface {
	Multicall(ctx context.Context, calls []chain.Call) ([]chain.CallResult, error)
	MulticallAt(ctx context.Context, calls []chain.Call, blockNumber *big.Int) ([]chain.CallResult, error)
}

type chainReader struct {
	chain *chain.Client
	calls multicaller
	log   logr.Logger
}

func newChainReader(c *chain.Client, log logr.Logger) *chainReader {
	return &chainReader{
		chain: c,
		calls: c,
		log:   log,
	}
}

const maxFeedDecimals = 36

func feedDecimalsInBounds(loanDec, ethDec uint8) bool {
	return loanDec <= maxFeedDecimals && ethDec <= maxFeedDecimals
}

func feedFresh(updatedAt, nowSec, maxAge int64) bool {
	age := nowSec - updatedAt
	return age >= 0 && age <= maxAge
}

func decodeLatestRoundData(data []byte) (answer, updatedAt *big.Int, err error) {
	out, e := feedABI.UnpackLatestRoundData(data)
	if e != nil {
		return nil, nil, errors.Errorf("decode latestRoundData: %w", e)
	}
	if out.Answer == nil || out.UpdatedAt == nil {
		return nil, nil, errors.New("decode latestRoundData: nil field")
	}
	return out.Answer, out.UpdatedAt, nil
}

func decodeFeedDecimals(data []byte) (uint8, error) {
	d, err := feedABI.UnpackDecimals(data)
	if err != nil {
		return 0, errors.Errorf("decode decimals: %w", err)
	}
	return d, nil
}

func (r *chainReader) ReadLoanEthRate(ctx context.Context, loanDecimals int, feed *loanEthFeed, now time.Time) *big.Int {
	if feed == nil {
		return nil
	}
	res, err := r.calls.Multicall(ctx, []chain.Call{
		{Target: feed.LoanUsdFeed, AllowFailure: true, Data: feedABI.PackLatestRoundData()},
		{Target: feed.LoanUsdFeed, AllowFailure: true, Data: feedABI.PackDecimals()},
		{Target: feed.EthUsdFeed, AllowFailure: true, Data: feedABI.PackLatestRoundData()},
		{Target: feed.EthUsdFeed, AllowFailure: true, Data: feedABI.PackDecimals()},
	})
	if err != nil || !allSuccess(res, 4) {
		return nil
	}
	loanAns, loanUp, e1 := decodeLatestRoundData(res[0].ReturnData)
	loanDecFeed, e2 := decodeFeedDecimals(res[1].ReturnData)
	ethAns, ethUp, e3 := decodeLatestRoundData(res[2].ReturnData)
	ethDecFeed, e4 := decodeFeedDecimals(res[3].ReturnData)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		return nil
	}
	if !feedDecimalsInBounds(loanDecFeed, ethDecFeed) {
		r.log.Error(errors.New("feed decimals out of bounds"),
			"loanPerEth feed rejected", "loanFeedDec", loanDecFeed, "ethFeedDec", ethDecFeed, "max", maxFeedDecimals)
		return nil
	}
	nowSec, maxAge := now.Unix(), int64((feed.MaxAge+time.Second-1)/time.Second)
	if !feedFresh(loanUp.Int64(), nowSec, maxAge) || !feedFresh(ethUp.Int64(), nowSec, maxAge) {
		r.log.V(1).Info("loan/ETH rate feeds stale",
			"loanFeed", feed.LoanUsdFeed.Hex(), "loanAgeSec", nowSec-loanUp.Int64(),
			"ethFeed", feed.EthUsdFeed.Hex(), "ethAgeSec", nowSec-ethUp.Int64(),
			"maxAgeSec", maxAge)
		return nil
	}
	return composeLoanPerEth(ethAns, loanAns, int(ethDecFeed), int(loanDecFeed), loanDecimals)
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
	res, err := r.calls.Multicall(ctx, calls)
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
		mp, derr := decodeMarketParams(res[i].ReturnData)
		if derr != nil {
			continue
		}
		if derived, verr := deriveMarketID(mp); verr != nil || derived != id {
			r.log.V(1).Info("market id mismatch; dropping", "id", id.Hex())
			continue
		}
		out[id] = mp
	}
	return out, nil
}

// ReadMarketStatesAt reads Morpho's exact accounting tuple and corresponding IRM rate at one pinned
// block. A failed or undecodable non-zero IRM excludes that market instead of under-accruing at zero.
func (r *chainReader) ReadMarketStatesAt(
	ctx context.Context,
	morphoAddr common.Address,
	params map[common.Hash]MarketParams,
	blockNumber *big.Int,
) (map[common.Hash]morpho.MarketState, error) {
	if morphoAddr == (common.Address{}) {
		return nil, errors.New("read market states: zero Morpho address")
	}
	if blockNumber == nil || blockNumber.Sign() < 0 {
		return nil, errors.New("read market states: block number must be non-negative")
	}
	if len(params) == 0 {
		return map[common.Hash]morpho.MarketState{}, nil
	}
	block := new(big.Int).Set(blockNumber)
	ids := slices.SortedFunc(maps.Keys(params), common.Hash.Cmp)
	marketCalls := make([]chain.Call, len(ids))
	for i, id := range ids {
		marketCalls[i] = chain.Call{Target: morphoAddr, AllowFailure: true, Data: morphoABI.PackMarket(id)}
	}
	marketResults, err := r.calls.MulticallAt(ctx, marketCalls, block)
	if err != nil {
		return nil, errors.Errorf("read Morpho markets at block %s: %w", block, err)
	}
	if len(marketResults) != len(marketCalls) {
		return nil, errors.Errorf("read Morpho markets at block %s: got %d results, want %d",
			block, len(marketResults), len(marketCalls))
	}

	type rateSlot struct{ id common.Hash }
	states := make(map[common.Hash]morpho.MarketState, len(ids))
	var rateCalls []chain.Call
	var rateSlots []rateSlot
	for i, id := range ids {
		if !marketResults[i].Success {
			continue
		}
		state, ok := decodeMarketState(marketResults[i].ReturnData, params[id])
		if !ok {
			continue
		}
		if params[id].Irm == (common.Address{}) {
			state.BorrowRatePerSec = new(big.Int)
			states[id] = state
			continue
		}
		rateSlots = append(rateSlots, rateSlot{id: id})
		rateCalls = append(rateCalls, chain.Call{
			Target: params[id].Irm, AllowFailure: true,
			Data: irmABI.PackBorrowRateView(irmParams(params[id]), irmMarket(state)),
		})
		states[id] = state
	}
	if len(rateCalls) == 0 {
		return states, nil
	}
	rateResults, err := r.calls.MulticallAt(ctx, rateCalls, block)
	if err != nil {
		return nil, errors.Errorf("read Morpho IRM rates at block %s: %w", block, err)
	}
	if len(rateResults) != len(rateCalls) {
		return nil, errors.Errorf("read Morpho IRM rates at block %s: got %d results, want %d",
			block, len(rateResults), len(rateCalls))
	}
	for i, slot := range rateSlots {
		if !rateResults[i].Success {
			delete(states, slot.id)
			continue
		}
		rate, unpackErr := irmABI.UnpackBorrowRateView(rateResults[i].ReturnData)
		if unpackErr != nil || rate == nil {
			delete(states, slot.id)
			continue
		}
		state := states[slot.id]
		state.BorrowRatePerSec = rate
		states[slot.id] = state
	}
	return states, nil
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

func (r *chainReader) ReadHeaderAt(ctx context.Context, blockNumber *big.Int) (*gethtypes.Header, error) {
	if blockNumber == nil || blockNumber.Sign() < 0 {
		return nil, errors.New("block number must be non-negative")
	}
	return r.chain.HeaderByNumber(ctx, new(big.Int).Set(blockNumber))
}

func (r *chainReader) ReadCallbackMorpho(ctx context.Context, callback common.Address) (common.Address, error) {
	res, err := r.calls.Multicall(ctx, []chain.Call{
		{Target: callback, AllowFailure: true, Data: callbackABI.PackMORPHO()},
	})
	if err != nil {
		return common.Address{}, err
	}
	if len(res) != 1 || !res[0].Success {
		return common.Address{}, errors.New("callback MORPHO read failed")
	}
	morphoAddr, err := callbackABI.UnpackMORPHO(res[0].ReturnData)
	if err != nil {
		return common.Address{}, errors.Errorf("decode callback MORPHO: %w", err)
	}
	if morphoAddr == (common.Address{}) {
		return common.Address{}, errors.New("callback MORPHO unresolved")
	}
	return morphoAddr, nil
}

func (r *chainReader) ReadTestMarketStates(
	ctx context.Context,
	morphoAddr common.Address,
	params map[common.Hash]MarketParams,
	blockNumber *big.Int,
) (map[common.Hash]MarketInfo, map[common.Hash]*big.Int, error) {
	states, err := r.ReadMarketStatesAt(ctx, morphoAddr, params, blockNumber)
	if err != nil {
		return nil, nil, err
	}
	ids := slices.SortedFunc(maps.Keys(states), common.Hash.Cmp)
	if len(ids) == 0 {
		return map[common.Hash]MarketInfo{}, map[common.Hash]*big.Int{}, nil
	}
	calls := make([]chain.Call, len(ids))
	for i, id := range ids {
		calls[i] = chain.Call{Target: params[id].Oracle, AllowFailure: true, Data: oracleABI.PackPrice()}
	}
	res, err := r.calls.MulticallAt(ctx, calls, blockNumber)
	if err != nil {
		return nil, nil, err
	}
	if len(res) != len(calls) {
		return nil, nil, errors.Errorf("testMonitor prices: got %d results, want %d", len(res), len(calls))
	}
	markets := make(map[common.Hash]MarketInfo, len(ids))
	prices := make(map[common.Hash]*big.Int, len(ids))
	for i, id := range ids {
		if !res[i].Success {
			continue
		}
		price, unpackErr := oracleABI.UnpackPrice(res[i].ReturnData)
		if unpackErr != nil || price == nil || price.Sign() <= 0 {
			continue
		}
		markets[id] = MarketInfo{Params: params[id], State: states[id]}
		prices[id] = price
	}
	return markets, prices, nil
}

func (r *chainReader) ReadTestPositions(ctx context.Context, morphoAddr common.Address, markets map[common.Hash]MarketInfo, borrowers []common.Address) (map[common.Hash]map[common.Address]morpho.PositionState, error) {
	ids := sortedMarketIDsFromInfo(markets)
	calls := make([]chain.Call, 0, len(ids)*len(borrowers))
	type slot struct {
		id       common.Hash
		borrower common.Address
	}
	slots := make([]slot, 0, cap(calls))
	for _, id := range ids {
		for _, borrower := range borrowers {
			slots = append(slots, slot{id: id, borrower: borrower})
			calls = append(calls, chain.Call{Target: morphoAddr, AllowFailure: true, Data: morphoABI.PackPosition(id, borrower)})
		}
	}
	res, err := r.calls.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(res) != len(calls) {
		return nil, errors.Errorf("testMonitor positions: got %d results, want %d", len(res), len(calls))
	}
	out := make(map[common.Hash]map[common.Address]morpho.PositionState, len(ids))
	for i, s := range slots {
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
		if out[s.id] == nil {
			out[s.id] = map[common.Address]morpho.PositionState{}
		}
		out[s.id][s.borrower] = morpho.PositionState{BorrowShares: pos.BorrowShares, Collateral: pos.Collateral}
	}
	return out, nil
}

func decodeMarketParams(data []byte) (MarketParams, error) {
	out, err := morphoABI.UnpackIdToMarketParams(data)
	if err != nil {
		return MarketParams{}, errors.Errorf("decode marketParams: %w", err)
	}
	if out.Lltv == nil {
		return MarketParams{}, errors.New("decode marketParams: lltv nil")
	}
	return MarketParams{
		LoanToken: out.LoanToken, CollateralToken: out.CollateralToken,
		Oracle: out.Oracle, Irm: out.Irm, Lltv: out.Lltv,
	}, nil
}

func decodeMarketState(data []byte, params MarketParams) (morpho.MarketState, bool) {
	out, err := morphoABI.UnpackMarket(data)
	if err != nil || out.TotalSupplyAssets == nil || out.TotalSupplyShares == nil ||
		out.TotalBorrowAssets == nil || out.TotalBorrowShares == nil || out.LastUpdate == nil ||
		out.Fee == nil || params.Lltv == nil || !out.LastUpdate.IsUint64() || out.LastUpdate.Sign() <= 0 {
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
	}, true
}

func irmParams(params MarketParams) irmbinding.Struct0 {
	return irmbinding.Struct0{
		LoanToken: params.LoanToken, CollateralToken: params.CollateralToken,
		Oracle: params.Oracle, Irm: params.Irm, Lltv: params.Lltv,
	}
}

func irmMarket(state morpho.MarketState) irmbinding.Struct1 {
	return irmbinding.Struct1{
		TotalSupplyAssets: state.TotalSupplyAssets,
		TotalSupplyShares: state.TotalSupplyShares,
		TotalBorrowAssets: state.TotalBorrowAssets,
		TotalBorrowShares: state.TotalBorrowShares,
		LastUpdate:        new(big.Int).SetUint64(state.LastUpdate),
		Fee:               state.Fee,
	}
}

func sortedMarketIDs(params map[common.Hash]MarketParams) []common.Hash {
	ids := make([]common.Hash, 0, len(params))
	for id := range params {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, common.Hash.Cmp)
	return ids
}

func sortedMarketIDsFromInfo(markets map[common.Hash]MarketInfo) []common.Hash {
	ids := make([]common.Hash, 0, len(markets))
	for id := range markets {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, common.Hash.Cmp)
	return ids
}
