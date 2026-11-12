package rfq

import (
	"bytes"
	"context"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/merkleaccount"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	merkletransaction "github.com/symbioticfi/merkle-executor/sdk/transaction"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/solver"
	merklerfq "github.com/symbioticfi/vault-solver/internal/solvers/merkle/rfq"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
	"gopkg.in/yaml.v3"
)

// Exercise factory route publication, account authentication and quote allocation.
func TestFactoryQuotesAuthenticateMerkleRouteWhileTransactionPending(t *testing.T) {
	rpcServer := rpc.NewServer()
	if err := rpcServer.RegisterName("eth", newMerkleQuoteRPC(t)); err != nil {
		t.Fatal(err)
	}
	defer rpcServer.Stop()
	httpServer := httptest.NewServer(rpcServer)
	defer httpServer.Close()
	client, err := chain.Dial(t.Context(), []string{httpServer.URL}, "", common.HexToAddress("0x66").Hex(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	signing, err := signer.NewFromHexKey(strings.Repeat("1", 64))
	if err != nil {
		t.Fatal(err)
	}
	manager := txmanager.New(client, signing, big.NewInt(1), txmanager.Config{}, logr.Discard())
	if err := manager.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	curator := httptest.NewServer(http.HandlerFunc(serveMerkleQuoteTree))
	defer curator.Close()
	t.Setenv("RFQ_BACKEND_SHARED_SECRET", "test-secret")
	text := minimalConfig + "recipeExecutors: ['0x0000000000000000000000000000000000000043']\ncurator: {url: " + curator.URL + "}\n"
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(text), &node); err != nil {
		t.Fatal(err)
	}
	merkleManager, err := merkletransaction.New(pendingQuoteChain{client}, signing, big.NewInt(1), merkletransaction.Config{MaxFeeGwei: 100, ShutdownTimeout: time.Millisecond}, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if err := merkleManager.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	built, err := factory(*node.Content[0], solver.Deps{Chain: client, Signer: signing, TxManager: manager, MerkleTxManager: func() (*merkletransaction.Manager, error) { return merkleManager, nil }, Log: logr.Discard()})
	if err != nil {
		t.Fatal(err)
	}
	s := built.(*Solver)
	s.server.quotes.reader = nil // All-discount quotes must use authenticated backend inventory.
	body := validQuoteBody()
	body.Amount = "456000000000000"
	id := "0x" + strings.Repeat("ab", 32)
	body.Adapters[0].DiscountID = &id
	body.Adapters[0].DiscountPrecision = "100000000"
	body.Adapters[0].RecipeExecutor = common.HexToAddress("0x43").Hex()
	// A shared backend response also lists native adapters for other solvers.
	body.Adapters = append(body.Adapters, quoteAdapter{Adapter: common.HexToAddress("0xbad").Hex(), Asset: tOut.Hex(), BlockNumber: "1", MaxAssets: "1000", Discount: "0"})
	// Quotes decline until the verified routes are published.
	if decision, err := s.server.quotes.quote(t.Context(), &body); err != nil || decision.response != nil {
		t.Fatalf("quote before route publication = %+v %v", decision, err)
	}
	if err := s.merkle.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); merkleManager.Start(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	sender := merkleManager.Sender("rfq-merkle")
	if err := sender.Submit(t.Context(), merkletransaction.ExecutionRequest{To: common.HexToAddress("0x10")}); err != nil {
		t.Fatal(err)
	}
	if !sender.Busy() || merkleManager.Available() {
		t.Fatal("fixture must retain an unresolved signed transaction")
	}
	for _, tc := range []struct {
		block, capacity string
		wrongAccount    bool
		quoted          bool
	}{
		{"7", "456", false, true},
		{"999", "456", false, true},
		{"7", "900", true, false},
		{"999", "900", true, false},
	} {
		body.Adapters[0].Account = vlt.Hex()
		if tc.wrongAccount {
			body.Adapters[0].Account = common.HexToAddress("0xbad").Hex()
		}
		body.Adapters[0].BlockNumber, body.Adapters[0].MaxAssets = tc.block, tc.capacity
		decision, err := s.server.quotes.quote(t.Context(), &body)
		if err != nil || (decision.response != nil) != tc.quoted {
			t.Fatalf("block=%s quote=%+v err=%v", tc.block, decision, err)
		}
		status := http.StatusNoContent
		if tc.quoted {
			status = http.StatusOK
		}
		if response := do(t, s.server.handler(), http.MethodPost, "/quote", "test-secret", body); response.Code != status {
			t.Fatalf("pending quote HTTP status = %d, want %d: %s", response.Code, status, response.Body.String())
		}
	}
}

// Only the submission RPCs are stubbed; route authentication still uses the real chain client.
type pendingQuoteChain struct{ *chain.Client }

func (pendingQuoteChain) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	return &types.Header{Number: big.NewInt(7), BaseFee: big.NewInt(1), GasLimit: 30_000_000}, nil
}
func (pendingQuoteChain) FeeHistory(context.Context, uint64, *big.Int, []float64) (*ethereum.FeeHistory, error) {
	return &ethereum.FeeHistory{Reward: [][]*big.Int{{big.NewInt(1)}}}, nil
}
func (pendingQuoteChain) EstimateGas(context.Context, ethereum.CallMsg) (uint64, error) {
	return 21_000, nil
}
func (pendingQuoteChain) TransactionSenderBalanceAt(context.Context, common.Address, *big.Int) (*big.Int, error) {
	return big.NewInt(1e18), nil
}
func (pendingQuoteChain) SendTransaction(context.Context, *types.Transaction) error { return nil }
func (pendingQuoteChain) TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error) {
	return nil, ethereum.NotFound
}

