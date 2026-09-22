//go:build integration

package delegation

import (
	"context"
	"encoding/json"
	"math/big"
	"net"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-logr/logr"
	"github.com/holiman/uint256"
	"github.com/symbioticfi/vault-solver/api/bindings/solverdelegate"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// Public, unfunded-outside-Anvil test keys.
const (
	anvilPrimaryKey   = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	anvilAuxiliaryKey = "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
)

type observedBackend struct {
	*ethclient.Client

	sent chan *types.Transaction
}

func (b *observedBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if err := b.Client.SendTransaction(ctx, tx); err != nil {
		return err
	}
	b.sent <- tx
	return nil
}

func TestAnvilParallelDelegatedCalls(t *testing.T) {
	for _, cancellation := range []bool{false, true} {
		name := "preserve caller and value"
		if cancellation {
			name = "cancel both independent nonces"
		}
		t.Run(name, func(t *testing.T) { testAnvilParallel(t, cancellation) })
	}
}

func testAnvilParallel(t *testing.T, cancellation bool) {
	t.Helper()
	rpcClient, client := delegateAnvil(t)
	primary, err := signer.NewFromHexKey(anvilPrimaryKey)
	if err != nil {
		t.Fatal(err)
	}
	auxiliary, err := signer.NewFromHexKey(anvilAuxiliaryKey)
	if err != nil {
		t.Fatal(err)
	}
	implementation := provisionDelegate(t, rpcClient, client, primary, auxiliary.Address())
	forwarder, err := New(client, primary.Address(), implementation, []common.Address{auxiliary.Address()})
	if err != nil {
		t.Fatal(err)
	}
	backend := &observedBackend{Client: client, sent: make(chan *types.Transaction, 20)}
	cfg := txmanager.Config{MaxFeeGwei: 100, TipGwei: 1, PollInterval: 10 * time.Millisecond,
		ReplacementInterval: time.Hour, PendingTimeout: time.Hour, ShutdownTimeout: 50 * time.Millisecond}
	if cancellation {
		cfg.PendingTimeout = 200 * time.Millisecond
	}
	cfg.CancellationRecipient = new(common.Address)
	first := txmanager.New(backend, primary, big.NewInt(31337), cfg, logr.Discard())
	cfg.CancellationRecipient = nil
	cfg.Preparer = forwarder
	second := txmanager.New(backend, auxiliary, big.NewInt(31337), cfg, logr.Discard())
	pool, err := txmanager.NewPool(first, second)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { pool.Start(ctx); close(done) }()
	t.Cleanup(func() { stop(); <-done })

	target := common.HexToAddress("0x4444444444444444444444444444444444444444")
	// Test-only recorder: storage[calldataload(0)] = caller();
	// storage[calldataload(0) + 16] = callvalue().
	if err := rpcClient.CallContext(t.Context(), nil, "anvil_setCode", target, "0x335f3555345f356010015500"); err != nil {
		t.Fatal(err)
	}
	firstResult, ok := pool.SendAsync(ctx, txmanager.Request{To: target, Data: common.LeftPadBytes([]byte{1}, 32), Value: big.NewInt(7), Label: "first"})
	if !ok {
		t.Fatal("primary not admitted")
	}
	firstTx := awaitDelegatedSend(t, backend.sent)
	secondResult, ok := pool.SendAsync(ctx, txmanager.Request{To: target, Data: common.LeftPadBytes([]byte{2}, 32), Value: big.NewInt(11), Label: "second"})
	if !ok {
		t.Fatal("auxiliary not admitted while primary pending")
	}
	secondTx := awaitDelegatedSend(t, backend.sent)
	if firstTx.Nonce() != 3 || secondTx.Nonce() != 0 || *firstTx.To() != target || *secondTx.To() != primary.Address() {
		t.Fatalf("wrong senders/targets/nonces: primary=%v auxiliary=%v", firstTx, secondTx)
	}
	if cancellation {
		for range 2 {
			tx := awaitDelegatedSend(t, backend.sent)
			from, err := types.Sender(types.LatestSignerForChainID(big.NewInt(31337)), tx)
			if err != nil {
				t.Fatal(err)
			}
			wantTo := from
			if from == primary.Address() {
				wantTo = common.Address{}
			}
			if tx.To() == nil || *tx.To() != wantTo || tx.Gas() != 21_000 || tx.Value().Sign() != 0 || len(tx.Data()) != 0 {
				t.Fatalf("cancellation was wrapped or executed delegated code: %v", tx)
			}
		}
	}
	if err := rpcClient.CallContext(t.Context(), nil, "anvil_mine", 1); err != nil {
		t.Fatal(err)
	}
	wantOutcome := txmanager.OutcomeConfirmed
	if cancellation {
		wantOutcome = txmanager.OutcomeCancelled
	}
	for _, result := range []<-chan txmanager.Result{firstResult, secondResult} {
		select {
		case got := <-result:
			if got.Outcome != wantOutcome || got.Receipt == nil || got.Receipt.Status != types.ReceiptStatusSuccessful {
				t.Fatalf("outcome = %+v, want %s with successful receipt", got, wantOutcome)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("transaction did not complete")
		}
	}
	for _, slot := range []int64{1, 2} {
		got, err := client.StorageAt(t.Context(), target, common.BigToHash(big.NewInt(slot)), nil)
		if err != nil {
			t.Fatal(err)
		}
		wantCaller := primary.Address()
		if cancellation {
			wantCaller = common.Address{}
		}
		if common.BytesToAddress(got) != wantCaller {
			t.Fatalf("slot %d caller = %x, want %s", slot, got, wantCaller)
		}
		value, err := client.StorageAt(t.Context(), target, common.BigToHash(big.NewInt(slot+16)), nil)
		if err != nil {
			t.Fatal(err)
		}
		wantValue := int64(7)
		if slot == 2 {
			wantValue = 11
		}
		if cancellation {
			wantValue = 0
		}
		if new(big.Int).SetBytes(value).Int64() != wantValue {
			t.Fatalf("slot %d value = %x, want %d", slot, value, wantValue)
		}
	}
}

func provisionDelegate(t *testing.T, rpcClient *rpc.Client, client *ethclient.Client, primary signer.Signer, auxiliary common.Address) common.Address {
	t.Helper()
	var artifact compilerArtifact
	if err := json.Unmarshal([]byte(solverdelegate.Artifact), &artifact); err != nil {
		t.Fatal(err)
	}
	data := append(hexutil.MustDecode(artifact.Bytecode.Object), solverdelegate.NewSolver7702Delegate().PackConstructor([5]common.Address{auxiliary})...)
	deploy := types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(31337), Nonce: 0, GasTipCap: big.NewInt(1e9), GasFeeCap: big.NewInt(100e9), Gas: 1_000_000, Data: data})
	signed, err := primary.SignTx(t.Context(), deploy, big.NewInt(31337))
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SendTransaction(t.Context(), signed); err != nil {
		t.Fatal(err)
	}
	if err := rpcClient.CallContext(t.Context(), nil, "anvil_mine", 1); err != nil {
		t.Fatal(err)
	}
	receipt, err := client.TransactionReceipt(t.Context(), signed.Hash())
	if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
		t.Fatalf("deployment: receipt=%v error=%v", receipt, err)
	}
	key, err := crypto.HexToECDSA(anvilPrimaryKey)
	if err != nil {
		t.Fatal(err)
	}
	// The authority also broadcasts, so authorization uses tx nonce + 1.
	auth, err := types.SignSetCode(key, types.SetCodeAuthorization{ChainID: *uint256.NewInt(31337), Address: receipt.ContractAddress, Nonce: 2})
	if err != nil {
		t.Fatal(err)
	}
	authorize := types.NewTx(&types.SetCodeTx{ChainID: uint256.NewInt(31337), Nonce: 1, GasTipCap: uint256.NewInt(1e9), GasFeeCap: uint256.NewInt(100e9), Gas: 100_000, Value: new(uint256.Int), AuthList: []types.SetCodeAuthorization{auth}})
	signed, err = primary.SignTx(t.Context(), authorize, big.NewInt(31337))
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SendTransaction(t.Context(), signed); err != nil {
		t.Fatal(err)
	}
	if err := rpcClient.CallContext(t.Context(), nil, "anvil_mine", 1); err != nil {
		t.Fatal(err)
	}
	return receipt.ContractAddress
}

func awaitDelegatedSend(t *testing.T, sent <-chan *types.Transaction) *types.Transaction {
	t.Helper()
	select {
	case tx := <-sent:
		return tx
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for broadcast")
		return nil
	}
}

func delegateAnvil(t *testing.T) (*rpc.Client, *ethclient.Client) {
	t.Helper()
	anvil, err := exec.LookPath("anvil")
	if err != nil {
		t.Skip("anvil is not installed")
	}
	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), anvil, "--no-mining", "--silent", "--port", port)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	client, err := rpc.DialContext(t.Context(), "http://127.0.0.1:"+port)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var chainID string
		if err := client.CallContext(t.Context(), &chainID, "eth_chainId"); err == nil {
			return client, ethclient.NewClient(client)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("anvil did not start")
	return nil, nil
}
