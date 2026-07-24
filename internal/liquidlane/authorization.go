package liquidlane

import "github.com/ethereum/go-ethereum/common"

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
