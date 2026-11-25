package defaultstrategy

import (
	"context"
	"fmt"
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

	live := make(map[liveKey]liveOffer, len(input.LiveOffers))
	for _, l := range input.LiveOffers {
		live[liveKey{l.AdapterID, l.AuctionID}] = liveOffer{cloneBig(l.Principal), cloneBig(l.ExpectedReturn), l.Open}
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

	var out types.OfferOutput
	for _, auction := range input.Auctions {
		requested := auction.AmountRequested
		if requested == nil || requested.Sign() <= 0 {
			out.Skips = append(out.Skips, types.OfferSkip{AuctionID: auction.AuctionID,
				Reason: fmt.Sprintf("amountRequested %v is not positive", requested)})
			continue
		}
		// Adapters are independent makers: each offers the full request within its own capacity,
		// regardless of what other adapters bid in this pass or already hold on the auction. An adapter
		// whose open offer is stale (freed capacity lets it cover more, or its min-yield floor changed)
		// re-posts it with the new terms; it never shrinks one.
		ranked, skips := rankEligibleAdapters(auction, order, live)
		out.Skips = append(out.Skips, skips...)
		for _, c := range ranked {
			st, held := c.st, c.held
			skip := func(format string, args ...any) {
				out.Skips = append(out.Skips, types.OfferSkip{AuctionID: auction.AuctionID, AdapterID: st.snapshot.ID,
					Reason: fmt.Sprintf(format, args...)})
			}
			capacity, noCapacity := st.capacity(held)
			principal := minBig(capacity, requested)
			if held != nil && principal.Cmp(held) < 0 {
				principal = cloneBig(held)
			}
			if principal.Sign() <= 0 {
				skip("%s", noCapacity)
				continue
			}
			if st.belowMinAssets(principal) {
				skip("principal %s is below minAssets %s", principal, st.snapshot.MinAssets)
				continue
			}
			expectedReturn, err := offerReturn(principal, st.snapshot.MinYieldPpm, auction)
			if err != nil {
				skip("principal %s cannot be priced: %v", principal, err)
				continue
			}
			if held != nil && !c.stale(principal, expectedReturn, st.snapshot.MinYieldPpm, auction) {
				skip("open offer (principal %s, return %v) already matches target (principal %s, return %s)",
					held, c.heldReturn, principal, expectedReturn)
				continue
			}
			out.Offers = append(out.Offers, types.OfferExecution{
				AuctionID:      auction.AuctionID,
				Request:        auction.Request,
				Maker:          st.snapshot.Adapter,
				Principal:      principal,
				ExpectedReturn: expectedReturn,
			})
			st.committed.Add(st.committed, principal)
			if held != nil {
				st.committed.Sub(st.committed, held) // the replaced offer's principal and slot return
			} else {
				st.opened++
			}
		}
	}
	return out, nil
}

// liveKey identifies an adapter's live offer on a given auction.
type liveKey struct {
	adapterID string
	auctionID int64
}

// liveOffer is what an adapter already holds on an auction. A nil principal is an offer of unknown
// size; open means 3F has not accepted it yet, so a re-post replaces it.
type liveOffer struct {
	principal, expectedReturn *big.Int
	open                      bool
}

// candidate is an adapter eligible to offer on an auction; held and heldReturn are the terms of its
// open offer on that auction, nil when it has none.
type candidate struct {
	st               *adapterState
	held, heldReturn *big.Int
}

// stale reports whether the adapter would post different terms now: a larger principal fits, or the
// open offer's price fails the current adapter/auction bounds or sits above the current target.
func (c candidate) stale(principal, target, floor *big.Int, auction types.AuctionSnapshot) bool {
	if principal.Cmp(c.held) > 0 {
		return true
	}
	return c.heldReturn != nil && (types.ValidateYield(c.heldReturn, c.held, floor, auction.MaxRateBps, auction.RateBasisSeconds) != nil || c.heldReturn.Cmp(target) > 0)
}

// rankEligibleAdapters filters adapters eligible to offer on the auction (matching collateral, and no
// live offer on it unless that offer is open and of known size) and orders them by available capacity,
// largest first, with a skip for each adapter it filters out. Capacity is computed once per adapter
// (not inside the comparator).
func rankEligibleAdapters(
	auction types.AuctionSnapshot,
	order []*adapterState,
	live map[liveKey]liveOffer,
) ([]candidate, []types.OfferSkip) {
	type scored struct {
		candidate

		capacity *big.Int
	}
	eligible := make([]scored, 0, len(order))
	var skips []types.OfferSkip
	skip := func(st *adapterState, reason string) {
		skips = append(skips, types.OfferSkip{AuctionID: auction.AuctionID, AdapterID: st.snapshot.ID, Reason: reason})
	}
	for _, st := range order {
		if auction.DepositAsset != st.snapshot.Collateral {
			skip(st, fmt.Sprintf("collateral %s does not match deposit asset %s", st.snapshot.Collateral.Hex(), auction.DepositAsset.Hex()))
			continue
		}
		l, holds := live[liveKey{st.snapshot.ID, auction.AuctionID}]
		switch {
		case holds && l.principal == nil:
			skip(st, "live offer has unknown principal")
			continue
		case holds && !l.open:
			skip(st, fmt.Sprintf("live offer (principal %s) is already accepted", l.principal))
			continue
		}
		capacity, _ := st.capacity(l.principal)
		eligible = append(eligible, scored{candidate{st, l.principal, l.expectedReturn}, capacity})
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		return eligible[i].capacity.Cmp(eligible[j].capacity) > 0
	})
	ranked := make([]candidate, len(eligible))
	for i := range eligible {
		ranked[i] = eligible[i].candidate
	}
	return ranked, skips
}

