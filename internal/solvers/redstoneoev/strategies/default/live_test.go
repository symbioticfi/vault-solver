//go:build live

// Live read-only checks for the production Morpho API monitor path. They are OPT-IN (`-tags live`) and
// never run in the normal gate.
package defaultstrategy

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

func liveAdapterSnapshot(loan, collateral common.Address) types.AdapterSnapshot {
	return types.AdapterSnapshot{
		Address:      common.HexToAddress("0x00000000000000000000000000000000000000ad"),
		Vault:        common.HexToAddress("0x00000000000000000000000000000000000000da"),
		Loan:         loan,
		LoanDecimals: 6,
		FreeAssets:   mustBig("100000000000"),
		Withdrawable: mustBig("100000000000"),
		Redeemable: []types.RedeemableSnapshot{{
			Asset:          collateral,
			Decimals:       18,
			MaxRate:        mustBig("1780000000000000000000"),
			MaxAssets:      mustBig("100000000000"),
			AcquireBalance: new(big.Int),
		}},
		Filler: true,
	}
}

// TestLiveAPIMonitorSnapshotAndCandidates exercises the same production API path the OEV monitor uses:
// adapter-derived token pair -> Morpho markets with state -> monitor snapshot validation -> positions ->
// hot-path candidates. It uses a known mainnet USDC/PAXG pair as the adapter-derived stand-in; no RPC or
// real adapter is needed because this test targets the API-backed Morpho side.
//
//	go test -tags live -run TestLiveAPIMonitorSnapshotAndCandidates -v ./internal/solvers/redstoneoev/strategies/default/
func TestLiveAPIMonitorSnapshotAndCandidates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	loan := common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48") // USDC
	coll := common.HexToAddress("0x45804880De22913dAFE09f4980848ECE6EcbAf78") // PAXG
	wantMarket := common.HexToHash("0x8eaf7b29f02ba8d8c1d7aeb587403dcb16e2e943e4e2f5f94b0963c2386406c9")

	mon := &apiMonitor{
		api:          newMorphoClient("https://api.morpho.org/graphql"),
		maxPositions: 100,
		maxHF:        1.30,
		log:          logr.Discard(),
	}

	apiMarkets, err := mon.api.DiscoverMarketData(ctx, 1, []common.Address{loan}, []common.Address{coll})
	if err != nil {
		t.Fatalf("DiscoverMarketData live API failed: %v", err)
	}
	apiSnap := mon.apiMarketSnapshot(apiMarkets, loan, []common.Address{coll})
	if len(apiSnap.markets) == 0 {
		t.Fatal("apiMonitor snapshot has no usable USDC/PAXG markets")
	}
	if _, ok := apiSnap.markets[wantMarket]; !ok {
		t.Fatalf("apiMonitor snapshot missing known market %s (got %d markets)", wantMarket.Hex(), len(apiSnap.markets))
	}
	if apiSnap.block == 0 || apiSnap.blockTime == 0 {
		t.Fatalf("apiMonitor snapshot missing epoch: block=%d blockTime=%d", apiSnap.block, apiSnap.blockTime)
	}
	for id, info := range apiSnap.markets {
		if info.Params.LoanToken != loan || info.Params.CollateralToken != coll || info.Params.Oracle == (common.Address{}) {
			t.Fatalf("bad market params for %s: %+v", id.Hex(), info.Params)
		}
		if got, err := deriveMarketID(info.Params); err != nil || got != id {
			t.Fatalf("market id verification failed for %s: derived=%s err=%v", id.Hex(), got.Hex(), err)
		}
		if _, ok := apiSnap.prices[id]; !ok {
			t.Fatalf("market %s missing API state price", id.Hex())
		}
	}

	ids := make([]common.Hash, 0, len(apiSnap.markets))
	for id := range apiSnap.markets {
		ids = append(ids, id)
	}
	apiPositions, err := mon.api.PositionsByMarket(ctx, ids, mon.maxPositions, &mon.maxHF)
	if err != nil {
		t.Fatalf("PositionsByMarket live API failed: %v", err)
	}
	positions := apiPositionsSnapshot(apiPositions, apiSnap.markets)
	if len(positions) == 0 {
		t.Skip("live API returned no USDC/PAXG positions inside healthFactor <= 1.30 right now")
	}

	mon.snap.Store(&snapshot{
		markets:   apiSnap.markets,
		prices:    apiSnap.prices,
		positions: positions,
		block:     apiSnap.block,
		blockTime: apiSnap.blockTime,
	})

	var targetMarket common.Hash
	var targetBorrower common.Address
	for id, byBorrower := range positions {
		for borrower := range byBorrower {
			targetMarket, targetBorrower = id, borrower
			break
		}
		if targetBorrower != (common.Address{}) {
			break
		}
	}
	oracle := apiSnap.markets[targetMarket].Params.Oracle
	price := apiSnap.prices[targetMarket]
	auction := types.AuctionSnapshot{Prices: []types.AuctionPrice{{Oracle: oracle, Price: price}}}
	cands := mon.candidates(auction, apiSnap.blockTime, liveAdapterSnapshot(loan, coll))
	if len(cands) == 0 {
		t.Fatal("apiMonitor.candidates returned no candidates for a snapshot position with matching oracle price")
	}
	found := false
	for _, c := range cands {
		if c.cand.MarketID == targetMarket && c.cand.Borrower == targetBorrower && c.price.Cmp(price) == 0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("apiMonitor.candidates did not include target %s/%s", targetMarket.Hex(), targetBorrower.Hex())
	}
	t.Logf("apiMonitor live snapshot: markets=%d positions=%d block=%d candidate=%s/%s",
		len(apiSnap.markets), len(apiPositions), apiSnap.block, targetMarket.Hex(), targetBorrower.Hex())
}
