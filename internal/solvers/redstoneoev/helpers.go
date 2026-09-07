package redstoneoev

import (
	"github.com/symbioticfi/vault-solver/internal/chain"
)

func allSuccess(res []chain.CallResult, expectedLen int) bool {
	if len(res) != expectedLen {
		return false
	}
	for i := range res {
		if !res[i].Success {
			return false
		}
	}
	return true
}
