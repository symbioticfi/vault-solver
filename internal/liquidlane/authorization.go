package liquidlane

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
)

// RouteAuthorizer is Reader.FilterAuthorizedRoutes or a reader that forwards to it.
type RouteAuthorizer func(ctx context.Context, routes []Route, filler common.Address, trace ...DropTrace) ([]Route, error)

// ValidateDirectAuthorization fails when executor cannot fill every adapter of routes directly,
// naming the adapters and the reads that dropped them.
func ValidateDirectAuthorization(ctx context.Context, authorize RouteAuthorizer, routes []Route, executor common.Address) error {
	var drops []Drop
	authorized, err := authorize(ctx, routes, executor, LogDrops(ctx), func(d Drop) { drops = append(drops, d) })
	if err != nil {
		return err
	}
	missing := UnauthorizedAdapters(routes, authorized)
	if len(missing) == 0 {
		return nil
	}
	err = errors.Errorf("executor %s is not authorized as direct filler for configured adapters: %v", executor.Hex(), missing)
	if dropped := SummarizeDrops(drops); dropped != "" {
		return errors.Errorf("%w (%s)", err, dropped)
	}
	return err
}

// UnauthorizedAdapters returns configured adapter addresses absent from the authorized route set.
// Authorization is adapter-wide, so duplicate physical routes produce one address in config order.
func UnauthorizedAdapters(routes, authorized []Route) []common.Address {
	allowed := make(map[common.Address]bool, len(authorized))
	for _, route := range authorized {
		allowed[route.Adapter] = true
	}

	seen := make(map[common.Address]bool)
	missing := make([]common.Address, 0)
	for _, route := range routes {
		if !allowed[route.Adapter] && !seen[route.Adapter] {
			missing = append(missing, route.Adapter)
			seen[route.Adapter] = true
		}
	}
	return missing
}
