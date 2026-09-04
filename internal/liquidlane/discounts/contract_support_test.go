package discounts

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

const testOfferID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func testPhysicalInventory() liquidlane.Inventory {
	route := liquidlane.NewRoute(1,
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
		common.HexToAddress("0x3333333333333333333333333333333333333333"),
		common.HexToAddress("0x4444444444444444444444444444444444444444"), 6, 6)
	inventory := liquidlane.DirectInventory(route, big.NewInt(1000), big.NewInt(900_000_000_000_000_000))
	inventory.AdapterMinDiscount = big.NewInt(100_000)
	return inventory
}
