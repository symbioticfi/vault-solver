//go:build integration

package rfq

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethmath "github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector/connectortest"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

// All settlement contracts, including AccessManager, execute pinned canonical bytecode.
// Only ERC20s, factories/root/oracle registries and the price feed are fixture contracts.
func TestAnvilConnectorOuterFillCanonicalBoundaries(t *testing.T) {
	for _, name := range []string{"valid fill", "bad signed terms", "bad discount signature", "bad order signature", "insufficient funding at simulation", "native recipe check", "unauthorized EOA", "manager fill", "manager invalid signature", "manager revoked"} {
		t.Run(name, func(t *testing.T) {
			f := newOuterFillEVM(t)
			if strings.HasPrefix(name, "manager ") {
				manager := f.deploy("AccessManager", f.signer)
				f.send(f.signer, f.selected.RecipeExecutor, "RecipeExecutor", "setCaller", manager)
				selector := [4]byte(recipeBinding.GetABI().Methods["execute"].ID)
				f.send(f.signer, manager, "AccessManager", "setTargetFunctionRole", f.selected.RecipeExecutor, [][4]byte{selector}, uint64(2))
				f.send(f.signer, manager, "AccessManager", "grantRole", uint64(2), f.workflow.Filler, uint32(0))
				if name == "manager revoked" {
					f.send(f.signer, manager, "AccessManager", "revokeRole", uint64(2), f.workflow.Filler)
				}
			}
			if name == "bad signed terms" {
				f.backend.discount.Discount.Discount = "0" // Valid signatures, below the connector's current minimum.
				signOuterFillDiscount(t, f.key, f.backend.discount)
			}
			if name == "bad discount signature" {
				signature, err := hexutil.Decode(f.backend.discount.ProtocolSignature)
				if err != nil {
					t.Fatal(err)
				}
				signature[0] ^= 1
				f.backend.discount.ProtocolSignature = hexutil.Encode(signature)
			}
			if name == "bad order signature" || name == "manager invalid signature" {
				f.protocolSignature[0] ^= 1
			}
			if name == "insufficient funding at simulation" {
				// Backend capacity was advertised, then cash disappears before
				// latest-state settlement simulation. The outer transaction must still fail.
				f.reader.beforeOuter = func() {
					// Mutate the observed head's cash without mining a later block.
					slot := crypto.Keccak256Hash(common.LeftPadBytes(f.account.Bytes(), 32), make([]byte, 32))
					f.rpcCall("anvil_setStorageAt", tOut, slot, common.Hash{})
				}
			}
			if name == "native recipe check" {
				// Funding remains valid, but the committed post-swap balance cap must fail.
				recipe := recipes.CloneRecipe(f.recipe)
				recipe.Steps = append(recipe.Steps, connectortest.DeltaBinary(t, recipes.LessOrEqual, connectortest.DeltaOperand(recipes.Result, 12), connectortest.DeltaOperand(recipes.Literal, 99)))
				f.send(f.signer, f.selected.RecipeExecutor, "RecipeExecutor", "setRecipe", recipe.Queries, recipe.Steps)
			}
			if name == "unauthorized EOA" {
				f.workflow.Signer = common.HexToAddress("0x8888")
			}
			built, err := buildFill(t.Context(), f.workflow, f.order, f.protocolSignature, []filler.Leg{{Source: f.selected}}, f.backend)
			data := built.Calldata
			if name == "manager revoked" {
				if err == nil || !strings.Contains(err.Error(), "immediate") || f.reader.outerCalls != 0 {
					t.Fatalf("revoked grant reached full settlement: %v", err)
				}
				return
			}
			if f.reader.outerCalls != 1 {
				t.Fatalf("full settlement was not simulated: calls=%d err=%v", f.reader.outerCalls, err)
			}
			if name != "valid fill" && name != "manager fill" {
				if err == nil || len(data) != 0 || len(f.reader.revertData) < 4 {
					t.Fatalf("canonical settlement accepted %s: data=%x revert=%x err=%v", name, data, f.reader.revertData, err)
				}
				if name == "bad signed terms" {
					f.assertRevert("LiquidLaneConnector", "InvalidSwapRate")
				}
				if name == "unauthorized EOA" {
					f.assertRevert("Executor", "NotCaller")
				}
				if name == "bad discount signature" {
					f.assertRevert("LiquidLaneConnector", "InvalidSignature")
				}
				if name == "bad order signature" || name == "manager invalid signature" {
					f.assertRevert("Reactor", "InvalidProtocolSignature")
				}
				if name == "native recipe check" {
					f.assertRevert("RecipeExecutor", "CheckFailed")
					if f.balance(tOut, f.account).Int64() != 90 {
						t.Fatal("native recipe rejection changed account cash")
					}
				}
				if name == "insufficient funding at simulation" {
					reason, err := abi.UnpackRevert(f.reader.revertData)
					if err != nil || !strings.Contains(reason, "underflow or overflow") || f.balance(tOut, f.account).Sign() != 0 {
						t.Fatalf("funding rejection was not the token cash shortfall: reason=%s err=%v", reason, err)
					}
				}
				if f.balance(tIn, f.signer).Int64() != 100 || f.balance(tIn, f.account).Sign() != 0 {
					t.Fatal("failed simulation leaked Reactor funding")
				}
				return
			}
			if err != nil || len(f.reader.outerReturn) != 0 {
				t.Fatalf("complete fill with empty return rejected: return=%x err=%v", f.reader.outerReturn, err)
			}
			if !bytes.Equal(data, f.reader.outerData) || f.balance(tOut, f.account).Int64() != 90 || f.balance(tIn, f.signer).Int64() != 100 {
				t.Fatal("simulation changed state or differed from submitted bytes")
			}
			receipt := f.transact(f.signer, &f.workflow.Filler, data)
			if receipt.Status != types.ReceiptStatusSuccessful || len(receipt.Logs) == 0 || f.balance(tOut, f.recipient).Int64() != 90 || f.balance(tOut, f.account).Sign() != 0 || f.balance(tIn, f.account).Int64() != 100 || f.balance(tIn, f.signer).Sign() != 0 {
				t.Fatalf("canonical fill did not settle balances: receipt=%+v", receipt)
			}
		})
	}
}

