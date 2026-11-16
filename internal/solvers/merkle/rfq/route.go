package rfq

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/erc20"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

var errRoutesNotListed = errors.New("no usable quote-only connector routes listed yet")

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

// DiscoverRoutes retries Initialize until routes are published or ctx ends, so a quote-only
// process keeps serving (and declining) through a chain, backend or curator outage at startup.
func (s *Solver) DiscoverRoutes(ctx context.Context, retry time.Duration) {
	for attempt := 0; ; attempt++ {
		err := s.Initialize(ctx)
		if err == nil || ctx.Err() != nil {
			return
		}
		log := observability.Log(ctx)
		if attempt == 0 && !errors.Is(err, errRoutesNotListed) {
			log.Error(err, "RFQ Merkle route discovery failed; quotes decline while it retries")
		} else {
			log.V(1).Info("RFQ Merkle route discovery retry failed", "attempt", attempt, "reason", err.Error())
		}
		timer := time.NewTimer(retry)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Solver) buildRoutes(ctx context.Context) (map[common.Address]recipeRoute, error) {
	prepared, err := s.workflow.discoverRecipes(ctx, s.recipeExecutors)
	if err != nil {
		return nil, err
	}
	// Only quote-only processes have no permission source; they skip the Merkle tree entirely.
	var listed listedVenues
	if s.workflow.source == nil {
		if listed, err = s.listedVenues(ctx); err != nil {
			return nil, err
		}
	}
	routes := make(map[common.Address]recipeRoute, len(prepared))
	for _, member := range prepared {
		snapshot, recipe := member.Snapshot, member.Recipe
		if _, err := recipes.RouteExecution(ctx, s.workflow.chain, snapshot, s.workflow.caller, recipeexecutor.NewRecipeExecutor().PackExecute(nil, nil, nil, nil)); err != nil {
			return nil, err
		}
		if s.workflow.source == nil {
			route, ok, err := s.listedRoute(ctx, snapshot, listed)
			if err != nil {
				return nil, err
			}
			if ok {
				routes[snapshot.Executor] = route
			}
			continue
		}
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
	if s.workflow.source == nil && len(routes) == 0 {
		return nil, errRoutesNotListed
	}
	return routes, nil
}

// listedVenues groups the backend discount list as venue -> asset -> RWA. Without the curator
// tree, quote-only routes trust these listings for the RWA a connector accepts.
type listedVenues map[common.Address]map[common.Address]map[common.Address]struct{}

func (s *Solver) listedVenues(ctx context.Context) (listedVenues, error) {
	list, err := s.backend.listDiscounts(ctx)
	if err != nil {
		return nil, errors.Errorf("list connector discounts: %w", err)
	}
	if list == nil {
		return nil, errors.New("list connector discounts: empty response")
	}
	out := make(listedVenues)
	for _, item := range list.Discounts {
		if !common.IsHexAddress(item.Adapter) || !common.IsHexAddress(item.TokenToRedeem) || !common.IsHexAddress(item.Collateral) {
			continue
		}
		venue, asset := common.HexToAddress(item.Adapter), common.HexToAddress(item.Collateral)
		if out[venue] == nil {
			out[venue] = make(map[common.Address]map[common.Address]struct{})
		}
		if out[venue][asset] == nil {
			out[venue][asset] = make(map[common.Address]struct{})
		}
		out[venue][asset][common.HexToAddress(item.TokenToRedeem)] = struct{}{}
	}
	return out, nil
}

// listedRoute picks the single listed connector that pays from the recipe account at its
// snapshot block. Several matching connectors are ambiguous and publish no route.
func (s *Solver) listedRoute(ctx context.Context, snapshot recipes.Snapshot, listed listedVenues) (recipeRoute, bool, error) {
	var route recipeRoute
	found := false
	for venue, assets := range listed {
		state, err := s.workflow.state(ctx, venue, new(big.Int).SetUint64(snapshot.BlockNumber))
		if err != nil {
			// The shared list also carries native adapters, which revert or decode as non-connectors.
			// Any other failure aborts discovery so the caller retries instead of dropping the route.
			if !notConnector(err) || ctx.Err() != nil {
				return recipeRoute{}, false, errors.Errorf("listed connector %s: %w", venue.Hex(), err)
			}
			observability.Log(ctx).V(1).Info("listed venue is not a connector", "venue", venue.Hex(), "reason", err.Error())
			continue
		}
		tokens := assets[state.Asset]
		if state.Account != snapshot.Account || len(tokens) == 0 {
			continue
		}
		if found {
			return recipeRoute{}, false, nil
		}
		route, found = recipeRoute{connector: venue, account: state.Account, asset: state.Asset, tokens: tokens}, true
	}
	return route, found, nil
}

func notConnector(err error) bool {
	var rpcErr rpc.Error
	return errors.Is(err, errNotConnector) || (errors.As(err, &rpcErr) && rpcErr.ErrorCode() == 3) ||
		strings.Contains(strings.ToLower(err.Error()), "execution reverted")
}

// ResolveRoute selects a unique compatible recipe using only the startup cache.
// Ambiguous routes decline rather than assigning capacity to an arbitrary recipe.
func (s *Solver) ResolveRoute(connector, rwa, asset common.Address) (executor, account common.Address, ok bool) {
	s.routesMu.RLock()
	defer s.routesMu.RUnlock()
	for candidate, route := range s.routes {
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

// Each vault may have its own AccessManager. Keep the authenticated on-chain
// caller in SDK snapshots and batch discovery for recipes sharing that caller.
func (w *connectorWorkflow) discoverRecipes(ctx context.Context, executors []common.Address) ([]recipes.PreparedRecipe, error) {
	identities, err := recipes.DiscoverExecutors(ctx, w.chain, executors)
	if err != nil {
		return nil, err
	}
	groups := make(map[common.Address][]common.Address)
	var callers []common.Address
	for _, executor := range executors {
		caller := identities[executor].Caller
		if _, exists := groups[caller]; !exists {
			callers = append(callers, caller)
		}
		groups[caller] = append(groups[caller], executor)
	}
	var prepared []recipes.PreparedRecipe
	for _, caller := range callers {
		members := groups[caller]
		accounts := make([]common.Address, len(members))
		for i, executor := range members {
			accounts[i] = identities[executor].Account
		}
		batch, err := recipes.DiscoverBatch(ctx, w.chain, members, accounts, caller, recipes.PreparationOptions{ChainID: w.chainID})
		if err != nil {
			return nil, err
		}
		prepared = append(prepared, batch...)
	}
	return prepared, nil
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
