//go:build integration

package txmanager

import (
	"bytes"
	"context"
	"math/big"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/signer"
)

func TestAnvilTxManagerPendingLifecycle(t *testing.T) {
	t.Run("fee bump replacement", testAnvilReplacement)
	t.Run("timeout replaces old call with fresh business payload", testAnvilAbandonment)
	t.Run("external inclusion stops silent relay replacements", testAnvilConsumedNonce)
}

// Reads use a real chain; sends model a private relay that acknowledges the
// signed bytes without forwarding them or checking the mined nonce.
type acceptingAnvilRelay struct {
	*chain.Client

	sends int
}

func (b *acceptingAnvilRelay) SendTransaction(context.Context, *types.Transaction) error {
	b.sends++
	return nil
}

func testAnvilConsumedNonce(t *testing.T) {
	t.Helper()
	rpcClient, ethClient, endpoint := startAnvilWithoutMining(t)
	mineAnvilBlock(t, rpcClient)
	relay := &acceptingAnvilRelay{Client: anvilManagerBackend(t, endpoint)}
	sgnr := anvilSigner(t)
	m := New(relay, sgnr, big.NewInt(31337), Config{Confirmations: 1, MaxFeeGwei: 100}, logr.Discard())
	if err := m.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	pending, err := m.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xdead"), GasLimit: 21_000, Label: "private fill",
	})
	if err != nil {
		t.Fatal(err)
	}
	m.trackUnminedTransaction(pending)
	// A different instance uses the same key and nonce with different calldata.
	to := common.HexToAddress("0xbeef")
	external, err := sgnr.SignTx(t.Context(), types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(31337), Nonce: 0, To: &to, Gas: 21_000,
		GasFeeCap: big.NewInt(10_000_000_000), GasTipCap: big.NewInt(1_000_000_000),
	}), big.NewInt(31337))
	if err != nil {
		t.Fatal(err)
	}
	if err := ethClient.SendTransaction(t.Context(), external); err != nil {
		t.Fatal(err)
	}
	mineAnvilBlock(t, rpcClient)
	receipt, err := ethClient.TransactionReceipt(t.Context(), external.Hash())
	if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
		t.Fatalf("external inclusion: receipt=%+v err=%v", receipt, err)
	}
	for range 3 {
		if _, err := m.tryReplace(t.Context(), pending, replaceIntent{}); err != nil {
			t.Fatal(err)
		}
	}
	if relay.sends != 1 || len(pending.attempts) != 1 || !m.Available() {
		t.Fatalf("consumed nonce was replaced: sends=%d attempts=%d available=%v", relay.sends, len(pending.attempts), m.Available())
	}
	mineAnvilBlock(t, rpcClient)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result := m.waitForPendingTransaction(ctx, pending)
	if result.Outcome != OutcomeNonceConsumed || !m.Available() {
		t.Fatalf("canonical consumption did not recover silent relay: %+v", result)
	}
	assertAnvilReconciliationOutcome(t, result, pending.originalHash)
	assertAnvilCanonicalTransaction(t, ethClient, external.Hash(), 0)
}

func testAnvilReplacement(t *testing.T) {
	t.Helper()
	rpcClient, ethClient, endpoint := startAnvilWithoutMining(t)
	mineAnvilBlock(t, rpcClient)
	sgnr := anvilSigner(t)
	manager := New(
		anvilManagerBackend(t, endpoint),
		sgnr,
		big.NewInt(31337),
		Config{
			Confirmations:       1,
			MaxFeeGwei:          100,
			PollInterval:        20 * time.Millisecond,
			ReplacementInterval: 200 * time.Millisecond,
			PendingTimeout:      5 * time.Second,
			Horizon:             HorizonConfig{BlockTime: 200 * time.Millisecond},
		},
		logr.Discard(),
	)
	if err := manager.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	startManagerForTest(t, manager)

	result, accepted := manager.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0x000000000000000000000000000000000000dEaD"), GasLimit: 21_000, Label: "replace",
	})
	if !accepted {
		t.Fatal("transaction was not accepted")
	}
	initial := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(poolTransaction) bool { return true })
	// Mine a block at a base fee above the initial fee cap: it excludes the call, and the base fee the
	// next block inherits is the evidence that reprices it.
	setAnvilNextBaseFee(t, rpcClient, big.NewInt(10_000_000_000))
	mineAnvilBlock(t, rpcClient)
	replacement := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(tx poolTransaction) bool {
		return tx.Hash != initial.Hash
	})
	assertAnvilReplacementFeeFloors(t, initial, replacement)

	mineAnvilBlock(t, rpcClient)
	mineAnvilBlock(t, rpcClient)
	got := waitForTxResult(t, result)
	assertAnvilCanonicalTransaction(t, ethClient, common.HexToHash(replacement.Hash), 0)
	expected := replacement.Hash
	if got.Outcome == OutcomeNonceConsumed {
		expected = initial.Hash // Without an owned receipt the manager retains the original identity.
	}
	assertAnvilReconciliationOutcome(t, got, common.HexToHash(expected))
}

