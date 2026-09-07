package redstoneoev

import (
	"math/big"
	"testing"
)

func TestStateCacheOwnsExecutorAccounting(t *testing.T) {
	original := cachedState{Exec: ExecutorState{Nonce: big.NewInt(7), Deposit: big.NewInt(100)}}
	var cache stateCache
	cache.store(original)
	original.Exec.Deposit.SetInt64(0)
	first, ok := cache.load()
	if !ok || first.Exec.Deposit.Int64() != 100 {
		t.Fatal("cache retained caller's deposit")
	}
	first.Exec.Nonce.SetInt64(99)
	second, _ := cache.load()
	if second.Exec.Nonce.Int64() != 7 {
		t.Fatal("caller mutated cached nonce")
	}
}
