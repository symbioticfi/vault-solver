package iris

import (
	"bytes"
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
)

// stateChain answers only the recipe state reads; anything else means a full discovery ran.
type stateChain struct {
	fakeChain

	words map[common.Address]map[[4]byte]common.Hash
	calls int
}

func (c *stateChain) CallContract(_ context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	c.calls++
	if word, found := c.words[*msg.To][[4]byte(msg.Data[:4])]; found {
		return word.Bytes(), nil
	}
	return nil, errors.New("unexpected call")
}

func TestCallerGroupsLeaveOutUnusableExecutors(t *testing.T) {
	account, caller, other := common.HexToAddress("0xACC"), common.HexToAddress("0xCA11"), common.HexToAddress("0xCA12")
	executors := []common.Address{common.HexToAddress("0xE0"), common.HexToAddress("0xE1"), common.HexToAddress("0xE2"), common.HexToAddress("0xE3")}
	word := func(a common.Address) common.Hash { return common.BytesToHash(a.Bytes()) }
	state := []common.Hash{
		word(account), word(caller), {}, {},
		word(account), {}, {}, {}, // paused
		word(common.HexToAddress("0xBAD")), word(caller), {}, {},
		word(account), word(other), {}, {},
		{}, // root
	}
	s := &Solver{cfg: Config{Account: account, RecipeExecutors: executors}}
	groups := s.callerGroups(context.Background(), state)
	equal(t, map[common.Address][]common.Address{caller: {executors[0]}, other: {executors[3]}}, groups)
}

func TestRefreshSkipsDiscoveryOnlyWhileRecipeStateIsUnchanged(t *testing.T) {
	executor, account := common.HexToAddress("0xE0"), common.HexToAddress("0xACC")
	built := []common.Hash{
		common.BytesToHash(account.Bytes()), common.BytesToHash(common.HexToAddress("0xCA11").Bytes()),
		common.HexToHash("0xAB"), common.BigToHash(big.NewInt(7)), common.HexToHash("0x0F"),
	}
	selector := func(data []byte) [4]byte { return [4]byte(data[:4]) }
	words := func() map[common.Address]map[[4]byte]common.Hash {
		return map[common.Address]map[[4]byte]common.Hash{
			executor: {
				selector(executorBinding.PackAccount()):     built[0],
				selector(executorBinding.PackCaller()):      built[1],
				selector(executorBinding.PackRecipeHash()):  built[2],
				selector(executorBinding.PackRecipeBlock()): built[3],
			},
			account: {selector(accountBinding.PackRoot()): built[4]},
		}
	}
	for _, tt := range []struct {
		name    string
		mutate  func(map[common.Address]map[[4]byte]common.Hash)
		skipped bool
	}{
		{"unchanged", func(map[common.Address]map[[4]byte]common.Hash) {}, true},
		{"caller", func(w map[common.Address]map[[4]byte]common.Hash) {
			w[executor][selector(executorBinding.PackCaller())] = common.BytesToHash(common.HexToAddress("0xCA12").Bytes())
		}, false},
		{"same recipe set again", func(w map[common.Address]map[[4]byte]common.Hash) {
			w[executor][selector(executorBinding.PackRecipeBlock())] = common.BigToHash(big.NewInt(8))
		}, false},
		{"recipe hash", func(w map[common.Address]map[[4]byte]common.Hash) {
			w[executor][selector(executorBinding.PackRecipeHash())] = common.HexToHash("0xAC")
		}, false},
		{"root", func(w map[common.Address]map[[4]byte]common.Hash) {
			w[account][selector(accountBinding.PackRoot())] = common.HexToHash("0x10")
		}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			chain := &stateChain{words: words()}
			tt.mutate(chain.words)
			s := &Solver{cfg: Config{Account: account, RecipeExecutors: []common.Address{executor}}, chain: chain, built: built}
			err := s.refresh(context.Background())
			if tt.skipped {
				must(t, err)
				equal(t, len(built), chain.calls)
				return
			}
			// The fake cannot serve discovery, so a changed word surfaces as a discovery error.
			fails(t, err)
			equal(t, built, s.built)
		})
	}
}

// morphoChain answers idToMarketParams with the given encoded parameters.
type morphoChain struct {
	fakeChain

	params []byte
}

func (c *morphoChain) CallContract(_ context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	if !bytes.HasPrefix(msg.Data, morphoBinding.PackIdToMarketParams([32]byte{})[:4]) {
		return nil, errors.New("unexpected call")
	}
	return c.params, nil
}

func TestMarketDataRecoversThePayload(t *testing.T) {
	morpho := Market{DataHash: crypto.Keccak256Hash(morphoMarketData(t)), CollateralToken: fixtureWstETH, DebtToken: fixtureUSDC}
	other := morphoMarketData(t)
	other[31] ^= 1 // a market with another loan token
	tests := []struct {
		name   string
		market Market
		morpho common.Address
		params []byte
		want   []byte
	}{
		{"pool venue", Market{DataHash: emptyDataHash}, common.Address{}, nil, []byte{}},
		{"morpho market", morpho, common.HexToAddress("0xB0B0"), morphoMarketData(t), morphoMarketData(t)},
		{"morpho not configured", morpho, common.Address{}, morphoMarketData(t), nil},
		{"unknown market", morpho, common.HexToAddress("0xB0B0"), other, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Solver{cfg: Config{Morpho: tt.morpho}, chain: &morphoChain{params: tt.params}}
			markets, err := s.withMarketData(context.Background(), []Market{tt.market})
			must(t, err)
			if tt.want == nil {
				equal(t, 0, len(markets))
				return
			}
			equal(t, tt.want, markets[0].Data)
		})
	}
}
