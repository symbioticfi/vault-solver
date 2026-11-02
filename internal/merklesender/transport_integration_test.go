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
	"github.com/symbioticfi/merkle-executor/sdk/state"
	"github.com/symbioticfi/merkle-executor/sdk/submission"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// Only the relay is held back. Signing, atomic journal, host sender, SDK recovery and
// canonical receipt checks are real; the EVM is an isolated funded simulated chain.
type journalRelay struct {
	*ethclient.Client

	directory string
	mu        sync.Mutex
	sent      []*types.Transaction
	persisted chan struct{}
	fault     error
}

func (b *journalRelay) SendTransaction(_ context.Context, tx *types.Transaction) error {
	var journal struct {
		Raw      hexutil.Bytes   `json:"raw"`
		Attempts []hexutil.Bytes `json:"attempts"`
	}
	found, err := state.Read(b.directory, "txmanager.active.v1", &journal)
	if err != nil {
		return err
	}
	durable := false
	for _, raw := range append([]hexutil.Bytes{journal.Raw}, journal.Attempts...) {
		var saved types.Transaction
		if saved.UnmarshalBinary(raw) == nil && saved.Hash() == tx.Hash() {
			durable = true
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !found || !durable {
		b.fault = errors.New("broadcast preceded signed journal persistence")
		return b.fault
	}
	b.sent = append(b.sent, tx)
	b.persisted <- struct{}{}
	return nil
}

func TestSDKJournalSharedSenderReplacementRestartAndAcknowledge(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signing, err := signer.NewFromHexKey(hexutil.Encode(crypto.FromECDSA(key)))
	if err != nil {
		t.Fatal(err)
	}
	chain, mine := journalAnvil(t, signing.Address())
	mine()
	directory := t.TempDir()
	store, err := state.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	relay := &journalRelay{Client: chain, directory: directory, persisted: make(chan struct{}, 64)}
	config := txmanager.Config{MaxFeeGwei: 100, PollInterval: time.Millisecond, ReplacementInterval: time.Second,
		PendingTimeout: 10 * time.Second, ShutdownTimeout: time.Millisecond, Horizon: txmanager.HorizonConfig{BlockTime: time.Second}}
	engine := txmanager.New(relay, signing, big.NewInt(1337), config, logr.Discard())
	if err := engine.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() { engine.Start(runCtx); close(stopped) }()
	t.Cleanup(func() { stop(); <-stopped; _ = store.Close() })
	journal, err := submission.New(relay, signing.Address(), big.NewInt(1337), store, submission.Config{PollInterval: time.Millisecond}, Transport{Manager: engine, Solver: "test"}, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	request := submission.Request{ID: "recipe-one", To: common.HexToAddress("0x1234"), Data: []byte{1}}
	sendCtx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { _, sendErr := journal.Send(sendCtx, request); result <- sendErr }()
	select {
	case <-relay.persisted:
	case err := <-result:
		t.Fatalf("initial SDK send: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("first signed attempt absent")
	}
	if engine.Available() || engine.LaneReady() {
		t.Fatal("foreign solver can enter owned lane")
	}
	if _, admitted := engine.TrySend(t.Context(), txmanager.Request{To: request.To, Label: "foreign"}); admitted {
		t.Fatal("foreign transaction admitted")
	}
	if err := journal.Acknowledge(t.Context(), request.ID); err == nil {
		t.Fatal("unresolved signed work acknowledged")
	}
	// A high next-block base fee forces a real replacement while relay bytes remain unmined.
	if err := chain.Client().CallContext(t.Context(), nil, "anvil_setNextBlockBaseFeePerGas", hexutil.EncodeBig(big.NewInt(40_000_000_000))); err != nil {
		t.Fatal(err)
	}
	mine()
	select {
	case <-relay.persisted:
	case <-time.After(5 * time.Second):
		t.Fatal("replacement absent")
	}
	cancel()
	select {
	case <-result:
	case <-time.After(time.Second):
		t.Fatal("SDK caller cancellation blocked")
	}
	stop()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("sender failed to drain")
	}
	relay.mu.Lock()
	attempts := append([]*types.Transaction(nil), relay.sent...)
	fault := relay.fault
	relay.mu.Unlock()
	if fault != nil {
		t.Fatal(fault)
	}
	if attempts[0].Hash() == attempts[len(attempts)-1].Hash() {
		t.Fatal("expected a fee replacement, got exact rebroadcast")
	}
	if attempts[0].Nonce() != attempts[len(attempts)-1].Nonce() {
		t.Fatal("replacement changed nonce")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// Restart from disk. No workflow admission is allowed before SDK New restores Hold.
	store, err = state.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	restarted := txmanager.New(relay, signing, big.NewInt(1337), config, logr.Discard())
	recovered, err := submission.New(relay, signing.Address(), big.NewInt(1337), store, submission.Config{PollInterval: time.Millisecond}, Transport{Manager: restarted, Solver: "test"}, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if recovered.PendingID() != request.ID || restarted.LaneReady() {
		t.Fatal("restart lost signer fence")
	}
	if err := restarted.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := chain.SendTransaction(t.Context(), attempts[len(attempts)-1]); err != nil {
		t.Fatal(err)
	}
	mine()
	verifyCtx, verifyCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer verifyCancel()
	terminal, err := recovered.Send(verifyCtx, request)
	if err != nil || !terminal.Confirmed || terminal.Execution == nil {
		t.Fatalf("canonical recovery: %+v %v", terminal, err)
	}
	if restarted.LaneReady() {
		t.Fatal("terminal receipt released lane before durable acknowledgement")
	}
	if err := store.Save("keeper.progress", map[string]string{"id": request.ID}); err != nil {
		t.Fatal(err)
	}
	if err := recovered.Acknowledge(verifyCtx, "foreign-id"); err == nil {
		t.Fatal("foreign acknowledgement released lane")
	}
	failedAck, err := submission.New(relay, signing.Address(), big.NewInt(1337), failingAcknowledgementStore{store}, submission.Config{}, Transport{Manager: restarted, Solver: "test"}, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if err := failedAck.Acknowledge(verifyCtx, request.ID); err == nil || restarted.LaneReady() {
		t.Fatal("failed acknowledgement released lane")
	}
	var retained any
	if found, err := store.Load("txmanager.active.v1", &retained); err != nil || !found {
		t.Fatal("failed acknowledgement lost durable journal", err)
	}
	if err := recovered.Acknowledge(verifyCtx, request.ID); err != nil {
		t.Fatal(err)
	}
	if !restarted.LaneReady() || recovered.PendingID() != "" {
		t.Fatal("acknowledged lane not released")
	}
	var active any
	if found, err := store.Load("txmanager.active.v1", &active); err != nil || found {
		t.Fatal("acknowledged journal still present", err)
	}
}

// The only RPC endpoint is a child process listening on loopback with a throwaway key.
func journalAnvil(t *testing.T, account common.Address) (*ethclient.Client, func()) {
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

// The durable SDK save fails, so the real transport must never reach the relay.
func TestSDKJournalPersistenceFailureBlocksBroadcast(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signing, err := signer.NewFromHexKey(hexutil.Encode(crypto.FromECDSA(key)))
	if err != nil {
		t.Fatal(err)
	}
	chain, _ := journalAnvil(t, signing.Address())
	directory := t.TempDir()
	store, err := state.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	relay := &journalRelay{Client: chain, directory: directory, persisted: make(chan struct{}, 4)}
	engine := txmanager.New(relay, signing, big.NewInt(1337), txmanager.Config{MaxFeeGwei: 100, ShutdownTimeout: time.Millisecond}, logr.Discard())
	if err := engine.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	journal, err := submission.New(relay, signing.Address(), big.NewInt(1337), store, submission.Config{}, Transport{Manager: engine, Solver: "test"}, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { engine.Start(ctx); close(done) }()
	t.Cleanup(func() { stop(); <-done })
	result, err := journal.Send(t.Context(), submission.Request{ID: "disk-failure", To: common.HexToAddress("0x1234")})
	if err == nil || !result.Pending || engine.LaneReady() {
		t.Fatal("persistence failure lost durable signer fence", err)
	}
	relay.mu.Lock()
	defer relay.mu.Unlock()
	if len(relay.sent) != 0 {
		t.Fatal("broadcast despite journal failure")
	}
}

type failingAcknowledgementStore struct{ *state.Store }

func (failingAcknowledgementStore) Delete(string) error {
	return errors.New("synthetic journal delete failure")
}
