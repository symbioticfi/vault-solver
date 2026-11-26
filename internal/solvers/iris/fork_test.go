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
	"github.com/symbioticfi/merkle-executor/sdk/bindings/merkleaccount"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"

	"github.com/symbioticfi/vault-solver/api/bindings/iris/blm"
)

// Mainnet addresses (iris-core deployments/1.json, Aave Core, Morpho Blue). The account is the
// one testdata/ui/recipes.json was compiled for.
var (
	forkIris     = fixtureIris
	forkBlm      = common.HexToAddress("0xB0ffB27D9D7f160d588c669329F807e58aDd4eeF")
	forkWstETH   = fixtureWstETH
	forkUSDC     = fixtureUSDC
	forkAavePool = common.HexToAddress("0x87870Bca3F3fD6335C3F4ce8392D69350B4fA4E2")
	forkAUSDC    = common.HexToAddress("0x98C23E9d8f34FEFb1B7BD6a91B7FF122F4e16F5c")
	forkMorpho   = common.HexToAddress("0xBBBBBbbBBb9cC5e90e3b3Af64bdAF62C37EEFFCb")
	// forkMorphoMarket is the Morpho wstETH/USDC market id, the payload hash of the second offer.
	forkMorphoMarket = common.HexToHash("0xb323495f7e4148be5643a4ea4a8221eef163e4bccfdedc2a6f4696baacbc86cc")
	forkAccount      = fixtureAccount
	forkAdmin        = forkAddress("iris-fork-admin")
	forkBorrower     = forkAddress("iris-fork-borrower")
	forkReceiver     = forkAddress("iris-fork-receiver")
	forkKeeper       = forkAddress("iris-fork-keeper")
	forkSignerKey    = crypto.ToECDSAUnsafe(crypto.Keccak256([]byte("iris-fork-signer")))
	forkSigner       = crypto.PubkeyToAddress(forkSignerKey.PublicKey)
)

const (
	forkAuthHeader = "x-api-key"
	forkAuthValue  = "fork-webhook-key"
	forkRate       = 60_000_000_000_000_000 // defaultFixedRate in the test config
	usdcBalances   = 9                      // FiatToken balanceAndBlacklistStates slot
	wstETHBalances = 0                      // OpenZeppelin ERC20 _balances slot
	keeperRole     = 2                      // AccessManager role allowed to call execute
	// Fork reads fetch state upstream, so slots leave the sender a few seconds like mainnet does.
	forkBlockTime = 4
)

func forkAddress(label string) common.Address {
	return common.BytesToAddress(crypto.Keccak256([]byte(label))[12:])
}

