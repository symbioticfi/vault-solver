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

func assembleConnectorRoute(recipe recipes.Recipe, tree *recipes.Tree, venue, token, recipient common.Address, amountIn, amountOut *big.Int, signed connector.ILiquidLaneConnectorDiscountSwap, signature []byte) (assembledConnector, error) {
	data, err := connectorBinding.TryPackSwap(signed, signature, recipient, amountIn)
	if err != nil {
		return assembledConnector{}, errors.Errorf("encode connector swap: %w", err)
	}
	actions := map[string]hexutil.Bytes{}
	inputs := map[uint]string{0: amountIn.String(), 1: amountOut.String()}
	managed, err := recipes.ManagedActions(recipe, tree)
	if err != nil {
		return assembledConnector{}, err
	}
	for _, action := range managed {
		boundToken, err := connectorActionToken(action, venue)
		if err != nil {
			return assembledConnector{}, err
		}
		if boundToken != token {
			continue
		}
		actions[action.Key] = data
	}
	if len(actions) == 0 {
		return assembledConnector{}, errors.New("connector swap action missing")
	}

	inner, leaves, err := recipes.Assemble(recipe, tree, inputs, actions, nil)
	if err != nil {
		return assembledConnector{}, err
	}
	return assembledConnector{inner, leaves}, nil
}

// connectorActionToken identifies a swap's statically committed RWA. Unrelated
// actions and dynamically selected tokens cannot advertise a cached route.
func connectorActionToken(action recipes.ManagedAction, venue common.Address) (common.Address, error) {
	if action.Leaf.Target != venue || len(action.Leaf.Data) < 164 || !bytes.Equal(action.Leaf.Data[:4], connectorBinding.GetABI().Methods["swap"].ID) {
		return common.Address{}, nil
	}
	if !bytes.Equal(action.Leaf.Data[4:36], common.LeftPadBytes([]byte{128}, 32)) {
		return common.Address{}, errors.New("connector swap tuple offset must be 128")
	}
	if err := action.Pinned(4, 32); err != nil {
		return common.Address{}, err
	}
	var token *big.Int
	if value, err := action.Field(132); err == nil {
		if value.Source != recipes.Literal {
			return common.Address{}, nil
		}
		token = value.Value
	} else {
		if err := action.Pinned(132, 32); err != nil {
			return common.Address{}, err
		}
		token = new(big.Int).SetBytes(action.Leaf.Data[132:164])
	}
	if token == nil || token.Sign() <= 0 || token.BitLen() > 160 {
		return common.Address{}, errors.New("invalid connector swap token")
	}
	return common.BigToAddress(token), nil
}

type permissionSource interface {
	PermissionTree(ctx context.Context, chainID *big.Int, snapshot recipes.Snapshot) ([]byte, error)
}

type connectorWorkflow struct {
	chain          recipes.Reader
	chainID        *big.Int
	maxGas         uint64
	source         permissionSource
	caller, signer common.Address
}

func (w *connectorWorkflow) permissionTree(ctx context.Context, snapshot recipes.Snapshot) (*recipes.Tree, error) {
	raw, err := w.source.PermissionTree(ctx, w.chainID, snapshot)
	if err != nil {
		return nil, errors.Errorf("connector permission tree: %w", err)
	}
	tree, err := recipes.DecodeTree(raw)
	if err != nil {
		return nil, errors.Errorf("connector permission tree: %w", err)
	}
	if tree.Root != snapshot.Root || tree.ChainID == nil || tree.ChainID.Cmp(w.chainID) != 0 || tree.Account == nil || *tree.Account != snapshot.Account {
		return nil, errors.New("connector permission tree scope mismatch")
	}
	return tree, nil
}

func (w *connectorWorkflow) state(ctx context.Context, venue common.Address, block *big.Int) (connectorState, error) {
	var account, asset common.Address
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
	return connectorState{Account: account, Asset: asset}, nil
}

type selectedRoute struct {
	Venue      common.Address
	Executor   common.Address
	DiscountID common.Hash
	MaxAssets  *big.Int
}

type discountResolver interface {
	resolveDiscount(ctx context.Context, id string) (*discounts.Resolved, error)
}

type builtConnector struct {
	Calldata   []byte
	ValidUntil time.Time
	State      connectorState
}

