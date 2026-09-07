package chain

import (
	"context"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/bindings/erc20"
)

var erc20B = erc20.NewERC20()

// Decimals memoizes immutable token metadata. The mutex protects the cache only;
// misses may overlap on RPC, so a slow token never blocks unrelated cached reads.
type Decimals struct {
	chain Multicaller
	mu    sync.Mutex
	cache map[common.Address]int
}

func NewDecimals(c Multicaller) *Decimals {
	return &Decimals{chain: c, cache: make(map[common.Address]int)}
}

func (d *Decimals) Get(ctx context.Context, token common.Address) (int, error) {
	d.mu.Lock()
	cached, found := d.cache[token]
	d.mu.Unlock()
	if found {
		return cached, nil
	}
	value, err := ReadOne(ctx, d.chain, Call{Target: token, AllowFailure: true, Data: erc20B.PackDecimals()}, erc20B.UnpackDecimals)
	if err != nil {
		return 0, errors.Errorf("erc20.decimals() unavailable for %s: %w", token, err)
	}
	d.mu.Lock()
	d.cache[token] = int(value)
	d.mu.Unlock()
	return int(value), nil
}
