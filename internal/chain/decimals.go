package chain

import (
	"context"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/erc20"
)

var erc20B = erc20.NewERC20()

type decimalsEntry struct {
	ready chan struct{}
	value int
	err   error
}

// Decimals memoizes immutable ERC-20 metadata and coalesces concurrent cache misses.
type Decimals struct {
	chain *Client
	mu    sync.Mutex
	cache map[common.Address]*decimalsEntry
}

func NewDecimals(client *Client) *Decimals {
	return &Decimals{chain: client, cache: make(map[common.Address]*decimalsEntry)}
}

func (d *Decimals) Get(ctx context.Context, token common.Address) (int, error) {
	d.mu.Lock()
	entry, exists := d.cache[token]
	if !exists {
		entry = &decimalsEntry{ready: make(chan struct{})}
		d.cache[token] = entry
	}
	d.mu.Unlock()
	if exists {
		select {
		case <-entry.ready:
			return entry.value, entry.err
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}

	entry.value, entry.err = d.read(ctx, token)
	d.mu.Lock()
	if entry.err != nil {
		delete(d.cache, token)
	}
	close(entry.ready)
	d.mu.Unlock()
	return entry.value, entry.err
}

func (d *Decimals) read(ctx context.Context, token common.Address) (int, error) {
	results, err := d.chain.Multicall(ctx, []Call{{Target: token, Data: erc20B.PackDecimals()}})
	if err != nil {
		return 0, err
	}
	if len(results) != 1 || !results[0].Success {
		return 0, errors.Errorf("erc20.decimals() reverted for %s", token)
	}
	value, err := erc20B.UnpackDecimals(results[0].ReturnData)
	if err != nil {
		return 0, errors.Errorf("unpack decimals: %w", err)
	}
	return int(value), nil
}