func (w *connectorWorkflow) build(ctx context.Context, order executor.IReactorOrder, protocolSig []byte, selected selectedRoute, backend discountResolver) (builtConnector, error) {
	token, required, err := orderTerms(order, w.caller)
	if err != nil {
		return builtConnector{}, err
	}
	if selected.Executor == (common.Address{}) {
		return builtConnector{}, errors.New("connector has no recipe executor")
	}
	if selected.Venue == (common.Address{}) || selected.DiscountID == (common.Hash{}) || selected.MaxAssets == nil || selected.MaxAssets.Sign() < 0 {
		return builtConnector{}, errors.New("connector requires recipe-derived routing and upstream discount")
	}
	if w.signer == (common.Address{}) {
		return builtConnector{}, errors.New("connector fill requires a host signer")
	}
	identities, err := recipes.DiscoverExecutors(ctx, w.chain, []common.Address{selected.Executor})
	if err != nil {
		return builtConnector{}, errors.Errorf("discover connector executor: %w", err)
	}
	prepared, err := recipes.DiscoverBatch(ctx, w.chain, []common.Address{selected.Executor}, []common.Address{identities[selected.Executor].Account}, w.caller, recipes.PreparationOptions{ChainID: w.chainID})
	if err != nil {
		return builtConnector{}, errors.Errorf("discover connector recipe: %w", err)
	}
	member := prepared[0]
	snapshot, recipe := member.Snapshot, member.Recipe
	block := new(big.Int).SetUint64(snapshot.BlockNumber)
	state, err := w.state(ctx, selected.Venue, block)
	if err != nil {
		return builtConnector{}, err
	}
	if state.Account != snapshot.Account {
		return builtConnector{}, errors.New("connector account differs from recipe account")
	}
	raw, err := w.chain.CallContract(ctx, ethereum.CallMsg{To: &selected.Venue, Data: connectorBinding.PackRemainingAssets(order.Request.TokenIn)}, block)
	if err != nil {
		return builtConnector{}, errors.Errorf("connector remainingAssets: %w", err)
	}
	remaining, err := connectorBinding.UnpackRemainingAssets(raw)
	if err != nil {
		return builtConnector{}, errors.Errorf("connector remainingAssets decode: %w", err)
	}
	if remaining.Cmp(required) < 0 || selected.MaxAssets.Cmp(required) < 0 {
		return builtConnector{}, errors.New("connector has insufficient capacity for the whole award")
	}
	if state.Asset != token {
		return builtConnector{}, errors.New("connector outputAsset differs from awarded output")
	}
	tree, err := w.permissionTree(ctx, snapshot)
	if err != nil {
		return builtConnector{}, err
	}
	resolved, err := backend.resolveDiscount(ctx, selected.DiscountID.Hex())
	if err != nil {
		return builtConnector{}, errors.Errorf("resolve connector discount: %w", err)
	}
	terms, err := parseConnectorDiscount(resolved, selected.DiscountID, selected.Venue, order.Request.TokenIn)
	if err != nil {
		return builtConnector{}, err
	}
	assembled, err := assembleConnectorRoute(recipe, tree, selected.Venue, order.Request.TokenIn, w.caller, order.Request.AmountIn, required, terms.Swap, terms.Signature)
	if err != nil {
		return builtConnector{}, errors.Errorf("assemble connector recipe: %w", err)
	}
	calls, err := executorCallsArgs.Pack([]executorCall{{Target: snapshot.Executor, Value: new(big.Int), Data: assembled.Calldata}})
	if err != nil {
		return builtConnector{}, errors.Errorf("encode recipe call: %w", err)
	}
	discountInputs := []executor.IReactorDiscountSwapInput{{
		Adapter: selected.Venue,
		DiscountSwap: executor.ILiquidLaneAdapterDiscountSwap{
			Discount:        executor.ILiquidLaneAdapterDiscount{TokenToRedeem: terms.Swap.Discount.TokenToRedeem, Discount: terms.Swap.Discount.Discount, Signer: terms.Swap.Discount.Signer, Protocol: terms.Swap.Discount.Protocol, Nonce: terms.Swap.Discount.Nonce, Deadline: terms.Swap.Discount.Deadline},
			SignerSignature: terms.Swap.SignerSignature, ProtocolDeadline: terms.Swap.ProtocolDeadline,
		},
		ProtocolSignature: terms.Signature, Recipient: w.caller, AmountIn: order.Request.AmountIn,
	}}
	calldata, err := executorBinding.TryPackFill(order, protocolSig, nil, discountInputs, calls)
	if err != nil {
		return builtConnector{}, errors.Errorf("encode recipe fill: %w", err)
	}
	// The SDK authenticates the inner call and pins around the unchanged outer fill.
	simulate := func(ctx context.Context, request recipes.PreparedBatchSimulationRequest) error {
		_, err := w.chain.CallContract(ctx, ethereum.CallMsg{From: w.signer, To: &w.caller, Data: bytes.Clone(calldata), Gas: request.GasLimit}, request.BlockNumber)
		return err
	}
	if err := recipes.ValidatePreparedBatchWithSimulation(ctx, w.chain, prepared, []recipes.ExecutorCall{{Executor: snapshot.Executor, Data: assembled.Calldata}}, [][]common.Hash{assembled.Leaves}, w.caller, simulate, w.maxGas); err != nil {
		return builtConnector{}, errors.Errorf("validate outer RFQ recipe fill: %w", err)
	}
	return builtConnector{Calldata: calldata, ValidUntil: terms.ValidUntil, State: state}, nil
}
