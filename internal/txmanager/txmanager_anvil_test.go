//go:build integration

package txmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/signer"
)

func TestAnvilTxManagerPendingLifecycle(t *testing.T) {
	t.Run("fee bump replacement", testAnvilReplacement)
	t.Run("timeout cancellation unblocks later nonce", testAnvilCancellation)
}

func TestAnvilTxManagerRecoversFromReceiptTimeoutAfterWriteRPCBroadcast(t *testing.T) {
	rpcClient, _, upstreamURL := startAnvilWithoutMiningWithURL(t)
	sgnr := anvilSigner(t)

	var receiptFaultArmed atomic.Bool
	var readBroadcasts atomic.Int64
	var readReceiptLookups atomic.Int64
	var writeBroadcasts atomic.Int64
	var writeReceiptLookups atomic.Int64
	receiptBlocked := make(chan struct{}, 1)

	readRPC := newAnvilRPCProxy(t, upstreamURL, func(r *http.Request, method string) bool {
		switch method {
		case "eth_sendRawTransaction":
			readBroadcasts.Add(1)
		case "eth_getTransactionReceipt":
			readReceiptLookups.Add(1)
			if receiptFaultArmed.CompareAndSwap(true, false) {
				select {
				case receiptBlocked <- struct{}{}:
				default:
				}
				<-r.Context().Done()
				return true
			}
		}
		return false
	})
	writeRPC := newAnvilRPCProxy(t, upstreamURL, func(_ *http.Request, method string) bool {
		switch method {
		case "eth_sendRawTransaction":
			writeBroadcasts.Add(1)
			receiptFaultArmed.Store(true)
		case "eth_getTransactionReceipt":
			writeReceiptLookups.Add(1)
		}
		return false
	})

	client, err := chain.Dial(
		t.Context(),
		[]string{readRPC.URL},
		writeRPC.URL,
		"0xcA11bde05977b3631167028862bE2a173976CA11",
		logr.Discard(),
	)
	if err != nil {
		t.Fatalf("dial proxied chain clients: %v", err)
	}
	t.Cleanup(client.Close)

	core, logs := observer.New(zapcore.ErrorLevel)
	manager := New(
		client,
		sgnr,
		big.NewInt(31337),
		Config{
			MaxFeeGwei:          100,
			TipGwei:             1,
			PollInterval:        20 * time.Millisecond,
			ReplacementInterval: 5 * time.Second,
			PendingTimeout:      10 * time.Second,
		},
		zapr.NewLogger(zap.New(core)),
	)
	go manager.Start(t.Context())

	result, accepted := manager.SendAsync(t.Context(), Request{
		To:       common.HexToAddress("0x000000000000000000000000000000000000dEaD"),
		GasLimit: 21_000,
		Label:    "rfq-fill",
	})
	if !accepted {
		t.Fatal("transaction was not accepted")
	}
	select {
	case <-receiptBlocked:
	case early := <-result:
		t.Fatalf(
			"transaction completed before injected receipt timeout: outcome %q, error %v; rpc counts write broadcasts %d, read broadcasts %d, read receipts %d",
			early.Outcome,
			early.Err,
			writeBroadcasts.Load(),
			readBroadcasts.Load(),
			readReceiptLookups.Load(),
		)
	case <-time.After(5 * time.Second):
		t.Fatalf(
			"txmanager did not reach the injected receipt timeout; rpc counts write broadcasts %d, read broadcasts %d, read receipts %d",
			writeBroadcasts.Load(),
			readBroadcasts.Load(),
			readReceiptLookups.Load(),
		)
	}
	// Mine while the read proxy is still holding the first receipt lookup. That lookup must fail on
	// its deadline; the next poll should see the same transaction confirmed instead of losing it.
	mineAnvilBlock(t, rpcClient)

	got := waitForTxResult(t, result)
	if got.Err != nil || got.Outcome != OutcomeConfirmed {
		t.Fatalf("transaction result = outcome %q, error %v; want confirmed", got.Outcome, got.Err)
	}
	if writeBroadcasts.Load() != 1 || readBroadcasts.Load() != 0 {
		t.Fatalf(
			"broadcast routing = write %d, read %d; want write 1, read 0",
			writeBroadcasts.Load(),
			readBroadcasts.Load(),
		)
	}
	if readReceiptLookups.Load() < 2 || writeReceiptLookups.Load() != 0 {
		t.Fatalf(
			"receipt routing = read %d, write %d; want at least two read lookups and no write lookups",
			readReceiptLookups.Load(),
			writeReceiptLookups.Load(),
		)
	}

	entries := logs.FilterMessage("pending transaction receipt unavailable").All()
	if len(entries) != 1 {
		t.Fatalf("receipt timeout logs = %d, want 1", len(entries))
	}
	errorText, ok := entries[0].ContextMap()["error"].(string)
	if !ok {
		t.Fatalf("receipt timeout error field = %#v, want string", entries[0].ContextMap()["error"])
	}
	if !strings.Contains(errorText, "rpc fallback: all 1 endpoints failed") ||
		!strings.Contains(errorText, "context deadline exceeded") {
		t.Fatalf("receipt timeout error = %q, want fallback deadline error", errorText)
	}
	t.Logf(
		"reproduced receipt failure %q; recovered after %d read receipt lookups (broadcasts: write %d, read %d)",
		errorText,
		readReceiptLookups.Load(),
		writeBroadcasts.Load(),
		readBroadcasts.Load(),
	)
}