// TestForkIrisEndToEnd runs docs/IRIS-PLAN.md §8.1 in order against the curator UI's own
// permissions and recipes. The test plays the IRIS coordinator (posting to the solver's
// endpoints and re-running its on-chain checks), the curator (allowance, claims) and the
// borrower (taking, repaying); everything else is the real binary, contracts and Iris.
func TestForkIrisEndToEnd(t *testing.T) {
	endpoint := os.Getenv("ETH_RPC_URL_MAINNET")
	if endpoint == "" {
		t.Skip("set ETH_RPC_URL_MAINNET to fork mainnet")
	}
	f := newForkEVM(t, endpoint)
	f.setup()
	// Paused at startup: discovery must keep the other markets.
	f.pause(f.morpho)
	f.startSolver()

	var firm, morphoFirm quoteResponse
	var firmRequest, morphoRequest quoteRequest
	var pod, morphoPod common.Address
	// Steps share chain state; make test-iris-fork runs with -failfast.
	t.Run("2 startup skips the paused and the claim executors", func(t *testing.T) {
		f.waitDiscoveries(t, 1)
		f.expectMarkets(t, 1)
		logs := f.logs.String()
		if !strings.Contains(logs, "iris executor paused") || !strings.Contains(logs, f.morpho.Hex()) {
			t.Fatalf("morpho executor not reported paused:\n%s", logs)
		}
		if !strings.Contains(logs, "iris executor skipped") || !strings.Contains(logs, f.claim.Hex()) {
			t.Fatalf("claim executor not skipped:\n%s", logs)
		}
		f.send(forkAdmin, f.morpho, executorBinding.PackSetCaller(f.manager))
		f.waitDiscoveries(t, 2)
		f.expectMarkets(t, 2)
	})
	t.Run("3 auth", func(t *testing.T) {
		for _, value := range []string{"", "wrong"} {
			status, _ := f.post(t, "/quote", f.request("auth"), value)
			if status != http.StatusUnauthorized {
				t.Fatalf("header %q: status %d", value, status)
			}
		}
	})
	t.Run("4 indicative quote picks the market the request fits", func(t *testing.T) {
		before := f.nonce(forkSigner)
		r := f.request("indicative") // 10k USDC for 30 days: above the Morpho market's caps
		status, body := f.post(t, "/quote/indicative", r, forkAuthValue)
		if status != http.StatusOK {
			t.Fatalf("status %d: %s", status, body)
		}
		var q quoteResponse
		mustJSON(t, body, &q)
		if !q.Indicative || q.Signature != "" || q.Deadline != "" || q.VenueID != "0" || q.Data != "0x" {
			t.Fatalf("indicative shape: %+v", q)
		}
		if want := f.expectedBond(r, q); q.Bond != want.String() {
			t.Fatalf("bond %s, want %s", q.Bond, want)
		}
		if f.nonce(forkSigner) != before {
			t.Fatal("indicative quote sent a transaction")
		}

		// Without cash or an Aave position the recipe cannot fund the bond.
		f.rpcCall("anvil_impersonateAccount", forkAccount)
		f.rpcCall("anvil_setBalance", forkAccount, "0x3635C9ADC5DEA00000")
		f.send(forkAccount, forkAUSDC, erc20Binding.PackTransfer(forkAdmin, f.balance(forkAUSDC, forkAccount)))
		f.expectDeclineAt(t, "/quote/indicative", f.request("unfunded"), "recipe_rejected")
		f.send(forkAdmin, forkAUSDC, erc20Binding.PackTransfer(forkAccount, f.balance(forkAUSDC, forkAdmin)))
		f.rpcCall("anvil_stopImpersonatingAccount", forkAccount)
	})
	t.Run("5 firm quote funds the bond from Aave", func(t *testing.T) {
		before, supplied := f.nonce(forkSigner), f.balance(forkAUSDC, forkAccount)
		equal(t, new(big.Int), f.balance(forkUSDC, forkAccount), "account starts without cash")
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
		if ahead := mustInt(t, firm.Deadline) - received; ahead < 109 || ahead > 121 {
			t.Fatalf("deadline %ds ahead of the response", ahead)
		}
		if f.nonce(forkSigner) != before+1 {
			t.Fatal("firm quote must send exactly one transaction")
		}
		bond := f.expectedBond(firmRequest, firm)
		equal(t, bond.String(), firm.Bond, "bond")
		equal(t, bond, f.balance(forkUSDC, forkAccount), "the recipe withdrew exactly the bond")
		// The aToken balance also accrues interest between the two reads.
		drawn := new(big.Int).Sub(supplied, f.balance(forkAUSDC, forkAccount))
		if drawn.Cmp(bond) > 0 || new(big.Int).Sub(bond, drawn).Cmp(big.NewInt(10_000)) > 0 {
			t.Fatalf("Aave supply fell by %s for a %s bond", drawn, bond)
		}
	})
	t.Run("6 coordinator checks", func(t *testing.T) {
		quote := f.quote(t, firmRequest, firm)
		raw := f.call(forkAccount, accountBinding.PackIsValidSignature(f.digest(t, quote), make([]byte, placeholderSignatureBytes)))
		magic, err := accountBinding.UnpackIsValidSignature(raw)
		must(t, err)
		equal(t, erc1271Magic, magic)
		if f.balance(forkUSDC, forkAccount).Cmp(quote.Bond) < 0 || f.allowance(forkUSDC, forkAccount, forkIris).Cmp(quote.Bond) < 0 {
			t.Fatal("account cannot fund the bond")
		}
	})
	t.Run("7 borrower takes the quote", func(t *testing.T) {
		pod = f.take(t, f.quote(t, firmRequest, firm))
	})
	t.Run("8 replay reverts", func(t *testing.T) {
		_, err := f.takeCall(f.quote(t, firmRequest, firm))
		f.expectRevert(t, err, "InvalidNonce()")
	})
	t.Run("9 per-market limits", func(t *testing.T) {
		for name, tc := range map[string]struct {
			mutate func(*quoteRequest)
			reason string
		}{
			"debt above both caps":      {func(r *quoteRequest) { r.Debt = "25000000000" }, "above_market_limits"},
			"debt above the Morpho cap": {func(r *quoteRequest) { r.VenueBitmap, r.Debt = "2", "6000000000" }, "above_market_limits"},
			"tenor above both caps":     {func(r *quoteRequest) { r.Duration = strconv.Itoa(60 * 86400) }, "above_market_limits"},
			"ceiling under the Morpho floor": {func(r *quoteRequest) {
				r.VenueBitmap, r.Debt, r.Duration, r.MaxFixedRate = "2", "4000000000", strconv.Itoa(7*86400), "62000000000000000"
			}, "rate_above_max_fixed_rate"},
		} {
			before := f.nonce(forkSigner)
			r := f.request(name)
			tc.mutate(&r)
			f.expectDecline(t, r, tc.reason)
			if f.nonce(forkSigner) != before {
				t.Fatalf("%s: sent a transaction", name)
			}
		}
	})
	t.Run("10 an open quote holds the cash the next one needs", func(t *testing.T) {
		morphoRequest = f.request("morpho")
		morphoRequest.VenueBitmap, morphoRequest.Debt, morphoRequest.Duration = "2", "4000000000", strconv.Itoa(7*86400)
		// The Aave quote is taken but stays reserved until its deadline; its bond left the account.
		f.expectDecline(t, morphoRequest, "insufficient_bond_balance")
		f.setUSDC(forkAccount, big.NewInt(1_000e6))
	})
	t.Run("11 Morpho market quote at the curator floor", func(t *testing.T) {
		status, body := f.post(t, "/quote", morphoRequest, forkAuthValue)
		if status != http.StatusOK {
			t.Fatalf("status %d: %s\n%s", status, body, f.logs.String())
		}
		mustJSON(t, body, &morphoFirm)
		equal(t, "1", morphoFirm.VenueID)
		equal(t, "65000000000000000", morphoFirm.FixedRate, "default rate raised to the 6.5% floor")
		equal(t, hexutil.Encode(f.morphoMarket()), morphoFirm.Data)
		equal(t, f.expectedBond(morphoRequest, morphoFirm).String(), morphoFirm.Bond)
		morphoPod = f.take(t, f.quote(t, morphoRequest, morphoFirm))
	})
	t.Run("12 solver declines", func(t *testing.T) {
		r := f.request("venue")
		r.VenueBitmap = "4"
		f.expectDecline(t, r, "no_market")
		f.approveIris(new(big.Int))
		f.expectDecline(t, f.request("no allowance"), "insufficient_bond_allowance")
		f.approveIris(big.NewInt(30_000e6))
	})
	var spare quoteResponse
	var spareRequest quoteRequest
	t.Run("13 one lane", func(t *testing.T) {
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
	t.Run("14 recipe and keeper changes", func(t *testing.T) {
		discovered := strings.Count(f.logs.String(), "iris markets discovered")
		time.Sleep(3 * f.refresh)
		equal(t, discovered, strings.Count(f.logs.String(), "iris markets discovered"), "unchanged state rediscovered")

		f.setRecipe(f.aave, f.fixture.Offers[0].Recipe)
		f.waitDiscoveries(t, discovered+1)

		// A caller the signer cannot route through leaves only that market out: the request
		// fits Aave alone, and the Morpho market's caps are below it.
		f.send(forkAdmin, f.aave, executorBinding.PackSetCaller(forkKeeper))
		f.waitDiscoveries(t, discovered+2)
		f.expectDecline(t, f.request("caller moved"), "above_market_limits")
		f.send(forkAdmin, f.aave, executorBinding.PackSetCaller(f.manager))
		f.waitDiscoveries(t, discovered+3)

		// A healthy market's change is picked up while another executor is paused.
		f.pause(f.morpho)
		f.waitDiscoveries(t, discovered+4)
		f.setRecipe(f.aave, f.fixture.Offers[0].Recipe)
		f.waitDiscoveries(t, discovered+5)
		f.send(forkAdmin, f.morpho, executorBinding.PackSetCaller(f.manager))
		f.waitDiscoveries(t, discovered+6)
		f.expectMarkets(t, 2)

		execute := [4]byte(executorBinding.GetABI().Methods["execute"].ID)
		f.sendABI(forkAdmin, f.manager, "AccessManager", "setTargetFunctionRole", f.aave, [][4]byte{execute}, uint64(85))
		f.setRecipe(f.aave, f.fixture.Offers[0].Recipe)
		f.waitDiscoveries(t, discovered+7)
		f.expectMarkets(t, 1)
		f.expectDecline(t, f.request("keeper cannot route at discovery"), "above_market_limits")
		f.sendABI(forkAdmin, f.manager, "AccessManager", "setTargetFunctionRole", f.aave, [][4]byte{execute}, uint64(keeperRole))
		f.waitDiscoveries(t, discovered+8)
		f.expectMarkets(t, 2)

		// A keeper role change leaves the recipe state alone: the quote path checks it.
		time.Sleep(2 * f.refresh)
		f.sendABI(forkAdmin, f.manager, "AccessManager", "revokeRole", uint64(keeperRole), forkSigner)
		f.expectDecline(t, f.request("signer revoked"), "recipe_rejected")
		f.sendABI(forkAdmin, f.manager, "AccessManager", "grantRole", uint64(keeperRole), forkSigner, uint32(0))
		time.Sleep(2 * f.refresh)
		status, body := f.post(t, "/quote", f.request("keeper restored"), forkAuthValue)
		if status != http.StatusOK {
			t.Fatalf("after restoring the keeper: %d %s", status, body)
		}
	})
	// Time moves past wall clock from here on, so the solver's firm path is no longer used.
	t.Run("15 expired quote reverts", func(t *testing.T) {
		quote := f.quote(t, spareRequest, spare)
		_, err := f.takeCall(quote)
		must(t, err) // takeable until the deadline
		f.warp(mustInt(t, spare.Deadline) - int64(f.head().Time) + 1)
		_, err = f.takeCall(quote)
		f.expectRevert(t, err, "QuoteExpired()")
	})
	t.Run("16 settlement returns the bond and the proceeds", func(t *testing.T) {
		quote := f.quote(t, firmRequest, firm)
		f.warp(quote.Duration.Int64())
		f.setUSDC(forkBorrower, big.NewInt(1_000_000e6))
		f.send(forkBorrower, forkUSDC, erc20Binding.PackApprove(forkIris, math.MaxBig256))
		f.send(forkBorrower, forkIris, f.pack("repay", pod))

		// The UI's claim recipe refuses a loan that is still open.
		_, err := f.client.CallContract(f.ctx, ethereum.CallMsg{From: forkKeeper, To: &f.manager, Data: f.claimCall(t, morphoPod)}, nil)
		f.expectRevert(t, err, "CheckFailed()")

		bond := f.position(pod).Bond
		if bond.Sign() == 0 {
			t.Fatal("no bond left to withdraw")
		}
		before := f.balance(forkUSDC, forkAccount)
		f.send(forkKeeper, f.manager, f.claimCall(t, pod))
		equal(t, new(big.Int).Add(before, bond), f.balance(forkUSDC, forkAccount), "bond claimed")
		equal(t, new(big.Int), f.position(pod).Bond)

		out, err := f.iris.Unpack("claimable", f.call(forkIris, f.pack("claimable", forkUSDC, forkAccount)))
		must(t, err)
		claimable := out[0].(*big.Int)
		ok(t, claimable.Sign() > 0, "no solver proceeds")
		before = f.balance(forkUSDC, forkAccount)
		claim := f.permission("claim(address,uint256,address,address)", forkIris)
		f.manage(claim, map[int]*big.Int{4: forkUSDC.Big(), 36: claimable})
		equal(t, new(big.Int).Add(before, claimable), f.balance(forkUSDC, forkAccount), "proceeds claimed")
		t.Logf("bond %s and proceeds %s returned to the account", bond, claimable)
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
	iris, pool                abi.ABI
	fixture                   uiFixture
	aave, morpho, claim       common.Address
	manager                   common.Address
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

func newForkEVM(t *testing.T, upstream string) *forkEVM {
	t.Helper()
	f := &forkEVM{t: t, ctx: t.Context(), abis: map[string]abi.ABI{}, refresh: 2 * time.Second,
		collateral: new(big.Int).Mul(big.NewInt(10), big.NewInt(1e18)), debt: big.NewInt(10_000e6), maxRate: big.NewInt(8e16)}
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
	var manager forkArtifact
	mustJSON(t, mustRead(t, "../merkle/rfq/testdata/outer-fill/access-manager.json"), &manager)
	f.artifacts["AccessManager"] = manager
	for _, name := range []string{"MerkleAccount", "RecipeExecutor", "FixtureWorld", "AccessManager"} {
		parsed, err := abi.JSON(bytes.NewReader(f.artifacts[name].ABI))
		must(t, err)
		f.abis[name] = parsed
	}
	f.iris, err = abi.JSON(strings.NewReader(irisABI))
	must(t, err)
	f.pool, err = abi.JSON(strings.NewReader(poolABI))
	must(t, err)
	f.fixture = loadFixture(t)
	return f
}

// setup is checklist item 1: the account with the UI's permission tree, one executor per
// market and the claim executor behind one AccessManager keeper, the curator's allowance and
// an Aave USDC position to fund bonds from.
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

	f.manager = f.deploy("AccessManager", forkAdmin)
	f.aave = f.deploy("RecipeExecutor", forkAdmin, forkAccount)
	f.morpho = f.deploy("RecipeExecutor", forkAdmin, forkAccount)
	f.claim = f.deploy("RecipeExecutor", forkAdmin, forkAccount)
	execute := [4]byte(executorBinding.GetABI().Methods["execute"].ID)
	for _, executor := range []common.Address{f.aave, f.morpho, f.claim} {
		f.send(forkAdmin, executor, executorBinding.PackSetCaller(f.manager))
		f.sendABI(forkAdmin, f.manager, "AccessManager", "setTargetFunctionRole", executor, [][4]byte{execute}, uint64(keeperRole))
	}
	for _, who := range []common.Address{forkSigner, forkKeeper} {
		f.sendABI(forkAdmin, f.manager, "AccessManager", "grantRole", uint64(keeperRole), who, uint32(0))
	}

	f.tree = fixtureTree(t, f.fixture.Calls)
	equal(t, f.fixture.Root, f.tree.Root, "UI root")
	adapter, err := accountBinding.UnpackAdapter(f.call(forkAccount, accountBinding.PackAdapter()))
	must(t, err)
	f.bundle = f.curatorBundle(adapter)
	f.sendABI(forkAdmin, world, "FixtureWorld", "setRoot", forkAccount, f.tree.Root)
	f.send(forkAdmin, forkAccount, accountBinding.PackAcceptRoot(f.tree.Root))

	sign := f.permission("sign(bytes32,bytes,(uint256,uint256,uint256)[])", forkAccount)
	withdraw := f.permission("withdraw(address,uint256,address)", forkAavePool)
	grants := map[common.Address][]uiCall{
		f.aave:   {sign, withdraw},
		f.morpho: {sign, withdraw},
		f.claim:  {f.permission("withdrawBond(address,uint256,address)", forkIris)},
		// The curator's Operate actions.
		forkAdmin: {f.permission("approve(address,uint256)", forkUSDC, forkIris), f.permission("claim(address,uint256,address,address)", forkIris)},
	}
	for who, calls := range grants {
		for _, c := range calls {
			f.send(forkAdmin, forkAccount, accountBinding.PackGrantRole(c.Hash, who))
		}
	}
	for i, executor := range []common.Address{f.aave, f.morpho} {
		f.setRecipe(executor, f.fixture.Offers[i].Recipe)
	}
	f.setRecipe(f.claim, f.fixture.BondClaim.Recipe)

	// The reserve's aToken is its ninth word: the UI compiled the matching withdrawal reads.
	reserve := f.call(forkAavePool, f.packPool("getReserveData", forkUSDC))
	equal(t, forkAUSDC, common.BytesToAddress(reserve[8*32:9*32]), "Aave USDC aToken")

	supply := big.NewInt(50_000e6)
	f.setUSDC(forkAdmin, supply)
	f.send(forkAdmin, forkUSDC, erc20Binding.PackApprove(forkAavePool, supply))
	f.send(forkAdmin, forkAavePool, f.packPool("supply", forkUSDC, supply, forkAccount, uint16(0)))
	f.approveIris(big.NewInt(30_000e6))

	f.setBalance(forkWstETH, wstETHBalances, forkBorrower, new(big.Int).Mul(f.collateral, big.NewInt(3)))
	f.send(forkBorrower, forkWstETH, erc20Binding.PackApprove(forkIris, math.MaxBig256))
}

// permission returns the UI call with this signature on target, and spender as its first
// argument when given.
func (f *forkEVM) permission(fn string, target common.Address, spender ...common.Address) uiCall {
	i := slices.IndexFunc(f.fixture.Calls, func(c uiCall) bool {
		return c.Fn == fn && c.Target == target && (len(spender) == 0 || common.BytesToAddress(c.Data[4:36]) == spender[0])
	})
	ok(f.t, i >= 0, fn)
	return f.fixture.Calls[i]
}

// manage runs one curator action through the account: the leaf with words written at their
// calldata offsets.
func (f *forkEVM) manage(c uiCall, words map[int]*big.Int) {
	data := bytes.Clone(c.Data)
	for offset, v := range words {
		copy(data[offset:offset+32], common.BigToHash(v).Bytes())
	}
	leaf := f.tree.Leaves[slices.IndexFunc(f.tree.Leaves, func(l recipes.Leaf) bool { return l.Hash == c.Hash })]
	proof := make([][32]byte, len(leaf.Proof))
	for i, p := range leaf.Proof {
		proof[i] = p
	}
	f.send(forkAdmin, forkAccount, accountBinding.PackManage([]merkleaccount.IMerkleAccountAction{{
		Target: c.Target, Value: new(big.Int), Data: data, DataMask: c.DataMask, Proof: proof,
	}}))
}

// approveIris sets the curator's standing USDC allowance to Iris, as Operate does: reset, then set.
func (f *forkEVM) approveIris(amount *big.Int) {
	approve := f.permission("approve(address,uint256)", forkUSDC, forkIris)
	f.manage(approve, map[int]*big.Int{36: new(big.Int)})
	if amount.Sign() > 0 {
		f.manage(approve, map[int]*big.Int{36: amount})
	}
	equal(f.t, amount, f.allowance(forkUSDC, forkAccount, forkIris), "Iris allowance")
}

// claimCall routes the UI bond-claim recipe for pod through the AccessManager, as its keeper does.
func (f *forkEVM) claimCall(t *testing.T, pod common.Address) []byte {
	t.Helper()
	recipe, err := recipes.DecodeRecipe(f.fixture.BondClaim.Recipe)
	must(t, err)
	inner, _, err := recipes.Assemble(recipe, f.tree, map[uint]string{0: pod.Big().String()}, nil, nil)
	must(t, err)
	data, err := f.abis["AccessManager"].Pack("execute", f.claim, inner)
	must(t, err)
	return data
}

// take has the borrower take the quote and checks what moved.
func (f *forkEVM) take(t *testing.T, quote blm.Quote) common.Address {
	t.Helper()
	accountBefore, receiverBefore := f.balance(forkUSDC, forkAccount), f.balance(forkUSDC, forkReceiver)
	receipt := f.send(forkBorrower, forkIris, f.pack("take", quote, make([]byte, placeholderSignatureBytes)))
	var pod common.Address
	for _, l := range receipt.Logs {
		if l.Address == forkIris && len(l.Topics) == 3 && l.Topics[0] == f.iris.Events["Take"].ID {
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
	return pod
}

// morphoMarket is the wstETH/USDC market's payload, read from Morpho by the id the UI fixes.
func (f *forkEVM) morphoMarket() []byte {
	return f.call(forkMorpho, morphoBinding.PackIdToMarketParams(forkMorphoMarket))
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

// pause clears the executor's caller, the way an operator stops it.
func (f *forkEVM) pause(executor common.Address) {
	f.send(forkAdmin, executor, executorBinding.PackSetCaller(common.Address{}))
}

// expectMarkets checks the latest discovery indexed want markets.
func (f *forkEVM) expectMarkets(t *testing.T, want int) {
	t.Helper()
	logs := f.logs.String()
	line := logs[strings.LastIndex(logs, "iris markets discovered"):]
	line = line[:strings.IndexByte(line+"\n", '\n')]
	if !strings.Contains(line, `"markets": `+strconv.Itoa(want)) && !strings.Contains(line, `"markets":`+strconv.Itoa(want)) {
		t.Fatalf("expected %d markets: %s", want, line)
	}
}

func (f *forkEVM) setRecipe(executor common.Address, encoded []byte) {
	recipe, err := recipes.DecodeRecipe(encoded)
	must(f.t, err)
	f.send(forkAdmin, executor, executorBinding.PackSetRecipe(recipe.Queries, recipe.Steps))
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
      morpho: "` + forkMorpho.Hex() + `"
      recipeExecutors: ["` + f.aave.Hex() + `", "` + f.morpho.Hex() + `", "` + f.claim.Hex() + `"]
      curator:
        url: ` + curator.URL + `
      refreshInterval: ` + f.refresh.String() + `
      server:
        listenAddress: "` + f.server + `"
        authHeader: ` + forkAuthHeader + `
        authValueEnv: IRIS_FORK_WEBHOOK_KEY
      pricing:
        defaultFixedRate: "` + strconv.Itoa(forkRate) + `"
        maxBond: "1000000000"
      timing:
        blockTime: ` + strconv.Itoa(forkBlockTime) + `s
        submitLead: 3s
        quoteTtl: 2m
        # Mining a block that touches fresh mainnet state can take several seconds on a fork.
        deadlineWindow: 10s
        responseBudget: 20s
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
		request, err := http.NewRequestWithContext(f.ctx, http.MethodGet, "http://"+f.server+"/ready", nil)
		must(t, err)
		if response, err := http.DefaultClient.Do(request); err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusNoContent {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("solver never became ready:\n%s", f.logs.String())
}

func (f *forkEVM) request(id string) quoteRequest {
	f.requests++
	return quoteRequest{
		RequestID: id, QuoteID: id + "-" + strconv.Itoa(f.requests), ChainID: 1,
		Borrower: forkBorrower.Hex(), Receiver: forkReceiver.Hex(),
		CollateralToken: forkWstETH.Hex(), Collateral: f.collateral.String(),
		DebtToken: forkUSDC.Hex(), Debt: f.debt.String(), MaxFixedRate: f.maxRate.String(),
		Duration: strconv.Itoa(30 * 86400), VenueBitmap: "3",
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
	f.expectDeclineAt(t, "/quote", r, reason)
}

func (f *forkEVM) expectDeclineAt(t *testing.T, path string, r quoteRequest, reason string) {
	t.Helper()
	status, body := f.post(t, path, r, forkAuthValue)
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

// expectedBond is the BLM requirement read independently, padded the way the UI recipe does:
// r + ceil(r/4).
func (f *forkEVM) expectedBond(r quoteRequest, q quoteResponse) *big.Int {
	quote := f.quote(f.t, r, q)
	quote.Bond, quote.Deadline, quote.Nonce = new(big.Int), new(big.Int), new(big.Int)
	requirement, err := blmBinding.UnpackBondRequirement(f.call(forkBlm, blmBinding.PackBondRequirement(quote)))
	must(f.t, err)
	ok(f.t, requirement.Sign() > 0, "zero requirement")
	quarter := new(big.Int).Add(requirement, big.NewInt(3))
	return quarter.Div(quarter, big.NewInt(4)).Add(quarter, requirement)
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

func (f *forkEVM) packPool(method string, args ...any) []byte {
	data, err := f.pool.Pack(method, args...)
	must(f.t, err)
	return data
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
	f.setBalance(forkUSDC, usdcBalances, owner, amount)
}

// setBalance writes a token balance into its balances mapping at slot.
func (f *forkEVM) setBalance(token common.Address, slot int64, owner common.Address, amount *big.Int) {
	key := crypto.Keccak256Hash(common.LeftPadBytes(owner.Bytes(), 32), common.LeftPadBytes(big.NewInt(slot).Bytes(), 32))
	f.rpcCall("anvil_setStorageAt", token, key, common.BigToHash(amount))
	equal(f.t, amount, f.balance(token, owner), "balance slot")
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
	return f.transact(forkAdmin, nil, append(hexutil.MustDecode(f.artifacts[name].Bytecode), constructor...)).ContractAddress
}

func (f *forkEVM) sendABI(from, to common.Address, contract, method string, args ...any) {
	data, err := f.abis[contract].Pack(method, args...)
	must(f.t, err)
	f.send(from, to, data)
}

func (f *forkEVM) send(from, to common.Address, data []byte) *types.Receipt {
	return f.transact(from, &to, data)
}

func (f *forkEVM) transact(from common.Address, to *common.Address, data []byte) *types.Receipt {
	f.t.Helper()
	args := map[string]any{"from": from, "data": hexutil.Encode(data), "gas": "0x1c9c380"}
	if to != nil {
		args["to"] = *to
	}
	var hash common.Hash
	must(f.t, f.rpc.CallContext(f.ctx, &hash, "eth_sendTransaction", args))
	// Blocks that touch fresh mainnet state take a while to mine on a fork.
	for range 300 {
		receipt, err := f.client.TransactionReceipt(f.ctx, hash)
		if err == nil {
			if receipt.Status != types.ReceiptStatusSuccessful {
				_, reason := f.client.CallContract(f.ctx, ethereum.CallMsg{From: from, To: to, Data: data}, new(big.Int).Sub(receipt.BlockNumber, big.NewInt(1)))
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

// waitDiscoveries waits until the solver has logged count market discoveries.
func (f *forkEVM) waitDiscoveries(t *testing.T, count int) {
	t.Helper()
	const message = "iris markets discovered"
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

// poolABI is the slice of the Aave V3 pool the curator side needs.
const poolABI = `[
{"type":"function","name":"supply","stateMutability":"nonpayable","inputs":[{"name":"asset","type":"address"},{"name":"amount","type":"uint256"},{"name":"onBehalfOf","type":"address"},{"name":"referralCode","type":"uint16"}],"outputs":[]},
{"type":"function","name":"getReserveData","stateMutability":"view","inputs":[{"name":"asset","type":"address"}],"outputs":[]}
]`
