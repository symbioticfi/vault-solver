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
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/parse"
)

const connectorDiscountPrecision = 100_000_000

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
	if r.DiscountPrecision != "100000000" {
		return connectorTerms{}, errors.Errorf("connector discount precision: expected 100000000, got %q", r.DiscountPrecision)
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

type connectorLeg struct {
	selectedRoute

	AmountIn *big.Int
}

type builtConnector struct {
	Calldata   []byte
	ValidUntil time.Time
	Amounts    liquidlane.CapacityReservations
}

func (w *connectorWorkflow) build(ctx context.Context, order executor.IReactorOrder, protocolSig []byte, selected selectedRoute, backend discountResolver) (builtConnector, error) {
	return w.buildFill(ctx, order, protocolSig, []connectorLeg{{selectedRoute: selected}}, backend)
}

func (w *connectorWorkflow) buildFill(ctx context.Context, order executor.IReactorOrder, protocolSig []byte, legs []connectorLeg, backend discountResolver) (builtConnector, error) {
	token, required, err := orderTerms(order, w.caller)
	if err != nil {
		return builtConnector{}, err
	}
	if w.signer == (common.Address{}) || len(legs) == 0 {
		return builtConnector{}, errors.New("connector fill requires a host signer and selected legs")
	}
	executors := make([]common.Address, 0, len(legs))
	seen := make(map[common.Address]bool, len(legs))
	totalIn := new(big.Int)
	for _, leg := range legs {
		if leg.Executor == (common.Address{}) {
			return builtConnector{}, errors.New("connector has no recipe executor")
		}
		if leg.Venue == (common.Address{}) || seen[leg.Venue] || leg.DiscountID == (common.Hash{}) || leg.MaxAssets == nil || leg.MaxAssets.Sign() < 0 {
			return builtConnector{}, errors.New("connector requires distinct recipe-derived routes and upstream discounts")
		}
		seen[leg.Venue] = true
		executors = append(executors, leg.Executor)
		if len(legs) == 1 && leg.AmountIn == nil {
			totalIn.Set(order.Request.AmountIn)
		} else {
			if leg.AmountIn == nil || leg.AmountIn.Sign() <= 0 {
				return builtConnector{}, errors.New("connector leg requires positive input")
			}
			totalIn.Add(totalIn, leg.AmountIn)
		}
	}
	if totalIn.Cmp(order.Request.AmountIn) != 0 {
		return builtConnector{}, errors.New("connector leg inputs do not sum to the signed order input")
	}
	prepared, err := w.discoverRecipes(ctx, executors)
	if err != nil {
		return builtConnector{}, errors.Errorf("discover connector recipes: %w", err)
	}
	byExecutor := make(map[common.Address]recipes.PreparedRecipe, len(prepared))
	for _, member := range prepared {
		byExecutor[member.Snapshot.Executor] = member
	}
	ordered := make([]recipes.PreparedRecipe, 0, len(legs))
	calls := make([]executorCall, 0, len(legs))
	innerCalls := make([]recipes.ExecutorCall, 0, len(legs))
	leaves := make([][]common.Hash, 0, len(legs))
	discountInputs := make([]executor.IReactorDiscountSwapInput, 0, len(legs))
	spends := make(liquidlane.CapacityReservations)
	totalOut := new(big.Int)
	validUntil := time.Unix(order.Request.Deadline.Int64(), 0)

	for _, leg := range legs {
		member, ok := byExecutor[leg.Executor]
		if !ok {
			return builtConnector{}, errors.New("connector recipe was not discovered")
		}
		built, err := w.buildLeg(ctx, order.Request, token, required, leg, member, backend)
		if err != nil {
			return builtConnector{}, err
		}
		ordered = append(ordered, member)
		calls = append(calls, built.Call)
		innerCalls = append(innerCalls, recipes.ExecutorCall{Executor: leg.Executor, Data: built.Inner.Calldata})
		leaves = append(leaves, built.Inner.Leaves)
		discountInputs = append(discountInputs, built.Input)
		spends.Add(liquidlane.NewCapacityID(w.chainID.Int64(), member.Snapshot.Account, token), built.AmountOut)
		totalOut.Add(totalOut, built.AmountOut)
		validUntil = minTime(validUntil, built.ValidUntil)
	}
	if totalOut.Cmp(required) < 0 {
		return builtConnector{}, errors.New("connector legs do not cover the signed order output")
	}
	data, err := executorCallsArgs.Pack(calls)
	if err != nil {
		return builtConnector{}, errors.Errorf("encode recipe calls: %w", err)
	}
	calldata, err := executorBinding.TryPackFill(order, protocolSig, nil, discountInputs, data)
	if err != nil {
		return builtConnector{}, errors.Errorf("encode recipe fill: %w", err)
	}
	simulate := func(ctx context.Context, request recipes.PreparedBatchSimulationRequest) error {
		_, err := w.chain.CallContract(ctx, ethereum.CallMsg{From: w.signer, To: &w.caller, Data: bytes.Clone(calldata), Gas: request.GasLimit}, request.BlockNumber)
		return err
	}
	if err := recipes.ValidatePreparedRoutesWithSimulation(ctx, w.chain, ordered, innerCalls, leaves, w.caller, simulate, w.maxGas); err != nil {
		return builtConnector{}, errors.Errorf("validate outer RFQ recipe fill: %w", err)
	}
	return builtConnector{Calldata: calldata, ValidUntil: validUntil, Amounts: spends}, nil
}

type builtConnectorLeg struct {
	Call       executorCall
	Input      executor.IReactorDiscountSwapInput
	Inner      assembledConnector
	AmountOut  *big.Int
	ValidUntil time.Time
}

func (w *connectorWorkflow) buildLeg(ctx context.Context, request executor.IReactorRequest, token common.Address, required *big.Int, leg connectorLeg, member recipes.PreparedRecipe, backend discountResolver) (builtConnectorLeg, error) {
	snapshot := member.Snapshot
	block := new(big.Int).SetUint64(snapshot.BlockNumber)
	state, err := w.state(ctx, leg.Venue, block)
	if err != nil {
		return builtConnectorLeg{}, err
	}
	if state.Account != snapshot.Account {
		return builtConnectorLeg{}, errors.New("connector account differs from recipe account")
	}
	if state.Asset != token {
		return builtConnectorLeg{}, errors.New("connector outputAsset differs from awarded output")
	}
	raw, err := w.chain.CallContract(ctx, ethereum.CallMsg{To: &leg.Venue, Data: connectorBinding.PackRemainingAssets(request.TokenIn)}, block)
	if err != nil {
		return builtConnectorLeg{}, errors.Errorf("connector remainingAssets: %w", err)
	}
	remaining, err := connectorBinding.UnpackRemainingAssets(raw)
	if err != nil {
		return builtConnectorLeg{}, errors.Errorf("connector remainingAssets decode: %w", err)
	}
	if leg.AmountIn == nil && (remaining.Cmp(required) < 0 || leg.MaxAssets.Cmp(required) < 0) {
		return builtConnectorLeg{}, errors.New("connector has insufficient capacity for the whole award")
	}
	tree, err := w.permissionTree(ctx, snapshot)
	if err != nil {
		return builtConnectorLeg{}, err
	}
	resolved, err := backend.resolveDiscount(ctx, leg.DiscountID.Hex())
	if err != nil {
		return builtConnectorLeg{}, errors.Errorf("resolve connector discount: %w", err)
	}
	terms, err := parseConnectorDiscount(resolved, leg.DiscountID, leg.Venue, request.TokenIn)
	if err != nil {
		return builtConnectorLeg{}, err
	}
	amountIn, amountOut := request.AmountIn, required
	if leg.AmountIn != nil {
		amountIn = leg.AmountIn
		data := connectorBinding.PackGetAmountOut(request.TokenIn, amountIn)
		raw, err = w.chain.CallContract(ctx, ethereum.CallMsg{To: &leg.Venue, Data: data}, block)
		if err != nil {
			return builtConnectorLeg{}, errors.Errorf("connector leg price: %w", err)
		}
		gross, err := connectorBinding.UnpackGetAmountOut(raw)
		if err != nil {
			return builtConnectorLeg{}, errors.Errorf("decode connector leg price: %w", err)
		}
		amountOut = liquidlane.AmountOutAfterDiscountPrecision(gross, terms.Swap.Discount.Discount, connectorDiscountPrecision)
		if amountOut.Sign() <= 0 || amountOut.Cmp(remaining) > 0 || amountOut.Cmp(leg.MaxAssets) > 0 {
			return builtConnectorLeg{}, errors.New("connector payout exceeds available leg capacity or is zero")
		}
	}
	assembled, err := assembleConnectorRoute(member.Recipe, tree, leg.Venue, request.TokenIn, w.caller, amountIn, amountOut, terms.Swap, terms.Signature)
	if err != nil {
		return builtConnectorLeg{}, errors.Errorf("assemble connector recipe: %w", err)
	}
	routed, err := recipes.RouteExecution(ctx, w.chain, snapshot, w.caller, assembled.Calldata)
	if err != nil {
		return builtConnectorLeg{}, err
	}
	return builtConnectorLeg{
		Call: executorCall{Target: routed.Executor, Value: new(big.Int), Data: routed.Data},
		Input: executor.IReactorDiscountSwapInput{
			Adapter: leg.Venue,
			DiscountSwap: executor.ILiquidLaneAdapterDiscountSwap{
				Discount:        executor.ILiquidLaneAdapterDiscount{TokenToRedeem: terms.Swap.Discount.TokenToRedeem, Discount: terms.Swap.Discount.Discount, Signer: terms.Swap.Discount.Signer, Protocol: terms.Swap.Discount.Protocol, Nonce: terms.Swap.Discount.Nonce, Deadline: terms.Swap.Discount.Deadline},
				SignerSignature: terms.Swap.SignerSignature, ProtocolDeadline: terms.Swap.ProtocolDeadline,
			},
			ProtocolSignature: terms.Signature, Recipient: w.caller, AmountIn: amountIn,
		},
		Inner: assembled, AmountOut: amountOut, ValidUntil: terms.ValidUntil,
	}, nil
}
