package rfq

import (
	"math/big"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// rateScale is the adapter's fixed-point rate scale (1e18).
var rateScale = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

// solverInventory is one candidate adapter leg, taken from the backend quote request's snapshot
// (the filler does not re-read maxAssets/maxRate/decimals on-chain in the quote path). "adapter" is
// the address that fills (placed in the on-chain Swap's vault slot); "asset" is the output token.
type solverInventory struct {
	Adapter       common.Address
	Asset         common.Address
	AssetDecimals int
	MaxAssets     *big.Int
	MaxRate       *big.Int
	DiscountID    *common.Hash // nil for a direct leg; set for a discount leg
}

// strategyLeg is one filled leg of a selected strategy.
type strategyLeg struct {
	Adapter    common.Address
	AmountIn   *big.Int
	AmountOut  *big.Int
	MaxRate    *big.Int
	DiscountID *common.Hash
}

// strategyRecord is the selected execution plan for a quote, persisted by quoteId so execution can
// recover it after the backend awards the order.
type strategyRecord struct {
	QuoteID         string
	RequestID       string
	TokenIn         common.Address
	TokenOut        common.Address
	AmountIn        *big.Int
	Asset           common.Address
	AssetDecimals   int
	AssetAmountOut  *big.Int
	QuotedAmountOut *big.Int
	Legs            []strategyLeg
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// strategyRequest is the subset of a quote request the selector needs.
type strategyRequest struct {
	RequestID string
	QuoteID   string
	TokenIn   common.Address
	TokenOut  common.Address
	Amount    *big.Int
}

// selectBestStrategy picks the best single-asset strategy for the request. It is a pure function:
// oracleByAsset supplies the pre-fetched adapter.getAmountOut(tokenIn, amount) for each candidate
// asset, and `now` is injected, so it is fully unit-testable.
//
// It groups inventories by asset, evaluates each group whose asset matches tokenOut, and picks the
// highest quotedAmountOut (tie-broken by assetAmountOut). Groups are iterated in sorted asset order
// for deterministic ties.
func selectBestStrategy(
	req strategyRequest,
	inventories []solverInventory,
	tokenInDecimals int,
	oracleByAsset map[common.Address]*big.Int,
	now time.Time,
) *strategyRecord {
	groups := groupByAsset(inventories)

	keys := make([]common.Address, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Hex() < keys[j].Hex() })

	var best *strategyRecord
	for _, asset := range keys {
		oracle := oracleByAsset[asset]
		if oracle == nil {
			continue // no oracle price fetched for this asset
		}
		cand := evaluateGroup(req, groups[asset], tokenInDecimals, oracle, now)
		if cand == nil {
			continue
		}
		if best == nil ||
			cand.QuotedAmountOut.Cmp(best.QuotedAmountOut) > 0 ||
			(cand.QuotedAmountOut.Cmp(best.QuotedAmountOut) == 0 &&
				cand.AssetAmountOut.Cmp(best.AssetAmountOut) > 0) {
			best = cand
		}
	}
	return best
}

func groupByAsset(inventories []solverInventory) map[common.Address][]solverInventory {
	groups := make(map[common.Address][]solverInventory)
	for _, inv := range inventories {
		groups[inv.Asset] = append(groups[inv.Asset], inv)
	}
	return groups
}

// eligibleLeg pairs an inventory with the effective rate it's filled at.
type eligibleLeg struct {
	inv  solverInventory
	rate *big.Int
}

func evaluateGroup(
	req strategyRequest,
	group []solverInventory,
	tokenInDecimals int,
	oracleAmountOut *big.Int,
	now time.Time,
) *strategyRecord {
	asset := group[0].Asset
	assetDecimals := group[0].AssetDecimals
	if asset != req.TokenOut {
		return nil // this filler only fills when the output token is the adapter asset
	}

	// The quoted output is the adapter's oracle amountOut (no extra quote discount is applied).
	privateRate := rateForAmountOut(oracleAmountOut, req.Amount, tokenInDecimals, assetDecimals)

	eligible := make([]eligibleLeg, 0, len(group))
	for _, inv := range group {
		// Direct legs fill at our discounted private rate; discount legs fill at the adapter's
		// advertised maxRate. A direct adapter that won't honor our private rate is dropped.
		effRate := privateRate
		if inv.DiscountID != nil {
			effRate = inv.MaxRate
		}
		if effRate.Sign() <= 0 {
			continue
		}
		if inv.DiscountID == nil && inv.MaxRate.Cmp(effRate) < 0 {
			continue
		}
		eligible = append(eligible, eligibleLeg{inv: inv, rate: effRate})
	}

	sort.SliceStable(eligible, func(i, j int) bool {
		if c := eligible[i].rate.Cmp(eligible[j].rate); c != 0 {
			return c > 0
		}
		if c := eligible[i].inv.MaxAssets.Cmp(eligible[j].inv.MaxAssets); c != 0 {
			return c > 0
		}
		return eligible[i].inv.MaxRate.Cmp(eligible[j].inv.MaxRate) > 0
	})
	eligible = dedupeByAdapter(eligible)
	if len(eligible) == 0 {
		return nil
	}

	remainingIn := new(big.Int).Set(req.Amount)
	assetAmountOut := new(big.Int)
	var legs []strategyLeg
	for _, e := range eligible {
		if remainingIn.Sign() == 0 {
			break
		}
		maxAmountIn := maxAmountInForRate(e.inv.MaxAssets, e.rate, tokenInDecimals, e.inv.AssetDecimals)
		if maxAmountIn.Sign() == 0 {
			continue
		}
		saturated := remainingIn.Cmp(maxAmountIn) > 0

		var amountIn, amountOut *big.Int
		if saturated {
			amountIn = minAmountInForAmountOut(e.inv.MaxAssets, e.rate, tokenInDecimals, e.inv.AssetDecimals)
			amountOut = new(big.Int).Set(e.inv.MaxAssets)
		} else {
			amountIn = new(big.Int).Set(remainingIn)
			amountOut = amountOutForRate(amountIn, e.rate, tokenInDecimals, e.inv.AssetDecimals)
		}
		if amountOut.Sign() == 0 {
			continue
		}

		remainingIn.Sub(remainingIn, amountIn)
		assetAmountOut.Add(assetAmountOut, amountOut)
		legs = append(legs, strategyLeg{
			Adapter:    e.inv.Adapter,
			AmountIn:   amountIn,
			AmountOut:  amountOut,
			MaxRate:    e.inv.MaxRate,
			DiscountID: e.inv.DiscountID,
		})
	}
	if len(legs) == 0 {
		return nil
	}
	// Any input we couldn't place at capacity is folded into the last leg (same output, more input).
	if remainingIn.Sign() != 0 {
		last := &legs[len(legs)-1]
		last.AmountIn = new(big.Int).Add(last.AmountIn, remainingIn)
	}

	return &strategyRecord{
		QuoteID:         req.QuoteID,
		RequestID:       req.RequestID,
		TokenIn:         req.TokenIn,
		TokenOut:        req.TokenOut,
		AmountIn:        new(big.Int).Set(req.Amount),
		Asset:           asset,
		AssetDecimals:   assetDecimals,
		AssetAmountOut:  assetAmountOut,
		QuotedAmountOut: assetAmountOut,
		Legs:            legs,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// dedupeByAdapter keeps the first (highest-sorted) leg per adapter+asset pair.
func dedupeByAdapter(legs []eligibleLeg) []eligibleLeg {
	seen := make(map[string]bool, len(legs))
	out := legs[:0]
	for _, e := range legs {
		key := e.inv.Adapter.Hex() + ":" + e.inv.Asset.Hex()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	return out
}

/* ───────── fixed-point rate math ───────── */

func pow10(n int) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }

// amountOutForRate = amountIn * rate * 10^assetDec / (RATE_SCALE * 10^tokenInDec).
func amountOutForRate(amountIn, rate *big.Int, tokenInDec, assetDec int) *big.Int {
	num := new(big.Int).Mul(amountIn, rate)
	num.Mul(num, pow10(assetDec))
	den := new(big.Int).Mul(rateScale, pow10(tokenInDec))
	if den.Sign() == 0 {
		return new(big.Int)
	}
	return num.Div(num, den)
}

// maxAmountInForRate = maxAssets * RATE_SCALE * 10^tokenInDec / (rate * 10^assetDec).
func maxAmountInForRate(maxAssets, rate *big.Int, tokenInDec, assetDec int) *big.Int {
	den := new(big.Int).Mul(rate, pow10(assetDec))
	if den.Sign() == 0 {
		return new(big.Int)
	}
	num := new(big.Int).Mul(maxAssets, rateScale)
	num.Mul(num, pow10(tokenInDec))
	return num.Div(num, den)
}

// minAmountInForAmountOut = ceil(amountOut * RATE_SCALE * 10^tokenInDec / (rate * 10^assetDec)).
func minAmountInForAmountOut(amountOut, rate *big.Int, tokenInDec, assetDec int) *big.Int {
	den := new(big.Int).Mul(rate, pow10(assetDec))
	if den.Sign() == 0 {
		return new(big.Int)
	}
	num := new(big.Int).Mul(amountOut, rateScale)
	num.Mul(num, pow10(tokenInDec))
	num.Add(num, new(big.Int).Sub(den, big.NewInt(1))) // ceil
	return num.Div(num, den)
}

// rateForAmountOut = amountOut * RATE_SCALE * 10^tokenInDec / (amountIn * 10^assetDec); 0 when amountIn == 0.
func rateForAmountOut(amountOut, amountIn *big.Int, tokenInDec, assetDec int) *big.Int {
	if amountIn.Sign() == 0 {
		return new(big.Int)
	}
	num := new(big.Int).Mul(amountOut, rateScale)
	num.Mul(num, pow10(tokenInDec))
	den := new(big.Int).Mul(amountIn, pow10(assetDec))
	return num.Div(num, den)
}
