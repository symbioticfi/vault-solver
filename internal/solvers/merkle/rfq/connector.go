package rfq

import (
	"bytes"
	"context"
	"math/big"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	sdkparse "github.com/symbioticfi/merkle-executor/sdk/parse"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/parse"
)

var connectorBinding = connector.NewLiquidLaneConnector()

type connectorTerms struct {
	Swap       connector.ILiquidLaneConnectorDiscountSwap
	Signature  []byte
	ValidUntil time.Time
}

type assembledConnector struct {
	Calldata []byte
	Leaves   []common.Hash
}

type connectorState struct {
	Account, Asset common.Address
	Remaining      *big.Int
}

// Signed terms are upstream-owned; only ABI framing and selected route identity are checked here.
func parseConnectorDiscount(r *discounts.Resolved, id common.Hash, venue, token common.Address) (connectorTerms, error) {
	var out connector.ILiquidLaneConnectorDiscountSwap
	if r == nil {
		return connectorTerms{}, errors.New("connector discount is nil")
	}
	rawID, err := hexutil.Decode(r.DiscountID)
	if err != nil || len(rawID) != 32 || common.BytesToHash(rawID) != id {
		return connectorTerms{}, errors.New("connector discount id does not match selected route")
	}
	a, err := parse.NonZeroAddress(r.Discount.Adapter, "connector adapter")
	if err != nil || a != venue {
		return connectorTerms{}, errors.New("connector discount route mismatch")
	}
	out.Discount.TokenToRedeem, err = parse.NonZeroAddress(r.Discount.TokenToRedeem, "tokenToRedeem")
	if err != nil || out.Discount.TokenToRedeem != token {
		return connectorTerms{}, errors.New("connector discount route mismatch")
	}
	out.Discount.Signer, err = parse.NonZeroAddress(r.Discount.Signer, "signer")
	if err != nil {
		return connectorTerms{}, err
	}
	out.Discount.Protocol, err = parse.NonZeroAddress(r.Discount.Protocol, "protocol")
	if err != nil {
		return connectorTerms{}, err
	}
	out.Discount.Discount, err = sdkparse.Uint256(r.Discount.Discount)
	if err != nil {
		return connectorTerms{}, errors.Errorf("discount: %w", err)
	}
	out.Discount.Nonce, err = sdkparse.Uint256(r.Discount.Nonce)
	if err != nil {
		return connectorTerms{}, errors.Errorf("nonce: %w", err)
	}
	if r.Discount.Deadline < 0 || r.Discount.Deadline >= 1<<48 || r.ProtocolDeadline < 0 || r.ProtocolDeadline >= 1<<48 {
		return connectorTerms{}, errors.New("connector discount deadline exceeds uint48")
	}
	out.Discount.Deadline = big.NewInt(r.Discount.Deadline)
	out.ProtocolDeadline = big.NewInt(r.ProtocolDeadline)
	out.SignerSignature, err = hexutil.Decode(r.SignerSignature)
	if err != nil {
		return connectorTerms{}, errors.Errorf("signerSignature: %w", err)
	}
	sig, err := hexutil.Decode(r.ProtocolSignature)
	if err != nil {
		return connectorTerms{}, errors.Errorf("protocolSignature: %w", err)
	}
	until := min(r.Discount.Deadline, r.ProtocolDeadline)
	return connectorTerms{out, sig, time.Unix(until, 0)}, nil
}

