package redstoneoev

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/morpho"
)

// mustBig parses a base-10 big.Int, panicking on malformed input — a test-only literal helper.
func mustBig(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("bad big int: " + s)
	}
	return n
}

// goldenMarket is the live Sepolia test market state read on-chain (docs/OEV-PLAN.md §6.5/§6.7):
// TLOAN(6dp)/TCOL(18dp), lltv 0.86, IRM borrowRateView = 182418302 wad/sec, lastUpdate 1780059204.
func goldenMarket() morpho.MarketState {
	return morpho.MarketState{
		TotalSupplyAssets: big.NewInt(100000000068),
		TotalSupplyShares: mustBig("100000000000000000"),
		TotalBorrowAssets: big.NewInt(4730000068),
		TotalBorrowShares: mustBig("4729999932892591"),
		LastUpdate:        1780059204,
		Fee:               big.NewInt(0),
		Lltv:              mustBig("860000000000000000"),
		BorrowRatePerSec:  big.NewInt(182418302),
	}
}

// goldenBorrower is 0x629d… — 1.0 TCOL collateral, borrowShares 1685600000000000.
func goldenBorrower() morpho.PositionState {
	return morpho.PositionState{BorrowShares: mustBig("1685600000000000"), Collateral: mustBig("1000000000000000000")}
}