func testAnvilReplacement(t *testing.T) {
	rpcClient, ethClient := startAnvilWithoutMining(t)
	sgnr := anvilSigner(t)
	manager := New(
		ethClient,
		sgnr,
		big.NewInt(31337),
		Config{
			MaxFeeGwei:          100,
			TipGwei:             1,
			PollInterval:        20 * time.Millisecond,
			ReplacementInterval: 200 * time.Millisecond,
			PendingTimeout:      5 * time.Second,
		},
		logr.Discard(),
	)
	go manager.Start(t.Context())

	result, accepted := manager.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0x000000000000000000000000000000000000dEaD"), GasLimit: 21_000, Label: "replace",
	})
	if !accepted {
		t.Fatal("transaction was not accepted")
	}
	initial := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(poolTransaction) bool { return true })
	replacement := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(tx poolTransaction) bool {
		return tx.Hash != initial.Hash
	})
	if compareHexQuantity(replacement.MaxFeePerGas, initial.MaxFeePerGas) <= 0 ||
		compareHexQuantity(replacement.MaxPriorityFeePerGas, initial.MaxPriorityFeePerGas) <= 0 {
		t.Fatalf(
			"replacement fees did not increase: first=%s/%s replacement=%s/%s",
			initial.MaxFeePerGas,
			initial.MaxPriorityFeePerGas,
			replacement.MaxFeePerGas,
			replacement.MaxPriorityFeePerGas,
		)
	}

	mineAnvilBlock(t, rpcClient)
	got := waitForTxResult(t, result)
	if got.Err != nil {
		t.Fatalf("replacement result: %v", got.Err)
	}
	if !strings.EqualFold(got.Hash.Hex(), replacement.Hash) {
		t.Fatalf("mined hash = %s, want replacement %s", got.Hash.Hex(), replacement.Hash)
	}
}

