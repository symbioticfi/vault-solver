package rfq

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

// Initialize scopes this process to the payment connector in its configured recipe.
// Recipe bytes are verified against the configured executor's commitment.
func (s *Solver) Initialize(ctx context.Context) (common.Address, error) {
	if s.workflow.connector != (common.Address{}) {
		return s.workflow.connector, nil
	}
	snapshot, recipe, err := recipes.Discover(ctx, s.workflow.chain, s.recipeExecutor, nil)
	if err != nil {
		return common.Address{}, err
	}
	if snapshot.Caller != s.workflow.caller {
		return common.Address{}, errors.New("connector recipe caller mismatch")
	}
	venue, asset, err := paymentConnector(recipe)
	if err != nil {
		return common.Address{}, err
	}
	state, err := s.workflow.state(ctx, venue, new(big.Int).SetUint64(snapshot.BlockNumber))
	if err != nil {
		return common.Address{}, err
	}
	if state.Account != snapshot.Account || state.Asset != asset {
		return common.Address{}, errors.New("recipe payment connector account or asset mismatch")
	}
	s.workflow.connector = venue
	return venue, nil
}

func paymentConnector(recipe recipes.Recipe) (venue, asset common.Address, err error) {
	inspection, err := recipes.Inspect(recipe)
	if err != nil {
		return common.Address{}, common.Address{}, err
	}
	for _, slot := range inspection.Slots {
		if slot.Operation != recipes.Approve || slot.Amount.Source != recipes.Input || slot.Amount.Value == nil || slot.Amount.Value.Cmp(big.NewInt(1)) != 0 {
			continue
		}
		if venue != (common.Address{}) || slot.Spender == (common.Address{}) || slot.Token == (common.Address{}) {
			return common.Address{}, common.Address{}, errors.New("recipe requires one payment approval from input 1")
		}
		venue, asset = slot.Spender, slot.Token
	}
	if venue == (common.Address{}) {
		return venue, asset, errors.New("recipe payment approval from input 1 missing")
	}
	return venue, asset, nil
}
