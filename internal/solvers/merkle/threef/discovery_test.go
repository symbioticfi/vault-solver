package threef

import (
	"context"
	"math/big"
	"strings"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/threef"
)

func TestDiscoverPolicyMarkets(t *testing.T) {
	pin, _, _ := fixture(t)
	manager, collateral, underlying := common.HexToAddress("0x91"), common.HexToAddress("0x92"), common.HexToAddress("0x93")
	policy := fixturePolicy(t)
	for _, tc := range []struct {
		name, status, offerStatus string
		metadata, invalid         bool
		want                      int
	}{
		{"open", "OPEN", "", true, false, 1},
		{"solvable", "SOLVABLE", "", true, false, 1},
		{"historical", "SUCCEEDED", "", false, false, 0},
		{"live submitted", "SOLVED", "SUBMITTED", true, false, 1},
		{"live accepted", "SOLVED", "ACCEPTED", true, false, 1},
		{"live mint", "SOLVED", "AWAITING_MINT", true, false, 1},
		{"live pending", "SOLVED", "PENDING", true, false, 1},
		{"terminal", "SUCCEEDED", "REPAID", false, false, 0},
		{"missing metadata", "OPEN", "", false, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testMarket()
			m.Account = pin.Account
			auction := testAuction(m, 1, common.HexToAddress("0x100"))
			auction.Status = tc.status
			if tc.metadata {
				auction.Asset.Set(&threef.ResolvedAssetDto{PositionManager: manager.Hex()})
			}
			var offers []threef.OfferDto
			if tc.offerStatus != "" {
				offer := testOffer(m, 1, tc.offerStatus, "10")
				offer.RequestId = auction.RequestId
				offers = append(offers, offer)
			}
			calls := 0
			chain := readFunc(func(_ context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
				calls++
				if block.Cmp(pin.BlockNumber) != 0 {
					t.Fatal("lost snapshot")
				}
				switch *msg.To {
				case manager:
					return positionManagerABI.GetABI().Methods["assets"].Outputs.Pack(collateral, common.Address{})
				case collateral:
					return underlyingAssetABI.GetABI().Methods["underlying"].Outputs.Pack(underlying)
				default:
					t.Fatal("unexpected target", msg.To)
					return nil, nil
				}
			})
			// Duplicate auctions share discovery reads and a single funding-pair budget.
			markets, err := discoverMarkets(t.Context(), chain, pin, policy, []threef.AuctionDto{auction, auction}, offers)
			if (err != nil) != tc.invalid {
				t.Fatalf("error: %v", err)
			}
			if len(markets) != tc.want {
				t.Fatalf("markets: %+v", markets)
			}
			if tc.want == 1 && (calls != 2 || markets[0].UnderlyingAsset != underlying || markets[0].Asset != m.Asset || markets[0].Policy != policy) {
				t.Fatalf("wrong discovery: %d calls, %+v", calls, markets)
			}
		})
	}
}

func TestDiscoverPolicyMarketsRejectsInvalidChainData(t *testing.T) {
	pin, _, _ := fixture(t)
	auction := testAuction(testMarket(), 1, common.HexToAddress("0x100"))
	auction.Asset.Set(&threef.ResolvedAssetDto{PositionManager: common.HexToAddress("0x91").Hex()})
	for _, tc := range []struct {
		name string
		data []byte
		err  error
	}{
		{"rpc", nil, errors.New("unavailable")},
		{"empty", nil, nil},
		{"zero collateral", make([]byte, 64), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chain := readFunc(func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) { return tc.data, tc.err })
			if _, err := discoverMarkets(t.Context(), chain, pin, fixturePolicy(t), []threef.AuctionDto{auction}, nil); err == nil {
				t.Fatal("invalid chain data accepted")
			}
		})
	}
}

func TestMintProviderRejectsAnotherPolicy(t *testing.T) {
	pin, recipe, tree := fixture(t)
	p := &mintProvider{Policy: common.HexToAddress("0xdead")}
	if _, err := p.RecipeRequirements(pin, recipe, tree); err == nil || !strings.Contains(err.Error(), "policy") {
		t.Fatalf("wrong policy accepted: %v", err)
	}
	if _, err := p.PrepareRecipe(t.Context(), pin, recipe, tree, nil); err == nil || !strings.Contains(err.Error(), "policy") {
		t.Fatalf("wrong policy prepared: %v", err)
	}
}
