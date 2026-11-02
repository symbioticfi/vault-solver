package chain

import (
	"context"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
)

type metadataBackend struct {
	mu           sync.Mutex
	calls        int
	decimals     uint8
	symbol       string
	failDecimals bool
	failSymbol   bool
	transport    bool
	block        <-chan struct{}
	started      chan struct{}
	stopped      chan struct{}
}

func (b *metadataBackend) Multicall(ctx context.Context, calls []Call) ([]CallResult, error) {
	if b.started != nil {
		b.started <- struct{}{}
		defer func() { b.stopped <- struct{}{} }()
	}
	if b.block != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-b.block:
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls++
	if b.transport {
		return nil, errors.New("offline")
	}
	result := make([]CallResult, len(calls))
	for i, c := range calls {
		var data []byte
		var err error
		if reflect.DeepEqual(c.Data, erc20B.PackDecimals()) {
			if b.failDecimals {
				continue
			}
			data, err = erc20B.GetABI().Methods["decimals"].Outputs.Pack(b.decimals)
		} else {
			if b.failSymbol {
				continue
			}
			data, err = tokenMetadataABI.GetABI().Methods["symbol"].Outputs.Pack(b.symbol)
		}
		if err != nil {
			return nil, err
		}
		result[i] = CallResult{Success: true, ReturnData: data}
	}
	return result, nil
}
func metadataCache(b *metadataBackend) *Decimals {
	return &Decimals{chain: b, cache: make(map[common.Address]int)}
}
func TestTokenMetadataUsesChainScopedCacheAndNoRequestIO(t *testing.T) {
	token := common.HexToAddress("0x1234")
	for _, dec := range []uint8{0, 6, 18} {
		t.Run(strconv.Itoa(int(dec)), func(t *testing.T) {
			backend := &metadataBackend{decimals: dec, symbol: "TEST"}
			cache := metadataCache(backend)
			if err := cache.WarmMetadata(t.Context(), []common.Address{token, token, {}}); err != nil {
				t.Fatal(err)
			}
			if backend.calls != 2 {
				t.Fatalf("calls=%d, want two batched metadata reads", backend.calls)
			}
			if err := cache.WarmMetadata(t.Context(), []common.Address{token}); err != nil {
				t.Fatal(err)
			}
			want := []any{"tokenInDecimals", int(dec), "tokenInSymbol", "TEST"}
			if got := cache.PairLogFields(token, common.Address{}); !reflect.DeepEqual(got, want) {
				t.Fatalf("fields=%v want %v", got, want)
			}
			for i := range 100 {
				if got := cache.PairLogFields(common.HexToAddress("0x"+strconv.FormatInt(int64(0x9000+i), 16)), common.Address{}); len(got) != 0 {
					t.Fatalf("untrusted fields=%v", got)
				}
			}
			if backend.calls != 2 || len(cache.symbols) != 1 {
				t.Fatal("cache grew or performed request-time I/O")
			}
		})
	}
}
func TestTokenMetadataFailuresStayUnknown(t *testing.T) {
	token := common.HexToAddress("0x1234")
	for _, tc := range []struct {
		name    string
		b       *metadataBackend
		want    []any
		wantErr bool
	}{
		{"decimals revert", &metadataBackend{symbol: "TEST", failDecimals: true}, []any{"tokenInSymbol", "TEST"}, false},
		{"symbol revert", &metadataBackend{decimals: 6, failSymbol: true}, []any{"tokenInDecimals", 6}, false},
		{"unsafe symbol", &metadataBackend{decimals: 6, symbol: "bad\nlabel"}, []any{"tokenInDecimals", 6}, false},
		{"transport", &metadataBackend{transport: true}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := metadataCache(tc.b)
			err := cache.WarmMetadata(t.Context(), []common.Address{token})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
			if got := cache.PairLogFields(token, common.Address{}); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
		})
	}
}
func TestTokenMetadataWarmNewTrustedToken(t *testing.T) {
	b := &metadataBackend{decimals: 6, symbol: "A"}
	cache := metadataCache(b)
	first, second := common.HexToAddress("0x01"), common.HexToAddress("0x02")
	if err := cache.WarmMetadata(t.Context(), []common.Address{first}); err != nil {
		t.Fatal(err)
	}
	b.decimals = 8
	b.symbol = "B"
	if err := cache.WarmMetadata(t.Context(), []common.Address{first, second}); err != nil {
		t.Fatal(err)
	}
	if got := cache.PairLogFields(first, second); !reflect.DeepEqual(got, []any{"tokenInDecimals", 6, "tokenInSymbol", "A", "tokenOutDecimals", 8, "tokenOutSymbol", "B"}) {
		t.Fatal(got)
	}
}

func TestTokenMetadataConcurrentReadsAndWarmup(t *testing.T) {
	backend := &metadataBackend{decimals: 6, symbol: "TEST"}
	cache := metadataCache(backend)
	token := common.HexToAddress("0x1234")
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := cache.WarmMetadata(t.Context(), []common.Address{token}); err != nil {
				t.Error(err)
			}
			for range 100 {
				cache.PairLogFields(token, token)
			}
		})
	}
	wg.Wait()
	if backend.calls != 2 {
		t.Fatalf("metadata calls=%d want2", backend.calls)
	}
	if got := cache.PairLogFields(token, common.Address{}); len(got) != 4 {
		t.Fatalf("fields=%v", got)
	}
}

func TestNewTrustedTokenMetadataAsyncDoesNotBlockAndHonorsCancellation(t *testing.T) {
	for _, cancelRead := range []bool{false, true} {
		t.Run(strconv.FormatBool(cancelRead), func(t *testing.T) {
			backend := &metadataBackend{decimals: 6, symbol: "NEW"}
			cache := metadataCache(backend)
			oldToken, newToken := common.HexToAddress("0x01"), common.HexToAddress("0x02")
			if err := cache.WarmMetadata(t.Context(), []common.Address{oldToken}); err != nil {
				t.Fatal(err)
			}
			release := make(chan struct{})
			backend.block = release
			backend.started = make(chan struct{}, 2)
			backend.stopped = make(chan struct{}, 2)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			// A blocked optional RPC must not block the caller (quote/publication path).
			cache.WarmMetadataAsync(ctx, []common.Address{oldToken, newToken}, logr.Discard())
			select {
			case <-backend.started:
			case <-time.After(time.Second):
				t.Fatal("no new-token lookup")
			}
			for range 10 {
				cache.WarmMetadataAsync(ctx, []common.Address{newToken}, logr.Discard())
			}
			if cancelRead {
				cancel()
				select {
				case <-backend.stopped:
				case <-time.After(time.Second):
					t.Fatal("metadata RPC ignored cancellation")
				}
				if fields := cache.PairLogFields(newToken, common.Address{}); len(fields) != 0 {
					t.Fatalf("cancelled metadata=%v", fields)
				}
			} else {
				close(release)
				ticker := time.NewTicker(time.Millisecond)
				defer ticker.Stop()
				deadline := time.After(time.Second)
				for len(cache.PairLogFields(newToken, common.Address{})) != 4 {
					select {
					case <-deadline:
						t.Fatal("new token not enriched")
					case <-ticker.C:
					}
				}
				backend.mu.Lock()
				calls := backend.calls
				backend.mu.Unlock()
				if calls != 4 {
					t.Fatalf("calls=%d, want two startup and two new-token reads", calls)
				}
			}
		})
	}
}