type outerFillArtifact struct {
	ABI      json.RawMessage `json:"abi"`
	Bytecode string          `json:"bytecode"`
}

type outerFillEVM struct {
	t                 *testing.T
	rpc               *rpc.Client
	client            *ethclient.Client
	artifacts         map[string]outerFillArtifact
	abis              map[string]abi.ABI
	signer, account   common.Address
	world, reactor    common.Address
	key               *ecdsa.PrivateKey
	recipient         common.Address
	workflow          *connector.Workflow
	reader            *outerFillEVMReader
	recipe            recipes.Recipe
	order             executor.IReactorOrder
	protocolSignature []byte
	selected          filler.Source
	backend           *fakeBackend
}

type outerFillEVMReader struct {
	client      *ethclient.Client
	caller      common.Address
	beforeOuter func()
	outerCalls  int
	outerData   []byte
	outerReturn []byte
	revertData  []byte
}

func (r *outerFillEVMReader) CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	outer := msg.To != nil && *msg.To == r.caller && bytes.HasPrefix(msg.Data, executorBinding.GetABI().Methods["fill"].ID)
	if outer {
		r.outerCalls++
		r.outerData = bytes.Clone(msg.Data)
		if block == nil || msg.Gas == 0 {
			return nil, errors.New("outer settlement simulation lost its block or gas budget")
		}
		header, err := r.client.HeaderByNumber(ctx, block)
		if err != nil || header == nil || msg.Gas != min(header.GasLimit, uint64(16_777_216)) {
			return nil, errors.New("outer settlement simulation did not use bounded transaction gas budget")
		}
		if r.beforeOuter != nil {
			r.beforeOuter()
			r.beforeOuter = nil
		}
	}
	data, err := r.client.CallContract(ctx, msg, block)
	if outer {
		r.outerReturn = bytes.Clone(data)
	}
	if outer && err != nil {
		var revert rpc.DataError
		if errors.As(err, &revert) {
			if encoded, ok := revert.ErrorData().(string); ok {
				r.revertData, _ = hexutil.Decode(encoded)
			}
		}
	}
	return data, err
}

func (r *outerFillEVMReader) HeaderByNumber(ctx context.Context, block *big.Int) (*types.Header, error) {
	return r.client.HeaderByNumber(ctx, block)
}

