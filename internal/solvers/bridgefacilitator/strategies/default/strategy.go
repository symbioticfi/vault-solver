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
	adapters := make(map[string]*adapterState, len(input.Adapters))
	for _, a := range input.Adapters {
		adapters[a.ID] = &adapterState{snapshot: a, committed: new(big.Int)}
	}

	candidatesByAuction := make(map[int64][]types.OfferCandidate)
	for _, c := range input.Candidates {
		candidatesByAuction[c.AuctionID] = append(candidatesByAuction[c.AuctionID], c)
	}

	var offers []types.OfferExecution
	for _, auction := range input.Auctions {
		remaining := cloneBig(auction.RemainingAmount)
		if remaining == nil || remaining.Sign() <= 0 {
			continue
		}
		ranked := rankEligibleCandidates(auction, candidatesByAuction[auction.AuctionID], adapters)
		for _, c := range ranked {
			if remaining.Sign() <= 0 {
				break
			}
			st := adapters[c.AdapterID]
			if st == nil {
				continue
			}
			capacity := candidateCapacity(c, st)
			if capacity.Sign() <= 0 {
				continue
			}
			principal := cloneBig(capacity)
			if principal.Cmp(remaining) > 0 {
				principal.Set(remaining)
			}
			if st.belowMinAssets(principal) {
				continue
			}
			offers = append(offers, types.OfferExecution{
				AuctionID:      auction.AuctionID,
				Request:        auction.Request,
				Maker:          st.snapshot.Adapter,
				Principal:      principal,
				ExpectedReturn: types.ExpectedReturn(principal, auction.MaxRateBps),
			})
			st.committed.Add(st.committed, principal)
			st.opened++
			remaining.Sub(remaining, principal)
		}
	}
	return types.OfferOutput{Offers: offers}, nil
}

func rankEligibleCandidates(
	auction types.AuctionSnapshot,
	candidates []types.OfferCandidate,
	adapters map[string]*adapterState,
) []types.OfferCandidate {
	ranked := make([]types.OfferCandidate, 0, len(candidates))
	for _, c := range candidates {
		st := adapters[c.AdapterID]
		if st == nil {
			continue
		}
		if c.HasLiveOffer || auction.DepositAsset != st.snapshot.Collateral {
			continue
		}
		if st.snapshot.MinYieldBps != nil && st.snapshot.MinYieldBps.Sign() > 0 &&
			auction.MaxRateBps < types.BpsToFloat(st.snapshot.MinYieldBps) {
			continue
		}
		ranked = append(ranked, c)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		left := candidateCapacity(ranked[i], adapters[ranked[i].AdapterID])
		right := candidateCapacity(ranked[j], adapters[ranked[j].AdapterID])
		return left.Cmp(right) > 0
	})
	return ranked
}

func candidateCapacity(c types.OfferCandidate, st *adapterState) *big.Int {
	if st == nil {
		return new(big.Int)
	}
	if st.full() || c.Capacity == nil || c.Capacity.Sign() <= 0 {
		return new(big.Int)
	}
	budget := st.remainingBudget()
	if budget.Sign() <= 0 {
		return new(big.Int)
	}
	if c.Capacity.Cmp(budget) <= 0 {
		return cloneBig(c.Capacity)
	}
	return budget
}

type adapterState struct {
	snapshot  types.AdapterSnapshot
	committed *big.Int
	opened    int
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

func cloneBig(n *big.Int) *big.Int {
	if n == nil {
		return nil
	}
	return new(big.Int).Set(n)
}

var _ types.Strategy = (*Strategy)(nil)
