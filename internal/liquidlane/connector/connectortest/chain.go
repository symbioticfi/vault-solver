package connectortest

import (
	"bytes"
	"context"
	"math/big"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/merkleaccount"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	binding "github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
)

var recipeBinding = recipeexecutor.NewRecipeExecutor()

// Group serves several fixture routes over one chain transport, dispatching each read to the
// chain that owns its connector, recipe executor or account. The first chain owns the block
// header and the outer settlement simulation; the others share its header.
type Group []*Chain

// NewGroup joins fixture chains under the first one's header.
func NewGroup(chains ...*Chain) Group {
	for _, chain := range chains[1:] {
		chain.Header = chains[0].Header
	}
	return chains
}

func (g Group) owner(address common.Address) *Chain {
	for _, chain := range g {
		if address == chain.venue() || address == chain.RecipeExecutor || address == chain.RecipeAccount {
			return chain
		}
	}
	return nil
}

func (g Group) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	return g[0].HeaderByNumber(ctx, number)
}

func (g Group) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	var logs []types.Log
	for _, address := range query.Addresses {
		chain := g.owner(address)
		if chain == nil {
			return nil, errors.New("SetRecipe evidence requested for an unknown executor")
		}
		single := query
		single.Addresses = []common.Address{address}
		found, err := chain.FilterLogs(ctx, single)
		if err != nil {
			return nil, err
		}
		logs = append(logs, found...)
	}
	return logs, nil
}

func (g Group) CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	if msg.To != nil {
		if chain := g.owner(*msg.To); chain != nil {
			return chain.CallContract(ctx, msg, block)
		}
	}
	return g[0].CallContract(ctx, msg, block)
}

// PermissionSet serves the curator permission responses of several fixture routes.
type PermissionSet []Permissions

func (s PermissionSet) PermissionTree(ctx context.Context, chainID *big.Int, snapshot recipes.Snapshot) ([]byte, error) {
	for _, permissions := range s {
		if permissions.Account == snapshot.Account {
			return permissions.PermissionTree(ctx, chainID, snapshot)
		}
	}
	return nil, errors.New("permission request for an unknown account")
}

// Permissions serves one curator permission response for the authenticated fixture route.
type Permissions struct {
	Raw      []byte
	Account  common.Address
	Executor common.Address
}

func (s Permissions) PermissionTree(_ context.Context, chainID *big.Int, snapshot recipes.Snapshot) ([]byte, error) {
	if chainID.Cmp(big.NewInt(1)) != 0 || snapshot.Account != s.Account || snapshot.Executor != s.Executor || snapshot.BlockNumber != 7 {
		return nil, errors.New("permission request escaped authenticated route")
	}
	return bytes.Clone(s.Raw), nil
}

// Chain replaces only the chain transport: one RecipeExecutor, its MerkleAccount and the
// connector at Venue, all pinned to block 7. Discovery, permission verification and calldata
// assembly stay in the SDK. A call to Filler carrying OuterSelector is the outer settlement
// simulation; every other unexpected call fails.
type Chain struct {
	// Connector reads.
	ConnectorVenue   common.Address // zero is Venue
	Remaining        *big.Int
	ConnectorAccount common.Address
	AmountOut        *big.Int // getAmountOut result; nil leaves the oracle unreadable
	// AmountOutFor, when set, prices getAmountOut from the requested input instead of AmountOut.
	AmountOutFor func(amountIn *big.Int) *big.Int
	MinDiscount  *big.Int // connector minimum discount at 1e8 precision; nil is zero
	DomainName   string   // EIP-712 domain name; empty is the canonical connector name

	RecipeExecutor, RecipeAccount, RecipeCaller, Signer, Filler common.Address
	OuterSelector                                               [4]byte
	RecipeHash, Root, ApprovalHash, SwapHash                    common.Hash
	Recipe                                                      recipes.Recipe
	Header                                                      *types.Header
	Allowed                                                     bool
	OuterCalls                                                  int
	OuterGas                                                    uint64
	IdentityReads                                               int
	OuterData                                                   []byte
	OuterErr                                                    error
	HeaderDelay                                                 time.Duration
	HeaderStarted                                               time.Time
}

