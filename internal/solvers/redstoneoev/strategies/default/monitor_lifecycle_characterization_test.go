package defaultstrategy

import (
	"context"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

func TestAPIMonitorPublishesInitialAndPreservesLastGoodSnapshot(t *testing.T) {
	loan := common.HexToAddress("0x0000000000000000000000000000000000000011")
	collateral := common.HexToAddress("0x0000000000000000000000000000000000000022")
	oracle := common.HexToAddress("0x0000000000000000000000000000000000000033")
	irm := common.HexToAddress("0x0000000000000000000000000000000000000044")
	borrower := common.HexToAddress("0x0000000000000000000000000000000000000055")
	lltv := mustBig("860000000000000000")
	marketID, err := deriveMarketID(MarketParams{LoanToken: loan, CollateralToken: collateral, Oracle: oracle, Irm: irm, Lltv: lltv})
	if err != nil {
		t.Fatal(err)
	}

	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "refresh unavailable", http.StatusServiceUnavailable)
			return
		}
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Errorf("read GraphQL request: %v", readErr)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(string(body), "MorphoDiscoverMarkets"):
			_, _ = fmt.Fprintf(w, `{"data":{"markets":{"items":[{
				"marketId":%q,"oracleAddress":%q,"irmAddress":%q,"lltv":%q,
				"loanAsset":{"address":%q},"collateralAsset":{"address":%q},
				"state":{"blockNumber":"101","borrowAssets":"4730000068","borrowShares":"4729999932892591",
				"supplyAssets":"100000000068","supplyShares":"100000000000000000","timestamp":"1781243340","price":"5000000000000000000000000000"}
			}]}}}`, marketID.Hex(), oracle.Hex(), irm.Hex(), lltv.String(), loan.Hex(), collateral.Hex())
		case strings.Contains(string(body), "MorphoPositionsByMarket"):
			_, _ = fmt.Fprintf(w, `{"data":{"marketPositions":{"items":[{
				"user":{"address":%q},"market":{"marketId":%q},
				"state":{"borrowShares":"1685600000000000","collateral":"1000000000000000000"},"healthFactor":1.1
			}]}}}`, borrower.Hex(), marketID.Hex())
		default:
			http.Error(w, "unexpected GraphQL operation", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	adapter := types.AdapterSnapshot{
		Loan:       loan,
		Redeemable: []types.RedeemableSnapshot{{Asset: collateral}},
	}
	monitor := newAPIMonitor(logr.Discard(), Config{
		MorphoAPIURL: server.URL, MaxTrackedPositions: 10, DiscoveryMaxHealthFactor: 1.3,
		MonitorPoll: time.Hour,
	}, 1, func() (types.AdapterSnapshot, bool) { return adapter, true })
	if initial := monitor.snapshot(); initial == nil || !initial.updatedAt.IsZero() || len(initial.markets) != 0 {
		t.Fatalf("initial unpublished snapshot = %+v", initial)
	}

	monitor.refresh(t.Context())
	published := cloneSnapshot(monitor.snapshot())
	if published.updatedAt.IsZero() || published.block != 101 || published.blockTime != 1_781_243_340 {
		t.Fatalf("published API epoch = block %d time %d updatedAt=%v", published.block, published.blockTime, published.updatedAt)
	}
	if len(published.markets) != 1 || len(published.positions[marketID]) != 1 {
		t.Fatalf("published API snapshot markets/positions = %d/%d", len(published.markets), len(published.positions[marketID]))
	}
	if got := published.positions[marketID][borrower]; got.BorrowShares.String() != "1685600000000000" || got.Collateral.String() != "1000000000000000000" {
		t.Fatalf("published API position = %+v", got)
	}

	fail.Store(true)
	monitor.refresh(t.Context())
	if got := monitor.snapshot(); !reflect.DeepEqual(got, published) {
		t.Fatalf("failed API refresh changed last-good publication\n got: %+v\nwant: %+v", got, published)
	}
}

type monitorCharacterizationReader struct {
	Reader

	fail      atomic.Bool
	headCalls atomic.Int32
	morpho    common.Address
	marketID  common.Hash
	params    MarketParams
	market    MarketInfo
	price     *big.Int
	borrower  common.Address
	position  morpho.PositionState
}

func (r *monitorCharacterizationReader) ReadHead(context.Context) (number uint64, timestamp uint64, err error) {
	if r.fail.Load() {
		return 0, 0, errors.New("head unavailable")
	}
	r.headCalls.Add(1)
	return 101, 1_781_243_340, nil
}

func (r *monitorCharacterizationReader) ReadCallbackMorpho(context.Context, common.Address) (common.Address, error) {
	return r.morpho, nil
}

func (r *monitorCharacterizationReader) ResolveParams(context.Context, common.Address, []common.Hash) (map[common.Hash]MarketParams, error) {
	return map[common.Hash]MarketParams{r.marketID: r.params}, nil
}

func (r *monitorCharacterizationReader) ReadTestMarketStates(context.Context, common.Address, map[common.Hash]MarketParams) (map[common.Hash]MarketInfo, map[common.Hash]*big.Int, error) {
	return map[common.Hash]MarketInfo{r.marketID: r.market}, map[common.Hash]*big.Int{r.marketID: cloneBig(r.price)}, nil
}

