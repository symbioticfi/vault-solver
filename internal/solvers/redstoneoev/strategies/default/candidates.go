package defaultstrategy

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

type evalItem struct {
	cand    Candidate
	price   *big.Int
	quote   AdapterQuote
	accrued *big.Int // totalBorrowAssets accrued to nowTs for cand's market
}

type priceLookup func(id common.Hash, info MarketInfo) *big.Int

func candidatesFromAuction(log logr.Logger, snap *snapshot, auction types.AuctionSnapshot, nowTs uint64) []evalItem {
	return candidatesFromAuctionWithAdapter(log, snap, auction, nowTs, types.AdapterSnapshot{})
}

func candidatesFromAuctionWithAdapter(log logr.Logger, snap *snapshot, auction types.AuctionSnapshot, nowTs uint64, adapter types.AdapterSnapshot) []evalItem {
	frame := auctionPrices(log, auction)
	return candidatesFromSnapshot(snap, nowTs, adapter, func(_ common.Hash, info MarketInfo) *big.Int {
		return auctionPriceForMarket(frame, info)
	})
}

func auctionPriceForMarket(frame map[common.Address]*big.Int, info MarketInfo) *big.Int {
	oracle := info.Params.Oracle
	if oracle == (common.Address{}) {
		return nil
	}
	return frame[oracle]
}

func candidatesFromSnapshot(snap *snapshot, nowTs uint64, adapter types.AdapterSnapshot, price priceLookup) []evalItem {
	if snap == nil {
		return nil
	}
	adapterQuotes := adapterQuotesByCollateral(adapter)
	var out []evalItem
	for id, info := range snap.markets {
		pos := snap.positions[id]
		if len(pos) == 0 {
			continue // no tracked positions here — skip before the price/quote/accrual work
		}
		px := price(id, info)
		if px == nil {
			continue // no settlement price for this market's oracle
		}
		quote, ok := quoteForMarket(snap, adapter, adapterQuotes, id, info)
		if !ok {
			continue // adapter doesn't serve this market (or can't price it) -> can't size an exit
		}
		accruedState := morpho.AccruedMarketState(info.State, nowTs)
		info.State = accruedState
		for b, p := range pos {
			out = append(out, evalItem{
				cand:    Candidate{MarketID: id, Borrower: b, Market: info, Position: p},
				price:   px,
				quote:   quote,
				accrued: accruedState.TotalBorrowAssets,
			})
		}
	}
	return out
}

func quoteForMarket(snap *snapshot, adapter types.AdapterSnapshot, adapterQuotes map[common.Address]AdapterQuote, id common.Hash, info MarketInfo) (AdapterQuote, bool) {
	if adapter.Address != (common.Address{}) {
		quote, ok := adapterQuotes[info.Params.CollateralToken]
		return quote, ok
	}
	if snap == nil {
		return AdapterQuote{}, false
	}
	quote, ok := snap.quotes[id]
	return quote, ok
}

func adapterQuotesByCollateral(adapter types.AdapterSnapshot) map[common.Address]AdapterQuote {
	if adapter.Paused || adapter.LoanDecimals < 0 {
		return nil
	}
	loanScale := exp10(adapter.LoanDecimals)
	out := make(map[common.Address]AdapterQuote, len(adapter.Redeemable))
	for _, r := range adapter.Redeemable {
		if r.Asset == (common.Address{}) || r.Decimals < 0 ||
			r.MaxRate == nil || r.MaxRate.Sign() <= 0 ||
			r.MaxAssets == nil || r.MaxAssets.Sign() <= 0 {
			continue
		}
		out[r.Asset] = AdapterQuote{
			MaxRate:   cloneBig(r.MaxRate),
			MaxAssets: cloneBig(r.MaxAssets),
			LoanScale: cloneBig(loanScale),
			CollScale: exp10(r.Decimals),
		}
	}
	return out
}

func auctionPrices(log logr.Logger, a types.AuctionSnapshot) map[common.Address]*big.Int {
	out := make(map[common.Address]*big.Int, len(a.Prices))
	for _, p := range a.Prices {
		if p.Oracle == (common.Address{}) {
			log.V(1).Info("dropping auction price with empty oracle address")
			continue
		}
		if p.Price == nil || p.Price.Sign() <= 0 {
			log.V(1).Info("dropping unparseable auction price", "oracle", p.Oracle.Hex())
			continue
		}
		out[p.Oracle] = new(big.Int).Set(p.Price)
	}
	return out
}
