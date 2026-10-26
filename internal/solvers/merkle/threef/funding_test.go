package threef

import (
	"context"
	"math/big"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
	"github.com/symbioticfi/vault-solver/internal/merklefunding"
)

func TestObservationIncludesConfiguredSynchronousFunding(t *testing.T) {
	snapshot, recipe, tree := fixture(t)
	markets, err := inspect(snapshot, recipe, tree)
	if err != nil {
		t.Fatal(err)
	}
	market := markets[0]
	multicall := common.HexToAddress(protocolio.StandardMulticall3)
	funding := merklefunding.Config{{Account: snapshot.Account.Hex(), Asset: market.Asset.Hex(), Sources: []merklefunding.Source{{ID: common.HexToHash("0x01").Hex(), Target: common.HexToAddress("0x4444").Hex(), Data: "0x12345678"}}}}
	used := false
	chain := marketReads(t, snapshot, recipe, tree, func(_ context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
		if msg.To == nil || *msg.To != multicall || block.Cmp(snapshot.BlockNumber) != 0 {
			t.Fatalf("wrong funding observation: %v at %v", msg.To, block)
		}
		used = true
		binding, err := multicall3.Multicall3MetaData.ParseABI()
		if err != nil {
			t.Fatal(err)
		}
		return binding.Methods["aggregate3"].Outputs.Pack([]multicall3.Multicall3Result{{Success: true, ReturnData: common.LeftPadBytes(big.NewInt(73).Bytes(), 32)}, {Success: true, ReturnData: common.LeftPadBytes(big.NewInt(527).Bytes(), 32)}})
	})
	got, err := observe(t.Context(), chain, snapshot, recipe, market, funding)
	if err != nil || !used || got.AvailableFunding.Int64() != 600 || got.Budget.Int64() != 45 {
		t.Fatalf("observation: %+v, %v, funding read %v", got, err, used)
	}
}
