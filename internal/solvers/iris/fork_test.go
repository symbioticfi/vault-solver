//go:build integration

package iris

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"

	"github.com/symbioticfi/vault-solver/api/bindings/iris/blm"
)

// Mainnet addresses (iris-core deployments/1.json) and the fixture account the recipes in
// testdata/fork/recipes.json were built for.
var (
	forkIris      = common.HexToAddress("0x77310475712D7a127D694B3e9D186d9f4a423bd9")
	forkBlm       = common.HexToAddress("0xB0ffB27D9D7f160d588c669329F807e58aDd4eeF")
	forkWETH      = common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")
	forkUSDC      = common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	forkAccount   = common.HexToAddress("0x0000000000000000000000000000000000ACC001")
	forkAdmin     = forkAddress("iris-fork-admin")
	forkBorrower  = forkAddress("iris-fork-borrower")
	forkReceiver  = forkAddress("iris-fork-receiver")
	forkKeeper    = forkAddress("iris-fork-keeper")
	forkSignerKey = crypto.ToECDSAUnsafe(crypto.Keccak256([]byte("iris-fork-signer")))
	forkSigner    = crypto.PubkeyToAddress(forkSignerKey.PublicKey)
)

const (
	forkAuthHeader = "x-api-key"
	forkAuthValue  = "fork-webhook-key"
	forkRate       = 60_000_000_000_000_000 // defaultFixedRate in the test config
	forkBufferBps  = 1000
	usdcBalances   = 9 // FiatToken balanceAndBlacklistStates slot
	// Fork reads fetch state upstream, so slots leave the sender a few seconds like mainnet does.
	forkBlockTime = 4
)

func forkAddress(label string) common.Address {
	return common.BytesToAddress(crypto.Keccak256([]byte(label))[12:])
}

