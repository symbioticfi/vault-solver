package chain

import (
	"context"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/erc20"
)

// erc20B is the generated minimal ERC-20 binding (decimals() only) this cache packs/unpacks through.
var erc20B = erc20.NewERC20()

// Decimals is a concurrency-safe, multicall-backed cache of ERC-20 token decimals. Token decimals are
// immutable, so a value is read once and memoized. Solvers serve quotes/refreshes concurrently, so the
// cache is mutex-guarded. It is a generic cross-solver primitive (both the RFQ and OEV readers need
// it), so it lives in the chain layer next to Multicall.
type Decimals struct {
	chain *Client
	mu    sync.Mutex
	cache map[common.Address]int
}

// NewDecimals builds a decimals cache over the given client.
func NewDecimals(c *Client) *Decimals {
	return &Decimals{chain: c, cache: make(map[common.Address]int)}
}

// Get returns token's decimals, reading (and caching) it on a miss.
func (d *Decimals) Get(ctx context.Context, token common.Address) (int, error) {
	d.mu.Lock()
	if v, ok := d.cache[token]; ok {
		d.mu.Unlock()
		return v, nil
	}
	d.mu.Unlock()

	res, err := d.chain.Multicall(ctx, []Call{{Target: token, Data: erc20B.PackDecimals()}})
	if err != nil {
		return 0, err
	}
	if len(res) != 1 || !res[0].Success {
		return 0, errors.Errorf("erc20.decimals() reverted for %s", token)
	}
	v, err := erc20B.UnpackDecimals(res[0].ReturnData)
	if err != nil {
		return 0, errors.Errorf("unpack decimals: %w", err)
	}
	decimals := int(v)
	d.mu.Lock()
	d.cache[token] = decimals
	d.mu.Unlock()
	return decimals, nil
}
