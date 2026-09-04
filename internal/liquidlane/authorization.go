package liquidlane

import "github.com/ethereum/go-ethereum/common"

// UnauthorizedAdapters returns missing adapter-wide authorizations in configuration order.
func UnauthorizedAdapters(configured, authorized []Route) []common.Address {
	allowed := make(map[common.Address]struct{}, len(authorized))
	for _, route := range authorized {
		allowed[route.Adapter] = struct{}{}
	}

	seen := make(map[common.Address]struct{})
	missing := make([]common.Address, 0)
	for _, route := range configured {
		if _, ok := allowed[route.Adapter]; ok {
			continue
		}
		if _, ok := seen[route.Adapter]; ok {
			continue
		}
		seen[route.Adapter] = struct{}{}
		missing = append(missing, route.Adapter)
	}
	return missing
}