func testAnvilCancellation(t *testing.T) {
	rpcClient, ethClient := startAnvilWithoutMining(t)
	sgnr := anvilSigner(t)
	manager := New(
		ethClient,
		sgnr,
		big.NewInt(31337),
		Config{
			MaxFeeGwei:          100,
			TipGwei:             1,
			PollInterval:        20 * time.Millisecond,
			ReplacementInterval: 5 * time.Second,
			PendingTimeout:      300 * time.Millisecond,
		},
		logr.Discard(),
	)
	go manager.Start(t.Context())

	first, accepted := manager.SendAsync(t.Context(), Request{
		To:           common.HexToAddress("0x000000000000000000000000000000000000dEaD"),
		GasLimit:     21_000,
		MaxFeePerGas: big.NewInt(3_000_000_000),
		Label:        "blocked",
	})
	if !accepted {
		t.Fatal("first transaction was not accepted")
	}
	type submission struct {
		result   <-chan Result
		accepted bool
	}
	secondSubmission := make(chan submission, 1)
	go func() {
		second, secondAccepted := manager.SendAsync(t.Context(), Request{
			To:       common.HexToAddress("0x000000000000000000000000000000000000bEEF"),
			GasLimit: 21_000,
			Label:    "later",
		})
		secondSubmission <- submission{result: second, accepted: secondAccepted}
	}()

	initial := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(tx poolTransaction) bool {
		return !strings.EqualFold(tx.To, sgnr.Address().Hex())
	})
	dropAnvilTransaction(t, rpcClient, initial.Hash)
	latest, err := ethClient.NonceAt(t.Context(), sgnr.Address(), nil)
	if err != nil {
		t.Fatalf("latest nonce: %v", err)
	}
	pending, err := ethClient.PendingNonceAt(t.Context(), sgnr.Address())
	if err != nil {
		t.Fatalf("pending nonce: %v", err)
	}
	if latest != 0 || pending != 0 {
		t.Fatalf("nonce after dropping blocker = latest %d pending %d, want 0/0", latest, pending)
	}
	if _, exists, err := poolTransactionAt(t.Context(), rpcClient, sgnr.Address(), 1); err != nil {
		t.Fatalf("inspect future nonce: %v", err)
	} else if exists {
		t.Fatal("future nonce was signed and queued behind the dropped blocker")
	}
	select {
	case got := <-secondSubmission:
		t.Fatalf("future request was admitted before nonce 0 completed: %+v", got)
	default:
	}
	cancellation := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(tx poolTransaction) bool {
		return strings.EqualFold(tx.To, sgnr.Address().Hex()) && tx.Input == "0x" && tx.Value == "0x0"
	})
	if cancellation.Gas != "0x5208" {
		t.Fatalf("cancellation gas = %s, want 0x5208", cancellation.Gas)
	}

	mineAnvilBlock(t, rpcClient)
	firstResult := waitForTxResult(t, first)
	if firstResult.Err == nil || !strings.Contains(firstResult.Err.Error(), "cancelled at nonce 0") {
		t.Fatalf("first result = %+v, want cancellation", firstResult)
	}
	var second submission
	select {
	case second = <-secondSubmission:
		if !second.accepted {
			t.Fatal("second transaction was not accepted after nonce 0 completed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second transaction remained blocked after nonce 0 completed")
	}
	waitForPoolTransaction(t, rpcClient, sgnr.Address(), 1, func(poolTransaction) bool { return true })
	mineAnvilBlock(t, rpcClient)
	if secondResult := waitForTxResult(t, second.result); secondResult.Err != nil {
		t.Fatalf("later transaction remained blocked: %v", secondResult.Err)
	}
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

func compareHexQuantity(left, right string) int {
	leftValue, leftErr := hexutil.DecodeBig(left)
	rightValue, rightErr := hexutil.DecodeBig(right)
	if leftErr != nil || rightErr != nil {
		return 0
	}
	return leftValue.Cmp(rightValue)
}

func mineAnvilBlock(t *testing.T, client *rpc.Client) {
	t.Helper()
	if err := client.CallContext(t.Context(), nil, "anvil_mine", 1); err != nil {
		t.Fatalf("anvil_mine: %v", err)
	}
}

func dropAnvilTransaction(t *testing.T, client *rpc.Client, hash string) {
	t.Helper()
	var dropped string
	if err := client.CallContext(t.Context(), &dropped, "anvil_dropTransaction", hash); err != nil {
		t.Fatalf("anvil_dropTransaction: %v", err)
	}
	if !strings.EqualFold(dropped, hash) {
		t.Fatalf("anvil dropped %s, want %s", dropped, hash)
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

func startAnvilWithoutMining(t *testing.T) (*rpc.Client, *ethclient.Client) {
	t.Helper()
	rpcClient, ethClient, _ := startAnvilWithoutMiningWithURL(t)
	return rpcClient, ethClient
}

func startAnvilWithoutMiningWithURL(t *testing.T) (*rpc.Client, *ethclient.Client, string) {
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

func newAnvilRPCProxy(
	t *testing.T,
	upstreamURL string,
	intercept func(*http.Request, string) bool,
) *httptest.Server {
	t.Helper()
	upstream, err := url.Parse(upstreamURL)
	if err != nil {
		t.Fatalf("parse anvil upstream url: %v", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read proxied rpc request: %v", err)
			http.Error(w, "read request", http.StatusInternalServerError)
			return
		}
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))
		var request struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("decode proxied rpc request: %v", err)
			http.Error(w, "decode request", http.StatusBadRequest)
			return
		}
		if intercept(r, request.Method) {
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}
