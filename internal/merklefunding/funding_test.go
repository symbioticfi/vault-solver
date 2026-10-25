package merklefunding

import (
	"bytes"
	"context"
	"math/big"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/bindings/merkle/fundinglens"
)

var account = common.HexToAddress("0x1111111111111111111111111111111111111111")
var asset = common.HexToAddress("0x2222222222222222222222222222222222222222")
var lens = common.HexToAddress("0x3333333333333333333333333333333333333333")

func fundingConfig() Config {
	return Config{{Account: account.Hex(), Asset: asset.Hex(), Lens: lens.Hex(), Sources: []Source{{ID: "0x0101010101010101010101010101010101010101010101010101010101010101", Target: "0x4444444444444444444444444444444444444444", Data: "0x12345678", Offset: 32}}}}
}

type testReader struct {
	t      *testing.T
	target common.Address
	fail   bool
	calls  int
}

func (r *testReader) CallContract(_ context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	r.calls++
	if msg.To == nil || *msg.To != r.target || block == nil || block.Int64() != 42 {
		r.t.Fatalf("wrong capacity read: %+v at %v", msg, block)
	}
	if r.fail {
		return nil, errors.New("RPC failed")
	}
	value := int64(73)
	if r.target == lens {
		binding, err := fundinglens.MerkleFundingLensMetaData.ParseABI()
		if err != nil {
			r.t.Fatal(err)
		}
		values, err := binding.Methods["getAvailableAssets"].Inputs.Unpack(msg.Data[4:])
		if err != nil || values[0] != account || values[1] != asset {
			r.t.Fatalf("wrong account/asset: %v %v", values, err)
		}
		decoded := *abi.ConvertType(values[2], new([]fundinglens.IMerkleFundingLensSource)).(*[]fundinglens.IMerkleFundingLensSource)
		if len(decoded) != 1 || decoded[0].Target != common.HexToAddress("0x4444444444444444444444444444444444444444") || decoded[0].Offset.Int64() != 32 || !bytes.Equal(decoded[0].Data, []byte{0x12, 0x34, 0x56, 0x78}) {
			r.t.Fatalf("wrong source binding: %+v", decoded)
		}
		value = 600
	}
	return common.LeftPadBytes(big.NewInt(value).Bytes(), 32), nil
}

func TestFundingAboveAccountCashAtPinnedBlock(t *testing.T) {
	c := fundingConfig()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	r := &testReader{t: t, target: lens}
	got, err := c.Available(context.Background(), r, account, asset, big.NewInt(42))
	if err != nil || got.Int64() != 600 {
		t.Fatalf("funding = %v, %v; want 600", got, err)
	}
}
func TestUnconfiguredAccountAssetUsesCashOnly(t *testing.T) {
	c := fundingConfig()
	c[0].Account = "0x5555555555555555555555555555555555555555"
	r := &testReader{t: t, target: asset}
	got, err := c.Available(context.Background(), r, account, asset, big.NewInt(42))
	if err != nil || got.Int64() != 73 {
		t.Fatalf("cash = %v, %v; want 73", got, err)
	}
}
func TestConfiguredFailureDoesNotFallback(t *testing.T) {
	r := &testReader{t: t, target: lens, fail: true}
	if _, err := fundingConfig().Available(context.Background(), r, account, asset, big.NewInt(42)); err == nil || r.calls != 1 {
		t.Fatalf("configured failure: %v, %d reads", err, r.calls)
	}
}
func TestFundingRequiresPinnedBlock(t *testing.T) {
	r := &testReader{t: t, target: lens}
	if _, err := fundingConfig().Available(context.Background(), r, account, asset, nil); err == nil || r.calls != 0 {
		t.Fatalf("unpinned read accepted: %v, %d", err, r.calls)
	}
}
func TestInvalidFundingConfiguration(t *testing.T) {
	cases := map[string]func(Config) Config{
		"zero account":     func(c Config) Config { c[0].Account = common.Address{}.Hex(); return c },
		"zero asset":       func(c Config) Config { c[0].Asset = common.Address{}.Hex(); return c },
		"zero lens":        func(c Config) Config { c[0].Lens = common.Address{}.Hex(); return c },
		"zero source":      func(c Config) Config { c[0].Sources[0].Target = common.Address{}.Hex(); return c },
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