// TestForkIrisEndToEnd runs docs/IRIS-PLAN.md §8.1 in order. The test plays the IRIS
// coordinator (posting to the solver's endpoints and re-running its on-chain checks) and the
// borrower (taking, repaying); everything else is the real binary and the real Iris.
func TestForkIrisEndToEnd(t *testing.T) {
	endpoint := os.Getenv("ETH_RPC_URL_MAINNET")
	if endpoint == "" {
		t.Skip("set ETH_RPC_URL_MAINNET to fork mainnet")
	}
	f := newForkEVM(t, endpoint)
	f.setup()
	f.startSolver()

	var firm quoteResponse
	var firmRequest quoteRequest
	var pod common.Address
	// Steps share chain state; make test-iris-fork runs with -failfast.
	t.Run("2 startup discovers the market", func(t *testing.T) {
		f.waitLog(t, "iris markets discovered", 1)
		if !strings.Contains(f.logs.String(), `"markets": 1`) && !strings.Contains(f.logs.String(), `"markets":1`) {
			t.Fatalf("expected one market:\n%s", f.logs.String())
		}
	})
	t.Run("3 auth", func(t *testing.T) {
		for _, value := range []string{"", "wrong"} {
			status, _ := f.post(t, "/quote", f.request("auth"), value)
			if status != http.StatusUnauthorized {
				t.Fatalf("header %q: status %d", value, status)
			}
		}
	})
	t.Run("4 indicative quote", func(t *testing.T) {
		before := f.nonce(forkSigner)
		r := f.request("indicative")
		status, body := f.post(t, "/quote/indicative", r, forkAuthValue)
		if status != http.StatusOK {
			t.Fatalf("status %d: %s", status, body)
		}
		var q quoteResponse
		mustJSON(t, body, &q)
		if !q.Indicative || q.Signature != "" || q.Deadline != "" {
			t.Fatalf("indicative shape: %+v", q)
		}
		if want := f.expectedBond(r, q); q.Bond != want.String() {
			t.Fatalf("bond %s, want %s", q.Bond, want)
		}
		if f.nonce(forkSigner) != before {
			t.Fatal("indicative quote sent a transaction")
		}
	})
	t.Run("5 firm quote", func(t *testing.T) {
		before := f.nonce(forkSigner)
		firmRequest = f.request("firm")
		status, body := f.post(t, "/quote", firmRequest, forkAuthValue)
		received := time.Now().Unix()
		if status != http.StatusOK {
			t.Fatalf("status %d: %s\n%s", status, body, f.logs.String())
		}
		mustJSON(t, body, &firm)
		if firm.Signature != hexutil.Encode(make([]byte, placeholderSignatureBytes)) {
			t.Fatalf("signature %s", firm.Signature)
		}
		ahead := mustInt(t, firm.Deadline) - received
		if ahead < 114 || ahead > 121 {
			t.Fatalf("deadline %ds ahead of the response", ahead)
		}
		if f.nonce(forkSigner) != before+1 {
			t.Fatal("firm quote must send exactly one transaction")
		}
		if want := f.expectedBond(firmRequest, firm); firm.Bond != want.String() {
			t.Fatalf("bond %s, want %s", firm.Bond, want)
		}
	})
	t.Run("6 coordinator checks", func(t *testing.T) {
		quote := f.quote(t, firmRequest, firm)
		digest := f.digest(t, quote)
		raw := f.call(forkAccount, accountBinding.PackIsValidSignature(digest, make([]byte, placeholderSignatureBytes)))
		magic, err := accountBinding.UnpackIsValidSignature(raw)
		must(t, err)
		equal(t, erc1271Magic, magic)
		if f.balance(forkUSDC, forkAccount).Cmp(quote.Bond) < 0 || f.allowance(forkUSDC, forkAccount, forkIris).Cmp(quote.Bond) < 0 {
			t.Fatal("account cannot fund the bond")
		}
	})
	t.Run("7 borrower takes the quote", func(t *testing.T) {
		quote := f.quote(t, firmRequest, firm)
		accountBefore, receiverBefore := f.balance(forkUSDC, forkAccount), f.balance(forkUSDC, forkReceiver)
		receipt := f.send(forkBorrower, forkIris, f.pack("take", quote, make([]byte, placeholderSignatureBytes)))
		take := f.iris.Events["Take"].ID
		for _, l := range receipt.Logs {
			if l.Address == forkIris && len(l.Topics) == 3 && l.Topics[0] == take {
				pod = common.BytesToAddress(l.Topics[2].Bytes())
			}
		}
		if pod == (common.Address{}) {
			t.Fatal("no Take event")
		}
		equal(t, quote.Bond, f.position(pod).Bond)
		equal(t, new(big.Int).Sub(accountBefore, quote.Bond), f.balance(forkUSDC, forkAccount))
		equal(t, new(big.Int).Add(receiverBefore, quote.Debt), f.balance(forkUSDC, forkReceiver))
		used, err := f.iris.Unpack("isQuoteNonceUsed", f.call(forkIris, f.pack("isQuoteNonceUsed", forkAccount, quote.Nonce)))
		must(t, err)
		ok(t, used[0].(bool), "nonce not used")
	})
	t.Run("8 replay reverts", func(t *testing.T) {
		quote := f.quote(t, firmRequest, firm)
		_, err := f.takeCall(quote)
		f.expectRevert(t, err, "InvalidNonce()")
	})
	t.Run("9 recipe limits", func(t *testing.T) {
		for name, mutate := range map[string]func(*quoteRequest){
			"debt above maxDebt":         func(r *quoteRequest) { r.Debt = "60000000000" },
			"duration above maxDuration": func(r *quoteRequest) { r.Duration = strconv.Itoa(100 * 86400) },
		} {
			before := f.nonce(forkSigner)
			r := f.request(name)
			mutate(&r)
			f.expectDecline(t, r, "recipe_rejected")
			if f.nonce(forkSigner) != before {
				t.Fatalf("%s: sent a transaction", name)
			}
		}
	})
	t.Run("10 solver declines", func(t *testing.T) {
		r := f.request("rate")
		r.MaxFixedRate = "50000000000000000"
		f.expectDecline(t, r, "rate_above_max_fixed_rate")
		r = f.request("venue")
		r.VenueBitmap = "2"
		f.expectDecline(t, r, "no_market")
		funded := f.balance(forkUSDC, forkAccount)
		f.setUSDC(forkAccount, big.NewInt(1))
		f.expectDecline(t, f.request("unfunded"), "insufficient_bond_balance")
		f.setUSDC(forkAccount, funded)
	})
	var spare quoteResponse
	var spareRequest quoteRequest
	t.Run("11 one lane", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make([]struct {
			status int
			body   []byte
		}, 2)
		requests := []quoteRequest{f.request("lane-a"), f.request("lane-b")}
		for i := range results {
			wg.Go(func() { results[i].status, results[i].body = f.post(t, "/quote", requests[i], forkAuthValue) })
		}
		wg.Wait()
		signed := slices.IndexFunc(results, func(r struct {
			status int
			body   []byte
		}) bool {
			return r.status == http.StatusOK
		})
		if signed < 0 {
			t.Fatalf("no quote signed: %s / %s", results[0].body, results[1].body)
		}
		mustJSON(t, results[signed].body, &spare)
		spareRequest = requests[signed]
		other := results[1-signed]
		if other.status != http.StatusNotFound || !bytes.Contains(other.body, []byte("solver_busy")) {
			t.Fatalf("second request: %d %s", other.status, other.body)
		}
	})
	t.Run("12 recipe changes", func(t *testing.T) {
		discovered := strings.Count(f.logs.String(), "iris markets discovered")
		time.Sleep(3 * f.refresh)
		equal(t, discovered, strings.Count(f.logs.String(), "iris markets discovered"), "unchanged state rediscovered")

		f.send(forkAdmin, f.signing, executorBinding.PackSetRecipe(f.signingRecipe.Queries, f.signingRecipe.Steps))
		f.waitLog(t, "iris markets discovered", discovered+1)

		f.send(forkAdmin, f.signing, executorBinding.PackSetCaller(forkKeeper))
		f.waitLog(t, "iris market refresh failed", 1)
		f.expectDecline(t, f.request("caller moved"), "recipe_rejected")
		f.send(forkAdmin, f.signing, executorBinding.PackSetCaller(forkSigner))
		time.Sleep(2 * f.refresh)
		status, body := f.post(t, "/quote", f.request("caller restored"), forkAuthValue)
		if status != http.StatusOK {
			t.Fatalf("after restoring the caller: %d %s", status, body)
		}
	})
	// Time moves past wall clock from here on, so the solver's firm path is no longer used.
	t.Run("13 expired quote reverts", func(t *testing.T) {
		quote := f.quote(t, spareRequest, spare)
		_, err := f.takeCall(quote)
		must(t, err) // takeable until the deadline
		f.warp(mustInt(t, spare.Deadline) - int64(f.head().Time) + 1)
		_, err = f.takeCall(quote)
		f.expectRevert(t, err, "QuoteExpired()")
	})
	t.Run("14 settlement returns capital", func(t *testing.T) {
		quote := f.quote(t, firmRequest, firm)
		f.warp(quote.Duration.Int64())
		f.setUSDC(forkBorrower, big.NewInt(1_000_000e6))
		f.send(forkBorrower, forkUSDC, erc20Binding.PackApprove(forkIris, math.MaxBig256))
		f.send(forkBorrower, forkIris, f.pack("repay", pod))

		out, err := f.iris.Unpack("claimable", f.call(forkIris, f.pack("claimable", forkUSDC, forkAccount)))
		must(t, err)
		claimable, bond := out[0].(*big.Int), f.position(pod).Bond
		if bond.Sign() == 0 {
			t.Fatal("no bond left to withdraw")
		}
		prepared, err := recipes.DiscoverBatch(f.ctx, f.client, []common.Address{f.settlement}, []common.Address{forkAccount}, forkKeeper, recipes.PreparationOptions{ChainID: big.NewInt(1)})
		must(t, err)
		inputs := map[uint]string{0: claimable.String(), 1: "0", 2: pod.Big().String(), 3: bond.String()}
		data, _, err := recipes.Assemble(prepared[0].Recipe, f.tree, inputs, nil, nil)
		must(t, err)
		before := f.balance(forkUSDC, forkAccount)
		f.send(forkKeeper, f.settlement, data)
		equal(t, new(big.Int).Add(before, new(big.Int).Add(claimable, bond)), f.balance(forkUSDC, forkAccount))
		equal(t, new(big.Int), f.position(pod).Bond)
		out, err = f.iris.Unpack("claimable", f.call(forkIris, f.pack("claimable", forkUSDC, forkAccount)))
		must(t, err)
		equal(t, new(big.Int), out[0].(*big.Int))
		t.Logf("bond %s and Iris claim %s returned to the account", bond, claimable)
	})
}

