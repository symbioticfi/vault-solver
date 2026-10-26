package merklefunding

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
)

var account = common.HexToAddress("0x1111111111111111111111111111111111111111")
var asset = common.HexToAddress("0x2222222222222222222222222222222222222222")

func fundingConfig() Config {
	return Config{{Account: account.Hex(), Asset: asset.Hex(), Sources: []Source{{ID: "0x0101010101010101010101010101010101010101010101010101010101010101", Target: "0x4444444444444444444444444444444444444444", Data: "0x12345678", Offset: 32}}}}
}

type testReader struct {
	t       *testing.T
	target  common.Address
	fail    bool
	calls   int
	results []multicall3.Multicall3Result
	sources []Source
	raw     []byte
}

func word(value int64) []byte { return common.LeftPadBytes(big.NewInt(value).Bytes(), 32) }

func (r *testReader) CallContract(_ context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	r.calls++
	if msg.To == nil || *msg.To != r.target || block == nil || block.Int64() != 42 || msg.From != (common.Address{}) {
		r.t.Fatalf("wrong capacity read: %+v at %v", msg, block)
	}
	if r.fail {
		return nil, errors.New("RPC failed")
	}
	if r.target == asset {
		if !bytes.Equal(msg.Data, tokenBinding.PackBalanceOf(account)) {
			r.t.Fatal("wrong cash query")
		}
		if r.raw != nil {
			return r.raw, nil
		}
		return word(73), nil
	}
	binding, err := multicall3.Multicall3MetaData.ParseABI()
	if err != nil {
		r.t.Fatal(err)
	}
	method := binding.Methods["aggregate3"]
	if !bytes.Equal(msg.Data[:4], method.ID) {
		r.t.Fatal("wrong multicall selector")
	}
	values, err := method.Inputs.Unpack(msg.Data[4:])
	if err != nil {
		r.t.Fatal(err)
	}
	calls := *abi.ConvertType(values[0], new([]multicall3.Multicall3Call3)).(*[]multicall3.Multicall3Call3)
	sources := r.sources
	if sources == nil {
		sources = fundingConfig()[0].Sources
	}
	if len(calls) != 1+len(sources) || calls[0].Target != asset || calls[0].AllowFailure || !bytes.Equal(calls[0].CallData, tokenBinding.PackBalanceOf(account)) {
		r.t.Fatalf("wrong funding cash call: %+v", calls)
	}
	for i, source := range sources {
		data, err := hexutil.Decode(source.Data)
		if err != nil {
			r.t.Fatal(err)
		}
		if calls[i+1].Target != common.HexToAddress(source.Target) || calls[i+1].AllowFailure || !bytes.Equal(calls[i+1].CallData, data) {
			r.t.Fatalf("wrong source call: %+v", calls[i+1])
		}
	}
	if r.raw != nil {
		return r.raw, nil
	}
	results := r.results
	if results == nil {
		results = []multicall3.Multicall3Result{{Success: true, ReturnData: word(73)}, {Success: true, ReturnData: append(word(999), word(527)...)}}
	}
	return method.Outputs.Pack(results)
}

