package iris

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
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

func TestSnapshotWordsMatchTheOnChainEncoding(t *testing.T) {
	snapshot := recipes.Snapshot{Caller: common.HexToAddress("0xCA11"), RecipeHash: common.HexToHash("0xAB"), RecipeBlock: 123456, Root: common.HexToHash("0x0F")}
	words := snapshotWords([]recipes.PreparedRecipe{{Snapshot: snapshot}})
	equal(t, 4, len(words))
	caller, err := executorBinding.UnpackCaller(words[0].Bytes())
	must(t, err)
	equal(t, snapshot.Caller, caller)
	hash, err := executorBinding.UnpackRecipeHash(words[1].Bytes())
	must(t, err)
	equal(t, snapshot.RecipeHash, common.Hash(hash))
	block, err := executorBinding.UnpackRecipeBlock(words[2].Bytes())
	must(t, err)
	equal(t, snapshot.RecipeBlock, block.Uint64())
	root, err := accountBinding.UnpackRoot(words[3].Bytes())
	must(t, err)
	equal(t, snapshot.Root, common.Hash(root))
}

func TestRefreshSkipsDiscoveryOnlyWhileRecipeStateIsUnchanged(t *testing.T) {
	executor, account := common.HexToAddress("0xE0"), common.HexToAddress("0xACC")
	snapshot := recipes.Snapshot{Executor: executor, Account: account, Caller: common.HexToAddress("0xCA11"), RecipeHash: common.HexToHash("0xAB"), RecipeBlock: 7, Root: common.HexToHash("0x0F")}
	built := snapshotWords([]recipes.PreparedRecipe{{Snapshot: snapshot}})
	selector := func(data []byte) [4]byte { return [4]byte(data[:4]) }
	words := func() map[common.Address]map[[4]byte]common.Hash {
		return map[common.Address]map[[4]byte]common.Hash{
			executor: {
				selector(executorBinding.PackCaller()):      built[0],
				selector(executorBinding.PackRecipeHash()):  built[1],
				selector(executorBinding.PackRecipeBlock()): built[2],
			},
			account: {selector(accountBinding.PackRoot()): built[3]},
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
				equal(t, 4, chain.calls)
				return
			}
			// The fake cannot serve discovery, so a changed word surfaces as a discovery error.
			fails(t, err)
			equal(t, built, s.built)
		})
	}
}