type forkEVM struct {
	t                         *testing.T
	ctx                       context.Context
	rpc                       *rpc.Client
	client                    *ethclient.Client
	endpoint                  string
	artifacts                 map[string]forkArtifact
	abis                      map[string]abi.ABI
	iris                      abi.ABI
	fixture                   forkFixture
	signing, settlement       common.Address
	signingRecipe             recipes.Recipe
	tree                      *recipes.Tree
	bundle                    []byte
	server, metrics           string
	refresh                   time.Duration
	logs                      lockedBuffer
	collateral, debt, maxRate *big.Int
	requests                  int
}

type forkArtifact struct {
	ABI      json.RawMessage `json:"abi"`
	Bytecode string          `json:"bytecode"`
}

type forkLeaf struct {
	data, mask []byte
	hash       common.Hash
	target     common.Address
}

type forkFixture struct {
	Signing    map[string]string `json:"signing"`
	Settlement map[string]string `json:"settlement"`
}

func newForkEVM(t *testing.T, upstream string) *forkEVM {
	t.Helper()
	f := &forkEVM{t: t, ctx: t.Context(), abis: map[string]abi.ABI{}, refresh: 2 * time.Second,
		collateral: new(big.Int).Mul(big.NewInt(20), big.NewInt(1e18)), debt: big.NewInt(10_000e6), maxRate: big.NewInt(8e16)}
	port := freePort(t)
	args := []string{"--fork-url", upstream, "--block-time", strconv.Itoa(forkBlockTime), "--port", strconv.Itoa(port), "--silent"}
	if block := os.Getenv("IRIS_FORK_BLOCK"); block != "" {
		args = append(args, "--fork-block-number", block)
	}
	//nolint:gosec // G702: test-only; the fork URL and block come from the developer's environment.
	startProcess(t, exec.CommandContext(t.Context(), "anvil", args...), nil)
	f.endpoint = "http://127.0.0.1:" + strconv.Itoa(port)
	var err error
	for range 120 {
		if f.rpc, err = rpc.DialContext(f.ctx, f.endpoint); err == nil {
			f.client = ethclient.NewClient(f.rpc)
			if _, err = f.client.ChainID(f.ctx); err == nil {
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	must(t, err)

	var catalog struct {
		Contracts map[string]forkArtifact `json:"contracts"`
	}
	mustJSON(t, mustRead(t, "../merkle/rfq/testdata/outer-fill/artifacts.json"), &catalog)
	f.artifacts = catalog.Contracts
	for _, name := range []string{"MerkleAccount", "RecipeExecutor", "FixtureWorld"} {
		parsed, err := abi.JSON(bytes.NewReader(f.artifacts[name].ABI))
		must(t, err)
		f.abis[name] = parsed
	}
	f.iris, err = abi.JSON(strings.NewReader(irisABI))
	must(t, err)
	mustJSON(t, mustRead(t, "testdata/fork/recipes.json"), &f.fixture)
	return f
}

// setup is checklist item 1: the account, its executors, root, roles and funding.
func (f *forkEVM) setup() {
	t := f.t
	for _, who := range []common.Address{forkAdmin, forkBorrower, forkKeeper, forkSigner} {
		f.rpcCall("anvil_setBalance", who, "0x3635C9ADC5DEA00000") // 1000 ETH
		f.rpcCall("anvil_impersonateAccount", who)
	}
	world := f.deploy("FixtureWorld", forkUSDC)
	implementation := f.deploy("MerkleAccount", world, world, world, world)
	code, err := f.client.CodeAt(f.ctx, implementation, nil)
	must(t, err)
	f.rpcCall("anvil_setCode", forkAccount, hexutil.Encode(code))
	address, err := abi.NewType("address", "", nil)
	must(t, err)
	raw, err := abi.NewType("bytes", "", nil)
	must(t, err)
	admin, err := abi.Arguments{{Type: address}}.Pack(forkAdmin)
	must(t, err)
	init, err := abi.Arguments{{Type: address}, {Type: raw}}.Pack(world, admin)
	must(t, err)
	f.sendABI(forkAdmin, forkAccount, "MerkleAccount", "initialize", uint64(1), forkAdmin, init)
	f.signing = f.deploy("RecipeExecutor", forkAdmin, forkAccount)
	f.settlement = f.deploy("RecipeExecutor", forkAdmin, forkAccount)
	f.send(forkAdmin, f.signing, executorBinding.PackSetCaller(forkSigner))
	f.send(forkAdmin, f.settlement, executorBinding.PackSetCaller(forkKeeper))

	leaves := map[common.Address][]forkLeaf{
		f.signing: {f.leaf("sign", forkAccount), f.leaf("approval", forkUSDC)},
		f.settlement: {
			f.leaf("claimDebt", forkIris), f.leaf("claimCollateral", forkIris), f.leaf("withdrawBond", forkIris),
		},
	}
	var all []forkLeaf
	for _, group := range leaves {
		all = append(all, group...)
	}
	f.buildTree(all)
	adapter, err := accountBinding.UnpackAdapter(f.call(forkAccount, accountBinding.PackAdapter()))
	must(t, err)
	f.bundle = f.curatorBundle(adapter)
	f.sendABI(forkAdmin, world, "FixtureWorld", "setRoot", forkAccount, f.tree.Root)
	f.send(forkAdmin, forkAccount, accountBinding.PackAcceptRoot(f.tree.Root))
	for executor, group := range leaves {
		for _, l := range group {
			f.sendABI(forkAdmin, forkAccount, "MerkleAccount", "grantRole", l.hash, executor)
		}
	}
	f.signingRecipe = f.setRecipe(f.signing, f.fixture.Signing["recipe"])
	f.setRecipe(f.settlement, f.fixture.Settlement["recipe"])

	f.setUSDC(forkAccount, big.NewInt(100_000e6))
	f.send(forkBorrower, forkWETH, hexutil.MustDecode("0xd0e30db0"), new(big.Int).Mul(f.collateral, big.NewInt(2)))
	f.send(forkBorrower, forkWETH, erc20Binding.PackApprove(forkIris, math.MaxBig256))
}

func (f *forkEVM) leaf(name string, target common.Address) forkLeaf {
	l := forkLeaf{data: hexutil.MustDecode(f.fixtureValue(name + "Data")), mask: hexutil.MustDecode(f.fixtureValue(name + "Mask")), target: target}
	hash, err := recipes.HashLeaf(recipes.Leaf{Target: target, Value: new(big.Int), Data: l.data, DataMask: l.mask})
	must(f.t, err)
	equal(f.t, common.HexToHash(f.fixtureValue(name+"Leaf")), hash, name)
	l.hash = hash
	return l
}

func (f *forkEVM) fixtureValue(key string) string {
	if v, found := f.fixture.Signing[key]; found {
		return v
	}
	v, found := f.fixture.Settlement[key]
	ok(f.t, found, key)
	return v
}

// buildTree lays out the leaves like the curator and the SDK bundle check: sorted leaves at
// the end of the array, sorted-pair hashing, proofs from sibling nodes.
func (f *forkEVM) buildTree(leaves []forkLeaf) {
	slices.SortFunc(leaves, func(a, b forkLeaf) int { return a.hash.Cmp(b.hash) })
	nodes := make([]common.Hash, 2*len(leaves)-1)
	for i, l := range leaves {
		nodes[len(nodes)-1-i] = l.hash
	}
	for i := len(leaves) - 2; i >= 0; i-- {
		a, b := nodes[2*i+1], nodes[2*i+2]
		if a.Cmp(b) > 0 {
			a, b = b, a
		}
		nodes[i] = crypto.Keccak256Hash(a[:], b[:])
	}
	account := forkAccount
	f.tree = &recipes.Tree{Root: nodes[0], ChainID: big.NewInt(1), Account: &account}
	for i, l := range leaves {
		var proof []common.Hash
		for k := len(nodes) - 1 - i; k > 0; k = (k - 1) / 2 {
			sibling := k + 1
			if k%2 == 0 {
				sibling = k - 1
			}
			proof = append(proof, nodes[sibling])
		}
		f.tree.Leaves = append(f.tree.Leaves, recipes.Leaf{Target: l.target, Value: new(big.Int), Data: l.data, DataMask: l.mask, Hash: l.hash, Proof: proof})
	}
}

func (f *forkEVM) curatorBundle(adapter common.Address) []byte {
	calls := make([]map[string]any, 0, len(f.tree.Leaves))
	hashes := make([]common.Hash, 0, len(f.tree.Leaves))
	for _, l := range f.tree.Leaves {
		calls = append(calls, map[string]any{"target": l.Target, "value": "0", "data": hexutil.Encode(l.Data), "dataMask": hexutil.Encode(l.DataMask), "hash": l.Hash, "proof": l.Proof})
		hashes = append(hashes, l.Hash)
	}
	slices.SortFunc(hashes, func(a, b common.Hash) int { return a.Cmp(b) })
	scope := map[string]any{"chainId": 1, "adapter": adapter, "account": forkAccount, "root": f.tree.Root}
	metadata := map[string]any{"calls": calls}
	bundle := map[string]any{"leaves": hashes}
	for k, v := range scope {
		metadata[k], bundle[k] = v, v
	}
	bundle["metadata"] = metadata
	raw, err := json.Marshal(map[string]any{"bundle": bundle})
	must(f.t, err)
	return raw
}

func (f *forkEVM) setRecipe(executor common.Address, encoded string) recipes.Recipe {
	recipe, err := recipes.DecodeRecipe(hexutil.MustDecode(encoded))
	must(f.t, err)
	f.send(forkAdmin, executor, executorBinding.PackSetRecipe(recipe.Queries, recipe.Steps))
	return recipe
}

func (f *forkEVM) startSolver() {
	t := f.t
	curator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/"+f.tree.Root.Hex()) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(f.bundle)
	}))
	t.Cleanup(curator.Close)
	dir := t.TempDir()
	binary := filepath.Join(dir, "vault-solver")
	build := exec.CommandContext(f.ctx, "go", "build", "-o", binary, "./cmd/vault-solver")
	build.Dir = "../../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	f.server = "127.0.0.1:" + strconv.Itoa(freePort(t))
	f.metrics = "127.0.0.1:" + strconv.Itoa(freePort(t))
	config := `chain:
  rpcUrl: ` + f.endpoint + `
  chainId: 1
signer:
  keyEnv: IRIS_FORK_SIGNER_KEY
txManager:
  confirmations: 1
  maxFeeGwei: 1000
observability:
  addr: "` + f.metrics + `"
solvers:
  - name: iris
    config:
      iris: "` + forkIris.Hex() + `"
      account: "` + forkAccount.Hex() + `"
      recipeExecutors: ["` + f.signing.Hex() + `"]
      curator:
        url: ` + curator.URL + `
      refreshInterval: ` + f.refresh.String() + `
      server:
        listenAddress: "` + f.server + `"
        authHeader: ` + forkAuthHeader + `
        authValueEnv: IRIS_FORK_WEBHOOK_KEY
      pricing:
        defaultFixedRate: "` + strconv.Itoa(forkRate) + `"
        bondBufferBps: ` + strconv.Itoa(forkBufferBps) + `
        maxBond: "1000000000"
      timing:
        blockTime: ` + strconv.Itoa(forkBlockTime) + `s
        submitLead: 3s
        quoteTtl: 2m
        deadlineWindow: 5s
        responseBudget: 20s
        pollInterval: 200ms
`
	path := filepath.Join(dir, "iris.yaml")
	must(t, os.WriteFile(path, []byte(config), 0o600))
	env := []string{"IRIS_FORK_SIGNER_KEY=" + hexutil.Encode(crypto.FromECDSA(forkSignerKey))[2:], "IRIS_FORK_WEBHOOK_KEY=" + forkAuthValue}
	exited := startProcess(t, exec.CommandContext(f.ctx, binary, "run", "--config", path), &f.logs, env...)
	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			t.Fatalf("solver exited:\n%s", f.logs.String())
		default:
		}
		request, err := http.NewRequestWithContext(f.ctx, http.MethodGet, "http://"+f.server+"/healthz", nil)
		must(t, err)
		if response, err := http.DefaultClient.Do(request); err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusNoContent {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("solver never became healthy:\n%s", f.logs.String())
}

