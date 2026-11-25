package iris

import (
	"bytes"
	"encoding/json"
	"math/big"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

// The fixture is the curator UI's own compile output (testdata/ui/generate.py) for this account
// against mainnet Iris.
var (
	fixtureAccount  = common.HexToAddress("0x0000000000000000000000000000000000ACC001")
	fixtureIris     = common.HexToAddress("0x77310475712D7a127D694B3e9D186d9f4a423bd9")
	fixtureExecutor = common.HexToAddress("0xE0")
	fixtureWstETH   = common.HexToAddress("0x7f39C581F595B53c5cb19bD0b3f8dA6c935E2Ca0")
	fixtureUSDC     = common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
)

type uiCall struct {
	Fn       string         `json:"fn"`
	Target   common.Address `json:"target"`
	Data     hexutil.Bytes  `json:"data"`
	DataMask hexutil.Bytes  `json:"dataMask"`
	Hash     common.Hash    `json:"hash"`
}

type uiRecipe struct {
	Market     string        `json:"market"`
	Recipe     hexutil.Bytes `json:"recipe"`
	RecipeHash common.Hash   `json:"recipeHash"`
}

type uiFixture struct {
	Root      common.Hash `json:"root"`
	Calls     []uiCall    `json:"calls"`
	Offers    []uiRecipe  `json:"offers"`
	BondClaim uiRecipe    `json:"bondClaim"`
}

func loadFixture(t *testing.T) uiFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/ui/recipes.json")
	must(t, err)
	var f uiFixture
	must(t, json.Unmarshal(raw, &f))
	return f
}

// fixtureTree lays the calls out like the curator bundle: sorted leaves at the end of the
// array, sorted-pair hashing, proofs from sibling nodes.
func fixtureTree(t *testing.T, calls []uiCall) *recipes.Tree {
	t.Helper()
	leaves := make([]recipes.Leaf, len(calls))
	for i, c := range calls {
		leaves[i] = recipes.Leaf{Target: c.Target, Value: new(big.Int), Data: c.Data, DataMask: c.DataMask}
		hash, err := recipes.HashLeaf(leaves[i])
		must(t, err)
		leaves[i].Hash = hash
	}
	slices.SortFunc(leaves, func(a, b recipes.Leaf) int { return a.Hash.Cmp(b.Hash) })
	nodes := make([]common.Hash, 2*len(leaves)-1)
	for i, l := range leaves {
		nodes[len(nodes)-1-i] = l.Hash
	}
	for i := len(leaves) - 2; i >= 0; i-- {
		a, b := nodes[2*i+1], nodes[2*i+2]
		if a.Cmp(b) > 0 {
			a, b = b, a
		}
		nodes[i] = crypto.Keccak256Hash(a[:], b[:])
	}
	for i := range leaves {
		for k := len(nodes) - 1 - i; k > 0; k = (k - 1) / 2 {
			sibling := k + 1
			if k%2 == 0 {
				sibling = k - 1
			}
			leaves[i].Proof = append(leaves[i].Proof, nodes[sibling])
		}
	}
	account := fixtureAccount
	return &recipes.Tree{Root: nodes[0], ChainID: big.NewInt(1), Account: &account, Leaves: leaves}
}

// fixtureMarket authenticates one offer recipe against the fixture's permission tree.
func fixtureMarket(t *testing.T, f uiFixture, offer int) (recipes.PreparedRecipe, *recipes.Tree) {
	t.Helper()
	recipe, err := recipes.DecodeRecipe(f.Offers[offer].Recipe)
	must(t, err)
	tree := fixtureTree(t, f.Calls)
	prepared := recipes.PreparedRecipe{
		Snapshot: recipes.Snapshot{Executor: fixtureExecutor, Account: fixtureAccount, Caller: common.HexToAddress("0xCA11"), Root: tree.Root},
		Recipe:   recipe,
	}
	return prepared, tree
}

func TestFixtureMatchesTheUIBuild(t *testing.T) {
	f := loadFixture(t)
	tree := fixtureTree(t, f.Calls)
	equal(t, f.Root, tree.Root)
	for _, l := range tree.Leaves {
		ok(t, slices.ContainsFunc(f.Calls, func(c uiCall) bool { return c.Hash == l.Hash }), l.Hash)
	}
	for _, r := range append(f.Offers, f.BondClaim) {
		recipe, err := recipes.DecodeRecipe(r.Recipe)
		must(t, err)
		hash, err := recipes.HashRecipe(recipe)
		must(t, err)
		equal(t, r.RecipeHash, hash, r.Market)
	}
}

