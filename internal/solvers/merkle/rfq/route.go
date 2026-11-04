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

// Initialize verifies every configured recipe and publishes its payment route.
// It is deferred while signed work is pending, so recovery never depends on the
// current recipe; quotes reject routes until they are published. Routes are built
// without routesMu so quotes keep declining while the RPC reads run.
func (s *Solver) Initialize(ctx context.Context) ([]common.Address, error) {
	if s.sender != nil && s.sender.PendingID() != "" {
		return nil, nil
	}
	s.routesMu.RLock()
	routes := s.routes
	s.routesMu.RUnlock()
	if routes == nil {
		built, err := s.buildRoutes(ctx)
		if err != nil {
			return nil, err
		}
		s.routesMu.Lock()
		if s.routes == nil {
			s.routes = built
		}
		routes = s.routes
		s.routesMu.Unlock()
	}
	venues := make([]common.Address, 0, len(routes))
	seen := make(map[common.Address]bool)
	for _, executor := range s.recipeExecutors {
		venue := routes[executor].connector
		if !seen[venue] {
			venues = append(venues, venue)
			seen[venue] = true
		}
	}
	return venues, nil
}

func (s *Solver) buildRoutes(ctx context.Context) (map[common.Address]recipeRoute, error) {
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
	return routes, nil
}

// AcceptsRoute binds backend inventory to a locally configured, verified recipe.
// The RWA-specific swap permission is verified again when building the fill.
func (s *Solver) AcceptsRoute(executor, connector, account, asset common.Address) bool {
	route, ok := s.recipeRoute(executor)
	return ok && route.connector == connector && route.account == account && route.asset == asset
}

// AcceptsConnector reports whether a published route pays through connector.
func (s *Solver) AcceptsConnector(connector common.Address) bool {
	s.routesMu.RLock()
	defer s.routesMu.RUnlock()
	for _, route := range s.routes {
		if route.connector == connector {
			return true
		}
	}
	return false
}

func (s *Solver) recipeRoute(executor common.Address) (recipeRoute, bool) {
	s.routesMu.RLock()
	defer s.routesMu.RUnlock()
	route, ok := s.routes[executor]
	return route, ok
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
