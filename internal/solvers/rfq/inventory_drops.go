package rfq

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidstrategies "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

// inventoryDrops counts the inventory dropped before the strategy runs, by failed check, and keeps
// the last drop with its values, so an empty decision can say what was dropped with debug off.
// Each drop is also logged at V(1). One quote or fill plan owns it; it is not safe for concurrent use.
type inventoryDrops struct {
	counts map[string]int
	trace  liquidstrategies.DecisionTrace
	last   func() string
}

func newInventoryDrops(ctx context.Context) *inventoryDrops {
	log := observability.Log(ctx).V(1)
	d := &inventoryDrops{counts: make(map[string]int)}
	d.trace, d.last = liquidstrategies.DecisionTrace(func(message string, keyValues ...any) {
		if len(keyValues) >= 2 && keyValues[0] == "reason" {
			if reason, ok := keyValues[1].(string); ok {
				d.counts[reason]++
			}
		}
		log.Info(message, keyValues...)
	}).RecordDeclines()
	return d
}

func (d *inventoryDrops) String() string {
	if len(d.counts) == 0 {
		return "none"
	}
	reasons := slices.Sorted(maps.Keys(d.counts))
	parts := make([]string, len(reasons))
	for i, reason := range reasons {
		parts[i] = fmt.Sprintf("%s x%d", reason, d.counts[reason])
	}
	return strings.Join(parts, ", ") + "; last: " + d.last()
}

// declineUnallocated records the inventory AllocateInventoryCapacity left without capacity:
// zero capacity, or capacity fully held by reservations of other won orders.
func declineUnallocated(
	trace liquidstrategies.DecisionTrace,
	inventory, allocated []solverInventory,
	reservations liquidlane.CapacityReservations,
) {
	kept := make(map[liquidlane.CandidateID]bool, len(allocated))
	for _, item := range allocated {
		kept[liquidlane.NewCandidateID(item.Route, item.DiscountID)] = true
	}
	for _, item := range inventory {
		if !kept[liquidlane.NewCandidateID(item.Route, item.DiscountID)] {
			trace.Decline("inventory", "no unreserved capacity", "route", item.ID,
				"maxAssets", item.MaxAssets, "reserved", reservations[liquidlane.RouteCapacityID(item.Route)])
		}
	}
}

// fillInventorySummary is what a fill plan read, appended to a strategy failure so the order's
// failure reason names the inventory it saw.
type fillInventorySummary struct {
	vaults, directRead, directAuthorized, discounts, inventory, candidates int
	discountsErr                                                           error
	drops                                                                  *inventoryDrops
}

func (s fillInventorySummary) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d configured vaults, %d of %d direct inventories authorized, %d discount inventories",
		s.vaults, s.directAuthorized, s.directRead, s.discounts)
	if s.discountsErr != nil {
		fmt.Fprintf(&b, " (list discounts failed: %v)", s.discountsErr)
	}
	fmt.Fprintf(&b, ", %d candidates from %d inventories, dropped: %s", s.candidates, s.inventory, s.drops)
	return b.String()
}

// liquidLaneDrops records the shared reader's dropped reads on trace under one bounded reason, with
// the adapter, token and decoded detail as fields, so a fill summary counts them and shows the last.
func liquidLaneDrops(trace liquidstrategies.DecisionTrace) liquidlane.DropTrace {
	return func(d liquidlane.Drop) {
		trace.Decline("inventory", "liquidlane read dropped",
			"adapter", d.Adapter.Hex(), "tokenIn", d.TokenIn.Hex(), "detail", d.Reason)
	}
}
