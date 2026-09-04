package morpho

import "math/big"

var (
	liquidationCursor = big.NewInt(0.3e18)
	maxLiqIncentive   = big.NewInt(1.15e18)
)

func LiquidationIncentiveFactor(lltv *big.Int) *big.Int {
	oneMinusLltv := new(big.Int).Sub(Wad, lltv)
	denominator := new(big.Int).Sub(Wad, WMulDown(liquidationCursor, oneMinusLltv))
	incentive := WDivDown(Wad, denominator)
	return minBig(incentive, maxLiqIncentive)
}

func RepaidAssetsForSeizeAt(
	seizedAssets *big.Int,
	collateralPrice *big.Int,
	incentive *big.Int,
	accruedBorrowAssets *big.Int,
	totalBorrowShares *big.Int,
) *big.Int {
	quotedSeize := MulDivUp(seizedAssets, collateralPrice, oraclePriceScale)
	repaidShares := ToSharesUp(
		WDivUp(quotedSeize, incentive),
		accruedBorrowAssets,
		totalBorrowShares,
	)
	return ToAssetsUp(repaidShares, accruedBorrowAssets, totalBorrowShares)
}

func ApplySeizeLiquidation(
	market MarketState,
	position PositionState,
	seizedAssets *big.Int,
	collateralPrice *big.Int,
) (LiquidationReplay, bool) {
	if !validLiquidationInput(market, position, seizedAssets, collateralPrice) {
		return LiquidationReplay{}, false
	}
	incentive := LiquidationIncentiveFactor(market.Lltv)
	quotedSeize := MulDivUp(seizedAssets, collateralPrice, oraclePriceScale)
	repaidShares := ToSharesUp(
		WDivUp(quotedSeize, incentive),
		market.TotalBorrowAssets,
		market.TotalBorrowShares,
	)
	repaidAssets := ToAssetsUp(repaidShares, market.TotalBorrowAssets, market.TotalBorrowShares)
	if position.BorrowShares.Cmp(repaidShares) < 0 ||
		market.TotalBorrowShares.Cmp(repaidShares) < 0 ||
		position.Collateral.Cmp(seizedAssets) < 0 {
		return LiquidationReplay{}, false
	}

	replay := LiquidationReplay{
		Market:        CloneMarketState(market),
		Position:      ClonePositionState(position),
		RepaidAssets:  repaidAssets,
		RepaidShares:  repaidShares,
		BadDebtAssets: new(big.Int),
		BadDebtShares: new(big.Int),
	}
	replay.Position.BorrowShares.Sub(replay.Position.BorrowShares, repaidShares)
	replay.Market.TotalBorrowShares.Sub(replay.Market.TotalBorrowShares, repaidShares)
	replay.Market.TotalBorrowAssets = zeroFloorSub(replay.Market.TotalBorrowAssets, repaidAssets)
	replay.Position.Collateral.Sub(replay.Position.Collateral, seizedAssets)
	if replay.Position.Collateral.Sign() != 0 {
		return replay, true
	}
	if !applyBadDebt(&replay) {
		return LiquidationReplay{}, false
	}
	return replay, true
}

func validLiquidationInput(
	market MarketState,
	position PositionState,
	seizedAssets *big.Int,
	collateralPrice *big.Int,
) bool {
	return seizedAssets != nil && seizedAssets.Sign() > 0 &&
		collateralPrice != nil && collateralPrice.Sign() > 0 &&
		market.TotalBorrowAssets != nil && market.TotalBorrowShares != nil &&
		market.TotalSupplyAssets != nil && position.BorrowShares != nil &&
		position.Collateral != nil && market.Lltv != nil
}

func applyBadDebt(replay *LiquidationReplay) bool {
	replay.BadDebtShares = new(big.Int).Set(replay.Position.BorrowShares)
	replay.BadDebtAssets = minBig(
		replay.Market.TotalBorrowAssets,
		ToAssetsUp(
			replay.BadDebtShares,
			replay.Market.TotalBorrowAssets,
			replay.Market.TotalBorrowShares,
		),
	)
	if replay.Market.TotalSupplyAssets.Cmp(replay.BadDebtAssets) < 0 ||
		replay.Market.TotalBorrowShares.Cmp(replay.BadDebtShares) < 0 {
		return false
	}
	replay.Market.TotalBorrowAssets.Sub(replay.Market.TotalBorrowAssets, replay.BadDebtAssets)
	replay.Market.TotalSupplyAssets.Sub(replay.Market.TotalSupplyAssets, replay.BadDebtAssets)
	replay.Market.TotalBorrowShares.Sub(replay.Market.TotalBorrowShares, replay.BadDebtShares)
	replay.Position.BorrowShares = new(big.Int)
	return true
}

func MaxSeizeForFullDebt(
	borrowShares *big.Int,
	collateralPrice *big.Int,
	incentive *big.Int,
	accruedBorrowAssets *big.Int,
	totalBorrowShares *big.Int,
) *big.Int {
	if borrowShares == nil || borrowShares.Sign() <= 0 ||
		collateralPrice == nil || collateralPrice.Sign() <= 0 {
		return new(big.Int)
	}
	debtAssets := ToAssetsDown(borrowShares, accruedBorrowAssets, totalBorrowShares)
	return MulDivDown(WMulDown(debtAssets, incentive), oraclePriceScale, collateralPrice)
}