type merkleQuoteRPC struct {
	connectorCapacityRPC

	recipe recipes.Recipe
	hash   common.Hash
}

func newMerkleQuoteRPC(t *testing.T) *merkleQuoteRPC {
	t.Helper()
	uintType, err := abi.NewType("uint256", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	addressType, err := abi.NewType("address", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	hashType, err := abi.NewType("bytes32", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	input := new(big.Int).Or(new(big.Int).Lsh(new(big.Int).SetUint64(uint64(recipes.Input)), 216), big.NewInt(1))
	data, err := (abi.Arguments{{Type: uintType}, {Type: addressType}, {Type: addressType}, {Type: hashType}}).Pack(input, tOut, vlt, common.HexToHash("0x1234"))
	if err != nil {
		t.Fatal(err)
	}
	recipe := recipes.Recipe{Steps: [][]byte{append([]byte{recipes.Approve}, data...)}}
	hash, err := recipes.HashRecipe(recipe)
	if err != nil {
		t.Fatal(err)
	}
	return &merkleQuoteRPC{recipe: recipe, hash: hash}
}

func (s *merkleQuoteRPC) Call(_ context.Context, args map[string]any, block string, _ *map[string]any) (hexutil.Bytes, error) {
	raw, _ := args["input"].(string)
	if raw == "" {
		raw, _ = args["data"].(string)
	}
	data, err := hexutil.Decode(raw)
	if err != nil {
		return nil, err
	}
	return s.read(common.HexToAddress(args["to"].(string)), data, block)
}

func (s *merkleQuoteRPC) read(to common.Address, data []byte, block string) ([]byte, error) {
	if block != "0x7" {
		return nil, errors.New("identity read escaped pinned block")
	}
	if to == common.HexToAddress("0x43") {
		contractABI := recipeexecutor.NewRecipeExecutor().GetABI()
		method, err := contractABI.MethodById(data)
		if err != nil {
			return nil, err
		}
		switch method.RawName {
		case "account":
			return method.Outputs.Pack(vlt)
		case "caller":
			return method.Outputs.Pack(common.HexToAddress("0x10"))
		case "recipeHash":
			return method.Outputs.Pack(s.hash)
		case "recipeBlock":
			return method.Outputs.Pack(big.NewInt(7))
		}
	}
	if to == vlt {
		if bytes.Equal(data, merkleaccount.NewMerkleAccount().PackRoot()) {
			return common.HexToHash("0x1234").Bytes(), nil
		}
		if bytes.Equal(data, merkleaccount.NewMerkleAccount().PackAdapter()) {
			return common.LeftPadBytes(merkleQuoteAdapter.Bytes(), 32), nil
		}
		// Use generated connector selectors for the immutable identity.
		binding := connector.NewLiquidLaneConnector()
		if bytes.Equal(data, binding.PackAccount()) {
			return common.LeftPadBytes(vlt.Bytes(), 32), nil
		}
		if bytes.Equal(data, binding.PackOutputAsset()) {
			return common.LeftPadBytes(tOut.Bytes(), 32), nil
		}
	}
	contractABI := multicall3.NewMulticall3().GetABI()
	method, err := contractABI.MethodById(data)
	if err != nil {
		return nil, err
	}
	values, err := method.Inputs.Unpack(data[4:])
	if err != nil {
		return nil, err
	}
	calls := *abi.ConvertType(values[0], new([]multicall3.Multicall3Call3)).(*[]multicall3.Multicall3Call3)
	results := make([]multicall3.Multicall3Result, len(calls))
	for index, call := range calls {
		raw, err := s.read(call.Target, call.CallData, block)
		if err != nil {
			return nil, err
		}
		results[index] = multicall3.Multicall3Result{Success: true, ReturnData: raw}
	}
	return method.Outputs.Pack(results)
}

var merkleQuoteAdapter = common.HexToAddress("0xad")

// A one-leaf bundle whose leaf is the account root; the plain approval recipe has no managed calls.
func serveMerkleQuoteTree(w http.ResponseWriter, _ *http.Request) {
	scope := `"chainId":"1","adapter":"` + strings.ToLower(merkleQuoteAdapter.Hex()) + `","account":"` + strings.ToLower(vlt.Hex()) + `","root":"` + common.HexToHash("0x1234").Hex() + `"`
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"bundle":{` + scope + `,"leaves":["` + common.HexToHash("0x1234").Hex() + `"],"metadata":{` + scope + `,"leaves":[]}}}`))
}

func (s *merkleQuoteRPC) GetLogs(context.Context, map[string]any) ([]types.Log, error) {
	event := recipeexecutor.NewRecipeExecutor().GetABI().Events["SetRecipe"]
	data, err := event.Inputs.NonIndexed().Pack(s.recipe.Queries, s.recipe.Steps)
	if err != nil {
		return nil, err
	}
	header := &types.Header{Number: big.NewInt(7), Difficulty: big.NewInt(1)}
	return []types.Log{{Address: common.HexToAddress("0x43"), Topics: []common.Hash{event.ID, s.hash}, Data: data, BlockNumber: 7, BlockHash: header.Hash()}}, nil
}

func (*merkleQuoteRPC) GetTransactionCount(context.Context, common.Address, string) (hexutil.Uint64, error) {
	return 0, nil
}

func TestRFQTransactionManagerFollowsExecutionMode(t *testing.T) {
	native := &Solver{}
	if !native.RequiresTxManager() || native.RequiresMerkleTxManager() {
		t.Fatal("native RFQ must retain the existing transaction manager")
	}
	merkle := &Solver{merkle: &merklerfq.Solver{}}
	if merkle.RequiresTxManager() || !merkle.RequiresMerkleTxManager() {
		t.Fatal("Merkle RFQ must use only the SDK transaction manager")
	}
}

func TestNativeFactoryDoesNotConfigureMerkleTransactions(t *testing.T) {
	rpcServer := rpc.NewServer()
	if err := rpcServer.RegisterName("eth", newMerkleQuoteRPC(t)); err != nil {
		t.Fatal(err)
	}
	defer rpcServer.Stop()
	httpServer := httptest.NewServer(rpcServer)
	defer httpServer.Close()
	client, err := chain.Dial(t.Context(), []string{httpServer.URL}, "", common.HexToAddress("0x66").Hex(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	signing, err := signer.NewFromHexKey(strings.Repeat("1", 64))
	if err != nil {
		t.Fatal(err)
	}
	// The native engine accepts confirmation depths beyond the SDK's bound.
	manager := txmanager.New(client, signing, big.NewInt(1), txmanager.Config{Confirmations: 1025}, logr.Discard())
	t.Setenv("RFQ_BACKEND_SHARED_SECRET", "test-secret")
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(minimalConfig+oneAdapter), &node); err != nil {
		t.Fatal(err)
	}
	calls := 0
	built, err := factory(*node.Content[0], solver.Deps{
		Chain: client, Signer: signing, TxManager: manager, Log: logr.Discard(),
		MerkleTxManager: func() (*merkletransaction.Manager, error) {
			calls++
			return merkletransaction.New(client, signing, big.NewInt(1), merkletransaction.Config{MaxFeeGwei: 100, Confirmations: 1025}, logr.Discard())
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || !solver.RequiresTxManager(built) || solver.RequiresMerkleTxManager(built) {
		t.Fatal("native RFQ configured the Merkle transaction manager")
	}
}