func (r *Chain) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	if r.HeaderStarted.IsZero() {
		r.HeaderStarted = time.Now()
	}
	if r.HeaderDelay > 0 {
		timer := time.NewTimer(r.HeaderDelay)
		r.HeaderDelay = 0
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if number != nil && number.Uint64() != 7 {
		return nil, errors.New("unexpected unpinned header")
	}
	return types.CopyHeader(r.Header), nil
}

func (r *Chain) FilterLogs(_ context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	event := recipeBinding.GetABI().Events["SetRecipe"]
	if query.FromBlock == nil || query.ToBlock == nil || query.FromBlock.Uint64() != 7 || query.ToBlock.Uint64() != 7 || len(query.Addresses) != 1 || query.Addresses[0] != r.RecipeExecutor || len(query.Topics) != 1 || len(query.Topics[0]) != 1 || query.Topics[0][0] != event.ID {
		return nil, errors.New("SetRecipe evidence escaped pinned executor")
	}
	data, err := event.Inputs.NonIndexed().Pack(r.Recipe.Queries, r.Recipe.Steps)
	if err != nil {
		return nil, err
	}
	return []types.Log{{Address: r.RecipeExecutor, Topics: []common.Hash{event.ID, r.RecipeHash}, Data: data, BlockNumber: 7, BlockHash: r.Header.Hash()}}, nil
}

func (r *Chain) venue() common.Address {
	if r.ConnectorVenue == (common.Address{}) {
		return Venue
	}
	return r.ConnectorVenue
}

func (r *Chain) connectorRead(data []byte) ([]byte, error) {
	for _, method := range connectorBinding.GetABI().Methods {
		if !bytes.Equal(data[:4], method.ID) {
			continue
		}
		switch method.RawName {
		case "account":
			return method.Outputs.Pack(r.ConnectorAccount)
		case "outputAsset":
			return method.Outputs.Pack(TokenOut)
		case "remainingAssets":
			return method.Outputs.Pack(r.Remaining)
		case "getAmountOut":
			if r.AmountOutFor != nil {
				args, err := method.Inputs.Unpack(data[4:])
				if err != nil {
					return nil, err
				}
				return method.Outputs.Pack(r.AmountOutFor(args[1].(*big.Int)))
			}
			if r.AmountOut != nil {
				return method.Outputs.Pack(r.AmountOut)
			}
		case "minDiscount":
			if r.MinDiscount != nil {
				return method.Outputs.Pack(r.MinDiscount)
			}
			return method.Outputs.Pack(new(big.Int))
		case "eip712Domain":
			name := "LiquidLaneConnector"
			if r.DomainName != "" {
				name = r.DomainName
			}
			return method.Outputs.Pack([1]byte{0x0f}, name, "1", big.NewInt(1), r.venue(), [32]byte{}, []*big.Int{})
		}
	}
	return nil, nil
}

