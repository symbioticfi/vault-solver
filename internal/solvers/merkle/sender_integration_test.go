//go:build integration

package merkle

import (
	"context"
	"math/big"
	"net"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	txmanager "github.com/symbioticfi/merkle-executor/sdk/transaction"
	"github.com/symbioticfi/vault-solver/internal/signer"
)

// Only relay delivery is held back. The host signer uses the shared SDK engine
// through its public sender and ordinary transaction APIs.
type ownedRelay struct {
	*ethclient.Client

	mu        sync.Mutex
	sent      []*types.Transaction
	observed  chan struct{}
	ambiguous bool
}

func (b *ownedRelay) TransactionSenderBalanceAt(ctx context.Context, account common.Address, block *big.Int) (*big.Int, error) {
	return b.BalanceAt(ctx, account, block)
}

func (b *ownedRelay) SendTransaction(_ context.Context, tx *types.Transaction) error {
	b.mu.Lock()
	b.sent = append(b.sent, tx)
	b.mu.Unlock()
	select {
	case b.observed <- struct{}{}:
	default:
	}
	if b.ambiguous {
		return errors.New("relay response lost")
	}
	return nil
}
func (b *ownedRelay) snapshot() []*types.Transaction {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*types.Transaction(nil), b.sent...)
}

func TestSDKRAMSenderReplacementAndReorg(t *testing.T) {
	signing := ramSigning(t)
	chain, mine := senderAnvil(t, signing.Address())
	relay := &ownedRelay{Client: chain, observed: make(chan struct{}, 64)}
	engine, err := txmanager.New(relay, signing, big.NewInt(1337), txmanager.Config{MaxFeeGwei: 100, Confirmations: 1, PollInterval: time.Millisecond, ReplacementInterval: time.Second, ShutdownTimeout: time.Millisecond}, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	registry := prometheus.NewRegistry()
	engine.Metrics, err = txmanager.NewMetrics(prometheus.WrapRegistererWithPrefix("solver_bot_merkle_", registry))
	if err != nil {
		t.Fatal(err)
	}
	sender := engine.Sender("test")
	runRAMManager(t, engine)
	if err := sender.Submit(t.Context(), txmanager.ExecutionRequest{To: common.HexToAddress("0x1234"), Data: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	awaitBroadcast(t, relay)
	assertRAMMetric(t, registry, "transaction_unresolved_nonce", 1)
	if engine.Available() || engine.LaneReady() {
		t.Fatal("new quote admitted while signed work owned")
	}
	if err := chain.Client().CallContext(t.Context(), nil, "anvil_setNextBlockBaseFeePerGas", hexutil.EncodeBig(big.NewInt(40_000_000_000))); err != nil {
		t.Fatal(err)
	}
	mine()
	awaitReplacement(t, relay)
	attempts := relay.snapshot()
	latest := attempts[len(attempts)-1]
	if attempts[0].Hash() == latest.Hash() || attempts[0].Nonce() != latest.Nonce() {
		t.Fatal("host replacement did not preserve exact nonce")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := sender.Poll(canceled); !errors.Is(err, context.Canceled) || !sender.Busy() {
		t.Fatal("canceled poll discarded owned transaction", err)
	}
	var snapshot string
	if err := chain.Client().CallContext(t.Context(), &snapshot, "evm_snapshot"); err != nil {
		t.Fatal(err)
	}
	if err := chain.SendTransaction(t.Context(), latest); err != nil {
		t.Fatal(err)
	}
	mine()
	if result, err := sender.Poll(t.Context()); result != nil || err != nil || !sender.Busy() {
		t.Fatalf("unconfirmed transaction released: %+v %v", result, err)
	}
	var reverted bool
	if err := chain.Client().CallContext(t.Context(), &reverted, "evm_revert", snapshot); err != nil || !reverted {
		t.Fatal("revert local chain", err)
	}
	// A receipt sweep may precede mining while its later nonce read follows it.
	// That execution-unknown result remains valid after the local reorg.
	result, err := sender.Poll(t.Context())
	if result != nil || !sender.Busy() || engine.Available() || engine.LaneReady() {
		t.Fatalf("reorg released ownership: %+v %v", result, err)
	}
	if err != nil && !errors.Is(err, txmanager.ErrNonceConsumed) {
		t.Fatalf("unexpected reorg observation: %v", err)
	}
	if err := chain.SendTransaction(t.Context(), latest); err != nil {
		t.Fatal(err)
	}
	mine()
	mine()
	result = awaitRAMTerminal(t, sender)
	if result.Hash != latest.Hash() || result.Execution == nil || sender.Busy() || !engine.Available() {
		t.Fatalf("canonical result = %+v", result)
	}
	receipt, err := chain.TransactionReceipt(t.Context(), result.Hash)
	if err != nil {
		t.Fatal(err)
	}
	paid, _ := new(big.Float).SetInt(new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice)).Float64()
	assertRAMMetric(t, registry, "transaction_unresolved_nonce", 0)
	assertRAMMetric(t, registry, "transaction_requests_total", 1)
	assertRAMMetric(t, registry, "transaction_gas_used_total", float64(receipt.GasUsed))
	assertRAMMetric(t, registry, "transaction_fee_paid_wei_total", paid)
}

func assertRAMMetric(t *testing.T, registry *prometheus.Registry, suffix string, want float64) {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	name := "solver_bot_merkle_" + suffix
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		var got float64
		for _, metric := range family.GetMetric() {
			got += metric.GetCounter().GetValue() + metric.GetGauge().GetValue()
		}
		if got != want {
			t.Fatalf("%s = %g, want %g", name, got, want)
		}
		return
	}
	t.Fatalf("metric %s missing", name)
}

func TestSDKRAMAmbiguousOutcomeResumesAfterDeadline(t *testing.T) {
	signing := ramSigning(t)
	chain, mine := senderAnvil(t, signing.Address())
	relay := &ownedRelay{Client: chain, observed: make(chan struct{}, 64), ambiguous: true}
	engine, err := txmanager.New(relay, signing, big.NewInt(1337), txmanager.Config{MaxFeeGwei: 100, PollInterval: time.Millisecond, ReplacementInterval: 20 * time.Millisecond, ShutdownTimeout: time.Millisecond}, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	sender := engine.Sender("test")
	sender.Deadline = func() time.Time { return time.Now().Add(80 * time.Millisecond) }
	runRAMManager(t, engine)
	if err := sender.Submit(t.Context(), txmanager.ExecutionRequest{To: common.HexToAddress("0x1234")}); err != nil {
		t.Fatal(err)
	}
	awaitBroadcast(t, relay)
	deadline := time.Now().Add(3 * time.Second)
	ambiguous := false
	for time.Now().Before(deadline) {
		result, err := sender.Poll(t.Context())
		if result != nil {
			t.Fatal("unmined call returned terminal receipt")
		}
		if err != nil {
			ambiguous = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !ambiguous || !sender.Busy() || engine.Available() {
		t.Fatal("ambiguous outcome lost signer ownership")
	}
	// The business deadline expires while the process retains the exact signed
	// transaction. Receipt observation still succeeds after that deadline.
	time.Sleep(100 * time.Millisecond)
	attempts := relay.snapshot()
	latest := attempts[len(attempts)-1]
	if err := chain.SendTransaction(t.Context(), latest); err != nil {
		t.Fatal(err)
	}
	mine()
	result := awaitRAMTerminal(t, sender)
	if result.Hash != latest.Hash() || sender.Busy() || !engine.Available() {
		t.Fatalf("late receipt = %+v", result)
	}
	after := relay.snapshot()
	if len(after) != len(attempts) {
		t.Fatal("expired request broadcast again during receipt-only resumption")
	}
}

func ramSigning(t *testing.T) signer.Signer {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signing, err := signer.NewFromHexKey(hexutil.Encode(crypto.FromECDSA(key)))
	if err != nil {
		t.Fatal(err)
	}
	return signing
}
func runRAMManager(t *testing.T, engine *txmanager.Manager) {
	t.Helper()
	if err := engine.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { engine.Start(ctx); close(done) }()
	t.Cleanup(func() { stop(); <-done })
}
func awaitBroadcast(t *testing.T, relay *ownedRelay) {
	t.Helper()
	select {
	case <-relay.observed:
	case <-time.After(5 * time.Second):
		t.Fatal("host broadcast absent")
	}
}
func awaitReplacement(t *testing.T, relay *ownedRelay) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	first := relay.snapshot()[0]
	for {
		attempts := relay.snapshot()
		if attempts[len(attempts)-1].Hash() != first.Hash() {
			return
		}
		select {
		case <-relay.observed:
		case <-ctx.Done():
			t.Fatal("host fee replacement absent")
		}
	}
}
func awaitRAMTerminal(t *testing.T, sender *txmanager.Sender) *txmanager.ExecutionResult {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		result, _ := sender.Poll(t.Context())
		if result != nil {
			return result
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("canonical RAM transaction did not resolve")
	return nil
}

// The only RPC endpoint is a child process listening on loopback with a throwaway key.
func senderAnvil(t *testing.T, account common.Address) (*ethclient.Client, func()) {
	t.Helper()
	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	executable, err := exec.LookPath("anvil")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), executable, "--host", "127.0.0.1", "--port", strconv.Itoa(port), "--chain-id", "1337", "--no-mining", "--silent")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	endpoint := "http://127.0.0.1:" + strconv.Itoa(port)
	var raw *rpc.Client
	deadline := time.Now().Add(5 * time.Second)
	for {
		raw, err = rpc.DialContext(t.Context(), endpoint)
		if err == nil {
			var id string
			if err = raw.CallContext(t.Context(), &id, "eth_chainId"); err == nil {
				break
			}
			raw.Close()
		}
		if time.Now().After(deadline) {
			t.Fatal("local Anvil failed to start", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Cleanup(raw.Close)
	if err := raw.CallContext(t.Context(), nil, "anvil_setBalance", account, "0x3635c9adc5dea00000"); err != nil {
		t.Fatal(err)
	}
	mine := func() {
		t.Helper()
		if err := raw.CallContext(t.Context(), nil, "evm_mine"); err != nil {
			t.Fatal(err)
		}
	}
	for range 32 {
		mine()
	}
	return ethclient.NewClient(raw), mine
}
