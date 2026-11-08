//go:build integration

package merkle

import (
	"context"
	"math/big"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	txmanager "github.com/symbioticfi/merkle-executor/sdk/transaction"
)

type fundingRelay struct {
	*ownedRelay

	funded atomic.Bool
}

func (r *fundingRelay) TransactionSenderBalanceAt(ctx context.Context, account common.Address, block *big.Int) (*big.Int, error) {
	if !r.funded.Load() {
		return new(big.Int), nil
	}
	return r.BalanceAt(ctx, account, block)
}

func TestSDKRAMFundingFailureReleasesUnsignedWork(t *testing.T) {
	signing := ramSigning(t)
	chain, mine := senderAnvil(t, signing.Address())
	relay := &fundingRelay{ownedRelay: &ownedRelay{Client: chain, observed: make(chan struct{}, 64)}}
	engine := txmanager.New(relay, signing, big.NewInt(1337), txmanager.Config{MaxFeeGwei: 100, PollInterval: time.Millisecond, ReplacementInterval: time.Second, ShutdownTimeout: time.Millisecond}, logr.Discard())
	sender := engine.Sender("test")
	runRAMManager(t, engine)
	request := txmanager.ExecutionRequest{To: common.HexToAddress("0x1234")}
	err := sender.Submit(t.Context(), request)
	if err == nil || !strings.Contains(err.Error(), "insufficient transaction funding") || sender.Busy() || len(relay.snapshot()) != 0 {
		t.Fatalf("unsigned funding failure retained work: err=%v busy=%v sends=%d", err, sender.Busy(), len(relay.snapshot()))
	}
	deadline := time.Now().Add(time.Second)
	for (!engine.Available() || !engine.LaneReady()) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !engine.Available() || !engine.LaneReady() {
		t.Fatal("unsigned funding failure did not release the shared lane")
	}
	relay.funded.Store(true)
	if err := sender.Submit(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	awaitBroadcast(t, relay.ownedRelay)
	attempts := relay.snapshot()
	if len(attempts) != 1 || attempts[0].Nonce() != 0 {
		t.Fatalf("funded retry did not start fresh: attempts=%d", len(attempts))
	}
	if err := chain.SendTransaction(t.Context(), attempts[0]); err != nil {
		t.Fatal(err)
	}
	mine()
	mine()
	result := awaitRAMTerminal(t, sender)
	if result.Hash != attempts[0].Hash() || sender.Busy() || !engine.Available() {
		t.Fatalf("funded retry did not complete canonically: %+v", result)
	}
}
