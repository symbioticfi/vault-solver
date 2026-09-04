package policy

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/morpho"
)

func mustBig(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("bad big int: " + s)
	}
	return n
}

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

func goldenBorrower() morpho.PositionState {
	return morpho.PositionState{BorrowShares: mustBig("1685600000000000"), Collateral: mustBig("1000000000000000000")}
}

var tokenA = common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