func (r *Chain) CallContract(_ context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	if msg.To == nil || len(msg.Data) < 4 {
		return nil, errors.New("invalid contract call")
	}
	if *msg.To == r.Filler && bytes.Equal(msg.Data[:4], r.OuterSelector[:]) {
		if msg.From != r.Signer || block == nil || block.Uint64() != 7 || msg.Gas == 0 {
			return nil, errors.New("outer fill must simulate from the host signer at the pinned block and gas budget")
		}
		r.OuterCalls++
		r.OuterGas = msg.Gas
		r.OuterData = bytes.Clone(msg.Data)
		return nil, r.OuterErr
	}
	if *msg.To == r.venue() && (bytes.Equal(msg.Data, connectorBinding.PackAccount()) || bytes.Equal(msg.Data, connectorBinding.PackOutputAsset())) {
		r.IdentityReads++
	}
	if block == nil || block.Uint64() != 7 {
		return nil, errors.New("metadata read escaped recipe snapshot")
	}
	if *msg.To == r.venue() {
		return r.connectorRead(msg.Data)
	}
	if *msg.To == r.RecipeExecutor {
		contractABI := recipeBinding.GetABI()
		method, err := contractABI.MethodById(msg.Data)
		if err != nil {
			return nil, err
		}
		switch method.RawName {
		case "account":
			return method.Outputs.Pack(r.RecipeAccount)
		case "caller":
			return method.Outputs.Pack(r.RecipeCaller)
		case "recipeHash":
			return method.Outputs.Pack(r.RecipeHash)
		case "recipeBlock":
			return method.Outputs.Pack(big.NewInt(7))
		}
	}
	if *msg.To == r.RecipeAccount {
		contractABI := merkleaccount.NewMerkleAccount().GetABI()
		method, err := contractABI.MethodById(msg.Data)
		if err != nil {
			return nil, err
		}
		switch method.RawName {
		case "root":
			return method.Outputs.Pack(r.Root)
		case "hasRole":
			args, err := method.Inputs.Unpack(msg.Data[4:])
			if err != nil {
				return nil, err
			}
			if !recipes.Proof(r.Root, common.Hash(args[0].([32]byte)), []common.Hash{r.ApprovalHash}) && !recipes.Proof(r.Root, common.Hash(args[0].([32]byte)), []common.Hash{r.SwapHash}) || args[1].(common.Address) != r.RecipeExecutor {
				return nil, errors.New("role check used a different leaf or EOA holder")
			}
			return method.Outputs.Pack(r.Allowed)
		}
	}
	return nil, errors.New("unexpected contract call or direct inner simulation")
}

// Fixture is one recipe route: a committed recipe whose swap leaf pins filler as the payment
// recipient, the chain serving it and the curator permissions authenticating it.
type Fixture struct {
	Chain       *Chain
	Permissions Permissions
	Recipe      recipes.Recipe
	Leaf        recipes.Leaf
}

// NewFixture commits a recipe for recipeExecutor whose caller and payment recipient is filler.
// The swap leaf frees only the swap amount and both signatures of the supplied signed terms.
func NewFixture(t *testing.T, swap binding.ILiquidLaneConnectorDiscountSwap, signature []byte, recipeExecutor, filler common.Address, outerSelector [4]byte) Fixture {
	t.Helper()
	return NewFixtureAt(t, Venue, common.HexToAddress("0x21"), swap, signature, recipeExecutor, filler, outerSelector)
}

// NewFixtureAt is NewFixture for the connector at venue funded by account.
func NewFixtureAt(t *testing.T, venue, account common.Address, swap binding.ILiquidLaneConnectorDiscountSwap, signature []byte, recipeExecutor, filler common.Address, outerSelector [4]byte) Fixture {
	t.Helper()
	signer := common.HexToAddress("0x99")
	data, mask := SignatureTemplate(swap, signature, filler, big.NewInt(0))
	for i := 100; i < 132; i++ {
		mask[i] = 255
	}
	leaf := recipes.Leaf{Target: venue, Value: new(big.Int), Data: data, DataMask: mask}
	var err error
	leaf.Hash, err = recipes.HashLeaf(leaf)
	if err != nil {
		t.Fatal(err)
	}
	recipe, tree, permission := CommittedFunding(t, leaf, account, filler, TokenOut, TokenIn)
	hash, err := recipes.HashRecipe(recipe)
	if err != nil {
		t.Fatal(err)
	}
	chain := &Chain{
		ConnectorVenue: venue, Remaining: big.NewInt(900), ConnectorAccount: account,
		RecipeExecutor: recipeExecutor, RecipeAccount: account, RecipeCaller: filler, Signer: signer, Filler: filler, OuterSelector: outerSelector,
		RecipeHash: hash, Root: tree.Root, Recipe: recipe, ApprovalHash: tree.Leaves[0].Hash, SwapHash: leaf.Hash,
		Header: &types.Header{Number: big.NewInt(7), Time: 7, GasLimit: 30_000_000}, Allowed: true,
	}
	return Fixture{Chain: chain, Permissions: Permissions{Raw: permission, Account: account, Executor: recipeExecutor}, Recipe: recipe, Leaf: leaf}
}
