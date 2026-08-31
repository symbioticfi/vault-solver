package defaultstrategy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

type apiLifecycleReader struct {
	native *big.Int
}

func (r *apiLifecycleReader) ReadNativeBalance(context.Context, common.Address) (*big.Int, error) {
	return characterizationBigCopy(r.native), nil
}

func (*apiLifecycleReader) ResolveParams(context.Context, common.Address, []common.Hash) (map[common.Hash]MarketParams, error) {
	return nil, errors.New("unexpected test-monitor params read")
}

func (*apiLifecycleReader) ReadHead(context.Context) (number uint64, timestamp uint64, err error) {
	return 0, 0, errors.New("unexpected test-monitor head read")
}

func (*apiLifecycleReader) ReadCallbackMorpho(context.Context, common.Address) (common.Address, error) {
	return common.Address{}, errors.New("unexpected test-monitor callback read")
}

func (*apiLifecycleReader) ReadTestMarketStates(context.Context, common.Address, map[common.Hash]MarketParams) (map[common.Hash]MarketInfo, map[common.Hash]*big.Int, error) {
	return nil, nil, errors.New("unexpected test-monitor market read")
}

func (*apiLifecycleReader) ReadTestPositions(context.Context, common.Address, map[common.Hash]MarketInfo, []common.Address) (map[common.Hash]map[common.Address]morpho.PositionState, error) {
	return nil, errors.New("unexpected test-monitor position read")
}

func TestAPIMonitorPublishesInitialAndPreservesLastGoodDecision(t *testing.T) {
	loan := characterizationLoan
	collateral := characterizationColl
	oracle := characterizationOracle
	irm := common.HexToAddress("0x0000000000000000000000000000000000000044")
	borrower := characterizationBorrower
	lltv := mustBig("860000000000000000")
	marketID, err := deriveMarketID(MarketParams{LoanToken: loan, CollateralToken: collateral, Oracle: oracle, Irm: irm, Lltv: lltv})
	if err != nil {
		t.Fatal(err)
	}

	var fail atomic.Bool
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
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

	adapter := characterizationAdapterSnapshot(big.NewInt(100_000_000_000))
	strategy, err := New(Config{
		MorphoAPIURL: server.URL, MaxTrackedPositions: 10, DiscoveryMaxHealthFactor: 1.3,
		BidWei: big.NewInt(500_000_000_000_000), CallbackAuthTTL: time.Minute,
		MonitorPoll: 2 * time.Millisecond, MaxStateAge: 10 * 365 * 24 * time.Hour,
		Sizing: SizingParams{AllowFullLiquidation: true},
	}, Deps{
		Reader: &apiLifecycleReader{native: big.NewInt(1_000_000_000_000_000_000)},
		Signer: newCharacterizationSigner(t), Log: logr.Discard(), ChainID: 11_155_111,
		Adapter: characterizationAdapter, Callback: characterizationCallback,
		LoadAdapterSnapshot: func() (types.AdapterSnapshot, bool) { return characterizationCloneAdapter(adapter), true },
	})
	if err != nil {
		t.Fatal(err)
	}
	input := characterizationLifecycleInput(adapter, oracle)
	if out, decideErr := strategy.DecideBid(t.Context(), input); decideErr != nil || out.Reason != types.SkipReasonStaleState {
		t.Fatalf("decision before initial publication = %+v, err=%v", out, decideErr)
	}

	cancel, done := runCharacterizationStrategy(t, strategy)
	baseline := awaitCharacterizationBid(t, strategy, input)
	withoutFrame := characterizationCloneBidInput(input)
	withoutFrame.Auction.ID += "-without-frame"
	withoutFrame.Auction.Prices = nil
	if out, decideErr := strategy.DecideBid(t.Context(), withoutFrame); decideErr != nil || out.Decision != types.DecisionSkip || out.Reason != types.SkipReasonNoLegs {
		t.Fatalf("production decision without settlement frame = %+v, err=%v", out, decideErr)
	}
	requestsBeforeFailure := requests.Load()
	fail.Store(true)
	characterizationAwaitCount(t, &requests, requestsBeforeFailure+1, "failed API refresh")
	afterFailure, err := strategy.DecideBid(t.Context(), input)
	if err != nil || afterFailure.Decision != types.DecisionBid ||
		afterFailure.BidAmount.Cmp(baseline.BidAmount) != 0 || !bytes.Equal(afterFailure.OperationData, baseline.OperationData) {
		t.Fatalf("decision after failed API refresh = %+v, err=%v", afterFailure, err)
	}
	cancel()
	awaitSignal(t, done, "API strategy shutdown")
}

type testLifecycleReader struct {
	marketID common.Hash
	market   MarketInfo
	price    *big.Int
	borrower common.Address
	position morpho.PositionState

	fail      atomic.Bool
	headCalls atomic.Int32
}

func (*testLifecycleReader) ReadNativeBalance(context.Context, common.Address) (*big.Int, error) {
	return big.NewInt(1_000_000_000_000_000_000), nil
}