type outerFillPermissions []byte

func (p outerFillPermissions) PermissionTree(context.Context, *big.Int, recipes.Snapshot) ([]byte, error) {
	return bytes.Clone(p), nil
}

func newOuterFillEVM(t *testing.T, deployed ...*capturedExecutor) *outerFillEVM {
	t.Helper()
	rc, client := startOuterFillAnvil(t)
	f := &outerFillEVM{t: t, rpc: rc, client: client, account: common.HexToAddress("0x21"), recipient: common.HexToAddress("0x55"), abis: make(map[string]abi.ABI)}
	raw, err := os.ReadFile("testdata/outer-fill/artifacts.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Contracts map[string]outerFillArtifact `json:"contracts"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	f.artifacts = catalog.Contracts
	raw, err = os.ReadFile("testdata/outer-fill/access-manager.json")
	if err != nil {
		t.Fatal(err)
	}
	var manager outerFillArtifact
	if err := json.Unmarshal(raw, &manager); err != nil {
		t.Fatal(err)
	}
	f.artifacts["AccessManager"] = manager
	for name, artifact := range f.artifacts {
		parsed, err := abi.JSON(bytes.NewReader(artifact.ABI))
		if err != nil {
			t.Fatal(err)
		}
		f.abis[name] = parsed
	}
	key, err := crypto.HexToECDSA(strings.Repeat("0", 63) + "1")
	if err != nil {
		t.Fatal(err)
	}
	f.signer = crypto.PubkeyToAddress(key.PublicKey)
	f.key = key
	f.rpcCall("anvil_setBalance", f.signer, "0x10000000000000000000")
	f.rpcCall("anvil_impersonateAccount", f.signer)
	for _, token := range []common.Address{tIn, tOut} {
		f.materialize("FixtureToken", token)
	}
	f.world = f.deploy("FixtureWorld", tOut)
	caller := common.HexToAddress("0x1000") // Outside the Osaka precompile address range.
	if len(deployed) > 0 && deployed[0] != nil {
		// Use the unmodified deployed proxy and implementation at their actual
		// addresses. This Anvil fixture supplies the Reactor and funding state.
		contract := deployed[0]
		caller, f.reactor = contract.Proxy, contract.Reactor
		f.materialize("Reactor", f.reactor, f.world)
		f.rpcCall("anvil_setCode", caller, hexutil.Encode(contract.ProxyCode))
		f.rpcCall("anvil_setCode", contract.Implementation, hexutil.Encode(contract.ImplementationCode))
		f.rpcCall("anvil_setStorageAt", caller, "0x360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc", common.BytesToHash(contract.Implementation.Bytes()))
	} else {
		f.reactor = f.deploy("Reactor", f.world)
		f.materialize("Executor", caller, f.reactor)
	}
	f.send(f.signer, caller, "Executor", "initialize", f.signer, []common.Address{f.signer})
	f.reader = &outerFillEVMReader{client: client, caller: caller}
	f.workflow = &connector.Workflow{Chain: f.reader, ChainID: big.NewInt(1), Filler: caller, Signer: f.signer}
	first := f.addConnector(f.account, vlt, 90, *connectortest.DiscountID())
	f.recipe = first.Recipe
	f.workflow.Source = outerFillPermissions(first.Permissions)
	f.send(f.signer, tIn, "FixtureToken", "mint", f.signer, big.NewInt(100))
	f.send(f.signer, tIn, "FixtureToken", "approve", f.reactor, big.NewInt(100))
	f.order, f.selected, f.backend = connectorExecutionAward(caller)
	f.selected.RecipeExecutor = first.Executor
	f.order.Request.AmountIn = big.NewInt(100)
	f.order.Request.Protocol, f.order.Swapper = f.signer, f.signer
	f.order.Outputs[0].Amount, f.order.Outputs[0].Recipient = big.NewInt(90), f.recipient
	f.backend.discount = first.Discount
	f.protocolSignature = signOuterFillOrder(t, key, f.reactor, &f.order)
	return f
}

type outerFillConnector struct {
	Executor    common.Address
	Recipe      recipes.Recipe
	Permissions []byte
	Discount    *discounts.Resolved
}

func (f *outerFillEVM) addConnector(account, venue common.Address, funding int64, id common.Hash) outerFillConnector {
	t, caller, world := f.t, f.workflow.Filler, f.world
	t.Helper()
	f.materialize("MerkleAccount", account, world, world, world, world)
	f.materialize("LiquidLaneConnector", venue, world, world, world)
	addressArgs := abi.Arguments{{Type: connectortest.DeltaType(t, "address")}}
	admin, err := addressArgs.Pack(f.signer)
	if err != nil {
		t.Fatal(err)
	}
	accountInit, err := (abi.Arguments{{Type: connectortest.DeltaType(t, "address")}, {Type: connectortest.DeltaType(t, "bytes")}}).Pack(world, admin)
	if err != nil {
		t.Fatal(err)
	}
	f.send(f.signer, account, "MerkleAccount", "initialize", uint64(1), f.signer, accountInit)
	connectorInit, err := (abi.Arguments{{Type: connectortest.DeltaType(t, "address")}, {Type: connectortest.DeltaType(t, "address")}}).Pack(account, tOut)
	if err != nil {
		t.Fatal(err)
	}
	f.send(f.signer, venue, "LiquidLaneConnector", "initialize", uint64(2), f.signer, connectorInit)
	f.rpcCall("anvil_setBalance", account, "0x10000000000000000000")
	f.rpcCall("anvil_impersonateAccount", account)
	f.send(f.signer, world, "FixtureWorld", "setOracle", venue, tIn)
	f.send(account, venue, "LiquidLaneConnector", "addTokenToRedeem", tIn, world)
	f.send(account, venue, "LiquidLaneConnector", "setMaxAssetsPerDuration", tIn, big.NewInt(1000), big.NewInt(86400))
	f.send(account, venue, "LiquidLaneConnector", "setSigner", f.signer)
	f.send(account, venue, "LiquidLaneConnector", "setMinDiscount", tIn, big.NewInt(10000000))
	recipeExecutor := f.deploy("RecipeExecutor", f.signer, account)
	f.send(f.signer, recipeExecutor, "RecipeExecutor", "setCaller", caller)
	f.send(f.signer, tOut, "FixtureToken", "mint", account, big.NewInt(funding))
	f.send(account, tOut, "FixtureToken", "approve", venue, big.NewInt(funding))
	resolved := connectortest.DiscountFixture()
	resolved.DiscountID, resolved.Discount.Adapter = id.Hex(), venue.Hex()
	resolved.Discount.Discount, resolved.Discount.Signer, resolved.Discount.Protocol = "10000000", f.signer.Hex(), f.signer.Hex()
	signOuterFillDiscount(t, f.key, resolved)
	terms, err := connector.ParseDiscount(resolved, id, venue, tIn)
	if err != nil {
		t.Fatal(err)
	}
	data, mask := connectortest.SignatureTemplate(terms.Swap, terms.Signature, caller, new(big.Int))
	for _, offset := range []int{100, 164} { // Runtime awarded amount and signed discount ratio.
		for i := offset; i < offset+32; i++ {
			mask[i] = 255
		}
	}
	leaf := recipes.Leaf{Target: venue, Value: new(big.Int), Data: data, DataMask: mask}
	leaf.Hash, err = recipes.HashLeaf(leaf)
	if err != nil {
		t.Fatal(err)
	}
	recipe, tree, permissions := connectortest.CommittedFunding(t, leaf, account, caller, tOut, tIn)
	f.send(f.signer, world, "FixtureWorld", "setRoot", account, tree.Root)
	f.send(f.signer, account, "MerkleAccount", "acceptRoot", tree.Root)
	for _, permission := range tree.Leaves {
		f.send(f.signer, account, "MerkleAccount", "grantRole", permission.Hash, recipeExecutor)
	}
	f.send(f.signer, recipeExecutor, "RecipeExecutor", "setRecipe", recipe.Queries, recipe.Steps)

	return outerFillConnector{Executor: recipeExecutor, Recipe: recipe, Permissions: permissions, Discount: resolved}
}

func (f *outerFillEVM) rpcCall(method string, args ...any) {
	f.t.Helper()
	if err := f.rpc.CallContext(f.t.Context(), nil, method, args...); err != nil {
		f.t.Fatalf("%s: %v", method, err)
	}
}

func (f *outerFillEVM) deploy(name string, args ...any) common.Address {
	f.t.Helper()
	contractABI := f.abis[name]
	constructor, err := contractABI.Pack("", args...)
	if err != nil {
		f.t.Fatal(err)
	}
	data, err := hexutil.Decode(f.artifacts[name].Bytecode)
	if err != nil {
		f.t.Fatal(err)
	}
	return f.transact(f.signer, nil, append(data, constructor...)).ContractAddress
}

// Give canonical implementation runtime fresh proxy-like storage at a stable fixture address.
// This avoids testing deployment/proxy administration; no settlement code is rewritten.
func (f *outerFillEVM) materialize(name string, address common.Address, args ...any) {
	f.t.Helper()
	implementation := f.deploy(name, args...)
	code, err := f.client.CodeAt(f.t.Context(), implementation, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	f.rpcCall("anvil_setCode", address, hexutil.Encode(code))
}

func (f *outerFillEVM) send(from, to common.Address, contract, method string, args ...any) {
	f.t.Helper()
	contractABI := f.abis[contract]
	data, err := contractABI.Pack(method, args...)
	if err != nil {
		f.t.Fatalf("%s.%s: %v", contract, method, err)
	}
	f.transact(from, &to, data)
}

func (f *outerFillEVM) transact(from common.Address, to *common.Address, data []byte, want ...uint64) *types.Receipt {
	f.t.Helper()
	wantStatus := types.ReceiptStatusSuccessful
	if len(want) > 0 {
		wantStatus = want[0]
	}
	args := map[string]any{"from": from, "data": hexutil.Encode(data), "gas": "0x1c9c380"}
	if to != nil {
		args["to"] = *to
	}
	var hash common.Hash
	if err := f.rpc.CallContext(f.t.Context(), &hash, "eth_sendTransaction", args); err != nil {
		f.t.Fatalf("fixture transaction: %v", err)
	}
	ctx, cancel := context.WithTimeout(f.t.Context(), 5*time.Second)
	defer cancel()
	for {
		receipt, err := f.client.TransactionReceipt(ctx, hash)
		if err == nil {
			if receipt.Status != wantStatus {
				f.t.Fatalf("unexpected fixture transaction status: hash=%s to=%v data=%x receipt=%+v", hash, to, data[:min(len(data), 4)], receipt)
			}
			return receipt
		}
		if !errors.Is(err, ethereum.NotFound) {
			f.t.Fatalf("fixture transaction receipt: hash=%s err=%v", hash, err)
		}
		select {
		case <-ctx.Done():
			f.t.Fatalf("fixture transaction not mined: hash=%s err=%v", hash, ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (f *outerFillEVM) balance(token, account common.Address) *big.Int {
	f.t.Helper()
	contractABI := f.abis["FixtureToken"]
	data, err := contractABI.Pack("balanceOf", account)
	if err != nil {
		f.t.Fatal(err)
	}
	raw, err := f.client.CallContract(f.t.Context(), ethereum.CallMsg{To: &token, Data: data}, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	return new(big.Int).SetBytes(raw)
}

func (f *outerFillEVM) assertRevert(contract, name string) {
	f.t.Helper()
	want := f.abis[contract].Errors[name].ID
	if !bytes.Equal(f.reader.revertData[:4], want[:4]) {
		f.t.Fatalf("wrong canonical rejection: got %x want %s.%s (%x)", f.reader.revertData, contract, name, want[:4])
	}
}

func signOuterFillDiscount(t *testing.T, key *ecdsa.PrivateKey, resolved *discounts.Resolved) {
	t.Helper()
	discount := apitypes.TypedDataMessage{"tokenToRedeem": tIn.Hex(), "discount": resolved.Discount.Discount, "signer": resolved.Discount.Signer, "protocol": resolved.Discount.Protocol, "nonce": resolved.Discount.Nonce, "deadline": strconv.FormatInt(resolved.Discount.Deadline, 10)}
	types := apitypes.Types{"Discount": {{Name: "tokenToRedeem", Type: "address"}, {Name: "discount", Type: "uint256"}, {Name: "signer", Type: "address"}, {Name: "protocol", Type: "address"}, {Name: "nonce", Type: "uint256"}, {Name: "deadline", Type: "uint48"}}, "DiscountSwap": {{Name: "discount", Type: "Discount"}, {Name: "signerSignature", Type: "bytes"}, {Name: "protocolDeadline", Type: "uint48"}}}
	resolved.SignerSignature = hexutil.Encode(signOuterFillTyped(t, key, "LiquidLaneConnector", common.HexToAddress(resolved.Discount.Adapter), types, "Discount", discount))
	resolved.ProtocolSignature = hexutil.Encode(signOuterFillTyped(t, key, "LiquidLaneConnector", common.HexToAddress(resolved.Discount.Adapter), types, "DiscountSwap", apitypes.TypedDataMessage{"discount": discount, "signerSignature": resolved.SignerSignature, "protocolDeadline": strconv.FormatInt(resolved.ProtocolDeadline, 10)}))
}

func signOuterFillOrder(t *testing.T, key *ecdsa.PrivateKey, reactor common.Address, order *executor.IReactorOrder) []byte {
	t.Helper()
	outputs := make([]any, len(order.Outputs))
	for i, output := range order.Outputs {
		outputs[i] = apitypes.TypedDataMessage{"token": output.Token.Hex(), "amount": output.Amount.String(), "recipient": output.Recipient.Hex()}
	}
	types := apitypes.Types{"Output": {{Name: "token", Type: "address"}, {Name: "amount", Type: "uint256"}, {Name: "recipient", Type: "address"}}, "Request": {{Name: "tokenIn", Type: "address"}, {Name: "amountIn", Type: "uint256"}, {Name: "outputs", Type: "Output[]"}, {Name: "deadline", Type: "uint256"}, {Name: "nonce", Type: "uint256"}, {Name: "protocol", Type: "address"}}, "Order": {{Name: "request", Type: "Request"}, {Name: "swapperSignature", Type: "bytes"}, {Name: "swapper", Type: "address"}, {Name: "filler", Type: "address"}, {Name: "outputs", Type: "Output[]"}}}
	request := apitypes.TypedDataMessage{"tokenIn": order.Request.TokenIn.Hex(), "amountIn": order.Request.AmountIn.String(), "outputs": outputs, "deadline": order.Request.Deadline.String(), "nonce": order.Request.Nonce.String(), "protocol": order.Request.Protocol.Hex()}
	order.SwapperSignature = signOuterFillTyped(t, key, "Reactor", reactor, types, "Request", request)
	return signOuterFillTyped(t, key, "Reactor", reactor, types, "Order", apitypes.TypedDataMessage{"request": request, "swapperSignature": hexutil.Encode(order.SwapperSignature), "swapper": order.Swapper.Hex(), "filler": order.Filler.Hex(), "outputs": outputs})
}

func signOuterFillTyped(t *testing.T, key *ecdsa.PrivateKey, name string, address common.Address, types apitypes.Types, primary string, message apitypes.TypedDataMessage) []byte {
	t.Helper()
	types["EIP712Domain"] = []apitypes.Type{{Name: "name", Type: "string"}, {Name: "version", Type: "string"}, {Name: "chainId", Type: "uint256"}, {Name: "verifyingContract", Type: "address"}}
	digest, _, err := apitypes.TypedDataAndHash(apitypes.TypedData{Types: types, PrimaryType: primary, Domain: apitypes.TypedDataDomain{Name: name, Version: "1", ChainId: (*ethmath.HexOrDecimal256)(big.NewInt(1)), VerifyingContract: address.Hex()}, Message: message})
	if err != nil {
		t.Fatal(err)
	}
	signature, err := crypto.Sign(digest, key)
	if err != nil {
		t.Fatal(err)
	}
	signature[64] += 27
	return signature
}

func startOuterFillAnvil(t *testing.T) (*rpc.Client, *ethclient.Client) {
	t.Helper()
	binary, err := exec.LookPath("anvil")
	if err != nil {
		t.Fatal("anvil is required for the outer-fill integration test")
	}
	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), binary, "--silent", "--chain-id", "1", "--hardfork", "osaka", "--port", strconv.Itoa(port))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	})
	endpoint := "http://127.0.0.1:" + strconv.Itoa(port)
	for range 100 {
		client, err := rpc.DialOptions(t.Context(), endpoint, rpc.WithHTTPClient(&http.Client{Transport: ordinaryFillRPC{http.DefaultTransport}}))
		if err == nil {
			var chainID string
			if err := client.CallContext(t.Context(), &chainID, "eth_chainId"); err == nil {
				ethClient := ethclient.NewClient(client)
				t.Cleanup(ethClient.Close)
				return client, ethClient
			}
			client.Close()
		}
		select {
		case err := <-done:
			t.Fatalf("anvil exited: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("anvil did not become ready")
	return nil, nil
}

func (r *outerFillEVMReader) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	if query.FromBlock == nil || query.ToBlock == nil || query.FromBlock.Cmp(query.ToBlock) != 0 || len(query.Addresses) != 1 {
		return nil, errors.New("recipe publication requires one known block and executor")
	}
	return r.client.FilterLogs(ctx, query)
}

// The canonical fixture permits only ordinary eth_call, so any state override fails the integration.
type ordinaryFillRPC struct{ http.RoundTripper }

func (r ordinaryFillRPC) RoundTrip(request *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	if err := request.Body.Close(); err != nil {
		return nil, err
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	var call struct {
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(body, &call); err != nil {
		return nil, err
	}
	if call.Method == "eth_call" && len(call.Params) != 2 {
		return nil, errors.New("outer fill fixture rejects code or state override RPC")
	}
	return r.RoundTripper.RoundTrip(request)
}

type capturedExecutor struct {
	Proxy              common.Address `json:"Proxy"`
	Implementation     common.Address `json:"Implementation"`
	Reactor            common.Address `json:"Reactor"`
	ProxyCode          hexutil.Bytes  `json:"ProxyCode"`
	ImplementationCode hexutil.Bytes  `json:"ImplementationCode"`
}

type splitPermissions map[common.Address][]byte

func (p splitPermissions) PermissionTree(_ context.Context, _ *big.Int, snapshot recipes.Snapshot) ([]byte, error) {
	return bytes.Clone(p[snapshot.Account]), nil
}

func TestAnvilSplitConnectorOrder(t *testing.T) {
	var deployed *capturedExecutor
	if path := os.Getenv("RFQ_OUTER_FILL_DEPLOYMENT"); path != "" {
		data, err := os.ReadFile(path) //nolint:gosec // The integration test runner chooses its local runtime fixture.
		if err != nil {
			t.Fatal(err)
		}
		deployed = new(capturedExecutor)
		if err := json.Unmarshal(data, deployed); err != nil {
			t.Fatal(err)
		}
		if deployed.Proxy == (common.Address{}) || deployed.Implementation == (common.Address{}) || deployed.Reactor == (common.Address{}) ||
			len(deployed.ProxyCode) == 0 || len(deployed.ImplementationCode) == 0 {
			t.Fatal("incomplete configured Executor runtime")
		}
		t.Logf("Configured Executor %s, implementation %s, Reactor %s", deployed.Proxy, deployed.Implementation, deployed.Reactor)
	}
	for _, name := range []string{"settlement", "second grant revoked"} {
		t.Run(name, func(t *testing.T) {
			f := newOuterFillEVM(t, deployed)
			secondAccount, secondVenue := common.HexToAddress("0x22"), common.HexToAddress("0x44")
			secondID := common.HexToHash("0x44")
			second := f.addConnector(secondAccount, secondVenue, 35, secondID)
			f.workflow.Source = splitPermissions{f.account: []byte(f.workflow.Source.(outerFillPermissions)), secondAccount: second.Permissions}
			f.backend.resolved = map[string]*discounts.Resolved{f.selected.DiscountID.Hex(): f.backend.discount, secondID.Hex(): second.Discount}
			managers := make([]common.Address, 0, 2)
			for _, recipe := range []common.Address{f.selected.RecipeExecutor, second.Executor} {
				manager := f.deploy("AccessManager", f.signer)
				f.send(f.signer, recipe, "RecipeExecutor", "setCaller", manager)
				selector := [4]byte(recipeBinding.GetABI().Methods["execute"].ID)
				f.send(f.signer, manager, "AccessManager", "setTargetFunctionRole", recipe, [][4]byte{selector}, uint64(2))
				f.send(f.signer, manager, "AccessManager", "grantRole", uint64(2), f.workflow.Filler, uint32(0))
				managers = append(managers, manager)
			}
			f.order.Outputs[0].Amount = big.NewInt(89)
			f.protocolSignature = signOuterFillOrder(t, f.key, f.reactor, &f.order)
			row := awardRow(t, f.order)
			signature := hexutil.Encode(f.protocolSignature)
			row.ProtocolSignature = &signature
			listed := []discounts.ListItem{
				{DiscountID: f.selected.DiscountID.Hex(), Adapter: vlt.Hex(), MaxAssets: "54"},
				{DiscountID: secondID.Hex(), Adapter: secondVenue.Hex(), MaxAssets: "35"},
			}
			for i := range listed {
				listed[i].DiscountPrecision, listed[i].Discount = "100000000", "10000000"
				listed[i].TokenToRedeem, listed[i].Collateral, listed[i].CollateralDecimals = tIn.Hex(), tOut.Hex(), 6
				listed[i].Deadline = f.backend.discount.Discount.Deadline
			}
			solver := &Solver{
				Engine: &filler.Engine{
					Routes:   connector.NewRoutes(f.workflow, []common.Address{f.selected.RecipeExecutor, second.Executor}),
					Workflow: f.workflow, Spends: connector.NewSpendLedger(),
					// The split prices each connector from the tokens' on-chain decimals.
					Decimals: func(ctx context.Context, token common.Address) (int, error) {
						data, err := f.abis["FixtureToken"].Pack("decimals")
						if err != nil {
							return 0, err
						}
						raw, err := f.client.CallContract(ctx, ethereum.CallMsg{To: &token, Data: data}, nil)
						if err != nil {
							return 0, err
						}
						return int(new(big.Int).SetBytes(raw).Int64()), nil
					},
				},
				backend: f.backend,
			}
			if err := solver.Initialize(t.Context()); err != nil {
				t.Fatal(err)
			}
			singleRoute, err := tokenpolicy.New(tokenpolicy.Permissioned, []common.Address{tIn})
			if err != nil {
				t.Fatal(err)
			}
			solver.tokenPolicy = singleRoute
			if intent, err := solver.prepare(t.Context(), row, listed); intent != nil || err != nil {
				t.Fatalf("token policy should decline a split-only award: intent=%v err=%v", intent, err)
			}
			solver.tokenPolicy = tokenpolicy.Policy{}
			intent, err := solver.prepare(t.Context(), row, listed)
			if err != nil || intent == nil || f.reader.outerCalls != 1 {
				t.Fatalf("split preparation did not simulate the full signed award once: simulations=%d err=%v", f.reader.outerCalls, err)
			}
			for i, account := range []common.Address{f.account, secondAccount} {
				amount := intent.Spend.Amounts[liquidlane.NewCapacityID(1, account, tOut)]
				if amount == nil || amount.Int64() != []int64{54, 35}[i] {
					t.Fatalf("split funding amount for %s: %v", account, amount)
				}
			}
			if f.balance(tIn, f.signer).Int64() != 100 || f.balance(tOut, f.recipient).Sign() != 0 {
				t.Fatal("split simulation persisted token transfers")
			}
			if name == "second grant revoked" {
				f.send(f.signer, managers[1], "AccessManager", "revokeRole", uint64(2), f.workflow.Filler)
				f.transact(f.signer, &f.workflow.Filler, intent.Data, types.ReceiptStatusFailed)
				if f.balance(tIn, f.signer).Int64() != 100 || f.balance(tOut, f.account).Int64() != 90 ||
					f.balance(tIn, f.account).Sign() != 0 || f.balance(tOut, secondAccount).Int64() != 35 ||
					f.balance(tOut, f.recipient).Sign() != 0 {
					t.Fatal("failure of the second leg left partial settlement")
				}
				return
			}
			receipt := f.transact(f.signer, &f.workflow.Filler, intent.Data)
			if f.balance(tIn, f.account).Int64() != 61 || f.balance(tIn, secondAccount).Int64() != 39 ||
				f.balance(tOut, f.account).Int64() != 36 || f.balance(tOut, secondAccount).Sign() != 0 ||
				f.balance(tOut, f.recipient).Int64() != 89 || f.balance(tIn, f.signer).Sign() != 0 {
				t.Fatal("split fill did not atomically settle the signed input and output")
			}
			t.Logf("split fill used %d gas", receipt.GasUsed)
		})
	}
}
