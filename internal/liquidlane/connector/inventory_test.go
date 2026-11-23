package connector

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector/connectortest"
)

func inventoryFixture(t *testing.T) (*connectortest.Chain, Offer) {
	t.Helper()
	terms, err := ParseDiscount(connectortest.DiscountFixture(), *connectortest.DiscountID(), vlt, tIn)
	if err != nil {
		t.Fatal(err)
	}
	f := connectortest.NewFixture(t, terms.Swap, terms.Signature, common.HexToAddress("0x43"), common.HexToAddress("0x10"), [4]byte{1})
	f.Chain.AmountOut = big.NewInt(2e18)
	return f.Chain, Offer{
		Venue: vlt, RecipeExecutor: f.Chain.RecipeExecutor, Account: f.Chain.ConnectorAccount, TokenIn: tIn, TokenOut: tOut,
		TokenInDecimals: 18, TokenOutDecimals: 6, DiscountID: *connectortest.DiscountID(),
		Discount: big.NewInt(2_000_000), MaxAssets: big.NewInt(900), BlockNumber: 5, ValidUntil: time.Unix(4102444800, 0),
	}
}

func TestInventoryClipsAdvertisedFundingByConnectorBudget(t *testing.T) {
	chain, offer := inventoryFixture(t)
	chain.Remaining = big.NewInt(700)
	late := offer
	late.DiscountID, late.BlockNumber, late.MaxAssets = common.HexToHash("0x99"), 99, big.NewInt(600)
	inventory, skipped, err := ReadInventory(t.Context(), chain, 1, []Offer{offer, late})
	if err != nil || skipped != nil || len(inventory) != 2 {
		t.Fatal(inventory, err)
	}
	item := inventory[0]
	if item.MaxAssets.Int64() != 700 || inventory[1].MaxAssets.Int64() != 600 {
		t.Fatalf("capacity = %s, %s; want funding clipped by the connector budget", item.MaxAssets, inventory[1].MaxAssets)
	}
	if item.BlockNumber != 5 || inventory[1].BlockNumber != 7 {
		t.Fatal("funding freshness was not bounded by the read block")
	}
	if item.RecipeExecutor != chain.RecipeExecutor || item.DiscountPrecision != Precision || item.Discount.Int64() != 2_000_000 ||
		item.Price.Cmp(big.NewInt(2e18)) != 0 || *item.DiscountID != *connectortest.DiscountID() || !item.ValidUntil.Equal(offer.ValidUntil) {
		t.Fatalf("inventory = %+v", item)
	}
	if item.Adapter != vlt || item.Vault != chain.ConnectorAccount || item.CapacityID != liquidlane.NewCapacityID(1, chain.ConnectorAccount, tOut) {
		t.Fatalf("route = %+v", item.Route)
	}
	if empty, dropped, err := ReadInventory(t.Context(), chain, 1, nil); empty != nil || dropped != nil || err != nil {
		t.Fatal("empty offers read the chain")
	}
}

func TestInventoryDropsOffersTheConnectorContradicts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*connectortest.Chain, *Offer)
		want   string
	}{
		{"below connector minimum", func(c *connectortest.Chain, _ *Offer) { c.MinDiscount = big.NewInt(2_000_001) }, ""},
		{"another contract", func(c *connectortest.Chain, _ *Offer) { c.DomainName = "Adapter" }, "domain"},
		{"another chain", func(*connectortest.Chain, *Offer) {}, "domain"},
		{"another output", func(_ *connectortest.Chain, o *Offer) { o.TokenOut = tIn }, "outputAsset"},
		{"another account", func(_ *connectortest.Chain, o *Offer) { o.Account = common.HexToAddress("0x88") }, "account"},
		{"unreadable oracle", func(c *connectortest.Chain, _ *Offer) { c.AmountOut = nil }, "price"},
		{"unsupported decimals", func(_ *connectortest.Chain, o *Offer) { o.TokenInDecimals, o.TokenOutDecimals = 0, 30 }, "decimals"},
		{"full discount", func(_ *connectortest.Chain, o *Offer) { o.Discount = big.NewInt(Precision) }, "advertised"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chain, offer := inventoryFixture(t)
			tc.mutate(chain, &offer)
			chainID := int64(1)
			if tc.name == "another chain" {
				chainID = 2
			}
			// A contradicted offer is dropped and reported; the read itself succeeds.
			inventory, err, readErr := ReadInventory(t.Context(), chain, chainID, []Offer{offer})
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(inventory) != 0 {
				t.Fatalf("contradicted offer priced: %+v", inventory)
			}
			if (err == nil) != (tc.want == "") || err != nil && !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestInventoryRejectsInvalidBlock(t *testing.T) {
	chain, offer := inventoryFixture(t)
	chain.Header.Number = new(big.Int).Lsh(big.NewInt(1), 64)
	if _, _, err := ReadInventory(t.Context(), chain, 1, []Offer{offer}); err == nil {
		t.Fatal("invalid block accepted")
	}
}