func testAnvilAbandonment(t *testing.T) {
	t.Helper()
	rpcClient, ethClient, endpoint := startAnvilWithoutMining(t)
	mineAnvilBlock(t, rpcClient)
	sgnr := anvilSigner(t)
	manager := New(
		anvilManagerBackend(t, endpoint), sgnr, big.NewInt(31337),
		Config{
			Confirmations: 1, MaxFeeGwei: 100,
			PollInterval: 20 * time.Millisecond, ReplacementInterval: 5 * time.Second,
			PendingTimeout: time.Second,
		}, logr.Discard(),
	)
	if err := manager.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	startManagerForTest(t, manager)

	first, accepted := manager.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xdead"), Data: []byte{0x11, 0x22},
		GasLimit: 50_000, MaxFeePerGas: big.NewInt(3_000_000_000), Label: "old business call",
	})
	if !accepted {
		t.Fatal("first business call was not accepted")
	}
	initial := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(poolTransaction) bool { return true })
	firstResult := waitForTxResult(t, first)
	if firstResult.Outcome != OutcomeAbandoned || !errors.Is(firstResult.Err, ErrAbandoned) ||
		firstResult.Receipt != nil || firstResult.Outcome.Included() || firstResult.Hash != common.HexToHash(initial.Hash) {
		t.Fatalf("first result = %+v, want abandoned original identity without inclusion", firstResult)
	}
	unchanged := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(poolTransaction) bool { return true })
	if unchanged.Hash != initial.Hash || common.HexToAddress(unchanged.To) != common.HexToAddress("0xdead") || unchanged.Input != "0x1122" {
		t.Fatalf("timeout changed the pending business transaction: %+v", unchanged)
	}
	if _, err := ethClient.TransactionReceipt(t.Context(), common.HexToHash(initial.Hash)); !errors.Is(err, ethereum.NotFound) {
		t.Fatalf("abandoned call receipt error = %v, want not found", err)
	}
	latest, err := ethClient.NonceAt(t.Context(), sgnr.Address(), nil)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := ethClient.PendingNonceAt(t.Context(), sgnr.Address())
	if err != nil {
		t.Fatal(err)
	}
	if latest != 0 || pending != 1 {
		t.Fatalf("abandoned nonce state = latest %d pending %d, want 0/1", latest, pending)
	}

	// A fresh decision supplies a different target, calldata, value and gas limit. Even though the
	// write RPC advertises pending nonce 1, this process remembers its unused nonce 0.
	second, accepted := manager.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xbeef"), Data: []byte{0x33, 0x44, 0x55},
		Value: big.NewInt(17), GasLimit: 55_000, Label: "fresh business call",
	})
	if !accepted {
		t.Fatal("fresh business call was not accepted")
	}
	replacement := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(tx poolTransaction) bool {
		return tx.Hash != initial.Hash
	})
	if common.HexToAddress(replacement.To) != common.HexToAddress("0xbeef") || replacement.Input != "0x334455" ||
		replacement.Value != "0x11" || replacement.Gas != "0xd6d8" {
		t.Fatalf("fresh transaction did not use the fresh business payload: %+v", replacement)
	}
	assertAnvilReplacementFeeFloors(t, initial, replacement)
	if queued, exists, err := poolTransactionAt(t.Context(), rpcClient, sgnr.Address(), 1); err != nil || exists {
		t.Fatalf("fresh call queued a later nonce: tx=%+v exists=%v err=%v", queued, exists, err)
	}
	mineAnvilBlock(t, rpcClient)
	mineAnvilBlock(t, rpcClient)
	assertAnvilReconciliationOutcome(t, waitForTxResult(t, second), common.HexToHash(replacement.Hash))
	assertAnvilCanonicalTransaction(t, ethClient, common.HexToHash(replacement.Hash), 0)
}