func (f *forkEVM) request(id string) quoteRequest {
	f.requests++
	return quoteRequest{
		RequestID: id, QuoteID: id + "-" + strconv.Itoa(f.requests), ChainID: 1,
		Borrower: forkBorrower.Hex(), Receiver: forkReceiver.Hex(),
		CollateralToken: forkWETH.Hex(), Collateral: f.collateral.String(),
		DebtToken: forkUSDC.Hex(), Debt: f.debt.String(), MaxFixedRate: f.maxRate.String(),
		Duration: strconv.Itoa(30 * 86400), VenueBitmap: "1",
	}
}

func (f *forkEVM) post(t *testing.T, path string, r quoteRequest, auth string) (int, []byte) {
	t.Helper()
	body, err := json.Marshal(r)
	must(t, err)
	request, err := http.NewRequestWithContext(f.ctx, http.MethodPost, "http://"+f.server+path, bytes.NewReader(body))
	must(t, err)
	if auth != "" {
		request.Header.Set(forkAuthHeader, auth)
	}
	response, err := http.DefaultClient.Do(request)
	must(t, err)
	defer response.Body.Close()
	out, err := io.ReadAll(response.Body)
	must(t, err)
	return response.StatusCode, out
}

func (f *forkEVM) expectDecline(t *testing.T, r quoteRequest, reason string) {
	t.Helper()
	status, body := f.post(t, "/quote", r, forkAuthValue)
	if status != http.StatusNotFound || !bytes.Contains(body, []byte(`"`+reason+`"`)) {
		t.Fatalf("%s: want %s, got %d %s", r.RequestID, reason, status, body)
	}
}

