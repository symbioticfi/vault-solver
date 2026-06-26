package redstoneoev

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/morpho"
)

type evalItem struct {
	cand    Candidate
	price   *big.Int
	quote   AdapterQuote
	accrued *big.Int // totalBorrowAssets accrued to nowTs for cand's market
}

type priceLookup func(id common.Hash, info MarketInfo) *big.Int

func candidatesFromAuction(log logr.Logger, snap *snapshot, auction AuctionMessage, nowTs uint64) []evalItem {
	frame := auctionPrices(log, auction)
	return candidatesFromSnapshot(snap, nowTs, func(_ common.Hash, info MarketInfo) *big.Int {
		return auctionPriceForMarket(frame, info)
	})
}

func candidatesFromCachedPrices(snap *snapshot, nowTs uint64) []evalItem {
	return candidatesFromSnapshot(snap, nowTs, func(id common.Hash, _ MarketInfo) *big.Int {
		return snap.prices[id]
	})
}

func auctionPriceForMarket(frame map[common.Address]*big.Int, info MarketInfo) *big.Int {
	oracle := info.Params.Oracle
	if oracle == (common.Address{}) {
		return nil
	}
	return frame[oracle]
}

func candidatesFromSnapshot(snap *snapshot, nowTs uint64, price priceLookup) []evalItem {
	if snap == nil {
		return nil
	}
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
		quote, ok := snap.quotes[id]
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

func auctionPrices(log logr.Logger, a AuctionMessage) map[common.Address]*big.Int {
	out := make(map[common.Address]*big.Int, len(a.Payload.Prices))
	for k, v := range a.Payload.Prices {
		if !common.IsHexAddress(k) {
			log.V(1).Info("dropping auction price with invalid oracle address", "oracle", k)
			continue
		}
		n, ok := new(big.Int).SetString(v, 10)
		if !ok || n.Sign() <= 0 {
			log.V(1).Info("dropping unparseable auction price", "oracle", k, "value", v)
			continue
		}
		out[common.HexToAddress(k)] = n
	}
	return out
}

// scoredLegs is I/O-free: it reads only the monitor snapshot and sizes one leg per liquidatable candidate.
func (s *Solver) scoredLegs(a AuctionMessage, now time.Time) []scoredLeg {
	nowTs := clampTsAt(a.Timestamp, now)
	cands := s.mon.candidates(a, nowTs)
	out := make([]scoredLeg, 0, len(cands))
	for _, it := range cands {
		if leg, profit, ok := sizeLeg(it.cand, it.price, it.quote, it.accrued, s.cfg.Sizing); ok {
			out = append(out, scoredLeg{
				leg:        leg,
				profit:     profit,
				collateral: it.cand.Market.Params.CollateralToken,
				maxAssets:  it.quote.MaxAssets, // legs sharing this collateral share its getMaxAssets budget
				source:     it,
				replay:     true,
			})
		}
	}
	return out
}