func TestFundingAboveAccountCashAtPinnedBlock(t *testing.T) {
	c := fundingConfig()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	r := &testReader{t: t, target: common.HexToAddress(protocolio.StandardMulticall3)}
	got, err := c.Available(context.Background(), r, account, asset, big.NewInt(42))
	if err != nil || got.Int64() != 600 || r.calls != 1 {
		t.Fatalf("funding = %v, %v; want 600", got, err)
	}
}
func TestUnconfiguredAccountAssetUsesCashOnly(t *testing.T) {
	other := common.HexToAddress("0x5555555555555555555555555555555555555555")
	wrongAccount, wrongAsset, emptySources := fundingConfig(), fundingConfig(), fundingConfig()
	wrongAccount[0].Account = other.Hex()
	wrongAsset[0].Asset = other.Hex()
	emptySources[0].Sources = nil
	for name, c := range map[string]Config{"no config": nil, "other account": wrongAccount, "other asset": wrongAsset, "empty sources": emptySources} {
		t.Run(name, func(t *testing.T) {
			r := &testReader{t: t, target: asset}
			got, err := c.Available(t.Context(), r, account, asset, big.NewInt(42))
			if err != nil || got.Int64() != 73 || r.calls != 1 {
				t.Fatalf("cash = %v, %v, calls %d; want 73 in one read", got, err, r.calls)
			}
		})
	}
}
func TestConfiguredFailureDoesNotFallback(t *testing.T) {
	r := &testReader{t: t, target: common.HexToAddress(protocolio.StandardMulticall3), fail: true}
	if _, err := fundingConfig().Available(context.Background(), r, account, asset, big.NewInt(42)); err == nil || r.calls != 1 {
		t.Fatalf("configured failure: %v, %d reads", err, r.calls)
	}
}
func TestFundingRequiresPinnedBlock(t *testing.T) {
	for name, block := range map[string]*big.Int{"missing": nil, "negative": big.NewInt(-1)} {
		t.Run(name, func(t *testing.T) {
			r := &testReader{t: t, target: common.HexToAddress(protocolio.StandardMulticall3)}
			got, err := fundingConfig().Available(t.Context(), r, account, asset, block)
			if got != nil || err == nil || !strings.Contains(err.Error(), "merkleFunding requires a pinned block") || r.calls != 0 {
				t.Fatalf("unpinned read accepted: %v, %v, %d", got, err, r.calls)
			}
		})
	}
}
func TestInvalidFundingConfiguration(t *testing.T) {
	cases := map[string]func(Config) Config{
		"zero account":   func(c Config) Config { c[0].Account = common.Address{}.Hex(); return c },
		"zero asset":     func(c Config) Config { c[0].Asset = common.Address{}.Hex(); return c },
		"zero source":    func(c Config) Config { c[0].Sources[0].Target = common.Address{}.Hex(); return c },
		"short id":       func(c Config) Config { c[0].Sources[0].ID = "0x01"; return c },
		"malformed id":   func(c Config) Config { c[0].Sources[0].ID = "0xzz"; return c },
		"malformed data": func(c Config) Config { c[0].Sources[0].Data = "0xzz"; return c },
		"long data":      func(c Config) Config { c[0].Sources[0].Data = "0x" + strings.Repeat("ff", 8193); return c },
		"large offset":   func(c Config) Config { c[0].Sources[0].Offset = 65568; return c },
		"pair bound":     func(c Config) Config { return make(Config, 65) },
		"source bound":   func(c Config) Config { c[0].Sources = make([]Source, 33); return c },
		"duplicate id": func(c Config) Config {
			s := c[0].Sources[0]
			s.Data = "0xabcdef01"
			c[0].Sources = append(c[0].Sources, s)
			return c
		},
		"zero id":          func(c Config) Config { c[0].Sources[0].ID = common.Hash{}.Hex(); return c },
		"short data":       func(c Config) Config { c[0].Sources[0].Data = "0x1234"; return c },
		"offset":           func(c Config) Config { c[0].Sources[0].Offset = 1; return c },
		"duplicate pair":   func(c Config) Config { return append(c, c[0]) },
		"duplicate source": func(c Config) Config { c[0].Sources = append(c[0].Sources, c[0].Sources[0]); return c },
		"duplicate query": func(c Config) Config {
			s := c[0].Sources[0]
			s.ID = "0x0202020202020202020202020202020202020202020202020202020202020202"
			c[0].Sources = append(c[0].Sources, s)
			return c
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			if err := change(fundingConfig()).Validate(); err == nil {
				t.Fatal("accepted invalid configuration")
			}
		})
	}
}

