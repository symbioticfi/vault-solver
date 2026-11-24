package iris

import (
	"bytes"
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

var (
	fixtureAccount  = common.HexToAddress("0xACC")
	fixtureIris     = common.HexToAddress("0x701")
	fixtureExecutor = common.HexToAddress("0xE0")
)

type signingFixture struct {
	Recipe       hexutil.Bytes `json:"recipe"`
	SignData     hexutil.Bytes `json:"signData"`
	SignMask     hexutil.Bytes `json:"signMask"`
	ApprovalData hexutil.Bytes `json:"approvalData"`
	ApprovalMask hexutil.Bytes `json:"approvalMask"`
	ApprovalLeaf common.Hash   `json:"approvalLeaf"`
	SignLeaf     common.Hash   `json:"signLeaf"`
}

func loadFixture(t *testing.T) signingFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/iris-signing.json")
	must(t, err)
	var f signingFixture
	must(t, json.Unmarshal(raw, &f))
	return f
}

// fixtureMarket builds the authenticated recipe and a two-leaf permission tree from leaf
// templates, so a test can tamper with a template and keep the tree consistent.
func fixtureMarket(t *testing.T, f signingFixture) (recipes.PreparedRecipe, *recipes.Tree) {
	t.Helper()
	recipe, err := recipes.DecodeRecipe(f.Recipe)
	must(t, err)
	sign := recipes.Leaf{Target: fixtureAccount, Value: new(big.Int), Data: f.SignData, DataMask: f.SignMask}
	approval := recipes.Leaf{Target: common.HexToAddress("0x704"), Value: new(big.Int), Data: f.ApprovalData, DataMask: f.ApprovalMask}
	sign.Hash, err = recipes.HashLeaf(sign)
	must(t, err)
	approval.Hash, err = recipes.HashLeaf(approval)
	must(t, err)
	sign.Proof, approval.Proof = []common.Hash{approval.Hash}, []common.Hash{sign.Hash}
	a, b := sign.Hash, approval.Hash
	if a.Cmp(b) > 0 {
		a, b = b, a
	}
	root := crypto.Keccak256Hash(a[:], b[:])
	account := fixtureAccount
	tree := &recipes.Tree{Root: root, ChainID: big.NewInt(1), Account: &account, Leaves: []recipes.Leaf{sign, approval}}
	prepared := recipes.PreparedRecipe{
		Snapshot: recipes.Snapshot{Executor: fixtureExecutor, Account: fixtureAccount, Caller: common.HexToAddress("0xCA11"), Root: root},
		Recipe:   recipe,
	}
	return prepared, tree
}

func TestFixtureLeavesMatchTheBuild(t *testing.T) {
	f := loadFixture(t)
	_, tree := fixtureMarket(t, f)
	equal(t, f.SignLeaf, tree.Leaves[0].Hash)
	equal(t, f.ApprovalLeaf, tree.Leaves[1].Hash)
}

func TestDecodeMarket(t *testing.T) {
	f := loadFixture(t)
	prepared, tree := fixtureMarket(t, f)
	m, err := decodeMarket(prepared, tree, fixtureIris, big.NewInt(1))
	must(t, err)
	equal(t, fixtureExecutor, m.Executor)
	equal(t, fixtureAccount, m.Account)
	equal(t, common.HexToAddress("0x702"), m.Blm)
	equal(t, common.HexToAddress("0x703"), m.CollateralToken)
	equal(t, common.HexToAddress("0x704"), m.DebtToken)
	equal(t, "50000000000000000", m.OverdueRate.String())
	equal(t, "604800", m.OverduePeriod.String())
	equal(t, "900000000000000000", m.BondLltv.String())
	equal(t, uint64(1), m.VenueID)
	equal(t, "0x64d65c9a2d91c36d56fbc42d69e979335320169b3df63bf92789e2c8883fcc64", hexutil.Encode(m.Data))
	equal(t, domainSeparator(big.NewInt(1), fixtureIris), m.Domain)
}

func TestDecodeMarketRejects(t *testing.T) {
	word := func(w int) int { return signBufferOffset + 32*w }
	tests := []struct {
		name    string
		iris    common.Address
		chainID int64
		mutate  func(*signingFixture, *recipes.PreparedRecipe)
	}{
		{name: "other iris", iris: common.HexToAddress("0x999"), chainID: 1},
		{name: "other chain", iris: fixtureIris, chainID: 5},
		{name: "other account", iris: fixtureIris, chainID: 1, mutate: func(_ *signingFixture, p *recipes.PreparedRecipe) {
			p.Snapshot.Account = common.HexToAddress("0xBEEF")
		}},
		{name: "pinned word left free", iris: fixtureIris, chainID: 1, mutate: func(f *signingFixture, _ *recipes.PreparedRecipe) {
			f.SignMask = bytes.Clone(f.SignMask)
			for i := range 32 {
				f.SignMask[word(5)+i] = 0xff
			}
		}},
		{name: "solver is not the account", iris: fixtureIris, chainID: 1, mutate: func(f *signingFixture, _ *recipes.PreparedRecipe) {
			f.SignData = bytes.Clone(f.SignData)
			copy(f.SignData[word(2)+12:word(2)+32], common.HexToAddress("0xBEEF").Bytes())
		}},
		{name: "not a quote", iris: fixtureIris, chainID: 1, mutate: func(f *signingFixture, _ *recipes.PreparedRecipe) {
			f.SignData = bytes.Clone(f.SignData)
			f.SignData[word(0)] ^= 1
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := loadFixture(t)
			var prepared recipes.PreparedRecipe
			if tt.mutate != nil {
				tt.mutate(&f, &prepared)
			}
			override := prepared.Snapshot.Account
			prepared, tree := fixtureMarket(t, f)
			if override != (common.Address{}) {
				prepared.Snapshot.Account = override
			}
			_, err := decodeMarket(prepared, tree, tt.iris, big.NewInt(tt.chainID))
			fails(t, err)
		})
	}
}

func TestAssembleSignsTheQuoteWords(t *testing.T) {
	f := loadFixture(t)
	prepared, tree := fixtureMarket(t, f)
	m, err := decodeMarket(prepared, tree, fixtureIris, big.NewInt(1))
	must(t, err)
	q := testQuote(m)
	calldata, leaves, err := recipes.Assemble(prepared.Recipe, tree, q.inputs(), nil, nil)
	must(t, err)
	ok(t, sameElements([]common.Hash{f.SignLeaf, f.ApprovalLeaf}, leaves), leaves)
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

func sameElements(want, got []common.Hash) bool {
	if len(want) != len(got) {
		return false
	}
	seen := make(map[common.Hash]int, len(want))
	for _, h := range want {
		seen[h]++
	}
	for _, h := range got {
		if seen[h]--; seen[h] < 0 {
			return false
		}
	}
	return true
}
