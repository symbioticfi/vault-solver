package bridgefacilitator

import (
	"context"
	"math/big"
	"sort"

	"gopkg.in/yaml.v3"

	parsepkg "github.com/symbioticfi/vault-solver/internal/parse"
)

type defaultPlanner struct{}

func validateDefaultPlannerConfig(raw yaml.Node) error {
	_, err := newDefaultPlannerFromConfig(raw)
	return err
}

func newDefaultPlannerFromConfig(raw yaml.Node) (Planner, error) {
	if err := parsepkg.DecodeStrict(raw, &struct{}{}); err != nil {
		return nil, err
	}
	return newDefaultPlanner(), nil
}

func newDefaultPlanner() *defaultPlanner {
	return &defaultPlanner{}
}

func (s *defaultPlanner) DecideOffers(
	_ context.Context,
	input OfferInput,
) (OfferOutput, error) {
	planner := newOfferPlanner(input)
	for _, auction := range input.Auctions {
		planner.planAuction(auction)
	}
	return OfferOutput{Offers: planner.offers}, nil
}

type offerPlanner struct {
	adapters []*adapterState
	live     map[liveKey]struct{}
	offers   []OfferExecution
}

func newOfferPlanner(input OfferInput) *offerPlanner {
	planner := &offerPlanner{
		adapters: make([]*adapterState, 0, len(input.Adapters)),
		live:     make(map[liveKey]struct{}, len(input.LiveOffers)),
	}
	for index := range input.Adapters {
		planner.adapters = append(planner.adapters, &adapterState{snapshot: input.Adapters[index], committed: new(big.Int)})
	}
	for _, offer := range input.LiveOffers {
		planner.live[liveKey{offer.AdapterID, offer.AuctionID}] = struct{}{}
	}
	return planner
}

func (planner *offerPlanner) planAuction(auction AuctionSnapshot) {
	remaining := cloneBig(auction.RemainingAmount)
	if remaining == nil || remaining.Sign() <= 0 {
		return
	}
	for _, adapter := range rankEligibleAdapters(auction, planner.adapters, planner.live) {
		if remaining.Sign() <= 0 {
			return
		}
		offer, ok := buildOffer(auction, adapter, remaining)
		if !ok {
			continue
		}
		planner.offers = append(planner.offers, offer)
		adapter.commit(offer.Principal)
		remaining.Sub(remaining, offer.Principal)
	}
}

func buildOffer(auction AuctionSnapshot, adapter *adapterState, remaining *big.Int) (OfferExecution, bool) {
	principal := minBig(adapter.capacity(), remaining)
	if principal.Sign() <= 0 || adapter.belowMinAssets(principal) {
		return OfferExecution{}, false
	}
	expectedReturn := priceOffer(principal, auction.MaxRateBps, adapter.snapshot)
	if ValidateYield(
		expectedReturn, principal, adapter.snapshot.MinAssets, adapter.snapshot.MinYieldPpm, auction.MaxRateBps,
	) != nil {
		return OfferExecution{}, false
	}
	return OfferExecution{
		AuctionID: auction.AuctionID, Request: auction.Request, Maker: adapter.snapshot.Adapter,
		Principal: principal, ExpectedReturn: expectedReturn,
	}, true
}

func priceOffer(principal *big.Int, maxRateBps float64, adapter AdapterSnapshot) *big.Int {
	minimum := PartialSafeMinYieldReturn(principal, adapter.MinAssets, adapter.MinYieldPpm)
	maximum := ExpectedReturn(principal, maxRateBps)
	if minimum.Sign() <= 0 || (maximum.Sign() > 0 && minimum.Cmp(maximum) > 0 &&
		MeetsMinYield(maximum, principal, adapter.MinYieldPpm)) {
		return maximum
	}
	return minimum
}

// liveKey dedups an adapter's live offer on a given auction.
type liveKey struct {
	adapterID string
	auctionID int64
}

// rankEligibleAdapters filters adapters eligible to offer on the auction (no live offer, matching
// collateral, meeting the adapter's min-yield floor) and orders them by available capacity, largest
// first. Capacity is computed once per adapter (not inside the comparator).
func rankEligibleAdapters(
	auction AuctionSnapshot,
	order []*adapterState,
	live map[liveKey]struct{},
) []*adapterState {
	type scored struct {
		st       *adapterState
		capacity *big.Int
	}
	eligible := make([]scored, 0, len(order))
	for _, st := range order {
		if _, exists := live[liveKey{st.snapshot.ID, auction.AuctionID}]; exists {
			continue
		}
		if !st.snapshot.Matches(auction.DepositAsset) {
			continue
		}
		eligible = append(eligible, scored{st, st.capacity()})
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		return eligible[i].capacity.Cmp(eligible[j].capacity) > 0
	})
	ranked := make([]*adapterState, len(eligible))
	for i := range eligible {
		ranked[i] = eligible[i].st
	}
	return ranked
}

type adapterState struct {
	snapshot  AdapterSnapshot
	committed *big.Int
	opened    int
}

// capacity is the max principal this adapter can fund for one more request: the smaller of its
// per-request ceiling (min(fundable, maxAssets); maxAssets 0 ⇒ reject-all) and its remaining budget
// (fundable minus this pass's commitments), gated by the concurrency and min-request-size limits.
func (s *adapterState) capacity() *big.Int {
	if s.full() || s.snapshot.Fundable == nil {
		return new(big.Int)
	}
	ceiling := new(big.Int).Set(s.snapshot.Fundable)
	if s.snapshot.MaxAssets != nil {
		ceiling = minBig(ceiling, s.snapshot.MaxAssets) // always-active ceiling; 0 ⇒ no bid
	}
	if ceiling.Sign() <= 0 {
		return new(big.Int)
	}
	if s.snapshot.MinAssets != nil && ceiling.Cmp(s.snapshot.MinAssets) < 0 {
		return new(big.Int) // capacity below the on-chain minimum request size
	}
	budget := s.remainingBudget()
	if budget.Sign() <= 0 {
		return new(big.Int)
	}
	return minBig(ceiling, budget)
}

func (s *adapterState) full() bool {
	return !s.snapshot.CanOpen(s.opened)
}

func (s *adapterState) remainingBudget() *big.Int {
	return new(big.Int).Sub(s.snapshot.Fundable, s.committed)
}

func (s *adapterState) belowMinAssets(amount *big.Int) bool {
	return s.snapshot.MinAssets != nil && s.snapshot.MinAssets.Sign() > 0 && amount.Cmp(s.snapshot.MinAssets) < 0
}

func (s *adapterState) commit(principal *big.Int) {
	s.committed.Add(s.committed, principal)
	s.opened++
}

func minBig(a, b *big.Int) *big.Int {
	if a.Cmp(b) <= 0 {
		return new(big.Int).Set(a)
	}
	return new(big.Int).Set(b)
}

var _ Planner = (*defaultPlanner)(nil)