// quote rebuilds the signed Quote the way a borrower does: request terms plus solver terms.
func (f *forkEVM) quote(t *testing.T, r quoteRequest, q quoteResponse) blm.Quote {
	t.Helper()
	n := func(s string) *big.Int {
		if s == "" { // indicative quotes carry no deadline or nonce
			return new(big.Int)
		}
		v, parsed := new(big.Int).SetString(s, 10)
		ok(t, parsed, s)
		return v
	}
	return blm.Quote{
		Borrower: common.HexToAddress(r.Borrower), Solver: common.HexToAddress(q.Solver), Receiver: common.HexToAddress(r.Receiver),
		Blm: common.HexToAddress(q.Blm), CollateralToken: common.HexToAddress(r.CollateralToken), DebtToken: common.HexToAddress(r.DebtToken),
		Collateral: n(r.Collateral), Debt: n(r.Debt), FixedRate: n(q.FixedRate), Duration: n(r.Duration),
		OverdueRate: n(q.OverdueRate), OverduePeriod: n(q.OverduePeriod), Bond: n(q.Bond), BondLltv: n(q.BondLltv),
		VenueBitmap: n(r.VenueBitmap), VenueId: n(q.VenueID), Deadline: n(q.Deadline), Nonce: n(q.Nonce), Data: hexutil.MustDecode(q.Data),
	}
}

