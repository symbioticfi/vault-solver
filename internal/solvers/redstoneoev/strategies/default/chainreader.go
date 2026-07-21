package defaultstrategy

import (
	"context"
	"math/big"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/chainlink/aggregator"
	"github.com/symbioticfi/vault-solver/api/bindings/oev/callback"
	morphobinding "github.com/symbioticfi/vault-solver/api/bindings/oev/morpho"
	"github.com/symbioticfi/vault-solver/api/bindings/oev/oracle"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/morpho"
)

var (
	callbackABI = callback.NewSymbioticOevSolver()
	feedABI     = aggregator.NewAggregatorV3()
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
	res, err := r.chain.Multicall(ctx, []chain.Call{
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
	res, err := r.chain.Multicall(ctx, []chain.Call{
		{Target: callback, AllowFailure: true, Data: callbackABI.PackMORPHO()},
	})
	if err != nil {
		return common.Address{}, err
	}
	if len(res) != 1 || !res[0].Success {
		return common.Address{}, nil
	}
	morphoAddr, err := callbackABI.UnpackMORPHO(res[0].ReturnData)
	if err != nil {
		return common.Address{}, errors.Errorf("decode callback MORPHO: %w", err)
	}
	return morphoAddr, nil
}

func (r *chainReader) ReadTestMarketStates(ctx context.Context, morphoAddr common.Address, params map[common.Hash]MarketParams) (map[common.Hash]MarketInfo, map[common.Hash]*big.Int, error) {
	ids := sortedMarketIDs(params)
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
	res, err := r.chain.Multicall(ctx, calls)
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
