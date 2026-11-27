package rfq

import (
	"context"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"
)

type splitPrices struct {
	head    common.Hash
	attempt uint64
	pairs   map[[2]string]splitPrice
}

type splitPrice struct {
	listings     []discounts.ListItem
	inventory    []liquidlane.Inventory
	skipped, err error
	attempt      uint64
}

// Reuse a complete read for the same head and listings. Retry incomplete reads
// in the next phase, while sharing the failure across awards in this phase.
func (p *splitPrices) reader(head common.Hash, load filler.InventoryReader) filler.InventoryReader {
	if p.pairs == nil || p.head != head {
		p.head, p.pairs = head, make(map[[2]string]splitPrice)
	}
	p.attempt++
	attempt := p.attempt
	return func(ctx context.Context, listings []discounts.ListItem) ([]liquidlane.Inventory, error, error) {
		pair := [2]string{listings[0].TokenToRedeem, listings[0].Collateral}
		if cached, ok := p.pairs[pair]; ok && slices.Equal(cached.listings, listings) &&
			(cached.attempt == attempt || cached.err == nil && cached.skipped == nil) {
			return cached.inventory, cached.skipped, cached.err
		}
		inventory, skipped, err := load(ctx, listings)
		p.pairs[pair] = splitPrice{listings, inventory, skipped, err, attempt}
		return inventory, skipped, err
	}
}
