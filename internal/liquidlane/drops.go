package liquidlane

import (
	"context"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

var coreReverts = RevertDecoder()

// Drop is a route or adapter a read left out, with the check that failed. A failed sub-call names its
// decoded revert, e.g. `getMaxAssets reverted: InvalidOracle()`.
type Drop struct {
	Adapter common.Address
	TokenIn common.Address // zero when the whole adapter was left out
	Reason  string
}

func (d Drop) String() string {
	if d.TokenIn == (common.Address{}) {
		return "adapter " + d.Adapter.Hex() + ": " + d.Reason
	}
	return "adapter " + d.Adapter.Hex() + " tokenIn " + d.TokenIn.Hex() + ": " + d.Reason
}

// DropTrace receives each Drop a read makes. A read given none logs its drops at V(1).
type DropTrace func(Drop)

func dropSink(ctx context.Context, traces []DropTrace) DropTrace {
	if len(traces) > 0 {
		return func(d Drop) {
			for _, trace := range traces {
				if trace != nil {
					trace(d)
				}
			}
		}
	}
	log := observability.Log(ctx).V(1)
	return func(d Drop) {
		log.Info("liquidlane: read dropped", "adapter", d.Adapter.Hex(), "tokenIn", d.TokenIn.Hex(), "reason", d.Reason)
	}
}

// collectDrops keeps drops for an error that must say why a read came back incomplete.
func collectDrops() (DropTrace, func() string) {
	var drops []string
	return func(d Drop) { drops = append(drops, d.String()) }, func() string {
		if len(drops) == 0 {
			return "no drop recorded"
		}
		return strings.Join(drops, "; ")
	}
}

func callFailure(method string, result chain.CallResult) string {
	return method + " reverted: " + coreReverts.Describe(result.ReturnData)
}

// decodeCall unpacks a multicall sub-call, or says why it cannot be used. Reasons are strings rather
// than errors so the per-block drop path does not capture stacks.
func decodeCall[T any](method string, result chain.CallResult, unpack func([]byte) (T, error)) (T, string) {
	var zero T
	if !result.Success {
		return zero, callFailure(method, result)
	}
	value, err := unpack(result.ReturnData)
	if err != nil {
		return zero, method + ": " + err.Error()
	}
	return value, ""
}
