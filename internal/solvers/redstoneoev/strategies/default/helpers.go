package defaultstrategy

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"

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

func cloneBig(v *big.Int) *big.Int {
	if v == nil {
		return nil
	}
	return new(big.Int).Set(v)
}

func orZero(v *big.Int) *big.Int {
	if v == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(v)
}

func exp10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

func ceilMulDiv(x, y, denom *big.Int) *big.Int {
	if x == nil || y == nil || denom == nil || x.Sign() <= 0 || y.Sign() <= 0 || denom.Sign() <= 0 {
		return new(big.Int)
	}
	num := new(big.Int).Mul(x, y)
	q, r := new(big.Int).QuoRem(num, denom, new(big.Int))
	if r.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	return q
}

func saturatingAddUint64(a, b uint64) uint64 {
	if b > ^uint64(0)-a {
		return ^uint64(0)
	}
	return a + b
}

func saturatingMulUint64(a, b uint64) uint64 {
	if a != 0 && b > ^uint64(0)/a {
		return ^uint64(0)
	}
	return a * b
}

func lowerHash(h common.Hash) string {
	return strings.ToLower(h.Hex())
}

func lowerAddresses(addrs []common.Address) []string {
	out := make([]string, len(addrs))
	for i, a := range addrs {
		out[i] = strings.ToLower(a.Hex())
	}
	return out
}