func (f *forkEVM) expectedBond(r quoteRequest, q quoteResponse) *big.Int {
	quote := f.quote(f.t, r, q)
	quote.Bond, quote.Deadline, quote.Nonce = new(big.Int), new(big.Int), new(big.Int)
	requirement, err := blmBinding.UnpackBondRequirement(f.call(forkBlm, blmBinding.PackBondRequirement(quote)))
	must(f.t, err)
	ok(f.t, requirement.Sign() > 0, "zero requirement")
	bond := new(big.Int).Mul(requirement, big.NewInt(bpsDenominator+forkBufferBps))
	return bond.Add(bond, big.NewInt(bpsDenominator-1)).Div(bond, big.NewInt(bpsDenominator))
}

// digest is computed independently of the solver: go-ethereum EIP-712 over the Iris domain,
// checked against the deployed DOMAIN_SEPARATOR.
func (f *forkEVM) digest(t *testing.T, q blm.Quote) [32]byte {
	t.Helper()
	fields := []apitypes.Type{
		{Name: "borrower", Type: "address"}, {Name: "solver", Type: "address"}, {Name: "receiver", Type: "address"},
		{Name: "blm", Type: "address"}, {Name: "collateralToken", Type: "address"}, {Name: "debtToken", Type: "address"},
		{Name: "collateral", Type: "uint256"}, {Name: "debt", Type: "uint256"}, {Name: "fixedRate", Type: "uint256"},
		{Name: "duration", Type: "uint256"}, {Name: "overdueRate", Type: "uint256"}, {Name: "overduePeriod", Type: "uint256"},
		{Name: "bond", Type: "uint256"}, {Name: "bondLltv", Type: "uint256"}, {Name: "venueBitmap", Type: "uint256"},
		{Name: "venueId", Type: "uint256"}, {Name: "deadline", Type: "uint256"}, {Name: "nonce", Type: "uint256"}, {Name: "data", Type: "bytes"},
	}
	d := func(v *big.Int) *math.HexOrDecimal256 { return (*math.HexOrDecimal256)(v) }
	typed := apitypes.TypedData{
		Types:       apitypes.Types{"EIP712Domain": {{Name: "chainId", Type: "uint256"}, {Name: "verifyingContract", Type: "address"}}, "Quote": fields},
		PrimaryType: "Quote",
		Domain:      apitypes.TypedDataDomain{ChainId: d(big.NewInt(1)), VerifyingContract: forkIris.Hex()},
		Message: apitypes.TypedDataMessage{
			"borrower": q.Borrower.Hex(), "solver": q.Solver.Hex(), "receiver": q.Receiver.Hex(), "blm": q.Blm.Hex(),
			"collateralToken": q.CollateralToken.Hex(), "debtToken": q.DebtToken.Hex(), "collateral": d(q.Collateral), "debt": d(q.Debt),
			"fixedRate": d(q.FixedRate), "duration": d(q.Duration), "overdueRate": d(q.OverdueRate), "overduePeriod": d(q.OverduePeriod),
			"bond": d(q.Bond), "bondLltv": d(q.BondLltv), "venueBitmap": d(q.VenueBitmap), "venueId": d(q.VenueId),
			"deadline": d(q.Deadline), "nonce": d(q.Nonce), "data": hexutil.Encode(q.Data),
		},
	}
	domain, err := typed.HashStruct("EIP712Domain", typed.Domain.Map())
	must(t, err)
	out, err := f.iris.Unpack("DOMAIN_SEPARATOR", f.call(forkIris, f.pack("DOMAIN_SEPARATOR")))
	must(t, err)
	equal(t, out[0].([32]byte), [32]byte(domain))
	digest, _, err := apitypes.TypedDataAndHash(typed)
	must(t, err)
	return [32]byte(digest)
}