func assembleConnectorRoute(snapshot recipes.Snapshot, recipe recipes.Recipe, tree *recipes.Tree, venue, token, outputAsset, recipient common.Address, amountIn, amountOut *big.Int, signed connector.ILiquidLaneConnectorDiscountSwap, signature []byte) (assembledConnector, error) {
	inspection, err := recipes.Inspect(recipe)
	if err != nil {
		return assembledConnector{}, err
	}
	if amountIn == nil || amountOut == nil || amountIn.Sign() <= 0 || amountOut.Sign() <= 0 || amountIn.BitLen() > 256 || amountOut.BitLen() > 256 {
		return assembledConnector{}, errors.New("connector awarded amounts must be positive uint256")
	}
	data, err := connectorBinding.TryPackSwap(signed, signature, recipient, amountIn)
	if err != nil {
		return assembledConnector{}, errors.Errorf("encode connector swap: %w", err)
	}
	actions := map[string]hexutil.Bytes{}
	owned := map[string]bool{}
	inputs := map[uint]string{0: amountIn.String(), 1: amountOut.String()}
	managed, err := recipes.ManagedActions(recipe, tree)
	if err != nil {
		return assembledConnector{}, err
	}
	for _, action := range managed {
		if action.Leaf.Target != venue {
			continue
		}
		if len(action.Leaf.Data) < 164 || !bytes.Equal(action.Leaf.Data[:4], data[:4]) {
			return assembledConnector{}, errors.New("connector permission does not pin the signed swap and RWA")
		}
		if err := action.Pinned(0, 4); err != nil {
			return assembledConnector{}, err
		}
		if err := action.Pinned(132, 32); err != nil {
			return assembledConnector{}, err
		}
		if len(owned) != 0 {
			return assembledConnector{}, errors.New("connector recipe must contain one signed swap for one asset")
		}
		if common.BytesToAddress(action.Leaf.Data[132:164]) != token {
			return assembledConnector{}, errors.New("connector recipe asset differs from award")
		}
		amount, err := action.Field(100)
		if err != nil || amount.Source != recipes.Input || amount.Value.Sign() != 0 {
			return assembledConnector{}, errors.New("connector swap must bind amountIn to input 0")
		}
		gate := action.Slot.Gate
		if gate.Source != recipes.Input || gate.Value.Sign() != 0 {
			return assembledConnector{}, errors.New("connector swap gate must equal its amount input")
		}
		if err := validateConnectorDeltas(recipe, action.Step, snapshot.Account, recipient, outputAsset, token); err != nil {
			return assembledConnector{}, err
		}
		owned[action.Key] = true
		actions[action.Key] = data
	}
	if len(owned) != 1 {
		return assembledConnector{}, errors.New("connector recipe must contain one signed swap for one asset")
	}
	if len(inspection.Inputs) != 2 || inspection.Inputs[0] != 0 || inspection.Inputs[1] != 1 {
		return assembledConnector{}, errors.New("connector recipe requires only amount input 0 and payment input 1")
	}
	if err := recipes.ValidateMutableActions(recipe, tree, owned); err != nil {
		return assembledConnector{}, err
	}

	inner, leaves, err := recipes.Assemble(recipe, tree, inputs, actions, nil)
	if err != nil {
		return assembledConnector{}, err
	}
	return assembledConnector{inner, leaves}, nil
}

type permissionSource interface {
	PermissionTree(ctx context.Context, chainID *big.Int, snapshot recipes.Snapshot) ([]byte, error)
}

type connectorWorkflow struct {
	chain          recipes.Reader
	chainID        *big.Int
	executors      map[common.Address]map[common.Address]common.Address
	source         permissionSource
	caller, signer common.Address
}

func (w *connectorWorkflow) state(ctx context.Context, venue, token common.Address, block *big.Int) (connectorState, error) {
	var account, asset common.Address
	var remaining *big.Int
	call := func(data []byte) ([]byte, error) {
		return w.chain.CallContract(ctx, ethereum.CallMsg{To: &venue, Data: data}, block)
	}
	raw, err := call(connectorBinding.PackAccount())
	if err != nil {
		return connectorState{}, errors.Errorf("connector account: %w", err)
	}
	account, err = connectorBinding.UnpackAccount(raw)
	if err != nil || account == (common.Address{}) {
		return connectorState{}, errors.New("invalid connector account")
	}
	raw, err = call(connectorBinding.PackOutputAsset())
	if err != nil {
		return connectorState{}, errors.Errorf("connector outputAsset: %w", err)
	}
	asset, err = connectorBinding.UnpackOutputAsset(raw)
	if err != nil || asset == (common.Address{}) {
		return connectorState{}, errors.New("invalid connector outputAsset")
	}
	raw, err = call(connectorBinding.PackRemainingAssets(token))
	if err != nil {
		return connectorState{}, errors.Errorf("connector remainingAssets: %w", err)
	}
	remaining, err = connectorBinding.UnpackRemainingAssets(raw)
	if err != nil {
		return connectorState{}, errors.Errorf("connector remainingAssets: %w", err)
	}
	return connectorState{account, asset, remaining}, nil
}

type selectedRoute struct {
	Venue      common.Address
	DiscountID common.Hash
	MaxAssets  *big.Int
}

type discountResolver interface {
	resolveDiscount(ctx context.Context, id string) (*discounts.Resolved, error)
}

