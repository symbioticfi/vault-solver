package connector

import (
	"context"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/internal/parse"
)

type route struct {
	connector, account, asset common.Address
	tokens                    map[common.Address]struct{} // immutable after publication
}

// Routes caches the verified payment route of every configured recipe executor.
type Routes struct {
	workflow  *Workflow
	executors []common.Address
	mu        sync.RWMutex
	routes    map[common.Address]route
}

func NewRoutes(workflow *Workflow, executors []common.Address) *Routes {
	return &Routes{workflow: workflow, executors: executors}
}

// ParseRecipeExecutors parses the recipeExecutors list shared by connector solver configurations.
func ParseRecipeExecutors(values []string) ([]common.Address, error) {
	out := make([]common.Address, 0, len(values))
	for _, value := range values {
		address, err := parse.NonZeroAddress(value, "recipeExecutors")
		if err != nil {
			return nil, err
		}
		out = append(out, address)
	}
	return out, nil
}

// Initialize publishes verified payment routes together. Quotes keep declining
// while discovery runs without holding the route lock.
func (r *Routes) Initialize(ctx context.Context) error {
	r.mu.RLock()
	initialized := r.routes != nil
	r.mu.RUnlock()
	if initialized {
		return nil
	}
	routes, err := r.build(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	if r.routes == nil {
		r.routes = routes
	}
	r.mu.Unlock()
	return nil
}

// Len reports how many recipe routes are published.
func (r *Routes) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.routes)
}

func (r *Routes) build(ctx context.Context) (map[common.Address]route, error) {
	w := r.workflow
	prepared, err := w.discoverRecipes(ctx, r.executors)
	if err != nil {
		return nil, err
	}
	routes := make(map[common.Address]route, len(prepared))
	for _, member := range prepared {
		snapshot, recipe := member.Snapshot, member.Recipe
		if _, err := recipes.RouteExecution(ctx, w.Chain, snapshot, w.Filler, recipeexecutor.NewRecipeExecutor().PackExecute(nil, nil, nil, nil)); err != nil {
			return nil, err
		}
		tree, err := w.permissionTree(ctx, snapshot)
		if err != nil {
			return nil, err
		}
		venue, asset, err := paymentConnector(recipe, tree)
		if err != nil {
			return nil, err
		}
		state, err := w.state(ctx, venue, new(big.Int).SetUint64(snapshot.BlockNumber))
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
			token, err := actionToken(action, venue)
			if err != nil {
				return nil, err
			}
			if token != (common.Address{}) {
				tokens[token] = struct{}{}
			}
		}
		routes[snapshot.Executor] = route{connector: venue, account: state.Account, asset: asset, tokens: tokens}
	}
	return routes, nil
}

// Resolve selects a unique compatible recipe using only the startup cache.
// Ambiguous routes decline rather than assigning capacity to an arbitrary recipe.
func (r *Routes) Resolve(connector, rwa, asset common.Address) (executor, account common.Address, ok bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for candidate, route := range r.routes {
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
