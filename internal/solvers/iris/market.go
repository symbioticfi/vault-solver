package iris

import (
	"bytes"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/merkleaccount"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

var accountBinding = merkleaccount.NewMerkleAccount()

// Market is one iris-signing recipe: the Quote terms its sign leaf pins and the recipe that
// signs them. The Quote's market is fixed by the curator, never by configuration.
type Market struct {
	Executor, Account, Iris              common.Address
	Blm, CollateralToken, DebtToken      common.Address
	OverdueRate, OverduePeriod, BondLltv *big.Int
	VenueID                              uint64
	Data                                 []byte
	Domain                               common.Hash

	prepared recipes.PreparedRecipe
	tree     *recipes.Tree
}

// decodeMarket accepts a recipe only when it is exactly the iris-signing program: one
// sign leaf on the account over the Iris domain and Quote type, the free Quote words
// bound to inputs 0 to 9 in order, every other word pinned, and one bond approval of the
// debt token to Iris.
func decodeMarket(prepared recipes.PreparedRecipe, tree *recipes.Tree, iris common.Address, chainID *big.Int) (Market, error) {
	snapshot := prepared.Snapshot
	inspection, err := recipes.Inspect(prepared.Recipe)
	if err != nil {
		return Market{}, err
	}
	var approvals []recipes.RuntimeSlot
	manages := 0
	for _, slot := range inspection.Slots {
		switch slot.Operation {
		case recipes.Manage:
			manages++
		case recipes.Approve:
			approvals = append(approvals, slot)
		}
	}
	actions, err := recipes.ManagedActions(prepared.Recipe, tree)
	if err != nil {
		return Market{}, err
	}
	if manages != 1 || len(actions) != 1 || len(approvals) != 1 {
		return Market{}, errors.New("recipe is not an iris-signing program: want one sign and one approval")
	}
	action := actions[0]
	if gate := action.Slot.Gate; gate.Source != recipes.Literal || gate.Value == nil || gate.Value.Cmp(big.NewInt(1)) != 0 {
		return Market{}, errors.New("sign step must be unconditional")
	}
	if action.Leaf.Target != snapshot.Account || (action.Leaf.Value != nil && action.Leaf.Value.Sign() != 0) {
		return Market{}, errors.New("sign leaf must call the account without value")
	}
	data := action.Leaf.Data
	if len(data) < signBufferOffset+quoteWords*32 || !bytes.Equal(data[:4], accountBinding.GetABI().Methods["sign"].ID) {
		return Market{}, errors.New("sign leaf is not an account sign call")
	}
	domain := common.BytesToHash(data[4:36])
	if domain != domainSeparator(chainID, iris) {
		return Market{}, errors.New("sign leaf domain is not the configured Iris")
	}
	size := new(big.Int).SetBytes(data[signBufferOffset-32 : signBufferOffset])
	if !size.IsInt64() || size.Int64() < quoteWords*32 || int(size.Int64()) > len(data)-signBufferOffset {
		return Market{}, errors.New("sign buffer must hold the Quote words and market data")
	}
	buffer := data[signBufferOffset : signBufferOffset+int(size.Int64())]
	marketData := bytes.Clone(buffer[quoteWords*32:])
	// Re-encoding with the expected hash plan pins the plan and the head layout exactly.
	plan := []merkleaccount.IMerkleAccountHash{
		{Offset: big.NewInt(quoteWords * 32), Length: big.NewInt(int64(len(marketData))), Destination: big.NewInt(dataHashWord * 32)},
		{Offset: new(big.Int), Length: big.NewInt(quoteWords * 32), Destination: new(big.Int)},
	}
	if !bytes.Equal(accountBinding.PackSign(domain, buffer, plan), data) {
		return Market{}, errors.New("sign leaf hash plan is not the Iris Quote plan")
	}
	if err := checkSignMask(action); err != nil {
		return Market{}, err
	}
	word := func(i int) []byte { return buffer[32*i : 32*(i+1)] }
	addressWord := func(i int) (common.Address, error) {
		w := word(i)
		if !bytes.Equal(w[:12], make([]byte, 12)) || bytes.Equal(w[12:], make([]byte, 20)) {
			return common.Address{}, errors.Errorf("Quote word %d is not a nonzero address", i)
		}
		return common.BytesToAddress(w), nil
	}
	if common.BytesToHash(word(0)) != quoteTypeHash {
		return Market{}, errors.New("sign buffer is not an Iris Quote")
	}
	market := Market{Executor: snapshot.Executor, Account: snapshot.Account, Iris: iris, Domain: domain, Data: marketData, prepared: prepared, tree: tree}
	solverAddress, err := addressWord(2)
	if err != nil {
		return Market{}, err
	}
	if solverAddress != snapshot.Account {
		return Market{}, errors.New("signed Quote solver is not the account")
	}
	for i, target := range map[int]*common.Address{4: &market.Blm, 5: &market.CollateralToken, 6: &market.DebtToken} {
		if *target, err = addressWord(i); err != nil {
			return Market{}, err
		}
	}
	market.OverdueRate = new(big.Int).SetBytes(word(11))
	market.OverduePeriod = new(big.Int).SetBytes(word(12))
	market.BondLltv = new(big.Int).SetBytes(word(14))
	venue := new(big.Int).SetBytes(word(16))
	if !venue.IsUint64() || venue.Uint64() > maxVenueID {
		return Market{}, errors.New("signed Quote venue id out of range")
	}
	market.VenueID = venue.Uint64()
	if approvals[0].Token != market.DebtToken || approvals[0].Spender != iris {
		return Market{}, errors.New("recipe approval must grant the debt token to Iris")
	}
	return market, nil
}

// checkSignMask requires the free Quote words, and nothing else, to be masked and bound to
// inputs 0 to 9 in calldata order.
func checkSignMask(action recipes.ManagedAction) error {
	mask := make([]byte, len(action.Leaf.Data))
	for _, w := range freeWords {
		offset := signBufferOffset + 32*w
		for i := range 32 {
			mask[offset+i] = 0xff
		}
	}
	if !bytes.Equal(mask, action.Leaf.DataMask) {
		return errors.New("sign leaf must mask exactly the free Quote words")
	}
	if len(action.Bindings) != len(freeWords) {
		return errors.New("sign leaf must bind every free Quote word")
	}
	for i, b := range action.Bindings {
		value := b.Value
		if int(b.Offset) != signBufferOffset+32*freeWords[i] || value.Source != recipes.Input || value.Value == nil || value.Value.Cmp(big.NewInt(int64(i))) != 0 {
			return errors.Errorf("Quote word %d must be bound to input %d", freeWords[i], i)
		}
	}
	return nil
}

// marketIndex maps a (collateral, debt) pair to its markets in configured executor order.
// It is replaced whole on refresh and read without locks through an atomic pointer.
type marketIndex map[[2]common.Address][]Market

func newMarketIndex(markets []Market) marketIndex {
	index := make(marketIndex)
	for _, m := range markets {
		key := [2]common.Address{m.CollateralToken, m.DebtToken}
		index[key] = append(index[key], m)
	}
	return index
}

// candidates returns the pair's markets whose venue the request allows.
func (x marketIndex) candidates(collateral, debt common.Address, venueBitmap *big.Int) []Market {
	var out []Market
	for _, m := range x[[2]common.Address{collateral, debt}] {
		if venueBitmap.Bit(int(m.VenueID)) == 1 {
			out = append(out, m)
		}
	}
	return out
}