func (w *connectorWorkflow) build(ctx context.Context, order executor.IReactorOrder, protocolSig []byte, selected selectedRoute, backend discountResolver) ([]byte, time.Time, error) {
	token, required, err := orderTerms(order, w.caller)
	if err != nil {
		return nil, time.Time{}, err
	}
	configured, ok := w.executors[selected.Venue]
	recipeExecutor, admitted := configured[order.Request.TokenIn]
	if !ok || !admitted || selected.DiscountID == (common.Hash{}) || selected.MaxAssets == nil || selected.MaxAssets.Sign() < 0 {
		return nil, time.Time{}, errors.New("connector requires a configured executor and upstream discount")
	}
	snapshot, recipe, err := recipes.Discover(ctx, w.chain, recipeExecutor, nil)
	if err != nil {
		return nil, time.Time{}, errors.Errorf("discover connector recipe: %w", err)
	}
	if snapshot.Caller != w.caller {
		return nil, time.Time{}, errors.New("RecipeExecutor caller differs from RFQ Executor")
	}
	state, err := w.state(ctx, selected.Venue, order.Request.TokenIn, new(big.Int).SetUint64(snapshot.BlockNumber))
	if err != nil {
		return nil, time.Time{}, err
	}
	if state.Remaining.Cmp(required) < 0 || selected.MaxAssets.Cmp(required) < 0 {
		return nil, time.Time{}, errors.New("connector has insufficient capacity for the whole award")
	}
	if state.Account != snapshot.Account {
		return nil, time.Time{}, errors.New("connector account differs from RecipeExecutor account")
	}
	if state.Asset != token {
		return nil, time.Time{}, errors.New("connector outputAsset differs from awarded output")
	}
	raw, err := w.source.PermissionTree(ctx, w.chainID, snapshot)
	if err != nil {
		return nil, time.Time{}, errors.Errorf("connector permission tree: %w", err)
	}
	tree, err := recipes.DecodeTree(raw)
	if err != nil {
		return nil, time.Time{}, errors.Errorf("connector permission tree: %w", err)
	}
	if tree.Root != snapshot.Root || tree.ChainID == nil || tree.ChainID.Cmp(w.chainID) != 0 || tree.Account == nil || *tree.Account != snapshot.Account {
		return nil, time.Time{}, errors.New("connector permission tree scope mismatch")
	}
	resolved, err := backend.resolveDiscount(ctx, selected.DiscountID.Hex())
	if err != nil {
		return nil, time.Time{}, errors.Errorf("resolve connector discount: %w", err)
	}
	terms, err := parseConnectorDiscount(resolved, selected.DiscountID, selected.Venue, order.Request.TokenIn)
	if err != nil {
		return nil, time.Time{}, err
	}
	assembled, err := assembleConnectorRoute(snapshot, recipe, tree, selected.Venue, order.Request.TokenIn, token, w.caller, order.Request.AmountIn, required, terms.Swap, terms.Signature)
	if err != nil {
		return nil, time.Time{}, errors.Errorf("assemble connector recipe: %w", err)
	}
	if err := recipes.VerifyRoles(ctx, w.chain, snapshot, assembled.Leaves); err != nil {
		return nil, time.Time{}, errors.Errorf("connector recipe roles: %w", err)
	}
	calls, err := executorCallsArgs.Pack([]executorCall{{Target: snapshot.Executor, Value: new(big.Int), Data: assembled.Calldata}})
	if err != nil {
		return nil, time.Time{}, errors.Errorf("encode recipe call: %w", err)
	}
	discountInputs := []executor.IReactorDiscountSwapInput{{
		Adapter: selected.Venue,
		DiscountSwap: executor.ILiquidLaneAdapterDiscountSwap{
			Discount:        executor.ILiquidLaneAdapterDiscount{TokenToRedeem: terms.Swap.Discount.TokenToRedeem, Discount: terms.Swap.Discount.Discount, Signer: terms.Swap.Discount.Signer, Protocol: terms.Swap.Discount.Protocol, Nonce: terms.Swap.Discount.Nonce, Deadline: terms.Swap.Discount.Deadline},
			SignerSignature: terms.Swap.SignerSignature, ProtocolDeadline: terms.Swap.ProtocolDeadline,
		},
		ProtocolSignature: terms.Signature, Recipient: w.caller, AmountIn: order.Request.AmountIn,
	}}
	calldata, err := executorBinding.TryPackFill1(order, protocolSig, nil, discountInputs, calls)
	if err != nil {
		return nil, time.Time{}, errors.Errorf("encode recipe fill: %w", err)
	}
	reader := outerFillReader{Reader: w.chain, snapshot: snapshot, inner: assembled.Calldata, outer: calldata, caller: w.caller, signer: w.signer}
	if err := recipes.ValidateExecution(ctx, reader, snapshot, w.caller, assembled.Calldata); err != nil {
		return nil, time.Time{}, errors.Errorf("validate outer RFQ recipe fill: %w", err)
	}
	return calldata, terms.ValidUntil, nil
}

// Validate the immutable inner authorization against the complete settlement transaction.
type outerFillReader struct {
	recipes.Reader

	snapshot       recipes.Snapshot
	inner, outer   []byte
	caller, signer common.Address
}

func (r outerFillReader) CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	if msg.To != nil && *msg.To == r.snapshot.Executor && msg.From == r.caller && bytes.Equal(msg.Data, r.inner) {
		return r.Reader.CallContract(ctx, ethereum.CallMsg{From: r.signer, To: &r.caller, Data: r.outer}, block)
	}
	return r.Reader.CallContract(ctx, msg, block)
}
