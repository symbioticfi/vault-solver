package liquidlane

import "github.com/ethereum/go-ethereum/common"

// UnauthorizedAdapters returns configured adapter addresses absent from the authorized route set.
// Authorization is adapter-wide, so duplicate physical routes produce one address in config order.
func UnauthorizedAdapters(routes, authorized []Route) []common.Address {
	covered := make(map[common.Address]struct{}, len(routes)+len(authorized))
	for _, route := range authorized {
		covered[route.Adapter] = struct{}{}
	}
	var missing []common.Address
	for _, route := range routes {
		if _, known := covered[route.Adapter]; known {
			continue
		}
		covered[route.Adapter] = struct{}{}
		missing = append(missing, route.Adapter)
	}
	return missing
}