func TestDecodeMarket(t *testing.T) {
	f := loadFixture(t)
	for i, want := range []struct {
		venue                     uint64
		dataHash                  string
		maxDebt, maxDuration, apr int64
	}{
		{0, "0xc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470", 20_000e6, 30 * 86400, 4e16},
		{1, "0xb323495f7e4148be5643a4ea4a8221eef163e4bccfdedc2a6f4696baacbc86cc", 5_000e6, 7 * 86400, 65e15},
	} {
		prepared, tree := fixtureMarket(t, f, i)
		m, err := decodeMarket(prepared, tree, fixtureIris, big.NewInt(1))
		must(t, err)
		equal(t, fixtureExecutor, m.Executor)
		equal(t, fixtureAccount, m.Account)
		equal(t, common.HexToAddress("0xB0ffB27D9D7f160d588c669329F807e58aDd4eeF"), m.Blm)
		equal(t, fixtureWstETH, m.CollateralToken)
		equal(t, fixtureUSDC, m.DebtToken)
		equal(t, "20000000000000000", m.OverdueRate.String())
		equal(t, "604800", m.OverduePeriod.String())
		equal(t, "900000000000000000", m.BondLltv.String())
		equal(t, want.venue, m.VenueID)
		equal(t, common.HexToHash(want.dataHash), m.DataHash)
		equal(t, big.NewInt(want.maxDebt), m.MaxDebt)
		equal(t, big.NewInt(want.maxDuration), m.MaxDuration)
		equal(t, big.NewInt(want.apr), m.MinFixedRate)
		equal(t, uint64(1800), m.MaxLifetime)
		equal(t, domainSeparator(big.NewInt(1), fixtureIris), m.Domain)
	}
}

func TestDecodeMarketRejects(t *testing.T) {
	word := func(w int) int { return signBufferOffset + 32*w }
	sign := func(f *uiFixture) *uiCall {
		i := slices.IndexFunc(f.Calls, func(c uiCall) bool { return strings.HasPrefix(c.Fn, "sign(") })
		f.Calls[i].Data, f.Calls[i].DataMask = bytes.Clone(f.Calls[i].Data), bytes.Clone(f.Calls[i].DataMask)
		return &f.Calls[i]
	}
	tests := []struct {
		name    string
		iris    common.Address
		chainID int64
		mutate  func(*uiFixture, *recipes.PreparedRecipe)
	}{
		{name: "other iris", iris: common.HexToAddress("0x999"), chainID: 1},
		{name: "other chain", iris: fixtureIris, chainID: 5},
		{name: "other account", iris: fixtureIris, chainID: 1, mutate: func(_ *uiFixture, p *recipes.PreparedRecipe) {
			p.Snapshot.Account = common.HexToAddress("0xBEEF")
		}},
		{name: "solver left free", iris: fixtureIris, chainID: 1, mutate: func(f *uiFixture, _ *recipes.PreparedRecipe) {
			for i := range 32 {
				sign(f).DataMask[word(solverWord)+i] = 0xff
			}
		}},
		{name: "solver is not the account", iris: fixtureIris, chainID: 1, mutate: func(f *uiFixture, _ *recipes.PreparedRecipe) {
			copy(sign(f).Data[word(solverWord)+12:word(solverWord)+32], common.HexToAddress("0xBEEF").Bytes())
		}},
		{name: "not a quote", iris: fixtureIris, chainID: 1, mutate: func(f *uiFixture, _ *recipes.PreparedRecipe) {
			sign(f).Data[word(0)] ^= 1
		}},
		{name: "bond from the keeper", iris: fixtureIris, chainID: 1, mutate: func(_ *uiFixture, p *recipes.PreparedRecipe) {
			rebind(t, p, bondWord, recipes.Value{Source: recipes.Input, Value: big.NewInt(9)})
		}},
		{name: "collateral from the keeper", iris: fixtureIris, chainID: 1, mutate: func(_ *uiFixture, p *recipes.PreparedRecipe) {
			rebind(t, p, 5, recipes.Value{Source: recipes.Input, Value: big.NewInt(0)})
		}},
		{name: "inputs out of order", iris: fixtureIris, chainID: 1, mutate: func(_ *uiFixture, p *recipes.PreparedRecipe) {
			rebind(t, p, 9, recipes.Value{Source: recipes.Input, Value: big.NewInt(4)})
		}},
		{name: "no debt cap", iris: fixtureIris, chainID: 1, mutate: func(_ *uiFixture, p *recipes.PreparedRecipe) {
			// The first step is debt <= cap; comparing the input with itself removes the cap.
			p.Recipe.Steps[0] = encodeStep(t, recipes.LessOrEqual, recipes.Value{Source: recipes.Input, Value: big.NewInt(debtInput)}, recipes.Value{Source: recipes.Input, Value: big.NewInt(debtInput)})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := loadFixture(t)
			prepared, _ := fixtureMarket(t, f, 0)
			if tt.mutate != nil {
				tt.mutate(&f, &prepared)
			}
			tree := fixtureTree(t, f.Calls)
			prepared.Snapshot.Root = tree.Root
			_, err := decodeMarket(prepared, tree, tt.iris, big.NewInt(tt.chainID))
			fails(t, err)
		})
	}
}

