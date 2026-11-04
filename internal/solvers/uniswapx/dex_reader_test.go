package uniswapx

import (
	"context"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
)

type dexReadCall struct {
	to    common.Address
	data  []byte
	block *big.Int
}

type dexReaderFixture struct {
	cfg     *DexConfig
	header  *gethtypes.Header
	mode    string
	mu      sync.Mutex
	calls   []dexReadCall
	hold    chan struct{}
	entered chan struct{}
	once    sync.Once
}

func (f *dexReaderFixture) HeaderByNumber(ctx context.Context, number *big.Int) (*gethtypes.Header, error) {
	if number == nil && f.hold != nil {
		f.once.Do(func() { close(f.entered) })
		select {
		case <-f.hold:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	h := gethtypes.CopyHeader(f.header)
	if f.mode == "zero header" {
		h.Number = new(big.Int)
	}
	if number != nil && f.mode == "reorg" {
		h.Time++
	}
	return h, nil
}

func (f *dexReaderFixture) CallContract(_ context.Context, call ethereum.CallMsg, block *big.Int) ([]byte, error) {
	f.mu.Lock()
	c := dexReadCall{to: *call.To, data: append([]byte(nil), call.Data...)}
	if block != nil {
		c.block = new(big.Int).Set(block)
	}
	f.calls = append(f.calls, c)
	f.mu.Unlock()
	word := func(value *big.Int) []byte { return common.LeftPadBytes(value.Bytes(), 32) }
	address := func(value common.Address) []byte { return word(new(big.Int).SetBytes(value.Bytes())) }
	if *call.To == f.cfg.Quoter && len(call.Data) > 4 {
		if f.mode == "rpc error" {
			return nil, errors.New("test rpc failure")
		}
		if f.mode == "malformed amount" {
			return []byte{1}, nil
		}
		parsed := dexABI.GetABI()
		method, err := parsed.MethodById(call.Data[:4])
		if err != nil {
			return nil, err
		}
		amount := big.NewInt(123456)
		if f.mode == "zero amount" {
			amount = new(big.Int)
		}
		return method.Outputs.Pack(amount, big.NewInt(1), uint32(1), big.NewInt(90_000))
	}
	if string(call.Data) == string(dexABI.PackFeeController()) {
		if f.mode == "fees enabled" {
			return address(common.HexToAddress("0x99")), nil
		}
		return make([]byte, 32), nil
	}
	if *call.To == f.cfg.Factory {
		if f.mode == "no pool" {
			return make([]byte, 32), nil
		}
		return address(common.HexToAddress("0x1234")), nil
	}
	if *call.To == f.cfg.Router || *call.To == f.cfg.Quoter {
		if f.mode == "wrong factory" {
			return make([]byte, 32), nil
		}
		return address(f.cfg.Factory), nil
	}
	for _, getter := range []struct {
		data  []byte
		value common.Address
	}{
		{dexABI.PackINPUTTOKEN(), f.cfg.InputToken}, {dexABI.PackBRIDGETOKEN(), f.cfg.BridgeToken},
		{dexABI.PackReactorAddress(), common.HexToAddress("0x99")},
		{dexABI.PackROUTER(), f.cfg.Router}, {dexABI.PackTREASURY(), f.cfg.Treasury},
		{dexABI.PackOUTPUT0(), f.cfg.Outputs[0].Token}, {dexABI.PackOUTPUT1(), f.cfg.Outputs[1].Token},
	} {
		if string(call.Data) == string(getter.data) {
			if f.mode == "wrong executor" {
				return make([]byte, 32), nil
			}
			return address(getter.value), nil
		}
	}
	if string(call.Data) == string(dexABI.PackFEE0()) {
		return word(new(big.Int).SetUint64(uint64(f.cfg.Outputs[0].Fee))), nil
	}
	if string(call.Data) == string(dexABI.PackFEE1()) {
		return word(new(big.Int).SetUint64(uint64(f.cfg.Outputs[1].Fee))), nil
	}
	return nil, errors.New("unexpected test contract call")
}

func newDexReaderFixture(t *testing.T) (*v3Dex, *dexReaderFixture) {
	t.Helper()
	cfg, err := parseDexConfig(rawDexFixture())
	if err != nil {
		t.Fatal(err)
	}
	f := &dexReaderFixture{cfg: cfg, header: &gethtypes.Header{Number: big.NewInt(123), Time: uint64(time.Now().Unix())}}
	return newV3Dex(f, cfg, common.HexToAddress("0x99")), f
}

func TestDexRPCReaderPinsBothModesAndValidatesResponse(t *testing.T) {
	for _, exactOut := range []bool{false, true} {
		d, f := newDexReaderFixture(t)
		quote, err := d.quote(t.Context(), d.cfg.Outputs[0].Token, big.NewInt(100), exactOut)
		if err != nil || quote.Amount.Cmp(big.NewInt(123456)) != 0 {
			t.Fatalf("quote=%+v err=%v", quote, err)
		}
		if len(f.calls) != 3 {
			t.Fatalf("RPC calls=%d", len(f.calls))
		}
		for _, call := range f.calls {
			if call.block == nil || call.block.Cmp(f.header.Number) != 0 {
				t.Fatal("read did not use the pinned block")
			}
		}
		parsed := dexABI.GetABI()
		method, err := parsed.MethodById(f.calls[2].data[:4])
		if err != nil {
			t.Fatal(err)
		}
		if (method.Name == "quoteExactOutputSingle") != exactOut {
			t.Fatalf("method=%s exactOut=%t", method.Name, exactOut)
		}
	}
}

func TestDexRPCReaderFailsClosed(t *testing.T) {
	for _, mode := range []string{"zero header", "fees enabled", "no pool", "rpc error", "malformed amount", "zero amount", "reorg"} {
		t.Run(mode, func(t *testing.T) {
			d, f := newDexReaderFixture(t)
			f.mode = mode
			if q, err := d.quote(t.Context(), d.cfg.Outputs[0].Token, big.NewInt(100), true); err == nil || q.Amount != nil {
				t.Fatalf("accepted bad market data: %+v err=%v", q, err)
			}
		})
	}
}

func TestDexRPCStartupChecksExecutorAndFactories(t *testing.T) {
	for _, mode := range []string{"valid", "wrong executor", "wrong factory", "no pool"} {
		d, f := newDexReaderFixture(t)
		f.mode = mode
		err := d.validate(t.Context(), common.HexToAddress("0x123"))
		if (err == nil) != (mode == "valid") {
			t.Fatalf("%s err=%v", mode, err)
		}
	}
}

func TestDexRPCConcurrencyAndCancellationReleaseSlot(t *testing.T) {
	d, f := newDexReaderFixture(t)
	d.cfg.MaxConcurrentQuotes = 1
	d.slots = make(chan struct{}, 1)
	f.hold, f.entered = make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { _, err := d.quote(ctx, d.cfg.Outputs[0].Token, big.NewInt(100), true); done <- err }()
	<-f.entered
	_, err := d.quote(t.Context(), d.cfg.Outputs[0].Token, big.NewInt(100), true)
	if err == nil || !strings.Contains(err.Error(), "concurrency") {
		t.Fatalf("second request was not bounded: %v", err)
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("cancellation ignored")
	}
	close(f.hold)
	if _, err := d.quote(t.Context(), d.cfg.Outputs[0].Token, big.NewInt(100), true); err != nil {
		t.Fatalf("slot leaked: %v", err)
	}
}
