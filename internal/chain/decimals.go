package chain

import (
	"context"
	"slices"
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
	v, err := decodeDecimals(res[0].ReturnData)
	if err != nil {
		return 0, err
	}
	d.store(token, v)
	return v, nil
}

// GetMany returns decimals for several tokens, reading every uncached one in a SINGLE multicall (so
// resolving N new tokens costs one round-trip, not N). Cached tokens are served without any call. It
// is lenient per token: a token whose decimals() reverts is simply omitted from the result (the caller
// fails that item closed) rather than failing the whole batch — only a transport error is returned.
func (d *Decimals) GetMany(ctx context.Context, tokens []common.Address) (map[common.Address]int, error) {
	out := make(map[common.Address]int, len(tokens))
	var miss []common.Address
	d.mu.Lock()
	for _, t := range tokens {
		if v, ok := d.cache[t]; ok {
			out[t] = v
		} else if !slices.Contains(miss, t) {
			miss = append(miss, t)
		}
	}
	d.mu.Unlock()
	if len(miss) == 0 {
		return out, nil
	}

	calls := make([]Call, len(miss))
	for i, t := range miss {
		calls[i] = Call{Target: t, AllowFailure: true, Data: erc20B.PackDecimals()}
	}
	res, err := d.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	for i, t := range miss {
		if i >= len(res) || !res[i].Success {
			continue // token's decimals() reverted — omit; caller skips that market (fail closed)
		}
		v, derr := decodeDecimals(res[i].ReturnData)
		if derr != nil {
			continue
		}
		d.store(t, v)
		out[t] = v
	}
	return out, nil
}

func (d *Decimals) store(token common.Address, v int) {
	d.mu.Lock()
	d.cache[token] = v
	d.mu.Unlock()
}

func decodeDecimals(data []byte) (int, error) {
	v, err := erc20B.UnpackDecimals(data)
	if err != nil {
		return 0, errors.Errorf("unpack decimals: %w", err)
	}
	return int(v), nil
}
