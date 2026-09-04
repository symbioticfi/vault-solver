package redstoneoev

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"

	"github.com/symbioticfi/vault-solver/internal/chain"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/decision"
)

type lifecycleStateReader struct {
	executor ExecutorState
	adapter  strategytypes.AdapterSnapshot
}

func (lifecycleStateReader) ReadNativeBalance(context.Context, common.Address) (*big.Int, error) {
	return big.NewInt(1_000_000_000_000_000_000), nil
}

func (r lifecycleStateReader) ReadExecutorState(
	context.Context,
	common.Address,
	common.Address,
) (ExecutorState, error) {
	return r.executor, nil
}

func (r lifecycleStateReader) ReadAdapterSnapshot(
	context.Context,
	common.Address,
	common.Address,
) (strategytypes.AdapterSnapshot, error) {
	return r.adapter, nil
}

func (lifecycleStateReader) ReadGasPrices(
	context.Context,
	strategytypes.AdapterSnapshot,
	time.Time,
) (*liquidlanegas.PriceSnapshot, error) {
	return nil, nil
}

type lifecycleBlockingStrategy struct {
	handlerStarted chan struct{}
	handlerRelease chan struct{}
	runCanceled    chan struct{}
	runRelease     chan struct{}
}

func (s *lifecycleBlockingStrategy) Run(ctx context.Context) {
	<-ctx.Done()
	s.runCanceled <- struct{}{}
	<-s.runRelease
}

func (*lifecycleBlockingStrategy) Snapshot(
	strategytypes.AuctionSnapshot,
	time.Time,
	strategytypes.AdapterSnapshot,
) strategytypes.MarketFacts {
	return strategytypes.MarketFacts{UpdatedAt: time.Now()}
}

func (s *lifecycleBlockingStrategy) DecideBid(
	context.Context,
	strategytypes.BidInput,
) (strategytypes.BidOutput, error) {
	s.handlerStarted <- struct{}{}
	<-s.handlerRelease
	return strategytypes.BidOutput{Decision: strategytypes.DecisionSkip, Reason: "released"}, nil
}

func TestRunWaitsForAuctionHandlers(t *testing.T) {
	s, _ := seededSolver(t)
	s.cfg.OpsPoll = time.Hour
	s.stateRefreshCh = make(chan struct{}, 1)
	s.reader = lifecycleStateReader{
		executor: ExecutorState{Nonce: big.NewInt(7), Deposit: mustBig("100000000000000000"), Locked: false},
		adapter:  seedAdapterSnapshot(),
	}
	s.chain = newLifecycleChain(t)

	strategy := &lifecycleBlockingStrategy{
		handlerStarted: make(chan struct{}, 1),
		handlerRelease: make(chan struct{}, 1),
		runCanceled:    make(chan struct{}, 1),
		runRelease:     make(chan struct{}, 1),
	}
	s.planner = strategy
	s.facts = strategy

	a := decodeAuction(t)
	a.Timestamp = time.Now().UnixMilli()
	connectionStopped := make(chan struct{}, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			_ = conn.Close()
			connectionStopped <- struct{}{}
		}()
		if _, _, err = conn.ReadMessage(); err != nil {
			return
		}
		if err = conn.WriteMessage(websocket.TextMessage, marshal(a)); err != nil {
			return
		}
		for {
			if _, _, err = conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)

	s.ws = newWSClient(wsConfig{
		URL:          "ws" + strings.TrimPrefix(server.URL, "http"),
		APIKey:       "test",
		Topics:       []string{"test"},
		PingInterval: time.Hour,
		MsgTimeout:   time.Hour,
		RotateAfter:  time.Hour,
	}, logr.Discard(), s.handleMessage)

	ctx, cancel := context.WithCancel(t.Context())
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx) }()
	defer func() {
		cancel()
		nonblockingSignal(strategy.runRelease)
		nonblockingSignal(strategy.handlerRelease)
	}()

	waitSignal(t, strategy.handlerStarted, "auction handler start")
	cancel()
	waitSignal(t, connectionStopped, "websocket pump shutdown")
	waitSignal(t, strategy.runCanceled, "strategy cancellation")
	strategy.runRelease <- struct{}{}

	select {
	case err := <-runDone:
		t.Fatalf("Run returned while auction handler was blocked: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	strategy.handlerRelease <- struct{}{}
	select {
	case err := <-runDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after auction handler stopped")
	}
}

func newLifecycleChain(t *testing.T) *chain.Client {
	t.Helper()
	header, err := json.Marshal(&ethtypes.Header{
		UncleHash:   ethtypes.EmptyUncleHash,
		TxHash:      ethtypes.EmptyTxsHash,
		ReceiptHash: ethtypes.EmptyReceiptsHash,
		Difficulty:  big.NewInt(1),
		Number:      big.NewInt(100),
		GasLimit:    2_000_000,
		Time:        uint64(time.Now().Unix()),
	})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "eth_chainId":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(request.ID) + `,"result":"0xaa36a7"}`))
		case "eth_getBlockByNumber":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(request.ID) + `,"result":` + string(header) + `}`))
		default:
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(request.ID) + `,"error":{"code":-32601,"message":"method not found"}}`))
		}
	}))
	t.Cleanup(server.Close)

	client, err := chain.Dial(
		t.Context(),
		[]string{server.URL},
		"",
		"0x0000000000000000000000000000000000000001",
		logr.Discard(),
	)
	if err != nil {
		t.Fatalf("dial lifecycle chain: %v", err)
	}
	t.Cleanup(client.Close)
	return client
}

func waitSignal(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func nonblockingSignal(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