func (r *monitorCharacterizationReader) ReadTestPositions(context.Context, common.Address, map[common.Hash]MarketInfo, []common.Address) (map[common.Hash]map[common.Address]morpho.PositionState, error) {
	return map[common.Hash]map[common.Address]morpho.PositionState{
		r.marketID: {r.borrower: morpho.ClonePositionState(r.position)},
	}, nil
}

func TestTestMonitorPublishesInitialAndPreservesLastGoodSnapshot(t *testing.T) {
	marketID := common.BigToHash(big.NewInt(1))
	loan := common.BigToAddress(big.NewInt(1))
	collateral := common.BigToAddress(big.NewInt(2))
	borrower := common.BigToAddress(big.NewInt(3))
	reader := &monitorCharacterizationReader{
		morpho: common.BigToAddress(big.NewInt(4)), marketID: marketID, borrower: borrower,
		params: MarketParams{LoanToken: loan, CollateralToken: collateral, Oracle: common.BigToAddress(big.NewInt(5)), Lltv: mustBig("860000000000000000")},
		market: MarketInfo{
			Params: MarketParams{LoanToken: loan, CollateralToken: collateral, Oracle: common.BigToAddress(big.NewInt(5)), Lltv: mustBig("860000000000000000")},
			State:  goldenMarket(),
		},
		price: mustBig("1550000000000000000000000000"), position: goldenBorrower(),
	}
	monitor, err := newTestMonitor(reader, logr.Discard(), Config{MonitorPoll: time.Hour}, characterizationCallback,
		func() (types.AdapterSnapshot, bool) {
			return types.AdapterSnapshot{Loan: loan, Redeemable: []types.RedeemableSnapshot{{Asset: collateral}}}, true
		},
		&TestMonitorConfig{Markets: []common.Hash{marketID}, Positions: []common.Address{borrower}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if initial := monitor.snapshot(); initial == nil || !initial.updatedAt.IsZero() || len(initial.markets) != 0 {
		t.Fatalf("initial unpublished test snapshot = %+v", initial)
	}

	monitor.refresh(t.Context())
	published := cloneSnapshot(monitor.snapshot())
	if published.updatedAt.IsZero() || published.block != 101 || published.blockTime != 1_781_243_340 {
		t.Fatalf("published test epoch = block %d time %d updatedAt=%v", published.block, published.blockTime, published.updatedAt)
	}
	if len(published.markets) != 1 || len(published.positions[marketID]) != 1 || published.prices[marketID].Cmp(reader.price) != 0 {
		t.Fatalf("published test snapshot = %+v", published)
	}
	if reader.headCalls.Load() != 2 {
		t.Fatalf("coherent test refresh head reads = %d, want 2", reader.headCalls.Load())
	}

	reader.fail.Store(true)
	monitor.refresh(t.Context())
	if got := monitor.snapshot(); !reflect.DeepEqual(got, published) {
		t.Fatalf("failed test refresh changed last-good publication\n got: %+v\nwant: %+v", got, published)
	}
}

type blockingMonitorSource struct {
	refreshCalls atomic.Int32
	blocked      chan struct{}
}

func (m *blockingMonitorSource) refresh(ctx context.Context) {
	if m.refreshCalls.Add(1) == 1 {
		return
	}
	select {
	case m.blocked <- struct{}{}:
	default:
	}
	<-ctx.Done()
}

func (m *blockingMonitorSource) run(ctx context.Context) {
	runMonitor(ctx, time.Millisecond, m.refresh)
}

func (*blockingMonitorSource) snapshot() *snapshot { return &snapshot{} }

func (*blockingMonitorSource) candidates(types.AuctionSnapshot, uint64, types.AdapterSnapshot) []evalItem {
	return nil
}

type livenessDecisionReader struct {
	Reader

	calls chan int32
	count atomic.Int32
}

func (r *livenessDecisionReader) ReadNativeBalance(context.Context, common.Address) (*big.Int, error) {
	call := r.count.Add(1)
	select {
	case r.calls <- call:
	default:
	}
	return big.NewInt(1), nil
}

func TestStrategyRunKeepsRefreshLoopsIndependentAndJoinsOnCancellation(t *testing.T) {
	monitor := &blockingMonitorSource{blocked: make(chan struct{}, 1)}
	reader := &livenessDecisionReader{calls: make(chan int32, 16)}
	strategy := &Strategy{
		cfg: Config{MonitorPoll: time.Millisecond}, reader: reader, mon: monitor,
		callback: characterizationCallback, log: logr.Discard(),
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		strategy.Run(ctx)
		close(done)
	}()

	awaitCallAtLeast(t, reader.calls, 1, "initial callback-state publication")
	awaitSignal(t, monitor.blocked, "periodic monitor refresh to block")
	awaitCallAtLeast(t, reader.calls, 2, "callback-state refresh while monitor is blocked")

	cancel()
	awaitSignal(t, done, "Strategy.Run cancellation join")
	if monitor.refreshCalls.Load() < 2 {
		t.Fatalf("monitor refresh calls = %d, want initial plus blocked periodic refresh", monitor.refreshCalls.Load())
	}
}

func awaitCallAtLeast(t *testing.T, calls <-chan int32, want int32, description string) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case got := <-calls:
			if got >= want {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %s", description)
		}
	}
}

func awaitSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", description)
	}
}
