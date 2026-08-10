package defaultstrategy

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestVerifyAdapterPair(t *testing.T) {
	loan := common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	other := common.HexToAddress("0x1111111111111111111111111111111111111111")
	collA := common.HexToAddress("0x45804880De22913dAFE09f4980848ECE6EcbAf78")
	collB := common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")
	collX := common.HexToAddress("0x2222222222222222222222222222222222222222")

	good := common.HexToHash("0xaa")
	wrongL := common.HexToHash("0xbb")
	wrongC := common.HexToHash("0xcc")
	params := map[common.Hash]MarketParams{
		good:   {LoanToken: loan, CollateralToken: collA},
		wrongL: {LoanToken: other, CollateralToken: collB},
		wrongC: {LoanToken: loan, CollateralToken: collX},
	}
	kept := verifyAdapterPair(params, loan, []common.Address{collA, collB})
	if len(kept) != 1 || kept[0] != good {
		t.Fatalf("want exactly the matching pair, got %+v", kept)
	}
}