type adapterState struct {
	snapshot  types.AdapterSnapshot
	committed *big.Int
	opened    int
}

// capacity is the max principal this adapter can fund for one more request: the smaller of its
// per-request ceiling (min(fundable, maxAssets); maxAssets 0 ⇒ reject-all) and its remaining budget
// (fundable minus this pass's commitments), gated by the concurrency and min-request-size limits.
// held is the live offer the request would replace: its principal and slot count as available.
// A zero capacity comes with the reason naming the limit that produced it.
func (s *adapterState) capacity(held *big.Int) (*big.Int, string) {
	inFlight := s.snapshot.OpenCount + s.opened
	if held != nil {
		inFlight--
	}
	if s.snapshot.MaxConcurrent > 0 && inFlight >= s.snapshot.MaxConcurrent {
		return new(big.Int), fmt.Sprintf("%d offers in flight reach maxConcurrent %d", inFlight, s.snapshot.MaxConcurrent)
	}
	if s.snapshot.Fundable == nil {
		return new(big.Int), "fundable liquidity is unknown"
	}
	ceiling := new(big.Int).Set(s.snapshot.Fundable)
	if s.snapshot.MaxAssets != nil {
		ceiling = minBig(ceiling, s.snapshot.MaxAssets) // always-active ceiling; 0 ⇒ no bid
	}
	if ceiling.Sign() <= 0 {
		return new(big.Int), fmt.Sprintf("per-request ceiling is %s (fundable %s, maxAssets %v)", ceiling, s.snapshot.Fundable, s.snapshot.MaxAssets)
	}
	if s.snapshot.MinAssets != nil && ceiling.Cmp(s.snapshot.MinAssets) < 0 {
		return new(big.Int), fmt.Sprintf("per-request ceiling %s is below minAssets %s", ceiling, s.snapshot.MinAssets)
	}
	budget := new(big.Int).Sub(s.snapshot.Fundable, s.committed)
	if held != nil {
		budget.Add(budget, held)
	}
	if budget.Sign() <= 0 {
		return new(big.Int), fmt.Sprintf("budget exhausted: fundable %s, committed %s, replaced %v", s.snapshot.Fundable, s.committed, held)
	}
	return minBig(ceiling, budget), ""
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

// offerReturn respects both floors, retaining the partial-consumption margin on the adapter floor.
// With no floor it uses the auction rate; an error means this principal cannot produce a valid paid offer.
func offerReturn(principal, floor *big.Int, auction types.AuctionSnapshot) (*big.Int, error) {
	maxRate, basis := auction.MaxRateBps, auction.RateBasisSeconds
	minimum, maximum, err := types.AuctionReturnBounds(principal, maxRate, basis)
	if err != nil {
		return nil, err
	}
	expectedReturn := types.PartialSafeMinYieldReturn(principal, floor)
	if expectedReturn.Sign() <= 0 {
		expectedReturn = maximum
	} else if expectedReturn.Cmp(maximum) > 0 && types.MeetsMinYield(maximum, principal, floor) {
		expectedReturn = maximum
	}
	if expectedReturn.Cmp(minimum) < 0 {
		expectedReturn = minimum
	}
	if err := types.ValidateYield(expectedReturn, principal, floor, maxRate, basis); err != nil {
		return nil, err
	}
	return expectedReturn, nil
}
