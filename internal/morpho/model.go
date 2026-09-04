// Package morpho reproduces Morpho Blue accounting with on-chain rounding semantics.
package morpho

import "math/big"

type MarketState struct {
	TotalSupplyAssets *big.Int
	TotalSupplyShares *big.Int
	TotalBorrowAssets *big.Int
	TotalBorrowShares *big.Int
	LastUpdate        uint64
	Fee               *big.Int
	Lltv              *big.Int
	BorrowRatePerSec  *big.Int
}

type PositionState struct {
	BorrowShares *big.Int
	Collateral   *big.Int
}

type LiquidationReplay struct {
	Market        MarketState
	Position      PositionState
	RepaidAssets  *big.Int
	RepaidShares  *big.Int
	BadDebtAssets *big.Int
	BadDebtShares *big.Int
}

func CloneMarketState(market MarketState) MarketState {
	return MarketState{
		TotalSupplyAssets: cloneBig(market.TotalSupplyAssets),
		TotalSupplyShares: cloneBig(market.TotalSupplyShares),
		TotalBorrowAssets: cloneBig(market.TotalBorrowAssets),
		TotalBorrowShares: cloneBig(market.TotalBorrowShares),
		LastUpdate:        market.LastUpdate,
		Fee:               cloneBig(market.Fee),
		Lltv:              cloneBig(market.Lltv),
		BorrowRatePerSec:  cloneBig(market.BorrowRatePerSec),
	}
}

func ClonePositionState(position PositionState) PositionState {
	return PositionState{
		BorrowShares: cloneBig(position.BorrowShares),
		Collateral:   cloneBig(position.Collateral),
	}
}

func cloneBig(value *big.Int) *big.Int {
	if value == nil {
		return nil
	}
	return new(big.Int).Set(value)
}
