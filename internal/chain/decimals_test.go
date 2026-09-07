package chain

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

type decimalsBackend func(context.Context, []Call) ([]CallResult, error)

func (f decimalsBackend) Multicall(ctx context.Context, calls []Call) ([]CallResult, error) {
	return f(ctx, calls)
}

func TestDecimalsRetriesFailuresAndCachesZero(t *testing.T) {
	rpcErr := errors.New("unavailable RPC")
	for _, tc := range []struct {
		name    string
		results []CallResult
		err     error
	}{
		{name: "transport", err: rpcErr},
		{name: "missing result"},
		{name: "revert", results: []CallResult{{ReturnData: make([]byte, 32)}}},
		{name: "malformed", results: []CallResult{{Success: true, ReturnData: []byte{18}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			cache := NewDecimals(decimalsBackend(func(context.Context, []Call) ([]CallResult, error) {
				calls++
				if calls == 1 {
					return tc.results, tc.err
				}
				return []CallResult{{Success: true, ReturnData: make([]byte, 32)}}, nil
			}))
			token := common.Address{19: 1}
			if _, err := cache.Get(t.Context(), token); err == nil || tc.err != nil && !errors.Is(err, tc.err) {
				t.Fatalf("read failure was lost: %v", err)
			}
			for range 2 {
				if value, err := cache.Get(t.Context(), token); err != nil || value != 0 {
					t.Fatalf("zero-decimal token: value=%d err=%v", value, err)
				}
			}
			if calls != 2 {
				t.Fatalf("RPC calls=%d: failure must retry and successful zero must cache", calls)
			}
		})
	}
}

func TestDecimalsSlowMissDoesNotBlockCachedToken(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	cache := NewDecimals(decimalsBackend(func(ctx context.Context, _ []Call) ([]CallResult, error) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil, errors.New("failed token")
	}))
	cached := common.Address{19: 1}
	cache.cache[cached] = 6
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = cache.Get(t.Context(), common.Address{19: 2})
	}()
	defer func() { close(release); <-done }()
	<-entered
	read := make(chan int, 1)
	go func() {
		value, _ := cache.Get(t.Context(), cached)
		read <- value
	}()
	if value := testcheck.ReceiveWithin(t, read, time.Second, "slow RPC held the cache lock"); value != 6 {
		t.Fatalf("cached decimals=%d, want 6", value)
	}
}
