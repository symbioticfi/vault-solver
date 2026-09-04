package morpho

import "math/big"

func AccruedMarketState(market MarketState, timestamp uint64) MarketState {
	accrued := CloneMarketState(market)
	if accrued.BorrowRatePerSec == nil || accrued.BorrowRatePerSec.Sign() == 0 ||
		timestamp <= accrued.LastUpdate {
		return accrued
	}
	elapsed := new(big.Int).SetUint64(timestamp - accrued.LastUpdate)
	interest := WMulDown(
		accrued.TotalBorrowAssets,
		WTaylorCompounded(accrued.BorrowRatePerSec, elapsed),
	)
	accrued.TotalBorrowAssets.Add(accrued.TotalBorrowAssets, interest)
	accrued.TotalSupplyAssets.Add(accrued.TotalSupplyAssets, interest)
	if accrued.Fee != nil && accrued.Fee.Sign() != 0 {
		feeAssets := WMulDown(interest, accrued.Fee)
		supplyExcludingFee := new(big.Int).Sub(accrued.TotalSupplyAssets, feeAssets)
		feeShares := ToSharesDown(feeAssets, supplyExcludingFee, accrued.TotalSupplyShares)
		accrued.TotalSupplyShares.Add(accrued.TotalSupplyShares, feeShares)
	}
	accrued.LastUpdate = timestamp
	return accrued
}

func BorrowedAssetsAt(
	position PositionState,
	accruedBorrowAssets *big.Int,
	totalBorrowShares *big.Int,
) *big.Int {
	if position.BorrowShares == nil || position.BorrowShares.Sign() == 0 {
		return new(big.Int)
	}
	return ToAssetsUp(position.BorrowShares, accruedBorrowAssets, totalBorrowShares)
}

func MaxBorrow(collateral, collateralPrice, lltv *big.Int) *big.Int {
	quotedCollateral := MulDivDown(collateral, collateralPrice, oraclePriceScale)
	return WMulDown(quotedCollateral, lltv)
}

func IsLiquidatableAt(
	position PositionState,
	collateralPrice *big.Int,
	lltv *big.Int,
	accruedBorrowAssets *big.Int,
	totalBorrowShares *big.Int,
) bool {
	borrowed := BorrowedAssetsAt(position, accruedBorrowAssets, totalBorrowShares)
	return borrowed.Sign() > 0 && MaxBorrow(position.Collateral, collateralPrice, lltv).Cmp(borrowed) < 0
}