func (r *testLifecycleReader) ResolveParams(context.Context, common.Address, []common.Hash) (map[common.Hash]MarketParams, error) {
	return map[common.Hash]MarketParams{r.marketID: r.market.Params}, nil
}

func (r *testLifecycleReader) ReadHead(context.Context) (number uint64, timestamp uint64, err error) {
	r.headCalls.Add(1)
	if r.fail.Load() {
		return 0, 0, errors.New("head unavailable")
	}
	return 101, uint64(characterizationNow.Unix()), nil
}

func (*testLifecycleReader) ReadCallbackMorpho(context.Context, common.Address) (common.Address, error) {
	return common.BigToAddress(big.NewInt(99)), nil
}

func (r *testLifecycleReader) ReadTestMarketStates(context.Context, common.Address, map[common.Hash]MarketParams) (map[common.Hash]MarketInfo, map[common.Hash]*big.Int, error) {
	return map[common.Hash]MarketInfo{r.marketID: r.market}, map[common.Hash]*big.Int{r.marketID: characterizationBigCopy(r.price)}, nil
}

func (r *testLifecycleReader) ReadTestPositions(context.Context, common.Address, map[common.Hash]MarketInfo, []common.Address) (map[common.Hash]map[common.Address]morpho.PositionState, error) {
	return map[common.Hash]map[common.Address]morpho.PositionState{
		r.marketID: {r.borrower: morpho.ClonePositionState(r.position)},
	}, nil
}

func TestTestMonitorPublishesInitialAndPreservesLastGoodDecision(t *testing.T) {
	reader := &testLifecycleReader{
		marketID: characterizationMarket, market: characterizationMarketInfo(characterizationOracle),
		price: mustBig("1550000000000000000000000000"), borrower: characterizationBorrower,
		position: goldenBorrower(),
	}
	adapter := characterizationAdapterSnapshot(big.NewInt(100_000_000_000))
	strategy, err := New(Config{
		TestMonitor: &TestMonitorConfig{
			Markets: []common.Hash{characterizationMarket}, Positions: []common.Address{characterizationBorrower},
		},
		BidWei: big.NewInt(500_000_000_000_000), CallbackAuthTTL: time.Minute,
		MonitorPoll: 2 * time.Millisecond, MaxStateAge: 10 * 365 * 24 * time.Hour,
		Sizing: SizingParams{AllowFullLiquidation: true},
	}, Deps{
		Reader: reader, Signer: newCharacterizationSigner(t), Log: logr.Discard(), ChainID: 11_155_111,
		Adapter: characterizationAdapter, Callback: characterizationCallback,
		LoadAdapterSnapshot: func() (types.AdapterSnapshot, bool) { return characterizationCloneAdapter(adapter), true },
	})
	if err != nil {
		t.Fatal(err)
	}
	input := characterizationLifecycleInput(adapter, characterizationOracle)
	// Sepolia dev settlement does not apply this conflicting frame price; the test monitor must
	// continue sizing from its cached on-chain oracle price.
	input.Auction.Prices[0].Price = mustBig("5000000000000000000000000000")
	if out, decideErr := strategy.DecideBid(t.Context(), input); decideErr != nil || out.Reason != types.SkipReasonStaleState {
		t.Fatalf("decision before initial publication = %+v, err=%v", out, decideErr)
	}

	cancel, done := runCharacterizationStrategy(t, strategy)
	baseline := awaitCharacterizationBid(t, strategy, input)
	withoutFrame := characterizationCloneBidInput(input)
	withoutFrame.Auction.ID += "-without-frame"
	withoutFrame.Auction.Prices = nil
	if out, decideErr := strategy.DecideBid(t.Context(), withoutFrame); decideErr != nil || out.Decision != types.DecisionBid {
		t.Fatalf("Sepolia test-monitor decision without frame = %+v, err=%v", out, decideErr)
	}
	headCallsBeforeFailure := reader.headCalls.Load()
	reader.fail.Store(true)
	characterizationAwaitCount(t, &reader.headCalls, headCallsBeforeFailure+1, "failed test-monitor refresh")
	afterFailure, err := strategy.DecideBid(t.Context(), input)
	if err != nil || afterFailure.Decision != types.DecisionBid ||
		afterFailure.BidAmount.Cmp(baseline.BidAmount) != 0 || !bytes.Equal(afterFailure.OperationData, baseline.OperationData) {
		t.Fatalf("decision after failed test-monitor refresh = %+v, err=%v", afterFailure, err)
	}
	cancel()
	awaitSignal(t, done, "test-monitor strategy shutdown")
}

type strategyLivenessReader struct {
	marketID common.Hash
	market   MarketInfo
	borrower common.Address

	headCalls   atomic.Int32
	nativeCalls atomic.Int32
	blocked     chan struct{}
	blockOnce   sync.Once
}

func (r *strategyLivenessReader) ReadNativeBalance(context.Context, common.Address) (*big.Int, error) {
	r.nativeCalls.Add(1)
	return big.NewInt(1_000_000_000_000_000_000), nil
}

func (r *strategyLivenessReader) ResolveParams(context.Context, common.Address, []common.Hash) (map[common.Hash]MarketParams, error) {
	return map[common.Hash]MarketParams{r.marketID: r.market.Params}, nil
}

