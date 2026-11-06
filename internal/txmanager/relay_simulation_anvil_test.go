//go:build integration

package txmanager

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

// simulatingRelay is a write RPC in front of Anvil that answers eth_sendRawTransaction the way
// MEV Blocker does when its pending-block simulation reverts, without forwarding the bytes.
type simulatingRelay struct {
	upstream string
	reject   atomic.Bool
	rejected atomic.Int32
}

func (r *simulatingRelay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var call struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	if json.Unmarshal(body, &call) == nil && call.Method == "eth_sendRawTransaction" && r.reject.Load() {
		r.rejected.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(call.ID) +
			`,"error":{"code":-32000,"message":"Failed in pending block with: Reverted"}}`))
		return
	}
	forward, err := http.NewRequestWithContext(req.Context(), http.MethodPost, r.upstream, bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	forward.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(forward)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func TestAnvilRelaySimulationRevertReleasesNonce(t *testing.T) {
	rpcClient, ethClient, endpoint := startAnvilWithoutMining(t)
	mineAnvilBlock(t, rpcClient)
	relay := &simulatingRelay{upstream: endpoint}
	relay.reject.Store(true)
	relayServer := httptest.NewServer(relay)
	t.Cleanup(relayServer.Close)

	backend, err := chain.Dial(t.Context(), []string{endpoint}, relayServer.URL, common.Address{}.Hex(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(backend.Close)
	sgnr := anvilSigner(t)
	m := New(backend, sgnr, big.NewInt(31337), Config{
		Confirmations: 1, MaxFeeGwei: 100, PollInterval: 20 * time.Millisecond,
		PendingTimeout: 3 * time.Second, // an unrecognized rejection holds the lane until abandonment
	}, logr.Discard())
	if err := m.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	startManagerForTest(t, m)
	req := Request{To: common.HexToAddress("0xdead"), GasLimit: 21_000, Label: "rfq-fill", Solver: "rfq"}

	rejected := m.Send(t.Context(), req)
	var revert *ExecutionRevertError
	if rejected.Outcome != OutcomeSubmissionError || !errors.As(rejected.Err, &revert) || revert.Data != nil {
		t.Fatalf("relay rejection = %v err %v, want a submission error wrapping a dataless revert",
			rejected.Outcome, rejected.Err)
	}
	if relay.rejected.Load() != 1 || !m.Available() {
		t.Fatalf("rejections=%d available=%v, want one rejection and a free lane", relay.rejected.Load(), m.Available())
	}
	assertAnvilPoolNonceAbsent(t, rpcClient, sgnr.Address(), 0)

	relay.reject.Store(false)
	result, accepted := m.SendAsync(t.Context(), req)
	if !accepted {
		t.Fatal("follow-up send was not admitted on the released lane")
	}
	pooled := waitForPoolTransaction(t, rpcClient, sgnr.Address(), 0, func(poolTransaction) bool { return true })
	mineAnvilBlock(t, rpcClient)
	mineAnvilBlock(t, rpcClient)
	assertAnvilReconciliationOutcome(t, waitForTxResult(t, result), common.HexToHash(pooled.Hash))
	if relay.rejected.Load() != 1 {
		t.Fatalf("rejected bytes were rebroadcast: rejections=%d", relay.rejected.Load())
	}
	assertAnvilCanonicalTransaction(t, ethClient, common.HexToHash(pooled.Hash), 0)
}
