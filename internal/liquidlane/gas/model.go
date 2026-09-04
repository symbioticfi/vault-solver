// Package gas predicts LiquidLane adapter paths and converts native gas into output-token units.
package gas

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

type Route uint8

const (
	RouteUnknown Route = iota
	RouteAcquire
	RouteAllocate
	RouteDeallocate
)

type State struct {
	FreeAssets   *big.Int
	Withdrawable *big.Int
	Acquire      map[common.Address]*big.Int
}

type AdapterState struct {
	Vault   common.Address              `json:"vault"`
	Acquire map[common.Address]*big.Int `json:"acquire"`
}

type VaultState struct {
	FreeAssets   *big.Int `json:"freeAssets"`
	Withdrawable *big.Int `json:"withdrawable"`
}

type Snapshot struct {
	Adapters map[common.Address]*AdapterState `json:"adapters"`
	Vaults   map[common.Address]*VaultState   `json:"vaults"`
}

type Demand struct {
	Collateral common.Address
	AmountOut  *big.Int
}

type AdapterDemand struct {
	Demand

	Adapter common.Address
	Vault   common.Address
}

type Prediction struct {
	Units  uint64
	Routes []Route
}

func (route Route) String() string {
	switch route { //nolint:exhaustive // Invalid values are intentionally conservative.
	case RouteAcquire:
		return "acquire"
	case RouteAllocate:
		return "allocate"
	case RouteDeallocate:
		return "deallocate"
	default:
		return "unknown"
	}
}

func RoutesString(routes []Route) string {
	labels := make([]string, len(routes))
	for index, route := range routes {
		labels[index] = route.String()
	}
	return strings.Join(labels, ",")
}
