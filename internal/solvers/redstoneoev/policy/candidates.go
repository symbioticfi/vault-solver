package policy

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/decision"
)

type evalItem struct {
	cand    Candidate
	price   *big.Int
	quote   AdapterQuote
	accrued *big.Int // totalBorrowAssets accrued to nowTs for cand's market
}

type priceLookup func(id common.Hash, info MarketInfo) *big.Int

func marketFacts(snap *snapshot, candidates []evalItem) decision.MarketFacts {
	if snap == nil {
		return decision.MarketFacts{}
	}
	facts := decision.MarketFacts{
		UpdatedAt: snap.updatedAt, Block: snap.block, BlockTime: snap.blockTime,
		HasPositions: snapshotHasPositions(snap),
		Candidates:   make([]decision.LiquidationCandidate, len(candidates)),
	}
	for index, item := range candidates {
		facts.Candidates[index] = decision.LiquidationCandidate{
			MarketID: item.cand.MarketID, Borrower: item.cand.Borrower,
			Market: item.cand.Market, Position: item.cand.Position,
			Price: cloneBig(item.price), Quote: cloneAdapterQuote(item.quote), Accrued: cloneBig(item.accrued),
		}
	}
	return facts
}

func evalItemFromCandidate(candidate decision.LiquidationCandidate) evalItem {
	return evalItem{
		cand: Candidate{
			MarketID: candidate.MarketID, Borrower: candidate.Borrower,
			Market: candidate.Market, Position: candidate.Position,
		},
		price: cloneBig(candidate.Price), quote: cloneAdapterQuote(candidate.Quote),
		accrued: cloneBig(candidate.Accrued),
	}
}

func cloneAdapterQuote(source AdapterQuote) AdapterQuote {
	return AdapterQuote{
		MaxRate: cloneBig(source.MaxRate), MaxAssets: cloneBig(source.MaxAssets),
		LoanScale: cloneBig(source.LoanScale), CollScale: cloneBig(source.CollScale),
	}
}

func candidatesFromAuctionWithAdapter(log logr.Logger, snap *snapshot, auction decision.AuctionSnapshot, nowTs uint64, adapter decision.AdapterSnapshot) []evalItem {
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

func candidatesFromSnapshot(snap *snapshot, nowTs uint64, adapter decision.AdapterSnapshot, price priceLookup) []evalItem {
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
		quote, ok := quoteForMarket(adapterQuotes, info)
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

func quoteForMarket(adapterQuotes map[common.Address]AdapterQuote, info MarketInfo) (AdapterQuote, bool) {
	quote, ok := adapterQuotes[info.Params.CollateralToken]
	return quote, ok
}

func adapterQuotesByCollateral(adapter decision.AdapterSnapshot) map[common.Address]AdapterQuote {
	if adapter.Paused || adapter.LoanDecimals < 0 {
		return nil
	}
	loanScale := chain.Exp10(adapter.LoanDecimals)
	out := make(map[common.Address]AdapterQuote, len(adapter.Redeemable))
	for _, redeemable := range adapter.Redeemable {
		if !validRedeemable(redeemable) {
			continue
		}
		out[redeemable.Asset] = AdapterQuote{
			MaxRate:   cloneBig(redeemable.MaxRate),
			MaxAssets: cloneBig(redeemable.MaxAssets),
			LoanScale: cloneBig(loanScale),
			CollScale: chain.Exp10(redeemable.Decimals),
		}
	}
	return out
}

func validRedeemable(snapshot decision.RedeemableSnapshot) bool {
	return snapshot.Asset != (common.Address{}) && snapshot.Decimals >= 0 &&
		snapshot.MaxRate != nil && snapshot.MaxRate.Sign() > 0 &&
		snapshot.MaxAssets != nil && snapshot.MaxAssets.Sign() > 0
}

func auctionPrices(log logr.Logger, a decision.AuctionSnapshot) map[common.Address]*big.Int {
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
