package connector

import (
	"bytes"
	"context"
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

// ErrInsufficientCapacity reports that the connector budget or the advertised funding cannot
// cover the whole swap right now. It is an expected miss, unlike a failed read or permission.
var ErrInsufficientCapacity = errors.New("connector has insufficient capacity for the whole award")

// PermissionSource serves the curator permission tree for an authenticated recipe snapshot.
type PermissionSource interface {
	PermissionTree(ctx context.Context, chainID *big.Int, snapshot recipes.Snapshot) ([]byte, error)
}

// ResolveDiscount fetches the signed terms of one upstream discount.
type ResolveDiscount func(ctx context.Context, id string) (*discounts.Resolved, error)

// Workflow prepares connector swaps for one settlement contract. Filler is the contract
// that calls the recipe (directly or through its AccessManager) and receives the payment;
// Signer is the EOA submitting the outer settlement transaction.
type Workflow struct {
	Chain          recipes.Reader
	ChainID        *big.Int
	MaxGas         uint64
	Source         PermissionSource
	Filler, Signer common.Address
	// Reverts decodes reverts with the lane's settlement contracts; nil decodes the connector
	// and recipe contracts only.
	Reverts *chain.RevertDecoder
}

// State is the connector identity observed at the recipe snapshot block.
type State struct {
	Account, Asset common.Address
}

// Request selects a swap for the connector's payment at AmountIn. Preparation
// reads the oracle amount at the recipe block and applies the signed discount.
type Request struct {
	Venue      common.Address // LiquidLaneConnector
	Executor   common.Address // RecipeExecutor funding the swap
	DiscountID common.Hash
	MaxAssets  *big.Int // upstream advertised capacity
	TokenIn    common.Address
	AmountIn   *big.Int
	TokenOut   common.Address
}

// Prepared is an authenticated recipe execution awaiting its outer settlement envelope.
type Prepared struct {
	// Call is the recipe execution as the filler must issue it.
	Call  recipes.ExecutorCall
	Terms Terms
	State State
	// AmountOut is the exact payment the recipe sends to the filler.
	AmountOut *big.Int

	member recipes.PreparedRecipe
	inner  recipes.ExecutorCall
	leaves []common.Hash
}

func (w *Workflow) permissionTree(ctx context.Context, snapshot recipes.Snapshot) (*recipes.Tree, error) {
	raw, err := w.Source.PermissionTree(ctx, w.ChainID, snapshot)
	if err != nil {
		return nil, errors.Errorf("connector permission tree: %w", err)
	}
	tree, err := recipes.DecodeTree(raw)
	if err != nil {
		return nil, errors.Errorf("connector permission tree: %w", err)
	}
	switch {
	case tree.Root != snapshot.Root:
		return nil, errors.Errorf("connector permission tree root %s is not the recipe root %s", tree.Root.Hex(), snapshot.Root.Hex())
	case tree.ChainID == nil || tree.ChainID.Cmp(w.ChainID) != 0:
		return nil, errors.Errorf("connector permission tree chain %v is not chain %s", tree.ChainID, w.ChainID)
	case tree.Account == nil || *tree.Account != snapshot.Account:
		return nil, errors.Errorf("connector permission tree account %v is not the recipe account %s", tree.Account, snapshot.Account.Hex())
	}
	return tree, nil
}

func (w *Workflow) state(ctx context.Context, venue common.Address, block *big.Int) (State, error) {
	var account, asset common.Address
	call := func(data []byte) ([]byte, error) {
		raw, err := w.Chain.CallContract(ctx, ethereum.CallMsg{To: &venue, Data: data}, block)
		return raw, w.Explain(err)
	}
	raw, err := call(connectorBinding.PackAccount())
	if err != nil {
		return State{}, errors.Errorf("connector account: %w", err)
	}
	account, err = connectorBinding.UnpackAccount(raw)
	if err != nil || account == (common.Address{}) {
		return State{}, errors.Errorf("connector %s account %s is invalid (decode: %v)", venue.Hex(), account.Hex(), err)
	}
	raw, err = call(connectorBinding.PackOutputAsset())
	if err != nil {
		return State{}, errors.Errorf("connector outputAsset: %w", err)
	}
	asset, err = connectorBinding.UnpackOutputAsset(raw)
	if err != nil || asset == (common.Address{}) {
		return State{}, errors.Errorf("connector %s outputAsset %s is invalid (decode: %v)", venue.Hex(), asset.Hex(), err)
	}
	return State{Account: account, Asset: asset}, nil
}

// Each vault may have its own AccessManager. Keep the authenticated on-chain
// caller in SDK snapshots and batch discovery for recipes sharing that caller.
func (w *Workflow) discoverRecipes(ctx context.Context, executors []common.Address) ([]recipes.PreparedRecipe, error) {
	// DiscoverExecutors drops the RPC cause, so its errors carry no revert to decode.
	identities, err := recipes.DiscoverExecutors(ctx, w.Chain, executors)
	if err != nil {
		return nil, err
	}
	groups := make(map[common.Address][]common.Address)
	var callers []common.Address
	for _, executor := range executors {
		caller := identities[executor].Caller
		if _, exists := groups[caller]; !exists {
			callers = append(callers, caller)
		}
		groups[caller] = append(groups[caller], executor)
	}
	var prepared []recipes.PreparedRecipe
	for _, caller := range callers {
		members := groups[caller]
		accounts := make([]common.Address, len(members))
		for i, executor := range members {
			accounts[i] = identities[executor].Account
		}
		batch, err := recipes.DiscoverBatch(ctx, w.Chain, members, accounts, caller, recipes.PreparationOptions{ChainID: w.ChainID})
		if err != nil {
			return nil, w.Explain(err)
		}
		prepared = append(prepared, batch...)
	}
	return prepared, nil
}

// PrepareLegs prepares one settlement over one connector or split across distinct ones. For
// each leg it authenticates the selected recipe, fills its connector swap with fresh signed
// terms and routes the unchanged recipe calldata through its on-chain caller; fresh
// permissions, connector policy budget and advertised capacity must cover the leg. Every recipe
// is discovered in one pass, so the legs are authenticated against the same recipe state.
func (w *Workflow) PrepareLegs(ctx context.Context, requests []Request, resolve ResolveDiscount) ([]Prepared, error) {
	switch {
	case w.Signer == (common.Address{}):
		return nil, errors.New("connector fill requires a host signer")
	case len(requests) == 0:
		return nil, errors.New("connector fill has no selected legs")
	}
	executors := make([]common.Address, 0, len(requests))
	venues := make(map[common.Address]bool, len(requests))
	for _, request := range requests {
		if request.Executor == (common.Address{}) {
			return nil, errors.New("connector has no recipe executor")
		}
		switch {
		case request.Venue == (common.Address{}):
			return nil, errors.New("connector leg has no connector")
		case venues[request.Venue]:
			return nil, errors.Errorf("connector %s appears in more than one leg", request.Venue.Hex())
		case request.DiscountID == (common.Hash{}):
			return nil, errors.Errorf("connector %s leg has no upstream discount", request.Venue.Hex())
		case request.MaxAssets == nil || request.MaxAssets.Sign() < 0:
			return nil, errors.Errorf("connector %s leg advertised capacity %v is not a non-negative amount", request.Venue.Hex(), request.MaxAssets)
		}
		if request.AmountIn == nil || request.AmountIn.Sign() <= 0 {
			return nil, errors.Errorf("connector %s leg input %v is not positive", request.Venue.Hex(), request.AmountIn)
		}
		venues[request.Venue] = true
		executors = append(executors, request.Executor)
	}
	members, err := w.discoverRecipes(ctx, executors)
	if err != nil {
		return nil, errors.Errorf("discover connector recipes: %w", err)
	}
	byExecutor := make(map[common.Address]recipes.PreparedRecipe, len(members))
	for _, member := range members {
		byExecutor[member.Snapshot.Executor] = member
	}
	legs := make([]Prepared, 0, len(requests))
	for _, request := range requests {
		member, ok := byExecutor[request.Executor]
		if !ok {
			return nil, errors.Errorf("connector recipe executor %s was not discovered", request.Executor.Hex())
		}
		leg, err := w.prepareLeg(ctx, request, member, resolve)
		if err != nil {
			return nil, err
		}
		legs = append(legs, leg)
	}
	return legs, nil
}

func (w *Workflow) prepareLeg(ctx context.Context, request Request, member recipes.PreparedRecipe, resolve ResolveDiscount) (Prepared, error) {
	snapshot, recipe := member.Snapshot, member.Recipe
	block := new(big.Int).SetUint64(snapshot.BlockNumber)
	state, err := w.state(ctx, request.Venue, block)
	if err != nil {
		return Prepared{}, err
	}
	if state.Account != snapshot.Account {
		return Prepared{}, errors.Errorf("connector %s account %s differs from recipe account %s", request.Venue.Hex(), state.Account.Hex(), snapshot.Account.Hex())
	}
	raw, err := w.Chain.CallContract(ctx, ethereum.CallMsg{To: &request.Venue, Data: connectorBinding.PackRemainingAssets(request.TokenIn)}, block)
	if err != nil {
		return Prepared{}, errors.Errorf("connector remainingAssets: %w", w.Explain(err))
	}
	remaining, err := connectorBinding.UnpackRemainingAssets(raw)
	if err != nil {
		return Prepared{}, errors.Errorf("connector remainingAssets decode: %w", err)
	}
	if state.Asset != request.TokenOut {
		return Prepared{}, errors.Errorf("connector %s outputAsset %s differs from awarded output %s", request.Venue.Hex(), state.Asset.Hex(), request.TokenOut.Hex())
	}
	tree, err := w.permissionTree(ctx, snapshot)
	if err != nil {
		return Prepared{}, err
	}
	resolved, err := resolve(ctx, request.DiscountID.Hex())
	if err != nil {
		return Prepared{}, errors.Errorf("resolve connector discount: %w", err)
	}
	terms, err := ParseDiscount(resolved, request.DiscountID, request.Venue, request.TokenIn)
	if err != nil {
		return Prepared{}, err
	}
	amountOut, err := w.payment(ctx, request, terms, block)
	if err != nil {
		return Prepared{}, err
	}
	if remaining.Cmp(amountOut) < 0 {
		return Prepared{}, errors.Errorf("%w: connector %s remainingAssets %s below required output %s",
			ErrInsufficientCapacity, request.Venue.Hex(), remaining, amountOut)
	}
	if request.MaxAssets.Cmp(amountOut) < 0 {
		return Prepared{}, errors.Errorf("%w: connector %s maxAssets %s below required output %s",
			ErrInsufficientCapacity, request.Venue.Hex(), request.MaxAssets, amountOut)
	}
	assembled, err := assembleRoute(recipe, tree, request.Venue, request.TokenIn, w.Filler, request.AmountIn, amountOut, terms.Swap, terms.Signature)
	if err != nil {
		return Prepared{}, errors.Errorf("assemble connector recipe: %w", err)
	}
	routed, err := recipes.RouteExecution(ctx, w.Chain, snapshot, w.Filler, assembled.Calldata)
	if err != nil {
		return Prepared{}, w.Explain(err)
	}
	return Prepared{
		Call: routed, Terms: terms, State: state, AmountOut: amountOut,
		member: member, inner: recipes.ExecutorCall{Executor: snapshot.Executor, Data: assembled.Calldata},
		leaves: assembled.Leaves,
	}, nil
}

// payment mirrors the connector's discounted swap pricing: the oracle amount for the whole
// input after the signed discount, floored like the contract.
func (w *Workflow) payment(ctx context.Context, request Request, terms Terms, block *big.Int) (*big.Int, error) {
	raw, err := w.Chain.CallContract(ctx, ethereum.CallMsg{To: &request.Venue, Data: connectorBinding.PackGetAmountOut(request.TokenIn, request.AmountIn)}, block)
	if err != nil {
		return nil, errors.Errorf("connector getAmountOut: %w", w.Explain(err))
	}
	gross, err := connectorBinding.UnpackGetAmountOut(raw)
	if err != nil {
		return nil, errors.Errorf("connector getAmountOut decode: %w", err)
	}
	amount := liquidlane.AmountOutAfterDiscountPrecision(gross, terms.Swap.Discount.Discount, Precision)
	if amount.Sign() <= 0 {
		return nil, errors.Errorf("connector pays nothing for the input: getAmountOut %s after discount %s", gross, terms.Swap.Discount.Discount)
	}
	return amount, nil
}

// Validate admits the complete outer settlement transaction that carries every leg's Call.
// The SDK authenticates each inner call against its recipe's own caller at one fresh head and
// runs the unchanged outer simulation from the signer against the filler at that block and
// gas budget.
func (w *Workflow) Validate(ctx context.Context, calldata []byte, legs ...Prepared) error {
	members := make([]recipes.PreparedRecipe, len(legs))
	inner := make([]recipes.ExecutorCall, len(legs))
	leaves := make([][]common.Hash, len(legs))
	for i, leg := range legs {
		members[i], inner[i], leaves[i] = leg.member, leg.inner, leg.leaves
	}
	simulate := func(ctx context.Context, request recipes.PreparedBatchSimulationRequest) error {
		_, err := w.Chain.CallContract(ctx, ethereum.CallMsg{From: w.Signer, To: &w.Filler, Data: bytes.Clone(calldata), Gas: request.GasLimit}, request.BlockNumber)
		return err
	}
	return w.Explain(recipes.ValidatePreparedRoutesWithSimulation(ctx, w.Chain, members, inner, leaves, w.Filler, simulate, w.MaxGas))
}
