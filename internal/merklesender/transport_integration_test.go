//go:build integration

package merklesender

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
	"github.com/symbioticfi/merkle-executor/sdk/transaction"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// Only relay delivery is held back. The real host signs/replaces; every network
// mutation must already have an immutable signed identity in the RAM owner.
type ownedRelay struct {
	*ethclient.Client

	sender    *Sender
	mu        sync.Mutex
	sent      []*types.Transaction
	observed  chan struct{}
	ambiguous bool
}

func (b *ownedRelay) SendTransaction(_ context.Context, tx *types.Transaction) error {
	b.sender.mu.Lock()
	work := b.sender.active
	b.sender.mu.Unlock()
	found := false
	if work != nil {
		for _, owned := range work.snapshot() {
			found = found || owned.Hash() == tx.Hash()
		}
	}
	if !found {
		return errors.New("broadcast preceded RAM ownership capture")
	}
	b.mu.Lock()
	b.sent = append(b.sent, tx)
	b.mu.Unlock()
	b.observed <- struct{}{}
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

func TestSDKRAMSharedSenderReplacementAndReorg(t *testing.T) {
	signing := ramSigning(t)
	chain, mine := senderAnvil(t, signing.Address())
	relay := &ownedRelay{Client: chain, observed: make(chan struct{}, 64)}
	engine := txmanager.New(relay, signing, big.NewInt(1337), txmanager.Config{MaxFeeGwei: 100, Confirmations: 1, PollInterval: time.Millisecond, ReplacementInterval: time.Second, PendingTimeout: 10 * time.Second, ShutdownTimeout: time.Millisecond, Horizon: txmanager.HorizonConfig{BlockTime: time.Second}}, logr.Discard())
	sender := New(engine, "test")
	relay.sender = sender
	runRAMManager(t, engine)
	if err := sender.Submit(t.Context(), transaction.Request{To: common.HexToAddress("0x1234"), Data: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	awaitBroadcast(t, relay)
	if engine.Available() || engine.LaneReady() {
		t.Fatal("foreign quote admitted while signed work owned")
	}
	if _, accepted := engine.TrySend(t.Context(), txmanager.Request{To: common.HexToAddress("0x5678")}); accepted {
		t.Fatal("foreign solver entered owned lane")
	}
	if err := chain.Client().CallContext(t.Context(), nil, "anvil_setNextBlockBaseFeePerGas", hexutil.EncodeBig(big.NewInt(40_000_000_000))); err != nil {
		t.Fatal(err)
	}
	mine()
	awaitBroadcast(t, relay)
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
	if err != nil && !errors.Is(err, transaction.ErrNonceConsumed) {
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
}

func TestSDKRAMAmbiguousOutcomeResumesAfterDeadline(t *testing.T) {
	signing := ramSigning(t)
	chain, mine := senderAnvil(t, signing.Address())
	relay := &ownedRelay{Client: chain, observed: make(chan struct{}, 64), ambiguous: true}
	engine := txmanager.New(relay, signing, big.NewInt(1337), txmanager.Config{MaxFeeGwei: 100, PollInterval: time.Millisecond, ReplacementInterval: 20 * time.Millisecond, PendingTimeout: 50 * time.Millisecond, ShutdownTimeout: time.Millisecond, Horizon: txmanager.HorizonConfig{BlockTime: time.Second}}, logr.Discard())
	sender := New(engine, "test")
	relay.sender = sender
	sender.Deadline = func() time.Time { return time.Now().Add(80 * time.Millisecond) }
	runRAMManager(t, engine)
	if err := sender.Submit(t.Context(), transaction.Request{To: common.HexToAddress("0x1234")}); err != nil {
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
		t.Fatal("ambiguous outcome lost shared signer ownership")
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
func awaitRAMTerminal(t *testing.T, sender *Sender) *transaction.Result {
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
