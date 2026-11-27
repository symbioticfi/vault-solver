package iris

import (
	"bytes"
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/merkleaccount"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

var accountBinding = merkleaccount.NewMerkleAccount()

// Market is one offer executor: the Quote terms its recipe fixes, the limits it enforces and
// the recipe that signs them. The market is fixed by the curator, never by configuration.
type Market struct {
	Executor, Account, Iris              common.Address
	Blm, CollateralToken, DebtToken      common.Address
	OverdueRate, OverduePeriod, BondLltv *big.Int
	VenueID                              uint64
	DataHash                             common.Hash
	// Data is the payload whose hash the recipe fixes, recovered after decoding.
	Data   []byte
	Domain common.Hash
	// MaxDebt, MaxDuration and MinFixedRate bound the recipe's inputs; MaxLifetime bounds the
	// deadline past the signing block's timestamp.
	MaxDebt, MaxDuration, MinFixedRate *big.Int
	MaxLifetime                        uint64

	paddedBond bool
	prepared   recipes.PreparedRecipe
	tree       *recipes.Tree
}

// fixedWords are the Quote words the recipe must bind to constants; the decoder reads the
// market from them.
var fixedWords = [...]int{4, 5, 6, 11, 12, 14, 16, 19}

// decodeMarket accepts a recipe only when it is the curator UI's single-market offer program:
// one unconditional sign on the account over the Iris domain and Quote type with the solver
// pinned, every other Quote word bound (the keeper's to inputs 0 to 8 in order, the market's to
// constants, the bond computed), and bounds on debt, duration, rate and deadline. Funding
// withdrawals before the sign are left to the recipe and the simulation.
func decodeMarket(prepared recipes.PreparedRecipe, tree *recipes.Tree, iris common.Address, chainID *big.Int) (Market, error) {
	snapshot, recipe := prepared.Snapshot, prepared.Recipe
	inspection, err := recipes.Inspect(recipe)
	if err != nil {
		return Market{}, err
	}
	if !slices.Equal(inspection.Inputs, []uint{0, 1, 2, 3, 4, 5, 6, 7, 8}) {
		return Market{}, errors.New("recipe is not an offer program: want the nine offer inputs")
	}
	for _, slot := range inspection.Slots {
		if slot.Operation == recipes.Approve {
			return Market{}, errors.New("recipe is not an offer program: unexpected approval")
		}
	}
	actions, err := recipes.ManagedActions(recipe, tree)
	if err != nil {
		return Market{}, err
	}
	sign := accountBinding.GetABI().Methods["sign"].ID
	var signs []recipes.ManagedAction
	for _, action := range actions {
		if action.Leaf.Target == snapshot.Account && bytes.HasPrefix(action.Leaf.Data, sign) {
			signs = append(signs, action)
		}
	}
	if len(signs) != 1 {
		return Market{}, errors.New("recipe is not an offer program: want exactly one sign")
	}
	action := signs[0]
	if gate := action.Slot.Gate; gate.Source != recipes.Literal || gate.Value == nil || gate.Value.Cmp(big.NewInt(1)) != 0 {
		return Market{}, errors.New("sign step must be unconditional")
	}
	if action.Leaf.Value != nil && action.Leaf.Value.Sign() != 0 {
		return Market{}, errors.New("sign leaf must call the account without value")
	}
	data := action.Leaf.Data
	if len(data) < signBufferOffset+quoteWords*32 {
		return Market{}, errors.New("sign leaf is not an account sign call")
	}
	domain := common.BytesToHash(data[4:36])
	if domain != domainSeparator(chainID, iris) {
		return Market{}, errors.New("sign leaf domain is not the configured Iris")
	}
	buffer := data[signBufferOffset : signBufferOffset+quoteWords*32]
	// Re-encoding with the expected hash plan pins the plan, the buffer size and the head layout.
	plan := []merkleaccount.IMerkleAccountHash{{Offset: new(big.Int), Length: big.NewInt(quoteWords * 32), Destination: new(big.Int)}}
	if !bytes.Equal(accountBinding.PackSign(domain, buffer, plan), data) {
		return Market{}, errors.New("sign leaf hash plan is not the Iris Quote plan")
	}
	word := func(i int) []byte { return buffer[32*i : 32*(i+1)] }
	if common.BytesToHash(word(0)) != quoteTypeHash {
		return Market{}, errors.New("sign buffer is not an Iris Quote")
	}
	if solver, ok := address(new(big.Int).SetBytes(word(solverWord))); !ok || solver != snapshot.Account {
		return Market{}, errors.New("signed Quote solver is not the account")
	}
	bound, err := signBindings(action)
	if err != nil {
		return Market{}, err
	}
	for i, w := range quoteInputs {
		if v := bound[w]; v.Source != recipes.Input || v.Value.Cmp(big.NewInt(int64(i))) != 0 {
			return Market{}, errors.Errorf("Quote word %d must be bound to input %d", w, i)
		}
	}
	paddedBond, recognized := bondPolicy(recipe, bound[bondWord])
	if !recognized {
		return Market{}, errors.New("recipe must use the Iris minimum bond or the legacy padded bond")
	}
	fixed := make(map[int]*big.Int, len(fixedWords))
	for _, w := range fixedWords {
		value, ok := constant(recipe, bound[w])
		if !ok {
			return Market{}, errors.Errorf("Quote word %d must be a constant", w)
		}
		fixed[w] = value
	}
	market := Market{
		Executor: snapshot.Executor, Account: snapshot.Account, Iris: iris, Domain: domain,
		OverdueRate: fixed[11], OverduePeriod: fixed[12], BondLltv: fixed[14], DataHash: common.BigToHash(fixed[19]),
		paddedBond: paddedBond, prepared: prepared, tree: tree,
	}
	for w, target := range map[int]*common.Address{4: &market.Blm, 5: &market.CollateralToken, 6: &market.DebtToken} {
		var ok bool
		if *target, ok = address(fixed[w]); !ok {
			return Market{}, errors.Errorf("Quote word %d is not a nonzero address", w)
		}
	}
	if !fixed[16].IsUint64() || fixed[16].Uint64() > maxVenueID {
		return Market{}, errors.New("signed Quote venue id out of range")
	}
	market.VenueID = fixed[16].Uint64()
	if err := market.readLimits(recipe); err != nil {
		return Market{}, err
	}
	return market, nil
}

// bondPolicy distinguishes the UI's exact minimum from its historical r + floor((r+3)/4).
func bondPolicy(recipe recipes.Recipe, bond recipes.Value) (padded, ok bool) {
	requirement := bond
	parts := resultOperands(recipe, bond, recipes.Add, 2)
	if padded = parts != nil; padded {
		requirement = parts[0]
		quarter := resultOperands(recipe, parts[1], recipes.MulDiv, 3)
		if quarter == nil || !literalIs(quarter[1], 1) || !literalIs(quarter[2], 4) {
			return false, false
		}
		numerator := resultOperands(recipe, quarter[0], recipes.Add, 2)
		if numerator == nil || numerator[0].Source != recipes.Result ||
			requirement.Source != recipes.Result || numerator[0].Value.Cmp(requirement.Value) != 0 ||
			!literalIs(numerator[1], 3) {
			return false, false
		}
	}

	// BLM computes floor(principal * ratio / WAD).
	operands := resultOperands(recipe, requirement, recipes.MulDiv, 3)
	return padded, operands != nil && isInput(operands[0], debtInput) && literalIs(operands[2], 1e18)
}

func resultOperands(recipe recipes.Recipe, v recipes.Value, op byte, count int) []recipes.Value {
	step, ok := resultStep(recipe, v)
	if !ok || recipe.Steps[step][0] != op {
		return nil
	}
	values, err := recipes.Operands(recipe, uint(step))
	if err != nil || len(values) != count {
		return nil
	}
	return values
}

func literalIs(v recipes.Value, n int64) bool {
	return v.Source == recipes.Literal && v.Value != nil && v.Value.Cmp(big.NewInt(n)) == 0
}

// signBindings requires the sign leaf to mask every Quote word except the type hash and the
// solver, and the recipe to bind each of them in order. It returns the bound value per word.
func signBindings(action recipes.ManagedAction) (map[int]recipes.Value, error) {
	var free []int
	mask := make([]byte, len(action.Leaf.Data))
	for w := 1; w < quoteWords; w++ {
		if w == solverWord {
			continue
		}
		free = append(free, w)
		for i := range 32 {
			mask[signBufferOffset+32*w+i] = 0xff
		}
	}
	if !bytes.Equal(mask, action.Leaf.DataMask) {
		return nil, errors.New("sign leaf must mask exactly the Quote words after the solver")
	}
	if len(action.Bindings) != len(free) {
		return nil, errors.New("sign leaf must bind every free Quote word")
	}
	bound := make(map[int]recipes.Value, len(free))
	for i, b := range action.Bindings {
		if int(b.Offset) != signBufferOffset+32*free[i] || b.Value.Value == nil {
			return nil, errors.Errorf("Quote word %d is not bound in order", free[i])
		}
		bound[free[i]] = b.Value
	}
	return bound, nil
}

// readLimits reads the recipe's bounds on its debt, duration and fixedRate inputs and on the
// deadline past the clock read.
func (m *Market) readLimits(recipe recipes.Recipe) error {
	for i, step := range recipe.Steps {
		if len(step) == 0 || step[0] != recipes.LessOrEqual {
			continue
		}
		operands, err := recipes.Operands(recipe, uint(i))
		if err != nil || len(operands) != 2 {
			continue
		}
		left, right := operands[0], operands[1]
		bound, isConstant := constant(recipe, right)
		switch {
		case isInput(left, debtInput) && isConstant:
			m.MaxDebt = lower(m.MaxDebt, bound)
		case isInput(left, durationInput) && isConstant:
			m.MaxDuration = lower(m.MaxDuration, bound)
		case isInput(right, fixedRateInput):
			if floor, ok := constant(recipe, left); ok && (m.MinFixedRate == nil || floor.Cmp(m.MinFixedRate) > 0) {
				m.MinFixedRate = floor
			}
		case isInput(left, deadlineInput):
			if lifetime, ok := clockOffset(recipe, right); ok && (m.MaxLifetime == 0 || lifetime < m.MaxLifetime) {
				m.MaxLifetime = lifetime
			}
		}
	}
	if m.MaxDebt == nil || m.MaxDuration == nil || m.MinFixedRate == nil || m.MaxLifetime == 0 {
		return errors.New("recipe must bound debt, duration, fixed rate and deadline")
	}
	return nil
}

func isInput(v recipes.Value, index int64) bool {
	return v.Source == recipes.Input && v.Value != nil && v.Value.Cmp(big.NewInt(index)) == 0
}

func lower(current, bound *big.Int) *big.Int {
	if current == nil || bound.Cmp(current) < 0 {
		return bound
	}
	return current
}

// clockOffset reads L from a deadline bound of the form Add(Read, L).
func clockOffset(recipe recipes.Recipe, v recipes.Value) (uint64, bool) {
	step, ok := resultStep(recipe, v)
	if !ok || recipe.Steps[step][0] != recipes.Add {
		return 0, false
	}
	operands, err := recipes.Operands(recipe, uint(step))
	if err != nil || len(operands) != 2 {
		return 0, false
	}
	read, ok := resultStep(recipe, operands[0])
	lifetime, isConstant := constant(recipe, operands[1])
	if !ok || recipe.Steps[read][0] != recipes.Read || !isConstant || !lifetime.IsUint64() {
		return 0, false
	}
	return lifetime.Uint64(), true
}

// constant folds a value built only from literals with Add and MulDiv, the form the UI uses for
// words wider than a literal operand.
func constant(recipe recipes.Recipe, v recipes.Value) (*big.Int, bool) {
	if v.Source == recipes.Literal && v.Value != nil {
		return new(big.Int).Set(v.Value), true
	}
	step, ok := resultStep(recipe, v)
	if !ok {
		return nil, false
	}
	operands, err := recipes.Operands(recipe, uint(step))
	if err != nil {
		return nil, false
	}
	args := make([]*big.Int, len(operands))
	for i, operand := range operands {
		if args[i], ok = constant(recipe, operand); !ok {
			return nil, false
		}
	}
	var out *big.Int
	switch op := recipe.Steps[step][0]; {
	case op == recipes.Add && len(args) == 2:
		out = new(big.Int).Add(args[0], args[1])
	case op == recipes.MulDiv && len(args) == 3 && args[2].Sign() != 0:
		out = new(big.Int).Mul(args[0], args[1])
		out.Quo(out, args[2])
	default:
		return nil, false
	}
	return out, out.BitLen() <= 256
}

func resultStep(recipe recipes.Recipe, v recipes.Value) (int, bool) {
	if v.Source != recipes.Result || v.Value == nil || !v.Value.IsInt64() || v.Value.Int64() >= int64(len(recipe.Steps)) || len(recipe.Steps[v.Value.Int64()]) == 0 {
		return 0, false
	}
	return int(v.Value.Int64()), true
}

func address(v *big.Int) (common.Address, bool) {
	if v.Sign() <= 0 || v.BitLen() > 160 {
		return common.Address{}, false
	}
	return common.BigToAddress(v), true
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