func assertAnvilReplacementFeeFloors(t *testing.T, original, replacement poolTransaction) {
	t.Helper()
	for name, fees := range map[string][2]string{
		"fee cap": {original.MaxFeePerGas, replacement.MaxFeePerGas},
		"tip cap": {original.MaxPriorityFeePerGas, replacement.MaxPriorityFeePerGas},
	} {
		previous, err := hexutil.DecodeBig(fees[0])
		if err != nil {
			t.Fatal(err)
		}
		next, err := hexutil.DecodeBig(fees[1])
		if err != nil {
			t.Fatal(err)
		}
		// Ethereum's replacement floor is 112.5 percent of each fee, rounded up to a whole wei.
		required := new(big.Int).Mul(previous, big.NewInt(9))
		required.Add(required, big.NewInt(7)).Div(required, big.NewInt(8))
		if next.Cmp(required) < 0 {
			t.Fatalf("%s = %s, want at least %s after %s", name, next, required, previous)
		}
	}
}

func anvilManagerBackend(t *testing.T, endpoint string) *chain.Client {
	t.Helper()
	client, err := chain.Dial(t.Context(), []string{endpoint}, "", common.Address{}.Hex(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

type poolTransaction struct {
	Hash                 string `json:"hash"`
	To                   string `json:"to"`
	Value                string `json:"value"`
	Input                string `json:"input"`
	Gas                  string `json:"gas"`
	MaxFeePerGas         string `json:"maxFeePerGas"`
	MaxPriorityFeePerGas string `json:"maxPriorityFeePerGas"`
}

type txPoolContent struct {
	Pending map[string]map[string]poolTransaction `json:"pending"`
	Queued  map[string]map[string]poolTransaction `json:"queued"`
}

func waitForPoolTransaction(
	t *testing.T,
	client *rpc.Client,
	sender common.Address,
	nonce uint64,
	accept func(poolTransaction) bool,
) poolTransaction {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		tx, ok, err := poolTransactionAt(t.Context(), client, sender, nonce)
		if err != nil {
			t.Fatalf("txpool_content: %v", err)
		}
		if ok && accept(tx) {
			return tx
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for sender %s nonce %d in txpool", sender.Hex(), nonce)
	return poolTransaction{}
}

func poolTransactionAt(
	ctx context.Context,
	client *rpc.Client,
	sender common.Address,
	nonce uint64,
) (poolTransaction, bool, error) {
	var content txPoolContent
	if err := client.CallContext(ctx, &content, "txpool_content"); err != nil {
		return poolTransaction{}, false, err
	}
	nonceKey := strconv.FormatUint(nonce, 10)
	for _, pool := range []map[string]map[string]poolTransaction{content.Pending, content.Queued} {
		for address, transactions := range pool {
			if !strings.EqualFold(address, sender.Hex()) {
				continue
			}
			tx, ok := transactions[nonceKey]
			if ok {
				return tx, true, nil
			}
		}
	}
	return poolTransaction{}, false, nil
}

func mineAnvilBlock(t *testing.T, client *rpc.Client) {
	t.Helper()
	if err := client.CallContext(t.Context(), nil, "anvil_mine", 1); err != nil {
		t.Fatalf("anvil_mine: %v", err)
	}
}

func setAnvilNextBaseFee(t *testing.T, client *rpc.Client, fee *big.Int) {
	t.Helper()
	if err := client.CallContext(t.Context(), nil, "anvil_setNextBlockBaseFeePerGas", hexutil.EncodeBig(fee)); err != nil {
		t.Fatalf("anvil_setNextBlockBaseFeePerGas: %v", err)
	}
}

func waitForTxResult(t *testing.T, result <-chan Result) Result {
	t.Helper()
	select {
	case got := <-result:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for transaction result")
		return Result{}
	}
}

func anvilSigner(t *testing.T) signer.Signer {
	t.Helper()
	sgnr, err := signer.NewFromHexKey(testKey)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	return sgnr
}

func startAnvilWithoutMining(t *testing.T) (*rpc.Client, *ethclient.Client, string) {
	t.Helper()
	anvil, err := exec.LookPath("anvil")
	if err != nil {
		t.Skip("anvil is not installed")
	}
	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve anvil port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release anvil port: %v", err)
	}

	var output bytes.Buffer

	cmd := exec.CommandContext(
		t.Context(),
		anvil,
		"--no-mining",
		"--silent",
		"--chain-id",
		"31337",
		"--port",
		strconv.Itoa(port),
	)
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start anvil: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	})

	url := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		client, dialErr := rpc.DialContext(t.Context(), url)
		if dialErr == nil {
			var chainID string
			if callErr := client.CallContext(t.Context(), &chainID, "eth_chainId"); callErr == nil {
				ethClient := ethclient.NewClient(client)
				t.Cleanup(ethClient.Close)
				return client, ethClient, url
			}
			client.Close()
		}
		select {
		case exitErr := <-done:
			t.Fatalf("anvil exited during startup: %v\n%s", exitErr, output.String())
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("anvil did not become ready:\n%s", output.String())
	return nil, nil, ""
}
