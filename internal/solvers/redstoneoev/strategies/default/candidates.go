package defaultstrategy

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

type evalItem struct {
	cand  Candidate
	price *big.Int
	quote AdapterQuote
}

// Join tracked positions with auction prices and current adapter exits. The auction never
// supplies the position set, and a refreshed adapter cannot make a cached market use another loan.
func candidatesFromAuctionWithAdapter(log logr.Logger, snap *snapshot, auction types.AuctionSnapshot, nowTs uint64, adapter types.AdapterSnapshot) []evalItem {
	if snap == nil {
		return nil
	}
	prices := auctionPrices(log, auction)
	exits := adapterQuotesByCollateral(adapter)
	var out []evalItem
	for id, market := range snap.markets {
		price := prices[market.Params.Oracle]
		if price == nil || len(snap.positions[id]) == 0 {
			continue
		}
		quote, exists := snap.quotes[id]
		if adapter.Address != (common.Address{}) {
			if market.Params.LoanToken != adapter.Loan {
				continue
			}
			quote, exists = exits[market.Params.CollateralToken]
		}
		if !exists {
			continue
		}
		market.State = morpho.AccruedMarketState(market.State, nowTs)
		for borrower, position := range snap.positions[id] {
			out = append(out, evalItem{cand: Candidate{MarketID: id, Borrower: borrower, Market: market, Position: position},
				price: price, quote: quote})
		}
	}
	return out
}

func adapterQuotesByCollateral(adapter types.AdapterSnapshot) map[common.Address]AdapterQuote {
	if adapter.Paused || adapter.LoanDecimals < 0 {
		return nil
	}
	loanScale := bigmath.Exp10(adapter.LoanDecimals)
	out := make(map[common.Address]AdapterQuote, len(adapter.Redeemable))
	for _, r := range adapter.Redeemable {
		if r.Asset == (common.Address{}) || r.Decimals < 0 ||
			r.MaxRate == nil || r.MaxRate.Sign() <= 0 ||
			r.MaxAssets == nil || r.MaxAssets.Sign() <= 0 {
			continue
		}
		out[r.Asset] = AdapterQuote{
			MaxRate:   bigmath.Clone(r.MaxRate),
			MaxAssets: bigmath.Clone(r.MaxAssets),
			LoanScale: bigmath.Clone(loanScale),
			CollScale: bigmath.Exp10(r.Decimals),
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