type forkPosition struct {
	Collateral, Debt, Bond, BondRequirement, CollateralIndex, DebtIndex, FixedLeg, FloatingLeg, Surplus *big.Int
	LastUpdate                                                                                          uint32
	VenueId                                                                                             uint8
	Data                                                                                                []byte
}

func (f *forkEVM) position(pod common.Address) forkPosition {
	out, err := f.iris.Unpack("getPosition", f.call(forkIris, f.pack("getPosition", pod)))
	must(f.t, err)
	return *abi.ConvertType(out[0], new(forkPosition)).(*forkPosition)
}

func (f *forkEVM) takeCall(quote blm.Quote) ([]byte, error) {
	return f.client.CallContract(f.ctx, ethereum.CallMsg{From: forkBorrower, To: &forkIris, Data: f.pack("take", quote, make([]byte, placeholderSignatureBytes))}, nil)
}

// expectRevert requires the call to revert with exactly the named custom error.
func (f *forkEVM) expectRevert(t *testing.T, err error, signature string) {
	t.Helper()
	var data rpc.DataError
	if !errors.As(err, &data) {
		t.Fatalf("want revert %s, got %v", signature, err)
	}
	revert, _ := data.ErrorData().(string)
	if want := hexutil.Encode(crypto.Keccak256([]byte(signature))[:4]); !strings.HasPrefix(revert, want) {
		t.Fatalf("want revert %s (%s), got %s", signature, want, revert)
	}
}

func (f *forkEVM) pack(method string, args ...any) []byte {
	data, err := f.iris.Pack(method, args...)
	must(f.t, err)
	return data
}

func (f *forkEVM) call(to common.Address, data []byte) []byte {
	out, err := f.client.CallContract(f.ctx, ethereum.CallMsg{To: &to, Data: data}, nil)
	must(f.t, err)
	return out
}

func (f *forkEVM) balance(token, owner common.Address) *big.Int {
	v, err := erc20Binding.UnpackBalanceOf(f.call(token, erc20Binding.PackBalanceOf(owner)))
	must(f.t, err)
	return v
}

func (f *forkEVM) allowance(token, owner, spender common.Address) *big.Int {
	v, err := erc20Binding.UnpackAllowance(f.call(token, erc20Binding.PackAllowance(owner, spender)))
	must(f.t, err)
	return v
}

func (f *forkEVM) setUSDC(owner common.Address, amount *big.Int) {
	slot := crypto.Keccak256Hash(common.LeftPadBytes(owner.Bytes(), 32), common.LeftPadBytes(big.NewInt(usdcBalances).Bytes(), 32))
	f.rpcCall("anvil_setStorageAt", forkUSDC, slot, common.BigToHash(amount))
	equal(f.t, amount, f.balance(forkUSDC, owner), "USDC balance slot")
}

func (f *forkEVM) nonce(who common.Address) uint64 {
	n, err := f.client.NonceAt(f.ctx, who, nil)
	must(f.t, err)
	return n
}

func (f *forkEVM) head() *types.Header {
	h, err := f.client.HeaderByNumber(f.ctx, nil)
	must(f.t, err)
	return h
}

func (f *forkEVM) warp(seconds int64) {
	f.rpcCall("evm_increaseTime", seconds)
	f.rpcCall("evm_mine")
}

func (f *forkEVM) deploy(name string, args ...any) common.Address {
	constructor, err := f.abis[name].Pack("", args...)
	must(f.t, err)
	return f.transact(forkAdmin, nil, append(hexutil.MustDecode(f.artifacts[name].Bytecode), constructor...), nil).ContractAddress
}

func (f *forkEVM) sendABI(from, to common.Address, contract, method string, args ...any) {
	data, err := f.abis[contract].Pack(method, args...)
	must(f.t, err)
	f.send(from, to, data)
}

func (f *forkEVM) send(from, to common.Address, data []byte, value ...*big.Int) *types.Receipt {
	var v *big.Int
	if len(value) > 0 {
		v = value[0]
	}
	return f.transact(from, &to, data, v)
}

