package lifi

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

// settlement is the fill path behind LI.FI quoting and order intake. Every solver built on
// this engine has exactly one: native adapters on the host transaction manager
// (lifi-samechain) or connector recipes on the Merkle lane (lifi-merkle).
type settlement interface {
	// initialize prepares the startup state the fill path needs before quotes are published.
	initialize(ctx context.Context) error
	// onMerkleLane reports whether fills are signed by the SDK transaction manager.
	onMerkleLane() bool
	// quoteSource reads the inventory one quote refresh is built from at chainTime.
	quoteSource(ctx context.Context, routes []route, chainTime time.Time) (quoteSource, error)
	// reservations is the capacity already committed to fills, which quotes must not reuse.
	reservations(inventory []liquidlane.Inventory) liquidlane.CapacityReservations
	// fillOrder plans and submits the fill of an opened order.
	fillOrder(ctx context.Context, routes []route, order *submittedOrder, orderID common.Hash, reservations *liquidlane.CapacityReservations) orderProcessingResult
	// retryReverted reports whether an order whose fill reverted on chain is planned again while
	// it is still open. Only the order worker calls it.
	retryReverted(order *submittedOrder) bool
}

// quoteSource is the inventory one quote refresh is built from.
type quoteSource struct {
	inventory []liquidlane.Inventory
	// direct and discountBases attribute adapter inventory in logs and rejection metrics.
	direct, discountBases []liquidlane.Inventory
	singleRouteTokens     map[common.Address]bool
	gasSnapshot           *liquidlanegas.Snapshot
	gasPrices             *liquidlanegas.PriceSnapshot
	// degraded reports discounts that could not be priced; discountErr is why none could be listed.
	degraded    bool
	discountErr error
}

// fillPath returns the solver's settlement; an engine built without one settles through adapters.
func (s *Solver) fillPath() settlement {
	if s.settlement != nil {
		return s.settlement
	}
	return adapterSettlement{s}
}

// adapterSettlement fills through native LiquidLane adapters on the host transaction manager.
type adapterSettlement struct{ s *Solver }

func (adapterSettlement) initialize(context.Context) error   { return nil }
func (adapterSettlement) onMerkleLane() bool                 { return false }
func (adapterSettlement) retryReverted(*submittedOrder) bool { return false }

func (a adapterSettlement) reservations([]liquidlane.Inventory) liquidlane.CapacityReservations {
	return a.s.capacity.Snapshot()
}

func (a adapterSettlement) quoteSource(ctx context.Context, routes []route, chainTime time.Time) (quoteSource, error) {
	s := a.s
	snapshots, err := s.reader.quoteSnapshots(ctx, routes, s.cfg.Executor)
	if err != nil {
		return quoteSource{}, err
	}
	direct := filterQuoteInventory(snapshots.Direct, s.cfg.TokenPolicy)
	discountBases := filterQuoteInventory(snapshots.Physical, s.cfg.TokenPolicy)
	discounted, degraded, discountErr := s.quoteDiscountInventories(ctx, discountBases, chainTime)
	return quoteSource{
		inventory: append(append([]liquidlane.Inventory(nil), direct...), discounted...),
		direct:    direct, discountBases: discountBases,
		singleRouteTokens: s.cfg.TokenPolicy.SingleRouteTokens(),
		gasSnapshot:       snapshots.GasSnapshot, gasPrices: snapshots.GasPrices,
		degraded: degraded, discountErr: discountErr,
	}, nil
}

func (a adapterSettlement) fillOrder(ctx context.Context, routes []route, order *submittedOrder, orderID common.Hash, reservations *liquidlane.CapacityReservations) orderProcessingResult {
	return a.s.fillAdapterOrder(ctx, routes, order, orderID, reservations)
}
