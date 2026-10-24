package defaultstrategy

import (
	"context"
	"math/big"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/types"
)

const Name = "default"

type Config struct{}

type Strategy struct{}

//nolint:gochecknoinits // solver-local strategy self-registration mirrors solver registration.
func init() {
	strategies.Register(Name, NewFromConfig)
}

func NewFromConfig(raw yaml.Node, _ strategies.Deps) (types.Strategy, error) {
	var cfg Config
	if err := decodeConfig(raw, &cfg); err != nil {
		return nil, err
	}
	return New(), nil
}

func New() *Strategy {
	return &Strategy{}
}

func decodeConfig(node yaml.Node, out any) error {
	if node.Kind == 0 {
		node = yaml.Node{Kind: yaml.MappingNode}
	}
	return solver.DecodeStrict(node, out)
}

func (s *Strategy) DecideOffers(
	_ context.Context,
	input types.OfferInput,
) (types.OfferOutput, error) {
	order := make([]*adapterState, 0, len(input.Adapters))
	for i := range input.Adapters {
		order = append(order, &adapterState{snapshot: input.Adapters[i], committed: new(big.Int)})
	}

	live := make(map[liveKey]bool, len(input.LiveOffers))
	for _, l := range input.LiveOffers {
		live[liveKey{l.AdapterID, l.AuctionID}] = true
		for _, st := range order {
			if st.snapshot.ID != l.AdapterID {
				continue
			}
			st.opened++
			if l.Principal == nil || l.Principal.Sign() < 0 {
				// Missing commitment size cannot authorize additional use of this adapter.
				st.committed = cloneBig(st.snapshot.Fundable)
			} else if st.committed != nil {
				st.committed.Add(st.committed, l.Principal)
			}
		}
	}

	var offers []types.OfferExecution
	for _, auction := range input.Auctions {
		requested := auction.AmountRequested
		if requested == nil || requested.Sign() <= 0 {
			continue
		}
		// Adapters are independent makers: each offers the full request within its own capacity,
		// regardless of what other adapters bid in this pass or already hold on the auction.
		for _, st := range rankEligibleAdapters(auction, order, live) {
			principal := minBig(st.capacity(), requested)
			if principal.Sign() <= 0 || st.belowMinAssets(principal) {
				continue
			}
			expectedReturn := offerReturn(principal, st.snapshot.MinYieldPpm, auction)
			if expectedReturn == nil {
				continue
			}
			offers = append(offers, types.OfferExecution{
				AuctionID:      auction.AuctionID,
				Request:        auction.Request,
				Maker:          st.snapshot.Adapter,
				Principal:      principal,
				ExpectedReturn: expectedReturn,
			})
			st.committed.Add(st.committed, principal)
			st.opened++
		}
	}
	return types.OfferOutput{Offers: offers}, nil
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
	auction types.AuctionSnapshot,
	order []*adapterState,
	live map[liveKey]bool,
) []*adapterState {
	type scored struct {
		st       *adapterState
		capacity *big.Int
	}
	eligible := make([]scored, 0, len(order))
	for _, st := range order {
		if live[liveKey{st.snapshot.ID, auction.AuctionID}] {
			continue
		}
		if auction.DepositAsset != st.snapshot.Collateral {
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
	snapshot  types.AdapterSnapshot
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
	return s.snapshot.MaxConcurrent > 0 && s.snapshot.OpenCount+s.opened >= s.snapshot.MaxConcurrent
}

func (s *adapterState) remainingBudget() *big.Int {
	if s.snapshot.Fundable == nil {
		return new(big.Int)
	}
	return new(big.Int).Sub(s.snapshot.Fundable, s.committed)
}

func (s *adapterState) belowMinAssets(amount *big.Int) bool {
	return s.snapshot.MinAssets != nil && s.snapshot.MinAssets.Sign() > 0 && amount.Cmp(s.snapshot.MinAssets) < 0
}

func minBig(a, b *big.Int) *big.Int {
	if a.Cmp(b) <= 0 {
		return new(big.Int).Set(a)
	}
	return new(big.Int).Set(b)
}

func cloneBig(n *big.Int) *big.Int {
	if n == nil {
		return nil
	}
	return new(big.Int).Set(n)
}

var _ types.Strategy = (*Strategy)(nil)

// offerReturn adds the partial-consumption margin to the on-chain floor, bounded by the auction cap.
// With no floor it uses the auction rate; nil means this principal cannot produce a valid paid offer.
func offerReturn(principal, floor *big.Int, auction types.AuctionSnapshot) *big.Int {
	maxRate, basis := auction.MaxRateBps, auction.RateBasisSeconds
	expectedReturn := types.PartialSafeMinYieldReturn(principal, floor)
	if expectedReturn.Sign() <= 0 {
		expectedReturn = types.ExpectedReturn(principal, maxRate, basis)
	} else if maxReturn := types.ExpectedReturn(principal, maxRate, basis); maxReturn.Sign() > 0 && expectedReturn.Cmp(maxReturn) > 0 && types.MeetsMinYield(maxReturn, principal, floor) {
		expectedReturn = maxReturn
	}
	if types.ValidateYield(expectedReturn, principal, floor, maxRate, basis) != nil {
		return nil
	}
	return expectedReturn
}