func (r *strategyLivenessReader) ReadHead(ctx context.Context) (number uint64, timestamp uint64, err error) {
	if r.headCalls.Add(1) > 2 {
		r.blockOnce.Do(func() { close(r.blocked) })
		<-ctx.Done()
		return 0, 0, ctx.Err()
	}
	return 101, uint64(characterizationNow.Unix()), nil
}

func (*strategyLivenessReader) ReadCallbackMorpho(context.Context, common.Address) (common.Address, error) {
	return common.BigToAddress(big.NewInt(99)), nil
}

func (r *strategyLivenessReader) ReadTestMarketStates(context.Context, common.Address, map[common.Hash]MarketParams) (map[common.Hash]MarketInfo, map[common.Hash]*big.Int, error) {
	return map[common.Hash]MarketInfo{r.marketID: r.market}, map[common.Hash]*big.Int{
		r.marketID: mustBig("1550000000000000000000000000"),
	}, nil
}

func (r *strategyLivenessReader) ReadTestPositions(context.Context, common.Address, map[common.Hash]MarketInfo, []common.Address) (map[common.Hash]map[common.Address]morpho.PositionState, error) {
	return map[common.Hash]map[common.Address]morpho.PositionState{
		r.marketID: {r.borrower: goldenBorrower()},
	}, nil
}

func TestStrategyRunKeepsRefreshLoopsIndependentAndJoinsOnCancellation(t *testing.T) {
	reader := &strategyLivenessReader{
		marketID: characterizationMarket,
		market:   characterizationMarketInfo(characterizationOracle),
		borrower: characterizationBorrower,
		blocked:  make(chan struct{}),
	}
	adapter := types.AdapterSnapshot{
		Address: characterizationAdapter,
		Loan:    characterizationLoan,
		Redeemable: []types.RedeemableSnapshot{{
			Asset: characterizationColl,
		}},
	}
	strategy, err := New(Config{
		TestMonitor: &TestMonitorConfig{
			Markets: []common.Hash{characterizationMarket}, Positions: []common.Address{characterizationBorrower},
		},
		BidWei: big.NewInt(1), MonitorPoll: time.Millisecond, MaxStateAge: time.Hour,
	}, Deps{
		Reader: reader, Signer: newCharacterizationSigner(t), Log: logr.Discard(), ChainID: 11_155_111,
		Adapter: characterizationAdapter, Callback: characterizationCallback,
		LoadAdapterSnapshot: func() (types.AdapterSnapshot, bool) { return adapter, true },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		strategy.Run(ctx)
		close(done)
	}()

	awaitSignal(t, reader.blocked, "periodic monitor read to block")
	callsWhileBlocked := reader.nativeCalls.Load()
	characterizationAwaitCount(t, &reader.nativeCalls, callsWhileBlocked+1, "callback-state refresh while monitor is blocked")

	cancel()
	awaitSignal(t, done, "Strategy.Run cancellation join")
}

func characterizationAdapterSnapshot(maxAssets *big.Int) types.AdapterSnapshot {
	return types.AdapterSnapshot{
		Address: characterizationAdapter, Loan: characterizationLoan, LoanDecimals: 6,
		FreeAssets: big.NewInt(100_000_000_000), Withdrawable: big.NewInt(100_000_000_000), Filler: true,
		Redeemable: []types.RedeemableSnapshot{{
			Asset: characterizationColl, Decimals: 18, MaxRate: mustBig("1780000000000000000000"),
			MaxAssets: characterizationBigCopy(maxAssets), AcquireBalance: big.NewInt(100_000_000_000),
		}},
	}
}

func characterizationLifecycleInput(adapter types.AdapterSnapshot, oracle common.Address) types.BidInput {
	return types.BidInput{
		Now: characterizationNow,
		Auction: types.AuctionSnapshot{
			ID: "monitor-lifecycle", Timestamp: characterizationNow.UnixMilli(), RawPriceCount: 1,
			Prices: []types.AuctionPrice{{Oracle: oracle, Price: mustBig("1550000000000000000000000000")}},
		},
		Adapter: characterizationCloneAdapter(adapter),
		Context: types.BidContext{
			ChainID: big.NewInt(11_155_111), Executor: characterizationExecutor, Callback: characterizationCallback,
			ExecutorDeposit: big.NewInt(1_000_000_000_000_000_000), ExecutorMinDeposit: big.NewInt(10_000_000_000_000),
			MaxTxGasPrice: big.NewInt(1_000_000_000), GasLimit: 2_000_000,
		},
	}
}

func runCharacterizationStrategy(t *testing.T, strategy *Strategy) (context.CancelFunc, <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		strategy.Run(ctx)
		close(done)
	}()
	return cancel, done
}

func awaitCharacterizationBid(t *testing.T, strategy *Strategy, input types.BidInput) types.BidOutput {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		out, err := strategy.DecideBid(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		if out.Decision == types.DecisionBid {
			return out
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for bid; last output = %+v", out)
		}
		time.Sleep(time.Millisecond)
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
