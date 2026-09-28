//go:build integration

package chain

import (
	"bytes"
	"math/big"
	"net"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
)

// TestAnvilPinnedReadsAndBlockOverrides runs the pinned balance read, the next-block estimate and the
// block-overrides probe against a real EVM, so the probe bytecode and the EIP-1898 parameter shapes
// are checked by a node rather than by the test's own stand-ins.
func TestAnvilPinnedReadsAndBlockOverrides(t *testing.T) {
	client := startAnvil(t)
	raw := client.Client.Client()
	account := common.HexToAddress("0x5641000000000000000000000000000000005480")

	// Balances move through real transfers from anvil's first unlocked dev account, so each value is
	// part of a mined block's state (anvil_setBalance rewrites the head's state in place instead).
	devAccount := common.HexToAddress("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266")
	transfer := func(wei int64) *types.Receipt {
		t.Helper()
		tx := map[string]any{"from": devAccount, "to": account, "value": hexutil.EncodeBig(big.NewInt(wei))}
		var hash common.Hash
		if err := raw.CallContext(t.Context(), &hash, "eth_sendTransaction", tx); err != nil {
			t.Fatalf("eth_sendTransaction: %v", err)
		}
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			if receipt, err := client.TransactionReceipt(t.Context(), hash); err == nil {
				return receipt
			}
		}
		t.Fatalf("transfer %s was not mined", hash.Hex())
		return nil
	}

	pinned := transfer(1_000) // the account holds 1000 wei in this receipt's block
	transfer(1_000)

	t.Run("balance pinned by number and hash", func(t *testing.T) {
		for _, block := range []rpc.BlockNumberOrHash{
			rpc.BlockNumberOrHashWithNumber(rpc.BlockNumber(pinned.BlockNumber.Int64())),
			rpc.BlockNumberOrHashWithHash(pinned.BlockHash, false),
			rpc.BlockNumberOrHashWithHash(pinned.BlockHash, true),
		} {
			balance, readErr := client.ReadBalanceAtBlock(t.Context(), account, block)
			if readErr != nil {
				t.Fatalf("ReadBalanceAtBlock(%s): %v", block.String(), readErr)
			}
			if balance.Int64() != 1_000 {
				t.Fatalf("ReadBalanceAtBlock(%s) = %s, want the pinned 1000 rather than the later 2000", block.String(), balance)
			}
		}
	})

	t.Run("missing pin is not found", func(t *testing.T) {
		for _, block := range []rpc.BlockNumberOrHash{
			rpc.BlockNumberOrHashWithNumber(rpc.BlockNumber(pinned.BlockNumber.Int64() + 100)),
			rpc.BlockNumberOrHashWithHash(common.HexToHash("0x11"), false),
		} {
			if _, readErr := client.ReadBalanceAtBlock(t.Context(), account, block); !errors.Is(readErr, ethereum.NotFound) {
				t.Fatalf("ReadBalanceAtBlock(%s) error = %v, want ethereum.NotFound", block.String(), readErr)
			}
		}
	})

	t.Run("next-block estimate", func(t *testing.T) {
		head, headErr := client.HeaderByNumber(t.Context(), nil)
		if headErr != nil {
			t.Fatal(headErr)
		}
		to := common.HexToAddress("0x1111111111111111111111111111111111111111")
		gas, estimateErr := client.EstimateGasWithBlockOverrides(t.Context(), ethereum.CallMsg{From: account, To: &to, Value: big.NewInt(1)},
			head.Number, NextBlockOverrides(head, 12*time.Second))
		if estimateErr != nil {
			t.Fatalf("EstimateGasWithBlockOverrides: %v", estimateErr)
		}
		if gas != 21_000 {
			t.Fatalf("gas = %d, want 21000", gas)
		}
	})

	t.Run("probe detects honoured overrides", func(t *testing.T) {
		supported, probeErr := client.ProbeBlockOverrides(t.Context(), 12*time.Second)
		if probeErr != nil || !supported {
			t.Fatalf("ProbeBlockOverrides = %t, %v; want supported", supported, probeErr)
		}
	})

	// These are the contexts an upstream that ignores the overrides would execute the probe in.
	t.Run("probe code reverts outside the overridden block", func(t *testing.T) {
		head, headErr := client.HeaderByNumber(t.Context(), nil)
		if headErr != nil {
			t.Fatal(headErr)
		}
		next := NextBlockOverrides(head, 12*time.Second)
		state := map[common.Address]ethereum.OverrideAccount{
			blockOverridesProbeTarget: {Code: blockOverridesProbeCode(next.Number.Uint64(), next.Time)},
		}
		for name, overrides := range map[string]ethereum.BlockOverrides{
			"head block":  {Number: head.Number, Time: head.Time},
			"number only": {Number: next.Number},
			"time only":   {Time: next.Time},
		} {
			_, estimateErr := client.estimateGasWithOverrides(t.Context(), ethereum.CallMsg{To: &blockOverridesProbeTarget}, head.Number, state, overrides)
			if !IsExecutionReverted(estimateErr) || IsBlockOverridesUnsupported(estimateErr) {
				t.Fatalf("%s: estimate error = %v, want a revert", name, estimateErr)
			}
		}
	})
}

func startAnvil(t *testing.T) *Client {
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
	cmd := exec.CommandContext(t.Context(), anvil, "--silent", "--chain-id", "31337", "--port", strconv.Itoa(port))
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start anvil: %v", err)
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

	url := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		client, dialErr := Dial(t.Context(), []string{url}, "", "", testMulticall, time.Second)
		if dialErr == nil {
			t.Cleanup(client.Close)
			return client
		}
		select {
		case exitErr := <-done:
			t.Fatalf("anvil exited during startup: %v\n%s", exitErr, output.String())
		case <-time.After(50 * time.Millisecond):
		}
	}
	t.Fatalf("anvil did not start\n%s", output.String())
	return nil
}
