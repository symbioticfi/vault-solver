package rfq

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

type recipeRoute struct {
	connector, account, asset common.Address
}

// Initialize verifies every configured recipe and derives its payment connector.
// Routes are published together before the quote server or fill loop starts.
func (s *Solver) Initialize(ctx context.Context) ([]common.Address, error) {
	if s.routes == nil {
		routes := make(map[common.Address]recipeRoute, len(s.recipeExecutors))
		for _, executor := range s.recipeExecutors {
			snapshot, recipe, err := recipes.Discover(ctx, s.workflow.chain, executor, nil)
			if err != nil {
				return nil, err
			}
			if snapshot.Caller != s.workflow.caller {
				return nil, errors.New("connector recipe caller mismatch")
			}
			venue, asset, err := paymentConnector(recipe)
			if err != nil {
				return nil, err
			}
			state, err := s.workflow.state(ctx, venue, new(big.Int).SetUint64(snapshot.BlockNumber))
			if err != nil {
				return nil, err
			}
			if state.Account != snapshot.Account || state.Asset != asset {
				return nil, errors.New("recipe payment connector account or asset mismatch")
			}
			routes[executor] = recipeRoute{connector: venue, account: state.Account, asset: asset}
		}
		s.routes = routes
	}
	venues := make([]common.Address, 0, len(s.routes))
	seen := make(map[common.Address]bool)
	for _, executor := range s.recipeExecutors {
		venue := s.routes[executor].connector
		if !seen[venue] {
			venues = append(venues, venue)
			seen[venue] = true
		}
	}
	return venues, nil
}

// AcceptsRoute binds backend inventory to a locally configured, verified recipe.
// The RWA-specific swap permission is verified again when building the fill.
func (s *Solver) AcceptsRoute(executor, connector, account, asset common.Address) bool {
	route, ok := s.routes[executor]
	return ok && route.connector == connector && route.account == account && route.asset == asset
}

func paymentConnector(recipe recipes.Recipe) (venue, asset common.Address, err error) {
	inspection, err := recipes.Inspect(recipe)
	if err != nil {
		return common.Address{}, common.Address{}, err
	}
	for _, slot := range inspection.Slots {
		if slot.Operation != recipes.Approve || slot.Amount.Source != recipes.Input || slot.Amount.Value == nil || slot.Amount.Value.Cmp(big.NewInt(1)) != 0 {
			continue
		}
		if venue != (common.Address{}) || slot.Spender == (common.Address{}) || slot.Token == (common.Address{}) {
			return common.Address{}, common.Address{}, errors.New("recipe requires one payment approval from input 1")
		}
		venue, asset = slot.Spender, slot.Token
	}
	if venue == (common.Address{}) {
		return venue, asset, errors.New("recipe payment approval from input 1 missing")
	}
	return venue, asset, nil
}
