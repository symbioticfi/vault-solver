package threef

import (
	"context"
	"math/big"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/merklefunding"
)

func TestObservationIncludesConfiguredSynchronousFunding(t *testing.T) {
	snapshot, recipe, tree := fixture(t)
	markets, err := inspect(snapshot, recipe, tree)
	if err != nil {
		t.Fatal(err)
	}
	market := markets[0]
	lens := common.HexToAddress("0x3333333333333333333333333333333333333333")
	funding := merklefunding.Config{{Account: snapshot.Account.Hex(), Asset: market.Asset.Hex(), Lens: lens.Hex()}}
	used := false
	chain := marketReads(t, snapshot, recipe, tree, func(_ context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
		if msg.To == nil || *msg.To != lens || block.Cmp(snapshot.BlockNumber) != 0 {
			t.Fatalf("wrong lens observation: %v at %v", msg.To, block)
		}
		used = true
		return common.LeftPadBytes(big.NewInt(600).Bytes(), 32), nil
	})
	got, err := observe(t.Context(), chain, snapshot, recipe, market, funding)
	if err != nil || !used || got.AvailableFunding.Int64() != 600 || got.Budget.Int64() != 45 {
		t.Fatalf("observation: %+v, %v, lens read %v", got, err, used)
	}
}
