package defaultstrategy

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func lowerAddresses(addrs []common.Address) []string {
	out := make([]string, len(addrs))
	for i, a := range addrs {
		out[i] = hexutil.Encode(a[:])
	}
	return out
}