func TestAssembleSignsTheQuoteWords(t *testing.T) {
	f := loadFixture(t)
	prepared, tree := fixtureMarket(t, f, 0)
	m, err := decodeMarket(prepared, tree, fixtureIris, big.NewInt(1))
	must(t, err)
	q := testQuote(m)
	calldata, leaves, err := recipes.Assemble(prepared.Recipe, tree, q.inputs(), nil, nil)
	must(t, err)
	equal(t, 2, len(leaves)) // the Aave withdrawal, then the sign
	ok(t, len(calldata) > 0)
}

func TestMarketIndexFiltersByPairAndVenue(t *testing.T) {
	collateral, debt := common.HexToAddress("0x1"), common.HexToAddress("0x2")
	index := newMarketIndex([]Market{
		{Executor: common.HexToAddress("0xA"), CollateralToken: collateral, DebtToken: debt, VenueID: 0},
		{Executor: common.HexToAddress("0xB"), CollateralToken: collateral, DebtToken: debt, VenueID: 1},
		{Executor: common.HexToAddress("0xC"), CollateralToken: debt, DebtToken: collateral, VenueID: 1},
	})
	got := index.candidates(collateral, debt, big.NewInt(0b10))
	equal(t, 1, len(got))
	equal(t, common.HexToAddress("0xB"), got[0].Executor)
	equal(t, 2, len(index.candidates(collateral, debt, big.NewInt(0b11))))
	ok(t, len(index.candidates(collateral, debt, big.NewInt(0b100))) == 0)
}

// rebind replaces the sign step's binding of a Quote word.
func rebind(t *testing.T, p *recipes.PreparedRecipe, word int, value recipes.Value) {
	t.Helper()
	inspection, err := recipes.Inspect(p.Recipe)
	must(t, err)
	slot := inspection.Slots[len(inspection.Slots)-1]
	operands, err := recipes.Operands(p.Recipe, slot.StepIndex)
	must(t, err)
	bindings := make([]*big.Int, len(slot.Bindings[0]))
	for i, b := range slot.Bindings[0] {
		v := operands[1+i]
		if int(b.Offset) == signBufferOffset+32*word {
			v = value
		}
		bindings[i] = new(big.Int).Or(new(big.Int).Lsh(big.NewInt(int64(b.Offset)), 224), packValue(v))
	}
	gate := packValue(slot.Gate)
	args, err := recipes.Arguments("uint256", "bytes32[]", "uint256[][]").Pack(gate, slot.Leaves, [][]*big.Int{bindings})
	must(t, err)
	p.Recipe.Steps[slot.StepIndex] = append([]byte{recipes.Manage}, args...)
}

func encodeStep(t *testing.T, op uint8, a, b recipes.Value) []byte {
	t.Helper()
	args, err := recipes.Arguments("uint256", "uint256").Pack(packValue(a), packValue(b))
	must(t, err)
	return append([]byte{op}, args...)
}

// packValue is the interpreter's operand word: the source above 216 value bits.
func packValue(v recipes.Value) *big.Int {
	return new(big.Int).Or(new(big.Int).Lsh(big.NewInt(int64(v.Source)), 216), v.Value)
}
