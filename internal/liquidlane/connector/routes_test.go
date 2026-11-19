package connector

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestCachedRouteResolutionRequiresUniqueRWA(t *testing.T) {
	first, second := common.HexToAddress("0x43"), common.HexToAddress("0x44")
	for name, sameRWA := range map[string]bool{"different RWA": false, "same RWA": true} {
		t.Run(name, func(t *testing.T) {
			otherToken := tOut
			if sameRWA {
				otherToken = tIn
			}
			r := &Routes{routes: map[common.Address]route{
				first:  {connector: vlt, account: vlt, asset: tOut, tokens: map[common.Address]struct{}{tIn: {}}},
				second: {connector: vlt, account: vlt, asset: tOut, tokens: map[common.Address]struct{}{otherToken: {}}},
			}}
			executor, account, ok := r.Resolve(vlt, tIn, tOut)
			if ok == sameRWA || (ok && (executor != first || account != vlt)) {
				t.Fatalf("route=(%s,%s,%t)", executor, account, ok)
			}
			if r.Len() != 2 {
				t.Fatal("published routes not counted")
			}
		})
	}
}
