package rfq

import (
	"bytes"
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/erc20"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

type recipeRoute struct {
	connector, account, asset common.Address
	tokens                    map[common.Address]struct{} // immutable after publication
}

// Initialize publishes verified payment routes together. Quotes keep declining
// while discovery runs without holding routesMu.
func (s *Solver) Initialize(ctx context.Context) error {
	s.routesMu.RLock()
	initialized := s.routes != nil
	s.routesMu.RUnlock()
	if initialized {
		return nil
	}
	routes, err := s.buildRoutes(ctx)
	if err != nil {
		return err
	}
	s.routesMu.Lock()
	if s.routes == nil {
		s.routes = routes
	}
	s.routesMu.Unlock()
	return nil
}

func (s *Solver) buildRoutes(ctx context.Context) (map[common.Address]recipeRoute, error) {
	identities, err := recipes.DiscoverExecutors(ctx, s.workflow.chain, s.recipeExecutors)
	if err != nil {
		return nil, err
	}
	accounts := make([]common.Address, len(s.recipeExecutors))
	for i, executor := range s.recipeExecutors {
		accounts[i] = identities[executor].Account
	}
	prepared, err := recipes.DiscoverBatch(ctx, s.workflow.chain, s.recipeExecutors, accounts, s.workflow.caller, recipes.PreparationOptions{ChainID: s.workflow.chainID})
	if err != nil {
		return nil, err
	}
	routes := make(map[common.Address]recipeRoute, len(prepared))
	for _, member := range prepared {
		snapshot, recipe := member.Snapshot, member.Recipe
		tree, err := s.workflow.permissionTree(ctx, snapshot)
		if err != nil {
			return nil, err
		}
		venue, asset, err := paymentConnector(recipe, tree)
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
		actions, err := recipes.ManagedActions(recipe, tree)
		if err != nil {
			return nil, err
		}
		tokens := make(map[common.Address]struct{})
		for _, action := range actions {
			token, err := connectorActionToken(action, venue)
			if err != nil {
				return nil, err
			}
			if token != (common.Address{}) {
				tokens[token] = struct{}{}
			}
		}
		routes[snapshot.Executor] = recipeRoute{connector: venue, account: state.Account, asset: asset, tokens: tokens}
	}
	return routes, nil
}

// ResolveRoute uses only the published startup cache. A capacity hint must name
// a compatible configured recipe. Without a hint the compatible route must be
// unique: capacities measured for different recipes are not interchangeable.
func (s *Solver) ResolveRoute(hint, connector, rwa, asset common.Address) (executor, account common.Address, ok bool) {
	s.routesMu.RLock()
	defer s.routesMu.RUnlock()
	for candidate, route := range s.routes {
		if hint != (common.Address{}) && hint != candidate {
			continue
		}
		if route.connector != connector || route.asset != asset {
			continue
		}
		if _, supported := route.tokens[rwa]; !supported {
			continue
		}
		if ok {
			return common.Address{}, common.Address{}, false
		}
		executor, account, ok = candidate, route.account, true
	}
	return executor, account, ok
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

func paymentConnector(recipe recipes.Recipe, tree *recipes.Tree) (venue, asset common.Address, err error) {
	inspection, err := recipes.Inspect(recipe)
	if err != nil {
		return common.Address{}, common.Address{}, err
	}
	selectPayment := func(token, spender common.Address) error {
		if venue != (common.Address{}) || spender == (common.Address{}) || token == (common.Address{}) {
			return errors.New("recipe requires one payment approval from input 1")
		}
		venue, asset = spender, token
		return nil
	}
	for _, slot := range inspection.Slots {
		if slot.Operation != recipes.Approve || slot.Amount.Source != recipes.Input || slot.Amount.Value == nil || slot.Amount.Value.Cmp(big.NewInt(1)) != 0 {
			continue
		}
		if err := selectPayment(slot.Token, slot.Spender); err != nil {
			return common.Address{}, common.Address{}, err
		}
	}
	actions, err := recipes.ManagedActions(recipe, tree)
	if err != nil {
		return common.Address{}, common.Address{}, err
	}
	selector := erc20.NewERC20().PackApprove(common.Address{}, new(big.Int))[:4]
	for _, action := range actions {
		if len(action.Leaf.Data) != 68 || !bytes.Equal(action.Leaf.Data[:4], selector) {
			continue
		}
		amount, err := action.Field(36)
		if err != nil || amount.Source != recipes.Input || amount.Value.Cmp(big.NewInt(1)) != 0 {
			continue
		}
		if action.Slot.Gate.Source != recipes.Literal || action.Slot.Gate.Value.Cmp(big.NewInt(1)) != 0 {
			return common.Address{}, common.Address{}, errors.New("recipe payment approval must be unconditional")
		}
		if err := action.Pinned(4, 32); err != nil {
			return common.Address{}, common.Address{}, err
		}
		if err := selectPayment(action.Leaf.Target, common.BytesToAddress(action.Leaf.Data[4:36])); err != nil {
			return common.Address{}, common.Address{}, err
		}
	}
	if venue == (common.Address{}) {
		return venue, asset, errors.New("recipe payment approval from input 1 missing")
	}
	return venue, asset, nil
}
