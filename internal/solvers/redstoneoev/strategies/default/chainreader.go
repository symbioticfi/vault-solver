package defaultstrategy

import (
	"context"
	"math/big"
	"slices"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/api/bindings/oev/aggregator"
	"github.com/symbioticfi/vault-solver/api/bindings/oev/callback"
	morphobinding "github.com/symbioticfi/vault-solver/api/bindings/oev/morpho"
	"github.com/symbioticfi/vault-solver/api/bindings/oev/oracle"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/morpho"
)

var (
	callbackABI   = callback.NewSymbioticOevSolver()
	feedABI       = aggregator.NewAggregatorV3()
	erc4626ABI    = erc4626.NewIERC4626()
	liquidLaneABI = adapter.NewLiquidLaneAdapter()
	morphoABI     = morphobinding.NewMorpho()
	oracleABI     = oracle.NewMorphoOracle()
)

type chainReader struct {
	chain    *chain.Client
	log      logr.Logger
	decimals *chain.Decimals

	mu          sync.Mutex
	adapterLoan map[common.Address]common.Address
	redeemColl  map[common.Address][]common.Address
}

func NewChainReader(c *chain.Client, log logr.Logger) Reader {
	return &chainReader{
		chain:       c,
		log:         log,
		decimals:    chain.NewDecimals(c),
		adapterLoan: map[common.Address]common.Address{},
		redeemColl:  map[common.Address][]common.Address{},
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

func (r *chainReader) ReadAdapterSnapshot(ctx context.Context, adapter common.Address) (adapterSnapshot, error) {
	loan, ok, err := r.adapterLoanToken(ctx, adapter)
	if err != nil {
		return adapterSnapshot{}, errors.Errorf("adapter loan token: %w", err)
	}
	if !ok || loan == (common.Address{}) {
		return adapterSnapshot{}, errors.New("adapter loan token unresolved")
	}
	redeemable, err := r.readRedeemableCollaterals(ctx, adapter)
	if err != nil {
		return adapterSnapshot{}, errors.Errorf("adapter redeemable collateral: %w", err)
	}
	if len(redeemable) == 0 {
		return adapterSnapshot{}, errors.New("adapter redeemable collateral unresolved")
	}
	return adapterSnapshot{Loan: loan, Redeemable: redeemable}, nil
}

func (r *chainReader) ReadLoanEthRate(ctx context.Context, adapter common.Address, feed *loanEthFeed, now time.Time) *big.Int {
	if feed == nil {
		return nil
	}
	token, ok, err := r.adapterLoanToken(ctx, adapter)
	if err != nil || !ok {
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
	loanDec, e := r.decimals.Get(ctx, token)
	if e != nil {
		return nil
	}
	return composeLoanPerEth(ethAns, loanAns, int(ethDecFeed), int(loanDecFeed), loanDec)
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
	return r.callAddress(ctx, callback, callbackABI.PackMORPHO(), callbackABI.UnpackMORPHO)
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

func (r *chainReader) adapterLoanToken(ctx context.Context, adapter common.Address) (common.Address, bool, error) {
	r.mu.Lock()
	lt, ok := r.adapterLoan[adapter]
	r.mu.Unlock()
	if ok {
		return lt, true, nil
	}
	vault, err := r.callAddress(ctx, adapter, liquidLaneABI.PackVault(), liquidLaneABI.UnpackVault)
	if err != nil {
		return common.Address{}, false, err
	}
	if vault == (common.Address{}) {
		return common.Address{}, false, nil
	}
	asset, err := r.callAddress(ctx, vault, erc4626ABI.PackAsset(), erc4626ABI.UnpackAsset)
	if err != nil {
		return common.Address{}, false, err
	}
	if asset == (common.Address{}) {
		return common.Address{}, false, nil
	}
	r.mu.Lock()
	r.adapterLoan[adapter] = asset
	r.mu.Unlock()
	return asset, true, nil
}

func (r *chainReader) callAddress(ctx context.Context, target common.Address, data []byte, unpack func([]byte) (common.Address, error)) (common.Address, error) {
	res, err := r.chain.Multicall(ctx, []chain.Call{
		{Target: target, AllowFailure: true, Data: data},
	})
	if err != nil {
		return common.Address{}, err
	}
	if len(res) != 1 || !res[0].Success {
		return common.Address{}, nil
	}
	out, derr := unpack(res[0].ReturnData)
	if derr != nil {
		out = common.Address{}
	}
	return out, nil
}

func (r *chainReader) readRedeemableCollaterals(ctx context.Context, adapter common.Address) ([]common.Address, error) {
	r.mu.Lock()
	c, ok := r.redeemColl[adapter]
	r.mu.Unlock()
	if ok {
		return slices.Clone(c), nil
	}
	lenRes, err := r.chain.Multicall(ctx, []chain.Call{
		{Target: adapter, AllowFailure: true, Data: liquidLaneABI.PackGetTokensToRedeemLength()},
	})
	if err != nil {
		return nil, err
	}
	count, ok := decodeRedeemCount(lenRes)
	if !ok {
		return nil, nil
	}
	if count == 0 {
		r.mu.Lock()
		r.redeemColl[adapter] = nil
		r.mu.Unlock()
		return nil, nil
	}
	calls := make([]chain.Call, count)
	for i := range count {
		calls[i] = chain.Call{Target: adapter, AllowFailure: true, Data: liquidLaneABI.PackTokensToRedeem(big.NewInt(int64(i)))}
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	toks, ok := decodeRedeemTokens(res, count)
	if !ok {
		return nil, nil
	}
	r.mu.Lock()
	r.redeemColl[adapter] = slices.Clone(toks)
	r.mu.Unlock()
	return toks, nil
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

func decodeRedeemCount(res []chain.CallResult) (int, bool) {
	if len(res) != 1 || !res[0].Success {
		return 0, false
	}
	n, err := liquidLaneABI.UnpackGetTokensToRedeemLength(res[0].ReturnData)
	if err != nil || n == nil || n.Sign() < 0 || !n.IsInt64() {
		return 0, false
	}
	return int(n.Int64()), true
}

func decodeRedeemTokens(res []chain.CallResult, count int) ([]common.Address, bool) {
	if len(res) != count {
		return nil, false
	}
	out := make([]common.Address, 0, count)
	for i := range res {
		if !res[i].Success {
			return nil, false
		}
		tok, err := liquidLaneABI.UnpackTokensToRedeem(res[i].ReturnData)
		if err != nil || tok == (common.Address{}) {
			return nil, false
		}
		out = append(out, tok)
	}
	return out, true
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
