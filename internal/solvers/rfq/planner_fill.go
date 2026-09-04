package rfq

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// FillPlanFromQuote maps a trusted RFQ decision onto current candidates and
// validates the structural invariants required by Executor calldata.
func FillPlanFromQuote(input QuoteInput, out QuoteOutput) (*liquidlane.Plan, error) {
	if out.Decision != DecisionQuote {
		return nil, errors.Errorf("invalid fill-plan decision %q", out.Decision)
	}
	if out.QuotedAmountOut == nil || out.QuotedAmountOut.Sign() <= 0 {
		return nil, errors.New("quote output has invalid quotedAmountOut")
	}
	if len(out.Legs) == 0 {
		return nil, errors.New("quote output has no legs")
	}
	if input.RequireSingleRoute && len(out.Legs) != 1 {
		return nil, errors.New("single-route input requires exactly one leg")
	}

	candidates, err := indexCandidates(input.Candidates)
	if err != nil {
		return nil, err
	}
	routes := make([]liquidlane.PlanLeg, 0, len(out.Legs))
	sumIn := new(big.Int)
	sumOut := new(big.Int)
	seen := make(map[string]struct{}, len(out.Legs))
	usedRoutes := make(map[liquidlane.RouteID]struct{}, len(out.Legs))
	for i, leg := range out.Legs {
		candidate, err := validateLeg(input.TokenOut, candidates, seen, leg, i)
		if err != nil {
			return nil, err
		}
		if _, duplicate := usedRoutes[candidate.Route.ID]; duplicate {
			return nil, errors.Errorf("fill repeats physical route %q", candidate.Route.ID)
		}
		usedRoutes[candidate.Route.ID] = struct{}{}
		sumIn.Add(sumIn, leg.AmountIn)
		sumOut.Add(sumOut, leg.AmountOut)
		routes = append(routes, liquidlane.PlanLeg{
			CandidateID:       candidate.ID,
			RouteID:           candidate.Route.ID,
			CapacityID:        liquidlane.RouteCapacityID(candidate.Route),
			Adapter:           candidate.Route.Adapter,
			AmountIn:          liquidlane.CloneBig(leg.AmountIn),
			ExpectedAmountOut: liquidlane.CloneBig(leg.AmountOut),
			MinAmountOut:      liquidlane.CloneBig(leg.AmountOut),
			ReservedAmountOut: liquidlane.CloneBig(leg.AmountOut),
			DiscountID:        liquidlane.CloneHash(candidate.DiscountID),
		})
	}
	if input.AmountIn == nil || sumIn.Cmp(input.AmountIn) != 0 {
		return nil, errors.Errorf("strategy amountIn sum %s does not match request %v", sumIn, input.AmountIn)
	}
	if sumOut.Cmp(out.QuotedAmountOut) != 0 {
		return nil, errors.Errorf("strategy amountOut sum %s does not match quotedAmountOut %s", sumOut, out.QuotedAmountOut)
	}
	if input.RequiredAmountOut != nil && out.QuotedAmountOut.Cmp(input.RequiredAmountOut) < 0 {
		return nil, errors.New("strategy output is below required amount out")
	}
	return &liquidlane.Plan{Routes: routes}, nil
}

func indexCandidates(input []liquidlane.QuoteCandidate) (map[string]liquidlane.QuoteCandidate, error) {
	candidates := make(map[string]liquidlane.QuoteCandidate, len(input))
	for _, candidate := range input {
		if candidate.ID == "" {
			return nil, errors.New("candidate id is empty")
		}
		id := string(candidate.ID)
		if _, exists := candidates[id]; exists {
			return nil, errors.Errorf("duplicate candidate id %q", candidate.ID)
		}
		if candidate.Route.ID == "" || candidate.ID != liquidlane.NewCandidateID(candidate.Route, candidate.DiscountID) {
			return nil, errors.Errorf("candidate %q has invalid route", candidate.ID)
		}
		candidates[id] = candidate
	}
	return candidates, nil
}

func validateLeg(
	tokenOut common.Address,
	candidates map[string]liquidlane.QuoteCandidate,
	seen map[string]struct{},
	leg QuoteLeg,
	index int,
) (liquidlane.QuoteCandidate, error) {
	if _, duplicate := seen[leg.CandidateID]; duplicate {
		return liquidlane.QuoteCandidate{}, errors.Errorf("duplicate candidate %q", leg.CandidateID)
	}
	seen[leg.CandidateID] = struct{}{}
	candidate, exists := candidates[leg.CandidateID]
	if !exists {
		return liquidlane.QuoteCandidate{}, errors.Errorf("unknown candidate %q", leg.CandidateID)
	}
	if candidate.Route.TokenOut != tokenOut {
		return liquidlane.QuoteCandidate{}, errors.Errorf("candidate %q asset does not match tokenOut", leg.CandidateID)
	}
	if leg.AmountIn == nil || leg.AmountIn.Sign() <= 0 {
		return liquidlane.QuoteCandidate{}, errors.Errorf("leg %d has invalid amountIn", index)
	}
	if leg.AmountOut == nil || leg.AmountOut.Sign() <= 0 {
		return liquidlane.QuoteCandidate{}, errors.Errorf("leg %d has invalid amountOut", index)
	}
	if candidate.MaxAmountOut == nil || candidate.MaxAmountOut.Sign() <= 0 ||
		leg.AmountOut.Cmp(candidate.MaxAmountOut) > 0 {
		return liquidlane.QuoteCandidate{}, errors.Errorf("leg %d exceeds candidate maxAmountOut", index)
	}
	if candidate.Rate == nil || candidate.Rate.Sign() <= 0 {
		return liquidlane.QuoteCandidate{}, errors.Errorf("candidate %q has invalid rate", leg.CandidateID)
	}
	achievable := liquidlane.AmountOutForRate(
		leg.AmountIn,
		candidate.Rate,
		candidate.Route.TokenInDecimals,
		candidate.Route.TokenOutDecimals,
	)
	if leg.AmountOut.Cmp(achievable) > 0 {
		return liquidlane.QuoteCandidate{}, errors.Errorf("leg %d exceeds output achievable at candidate rate", index)
	}
	return candidate, nil
}