func (f *forkEVM) transact(from common.Address, to *common.Address, data []byte, value *big.Int) *types.Receipt {
	f.t.Helper()
	args := map[string]any{"from": from, "data": hexutil.Encode(data), "gas": "0x1c9c380"}
	if to != nil {
		args["to"] = *to
	}
	if value != nil {
		args["value"] = hexutil.EncodeBig(value)
	}
	var hash common.Hash
	must(f.t, f.rpc.CallContext(f.ctx, &hash, "eth_sendTransaction", args))
	for range 100 {
		receipt, err := f.client.TransactionReceipt(f.ctx, hash)
		if err == nil {
			if receipt.Status != types.ReceiptStatusSuccessful {
				_, reason := f.client.CallContract(f.ctx, ethereum.CallMsg{From: from, To: to, Data: data, Value: value}, new(big.Int).Sub(receipt.BlockNumber, big.NewInt(1)))
				f.t.Fatalf("transaction %s from %s reverted: %v", hash.Hex(), from.Hex(), reason)
			}
			return receipt
		}
		time.Sleep(200 * time.Millisecond)
	}
	f.t.Fatalf("transaction %s was not mined", hash.Hex())
	return nil
}

func (f *forkEVM) rpcCall(method string, args ...any) {
	f.t.Helper()
	must(f.t, f.rpc.CallContext(f.ctx, nil, method, args...))
}

func (f *forkEVM) waitLog(t *testing.T, message string, count int) {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if strings.Count(f.logs.String(), message) >= count {
			return
		}
	}
	t.Fatalf("%q never logged %d times:\n%s", message, count, f.logs.String())
}

// startProcess runs cmd until the test ends; the returned channel closes when it exits.
func startProcess(t *testing.T, cmd *exec.Cmd, logs io.Writer, env ...string) <-chan struct{} {
	t.Helper()
	cmd.Env = append(os.Environ(), env...)
	if logs != nil {
		cmd.Stdout, cmd.Stderr = logs, logs
	}
	must(t, cmd.Start())
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-exited:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
		}
	})
	return exited
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	must(t, err)
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	must(t, err)
	return raw
}

func mustJSON(t *testing.T, raw []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
}

func mustInt(t *testing.T, s string) int64 {
	t.Helper()
	v, err := strconv.ParseInt(s, 10, 64)
	must(t, err)
	return v
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// irisABI is the slice of iris-core src/interfaces/IIris.sol the borrower side needs.
const quoteTuple = `{"name":"quote","type":"tuple","components":[` +
	`{"name":"borrower","type":"address"},{"name":"solver","type":"address"},{"name":"receiver","type":"address"},` +
	`{"name":"blm","type":"address"},{"name":"collateralToken","type":"address"},{"name":"debtToken","type":"address"},` +
	`{"name":"collateral","type":"uint256"},{"name":"debt","type":"uint256"},{"name":"fixedRate","type":"uint256"},` +
	`{"name":"duration","type":"uint256"},{"name":"overdueRate","type":"uint256"},{"name":"overduePeriod","type":"uint256"},` +
	`{"name":"bond","type":"uint256"},{"name":"bondLltv","type":"uint256"},{"name":"venueBitmap","type":"uint256"},` +
	`{"name":"venueId","type":"uint256"},{"name":"deadline","type":"uint256"},{"name":"nonce","type":"uint256"},{"name":"data","type":"bytes"}]}`

var irisABI = `[
{"type":"function","name":"take","stateMutability":"nonpayable","inputs":[` + quoteTuple + `,{"name":"signature","type":"bytes"}],"outputs":[{"name":"","type":"address"}]},
{"type":"function","name":"repay","stateMutability":"nonpayable","inputs":[{"name":"pod","type":"address"}],"outputs":[{"name":"","type":"uint256"}]},
{"type":"function","name":"claimable","stateMutability":"view","inputs":[{"name":"token","type":"address"},{"name":"account","type":"address"}],"outputs":[{"name":"","type":"uint256"}]},
{"type":"function","name":"isQuoteNonceUsed","stateMutability":"view","inputs":[{"name":"solver","type":"address"},{"name":"nonce","type":"uint256"}],"outputs":[{"name":"","type":"bool"}]},
{"type":"function","name":"DOMAIN_SEPARATOR","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"bytes32"}]},
{"type":"function","name":"getPosition","stateMutability":"view","inputs":[{"name":"pod","type":"address"}],"outputs":[{"name":"","type":"tuple","components":[
{"name":"collateral","type":"uint128"},{"name":"debt","type":"uint128"},{"name":"bond","type":"uint128"},{"name":"bondRequirement","type":"uint128"},
{"name":"collateralIndex","type":"uint128"},{"name":"debtIndex","type":"uint128"},{"name":"fixedLeg","type":"uint128"},{"name":"floatingLeg","type":"uint128"},
{"name":"surplus","type":"uint128"},{"name":"lastUpdate","type":"uint32"},{"name":"venueId","type":"uint8"},{"name":"data","type":"bytes"}]}]},
{"type":"event","name":"Take","anonymous":false,"inputs":[{"name":"caller","type":"address","indexed":true},{"name":"pod","type":"address","indexed":true},` +
	strings.Replace(quoteTuple, `"type":"tuple"`, `"type":"tuple","indexed":false`, 1) +
	`,{"name":"collateralIndex","type":"uint256","indexed":false},{"name":"debtIndex","type":"uint256","indexed":false}]}
]`
