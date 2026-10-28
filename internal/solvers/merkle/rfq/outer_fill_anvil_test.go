//go:build integration

package rfq

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"math/big"
	"net"
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
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

// All five contracts on the settlement path execute their pinned canonical bytecode.
// Only ERC20s, factories/root/oracle registries and the price feed are fixture contracts.
func TestAnvilConnectorOuterFillCanonicalBoundaries(t *testing.T) {
	for _, name := range []string{"valid fill", "bad signed terms", "bad discount signature", "bad order signature", "insufficient funding at simulation", "unauthorized EOA"} {
		t.Run(name, func(t *testing.T) {
			f := newOuterFillEVM(t)
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
			if name == "bad order signature" {
				f.protocolSignature[0] ^= 1
			}
			if name == "insufficient funding at simulation" {
				// Backend capacity was advertised, then cash disappears before
				// latest-state settlement simulation. The outer transaction must still fail.
				f.reader.beforeOuter = func() { f.send(f.signer, tOut, "FixtureToken", "burn", f.account, big.NewInt(90)) }
			}
			if name == "unauthorized EOA" {
				f.workflow.signer = common.HexToAddress("0x8888")
			}
			data, _, err := f.workflow.build(t.Context(), f.order, f.protocolSignature, f.selected, f.backend)
			if f.reader.outerCalls != 1 {
				t.Fatalf("full settlement was not simulated: calls=%d err=%v", f.reader.outerCalls, err)
			}
			if name != "valid fill" {
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
				if name == "bad order signature" {
					f.assertRevert("Reactor", "InvalidProtocolSignature")
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
			if !bytes.Equal(data, f.reader.outerData) || f.balance(tOut, f.account).Int64() != 90 || f.balance(tIn, f.signer).Int64() != 100 {
				t.Fatal("simulation changed state or differed from submitted bytes")
			}
			receipt := f.transact(f.signer, &f.workflow.caller, data)
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
	key               *ecdsa.PrivateKey
	recipient         common.Address
	workflow          *connectorWorkflow
	reader            *outerFillEVMReader
	order             executor.IReactorOrder
	protocolSignature []byte
	selected          selectedRoute
	backend           *fakeBackend
}

type outerFillEVMReader struct {
	*ethclient.Client

	caller      common.Address
	beforeOuter func()
	outerCalls  int
	outerData   []byte
	revertData  []byte
}

func (r *outerFillEVMReader) CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	outer := msg.To != nil && *msg.To == r.caller && bytes.HasPrefix(msg.Data, executorBinding.GetABI().Methods["fill1"].ID)
	if outer {
		r.outerCalls++
		r.outerData = bytes.Clone(msg.Data)
		if r.beforeOuter != nil {
			r.beforeOuter()
			r.beforeOuter = nil
		}
	}
	data, err := r.Client.CallContract(ctx, msg, block)
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

type outerFillPermissions []byte

func (p outerFillPermissions) PermissionTree(context.Context, *big.Int, recipes.Snapshot) ([]byte, error) {
	return bytes.Clone(p), nil
}

func newOuterFillEVM(t *testing.T) *outerFillEVM {
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
	world := f.deploy("FixtureWorld", tOut)
	f.materialize("MerkleAccount", f.account, world, world, world, world)
	f.materialize("LiquidLaneConnector", vlt, world, world, world)
	reactor := f.deploy("Reactor", world)
	caller := common.HexToAddress("0x1000") // Outside the Osaka precompile address range.
	f.materialize("Executor", caller, reactor)
	f.send(f.signer, caller, "Executor", "initialize", f.signer, []common.Address{f.signer})
	addressArgs := abi.Arguments{{Type: connectorDeltaType(t, "address")}}
	admin, err := addressArgs.Pack(f.signer)
	if err != nil {
		t.Fatal(err)
	}
	accountInit, err := (abi.Arguments{{Type: connectorDeltaType(t, "address")}, {Type: connectorDeltaType(t, "bytes")}}).Pack(world, admin)
	if err != nil {
		t.Fatal(err)
	}
	f.send(f.signer, f.account, "MerkleAccount", "initialize", uint64(1), f.signer, accountInit)
	connectorInit, err := (abi.Arguments{{Type: connectorDeltaType(t, "address")}, {Type: connectorDeltaType(t, "address")}}).Pack(f.account, tOut)
	if err != nil {
		t.Fatal(err)
	}
	f.send(f.signer, vlt, "LiquidLaneConnector", "initialize", uint64(2), f.signer, connectorInit)
	f.rpcCall("anvil_setBalance", f.account, "0x10000000000000000000")
	f.rpcCall("anvil_impersonateAccount", f.account)
	f.send(f.signer, world, "FixtureWorld", "setOracle", vlt, tIn)
	f.send(f.account, vlt, "LiquidLaneConnector", "addTokenToRedeem", tIn, world)
	f.send(f.account, vlt, "LiquidLaneConnector", "setMaxAssetsPerDuration", tIn, big.NewInt(1000), big.NewInt(86400))
	f.send(f.account, vlt, "LiquidLaneConnector", "setSigner", f.signer)
	f.send(f.account, vlt, "LiquidLaneConnector", "setMinDiscount", tIn, big.NewInt(10000000))
	recipeExecutor := f.deploy("RecipeExecutor", f.signer, f.account)
	f.send(f.signer, recipeExecutor, "RecipeExecutor", "setCaller", caller)
	f.send(f.signer, tOut, "FixtureToken", "mint", f.account, big.NewInt(90))
	f.send(f.signer, tIn, "FixtureToken", "mint", f.signer, big.NewInt(100))
	f.send(f.signer, tIn, "FixtureToken", "approve", reactor, big.NewInt(100))
	f.send(f.account, tOut, "FixtureToken", "approve", vlt, big.NewInt(90))
	resolved := connectorDiscountFixture()
	resolved.Discount.Discount, resolved.Discount.Signer, resolved.Discount.Protocol = "10000000", f.signer.Hex(), f.signer.Hex()
	signOuterFillDiscount(t, key, resolved)
	terms, err := parseConnectorDiscount(resolved, *connectorDiscountID(), vlt, tIn)
	if err != nil {
		t.Fatal(err)
	}
	data, mask := connectorSignatureTemplate(terms, caller, new(big.Int))
	for _, offset := range []int{100, 164} { // Runtime awarded amount and signed discount ratio.
		for i := offset; i < offset+32; i++ {
			mask[i] = 255
		}
	}
	leaf := recipes.Leaf{Target: vlt, Value: new(big.Int), Data: data, DataMask: mask}
	leaf.Hash, err = recipes.HashLeaf(leaf)
	if err != nil {
		t.Fatal(err)
	}
	recipe, tree, permissions := connectorCommittedFunding(t, leaf, f.account, caller, tOut, tIn)
	f.send(f.signer, world, "FixtureWorld", "setRoot", f.account, tree.Root)
	f.send(f.signer, f.account, "MerkleAccount", "acceptRoot", tree.Root)
	for _, permission := range tree.Leaves {
		f.send(f.signer, f.account, "MerkleAccount", "grantRole", permission.Hash, recipeExecutor)
	}
	f.send(f.signer, recipeExecutor, "RecipeExecutor", "setRecipe", recipe.Queries, recipe.Steps)

	f.reader = &outerFillEVMReader{Client: client, caller: caller}
	f.workflow = &connectorWorkflow{chain: f.reader, chainID: big.NewInt(1), connectors: map[common.Address]bool{vlt: true}, source: outerFillPermissions(permissions), caller: caller, signer: f.signer}
	f.order, f.selected, f.backend = connectorExecutionAward(caller)
	f.selected.Executor = recipeExecutor
	f.order.Request.AmountIn = big.NewInt(100)
	f.order.Request.Protocol, f.order.Swapper = f.signer, f.signer
	f.order.Outputs[0].Amount, f.order.Outputs[0].Recipient = big.NewInt(90), f.recipient
	f.backend.discount = resolved
	f.protocolSignature = signOuterFillOrder(t, key, reactor, &f.order)
	return f
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

func (f *outerFillEVM) transact(from common.Address, to *common.Address, data []byte) *types.Receipt {
	f.t.Helper()
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
			if receipt.Status != types.ReceiptStatusSuccessful {
				f.t.Fatalf("fixture transaction reverted: hash=%s to=%v data=%x receipt=%+v", hash, to, data[:min(len(data), 4)], receipt)
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
	resolved.SignerSignature = hexutil.Encode(signOuterFillTyped(t, key, "LiquidLaneConnector", vlt, types, "Discount", discount))
	resolved.ProtocolSignature = hexutil.Encode(signOuterFillTyped(t, key, "LiquidLaneConnector", vlt, types, "DiscountSwap", apitypes.TypedDataMessage{"discount": discount, "signerSignature": resolved.SignerSignature, "protocolDeadline": strconv.FormatInt(resolved.ProtocolDeadline, 10)}))
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
		client, err := rpc.DialContext(t.Context(), endpoint)
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