func TestFundingRejectsIncompleteOrOverflowedRead(t *testing.T) {
	maxWord := bytes.Repeat([]byte{0xff}, 32)
	cases := map[string]struct {
		results []multicall3.Multicall3Result
		raw     []byte
		want    string
	}{
		"missing reply":   {results: []multicall3.Multicall3Result{{Success: true, ReturnData: word(73)}}, want: "merkleFunding result count"},
		"failed cash":     {results: []multicall3.Multicall3Result{{}, {Success: true, ReturnData: append(word(999), word(527)...)}}, want: "merkleFunding call 0 failed"},
		"failed source":   {results: []multicall3.Multicall3Result{{Success: true, ReturnData: word(73)}, {}}, want: "merkleFunding call 1 failed"},
		"short cash":      {results: []multicall3.Multicall3Result{{Success: true, ReturnData: []byte{1}}, {Success: true, ReturnData: append(word(999), word(527)...)}}, want: "invalid merkleFunding cash reply"},
		"long cash":       {results: []multicall3.Multicall3Result{{Success: true, ReturnData: append(word(73), word(1)...)}, {Success: true, ReturnData: append(word(999), word(527)...)}}, want: "invalid merkleFunding cash reply"},
		"short source":    {results: []multicall3.Multicall3Result{{Success: true, ReturnData: word(73)}, {Success: true, ReturnData: word(527)}}, want: "invalid merkleFunding source reply"},
		"overflow":        {results: []multicall3.Multicall3Result{{Success: true, ReturnData: maxWord}, {Success: true, ReturnData: append(word(999), word(1)...)}}, want: "merkleFunding exceeds uint256"},
		"malformed batch": {raw: []byte{1}, want: "decode merkleFunding"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := &testReader{t: t, target: common.HexToAddress(protocolio.StandardMulticall3), results: tc.results, raw: tc.raw}
			got, err := fundingConfig().Available(t.Context(), r, account, asset, big.NewInt(42))
			if got != nil || err == nil || !strings.Contains(err.Error(), tc.want) || r.calls != 1 {
				t.Fatalf("funding = %v, %v, calls %d; want %q", got, err, r.calls, tc.want)
			}
		})
	}
}

func TestFundingAcceptsUint256Maximum(t *testing.T) {
	maxWord := bytes.Repeat([]byte{0xff}, 32)
	r := &testReader{t: t, target: common.HexToAddress(protocolio.StandardMulticall3), results: []multicall3.Multicall3Result{{Success: true, ReturnData: maxWord}, {Success: true, ReturnData: append(word(999), word(0)...)}}}
	got, err := fundingConfig().Available(t.Context(), r, account, asset, big.NewInt(42))
	if err != nil || got.Cmp(new(big.Int).SetBytes(maxWord)) != 0 {
		t.Fatalf("funding = %v, %v", got, err)
	}
}

func TestFundingSumsDistinctSourceOffsets(t *testing.T) {
	c := fundingConfig()
	c[0].Sources = append(c[0].Sources, Source{ID: common.HexToHash("0x02").Hex(), Target: common.HexToAddress("0x5555").Hex(), Data: "0xabcdef01", Offset: 0})
	r := &testReader{t: t, target: common.HexToAddress(protocolio.StandardMulticall3), sources: c[0].Sources, results: []multicall3.Multicall3Result{{Success: true, ReturnData: word(73)}, {Success: true, ReturnData: append(word(999), word(527)...)}, {Success: true, ReturnData: word(123)}}}
	got, err := c.Available(t.Context(), r, account, asset, big.NewInt(42))
	if err != nil || got.Int64() != 723 || r.calls != 1 {
		t.Fatalf("funding = %v, %v, calls %d; want 723", got, err, r.calls)
	}
}

func TestCashOnlyRejectsInvalidReply(t *testing.T) {
	for name, raw := range map[string][]byte{"short": {1}, "long": append(word(73), word(1)...)} {
		t.Run(name, func(t *testing.T) {
			r := &testReader{t: t, target: asset, raw: raw}
			got, err := Config(nil).Available(t.Context(), r, account, asset, big.NewInt(42))
			if got != nil || err == nil || !strings.Contains(err.Error(), "invalid merkleFunding cash reply") || r.calls != 1 {
				t.Fatalf("cash = %v, %v, calls %d", got, err, r.calls)
			}
		})
	}
}
