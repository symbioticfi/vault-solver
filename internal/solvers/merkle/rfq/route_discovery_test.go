package rfq

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

// The backend successfully serves an empty/unusable catalog on the first read,
// then advertises a usable connector discount on the next read.
type delayedQuoteOnlyCatalog struct {
	*fakeBackend

	initial []discounts.ListItem
}

func (b *delayedQuoteOnlyCatalog) listDiscounts(ctx context.Context) (*discounts.List, error) {
	if b.reads == 0 {
		b.reads++
		return &discounts.List{Discounts: b.initial}, nil
	}
	return b.fakeBackend.listDiscounts(ctx)
}

func TestQuoteOnlyRouteDiscoveryRetriesEmptyCatalog(t *testing.T) {
	for _, tc := range []struct {
		name    string
		initial []discounts.ListItem
	}{
		{name: "empty successful list", initial: []discounts.ListItem{}},
		{
			name: "no matching usable connector",
			// A real connector, but this catalog entry pays a different asset.
			initial: []discounts.ListItem{{Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tIn.Hex()}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := connectorExecutionFixture(t, connectorDiscountFixture())
			fixture.workflow.source = nil
			backend := &delayedQuoteOnlyCatalog{
				fakeBackend: &fakeBackend{listed: &discounts.List{Discounts: []discounts.ListItem{
					{Adapter: vlt.Hex(), TokenToRedeem: tIn.Hex(), Collateral: tOut.Hex()},
				}}},
				initial: tc.initial,
			}
			s := &Solver{
				recipeExecutors: []common.Address{fixture.reader.recipeExecutor},
				workflow:        fixture.workflow,
				backend:         backend,
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			s.DiscoverRoutes(ctx, time.Millisecond)
			executor, account, ok := s.ResolveRoute(vlt, tIn, tOut)
			if !ok || executor != fixture.reader.recipeExecutor || account != fixture.reader.recipeAccount {
				t.Fatalf("later discount never discovered after successful empty/unusable list: reads=%d route=(%s,%s,%t)", backend.reads, executor, account, ok)
			}
			if backend.reads != 2 {
				t.Fatalf("discovery reads=%d, want empty/unusable then usable catalog", backend.reads)
			}
		})
	}
}
