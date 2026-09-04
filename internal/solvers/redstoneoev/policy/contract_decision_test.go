package policy

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"math/big"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/decision"
)

var (
	contractAdapter  = common.HexToAddress("0x0000000000000000000000000000000000000a01")
	contractCallback = common.HexToAddress("0x0000000000000000000000000000000000000c01")
	contractExecutor = common.HexToAddress("0x0000000000000000000000000000000000000e01")
	contractLoan     = common.HexToAddress("0x0000000000000000000000000000000000000101")
	contractColl     = common.HexToAddress("0x0000000000000000000000000000000000000201")
	contractOracle   = common.HexToAddress("0x0000000000000000000000000000000000000301")
	contractMarket   = common.HexToHash("0x6209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5")
	contractBorrower = common.HexToAddress("0x629d764eC8563AFA701709B52c1a215e865632dE")
	contractNow      = time.Unix(1_781_243_340, 0).UTC()
)

const (
	contractPrivateKey = "0000000000000000000000000000000000000000000000000000000000000001"
	contractNoGasSig   = "69fdf90e8e8914d459b42c66766a30aa7d74f04ecc05348b3a100c4b100f99272767822c91fa1a0e5e9c8b117f941c79231381e66fa22e63e7ca5cfba3f5d4471c"
	contractGasSig     = "65dc35d10d1e6574d68780ad25e01b6756d0016081d1eac8904781498789cac453643cd40ba18dcd7c92775969b7514ff2ea1bf794aea1fadd917fd0f185b0221b"
	contractNoGasData  = "0000000000000000000000000000000000000000000000000000000000000020c9873a0705f8c269cbec0734d109d7ffad510958302d0f6dea2cef8e2ba264820000000000000000000000000000000000000000000000000001c6bf526340000000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000000006a2b9e0800000000000000000000000000000000000000000000000000000000000000c0000000000000000000000000000000000000000000000000000000000000016000000000000000000000000000000000000000000000000000000000000000016209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5000000000000000000000000629d764ec8563afa701709b52c1a215e865632de0000000000000000000000000000000000000000000000000de0b6b3a76400000000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000000000000004169fdf90e8e8914d459b42c66766a30aa7d74f04ecc05348b3a100c4b100f99272767822c91fa1a0e5e9c8b117f941c79231381e66fa22e63e7ca5cfba3f5d4471c00000000000000000000000000000000000000000000000000000000000000"
	contractGasData    = "0000000000000000000000000000000000000000000000000000000000000020c9873a0705f8c269cbec0734d109d7ffad510958302d0f6dea2cef8e2ba264820000000000000000000000000000000000000000000000000001c6bf52634000000000000000000000000000000000000000000000000000000000000025317c000000000000000000000000000000000000000000000000000000006a2b9e0800000000000000000000000000000000000000000000000000000000000000c0000000000000000000000000000000000000000000000000000000000000016000000000000000000000000000000000000000000000000000000000000000016209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5000000000000000000000000629d764ec8563afa701709b52c1a215e865632de0000000000000000000000000000000000000000000000000de0b6b3a764000000000000000000000000000000000000000000000000000000000000000b71b0000000000000000000000000000000000000000000000000000000000000004165dc35d10d1e6574d68780ad25e01b6756d0016081d1eac8904781498789cac453643cd40ba18dcd7c92775969b7514ff2ea1bf794aea1fadd917fd0f185b0221b00000000000000000000000000000000000000000000000000000000000000"
)

type contractSigner struct {
	key *ecdsa.PrivateKey
}

func newContractSigner(tb testing.TB) *contractSigner {
	tb.Helper()
	key, err := crypto.HexToECDSA(contractPrivateKey)
	if err != nil {
		tb.Fatal(err)
	}
	return &contractSigner{key: key}
}

func (s *contractSigner) SignHash(hash common.Hash) ([]byte, error) {
	sig, err := crypto.Sign(hash.Bytes(), s.key)
	if err == nil {
		sig[64] += 27
	}
	return sig, err
}

type contractWireAuth struct {
	AuctionKey      common.Hash
	BidAmount       *big.Int
	MinBundleProfit *big.Int
	Deadline        *big.Int
}

type contractWireLeg struct {
	MarketId       common.Hash
	Borrower       common.Address
	MaxSeizeAssets *big.Int
	MinProfit      *big.Int
}

type contractWireData struct {
	Auth    contractWireAuth
	Legs    []contractWireLeg
	AuthSig []byte
}

type normalizedDecision struct {
	Decision        decision.Decision
	Reason          string
	BidAmount       string
	OperationData   string
	AuctionKey      common.Hash
	MinBundleProfit string
	Deadline        string
	AuthSignature   string
	Legs            []normalizedLeg
}

type normalizedLeg struct {
	MarketID       common.Hash
	Borrower       common.Address
	MaxSeizeAssets string
	MinProfit      string
}

type contractReader struct {
	markets   map[common.Hash]MarketInfo
	prices    map[common.Hash]*big.Int
	positions map[common.Hash]map[common.Address]morpho.PositionState

	failHead  bool
	headCalls atomic.Int32
	blocked   chan struct{}
	blockOnce sync.Once
}

func (r *contractReader) ResolveParams(_ context.Context, _ common.Address, ids []common.Hash) (map[common.Hash]MarketParams, error) {
	out := make(map[common.Hash]MarketParams, len(ids))
	for _, id := range ids {
		if market, ok := r.markets[id]; ok {
			params := market.Params
			params.Lltv = contractBigCopy(params.Lltv)
			out[id] = params
		}
	}
	return out, nil
}

func (r *contractReader) ReadHead(ctx context.Context) (number uint64, timestamp uint64, err error) {
	call := r.headCalls.Add(1)
	if r.failHead {
		return 0, 0, errors.New("head unavailable")
	}
	if call > 2 {
		r.blockOnce.Do(func() { close(r.blocked) })
		<-ctx.Done()
		return 0, 0, ctx.Err()
	}
	return 100, uint64(contractNow.Unix()), nil
}

func (*contractReader) ReadCallbackMorpho(context.Context, common.Address) (common.Address, error) {
	return common.BigToAddress(big.NewInt(99)), nil
}

func (r *contractReader) ReadTestMarketStates(_ context.Context, _ common.Address, params map[common.Hash]MarketParams) (map[common.Hash]MarketInfo, map[common.Hash]*big.Int, error) {
	markets := make(map[common.Hash]MarketInfo, len(params))
	prices := make(map[common.Hash]*big.Int, len(params))
	for id := range params {
		if market, ok := r.markets[id]; ok {
			market.Params.Lltv = contractBigCopy(market.Params.Lltv)
			market.State = morpho.CloneMarketState(market.State)
			markets[id] = market
			prices[id] = contractBigCopy(r.prices[id])
		}
	}
	return markets, prices, nil
}

func (r *contractReader) ReadTestPositions(_ context.Context, _ common.Address, markets map[common.Hash]MarketInfo, borrowers []common.Address) (map[common.Hash]map[common.Address]morpho.PositionState, error) {
	allowed := make(map[common.Address]struct{}, len(borrowers))
	for _, borrower := range borrowers {
		allowed[borrower] = struct{}{}
	}
	out := make(map[common.Hash]map[common.Address]morpho.PositionState, len(markets))
	for id := range markets {
		for borrower, position := range r.positions[id] {
			if _, ok := allowed[borrower]; !ok {
				continue
			}
			if out[id] == nil {
				out[id] = make(map[common.Address]morpho.PositionState)
			}
			out[id][borrower] = morpho.ClonePositionState(position)
		}
	}
	return out, nil
}

type contractFixtureOptions struct {
	gasAccounting    bool
	callbackNative   *big.Int
	failHead         bool
	markets          map[common.Hash]MarketInfo
	prices           map[common.Hash]*big.Int
	positions        map[common.Hash]map[common.Address]morpho.PositionState
	adapterMaxAssets *big.Int
}

func newDecisionContractFixture(tb testing.TB, gasAccounting bool) (*Strategy, decision.BidInput) {
	tb.Helper()
	return newDecisionContractFixtureWith(tb, contractFixtureOptions{gasAccounting: gasAccounting})
}

func newDecisionContractFixtureWith(tb testing.TB, opts contractFixtureOptions) (*Strategy, decision.BidInput) {
	tb.Helper()
	if opts.markets == nil {
		opts.markets = map[common.Hash]MarketInfo{
			contractMarket: contractMarketInfo(contractOracle),
		}
	}
	if opts.prices == nil {
		opts.prices = map[common.Hash]*big.Int{
			contractMarket: mustBig("1550000000000000000000000000"),
		}
	}
	if opts.positions == nil {
		opts.positions = map[common.Hash]map[common.Address]morpho.PositionState{
			contractMarket: {contractBorrower: goldenBorrower()},
		}
	}
	if opts.callbackNative == nil {
		opts.callbackNative = big.NewInt(1_000_000_000_000_000_000)
	}
	if opts.adapterMaxAssets == nil {
		opts.adapterMaxAssets = big.NewInt(100_000_000_000)
	}

	marketIDs := make([]common.Hash, 0, len(opts.markets))
	borrowerSet := make(map[common.Address]struct{})
	for id := range opts.markets {
		marketIDs = append(marketIDs, id)
		for borrower := range opts.positions[id] {
			borrowerSet[borrower] = struct{}{}
		}
	}
	borrowers := make([]common.Address, 0, len(borrowerSet))
	for borrower := range borrowerSet {
		borrowers = append(borrowers, borrower)
	}
	if len(borrowers) == 0 {
		borrowers = append(borrowers, contractBorrower)
	}

	adapter := decision.AdapterSnapshot{
		Address: contractAdapter, Loan: contractLoan, LoanDecimals: 6,
		FreeAssets: big.NewInt(100_000_000_000), Withdrawable: big.NewInt(100_000_000_000), Filler: true,
		Redeemable: []decision.RedeemableSnapshot{{
			Asset: contractColl, Decimals: 18, MaxRate: mustBig("1780000000000000000000"),
			MaxAssets: contractBigCopy(opts.adapterMaxAssets), AcquireBalance: big.NewInt(100_000_000_000),
		}},
	}
	reader := &contractReader{
		markets: opts.markets, prices: opts.prices, positions: opts.positions,
		failHead: opts.failHead,
		blocked:  make(chan struct{}),
	}
	cfg := Config{
		TestMonitor: &TestMonitorConfig{Markets: marketIDs, Positions: borrowers},
		BidWei:      big.NewInt(500_000_000_000_000), CallbackAuthTTL: time.Minute,
		MonitorPoll: time.Millisecond, MaxStateAge: 10 * 365 * 24 * time.Hour,
		Sizing: SizingParams{AllowFullLiquidation: true},
	}
	strategy, source, err := New(cfg, Deps{
		Reader: reader, Signer: newContractSigner(tb), Log: logr.Discard(), ChainID: 11_155_111,
		Adapter: contractAdapter, Callback: contractCallback,
		LoadAdapterSnapshot: func() (decision.AdapterSnapshot, bool) { return contractCloneAdapter(adapter), true },
		GasAccounting:       opts.gasAccounting,
	})
	if err != nil {
		tb.Fatal(err)
	}
	ctx, cancel := context.WithCancel(tb.Context())
	done := make(chan struct{})
	go func() {
		source.Run(ctx)
		close(done)
	}()
	if opts.failHead {
		contractAwaitCount(tb, &reader.headCalls, 1, "failed monitor refresh")
	} else {
		contractAwaitSignal(tb, reader.blocked, "initial monitor publication")
	}
	tb.Cleanup(func() {
		cancel()
		contractAwaitSignal(tb, done, "strategy shutdown")
	})

	input := decision.BidInput{
		Now: contractNow,
		Auction: decision.AuctionSnapshot{
			ID: "decision-fixture", Timestamp: contractNow.UnixMilli(), RawPriceCount: 1,
			Prices: []decision.AuctionPrice{{Oracle: contractOracle, Price: mustBig("5000000000000000000000000000")}},
		},
		Adapter: contractCloneAdapter(adapter),
		Context: decision.BidContext{
			ChainID: big.NewInt(11_155_111), Executor: contractExecutor, Callback: contractCallback,
			CallbackNative:  contractBigCopy(opts.callbackNative),
			ExecutorDeposit: big.NewInt(1_000_000_000_000_000_000), ExecutorMinDeposit: big.NewInt(10_000_000_000_000),
			MaxTxGasPrice: big.NewInt(1_000_000_000), GasLimit: 2_000_000,
		},
	}
	if opts.gasAccounting {
		input.Context.GasPrices = liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{
			contractLoan: big.NewInt(2_500_000_000),
		})
	}
	input.Market = source.Snapshot(input.Auction, input.Now, input.Adapter)
	return strategy, input
}

func TestDefaultStrategyDecideBidContract(t *testing.T) {
	tests := []struct {
		name          string
		gasAccounting bool
		fixture       contractFixtureOptions
		mutate        func(*decision.BidInput)
		wantReason    string
		wantSuccess   bool
	}{
		{name: "adapter mismatch", mutate: func(in *decision.BidInput) { in.Adapter.Address[19]++ }, wantReason: decision.SkipReasonNoLegs},
		{name: "callback mismatch", mutate: func(in *decision.BidInput) { in.Context.Callback[19]++ }, wantReason: decision.SkipReasonNoLegs},
		{name: "filler denied", mutate: func(in *decision.BidInput) { in.Adapter.Filler = false }, wantReason: decision.SkipReasonNoLegs},
		{name: "adapter paused", mutate: func(in *decision.BidInput) { in.Adapter.Paused = true }, wantReason: decision.SkipReasonNoLegs},
		{name: "monitor state stale", fixture: contractFixtureOptions{failHead: true}, wantReason: decision.SkipReasonStaleState},
		{name: "callback state missing", mutate: func(in *decision.BidInput) { in.Context.CallbackNative = nil }, wantReason: decision.SkipReasonStaleState},
		{name: "snapshot epoch stale", fixture: contractFixtureOptions{markets: map[common.Hash]MarketInfo{
			contractMarket: contractMarketInfo(contractOracle),
		}}, mutate: func(in *decision.BidInput) {
			in.Auction.Timestamp += int64(37 * time.Second / time.Millisecond)
		}, wantReason: decision.SkipReasonStaleEpoch},
		{name: "gas rate unavailable", gasAccounting: true, mutate: func(in *decision.BidInput) { in.Context.GasPrices = nil }, wantReason: decision.SkipReasonGasUnprofitable},
		{name: "deposit unavailable", mutate: func(in *decision.BidInput) {
			in.Context.ExecutorDeposit = new(big.Int).Set(in.Context.ExecutorMinDeposit)
		}, wantReason: decision.SkipReasonDepositLow},
		{name: "callback funding unavailable", fixture: contractFixtureOptions{callbackNative: big.NewInt(499_999_999_999_999)}, wantReason: decision.SkipReasonCallbackBalance},
		{name: "direct bid without gas accounting", wantSuccess: true},
		{name: "direct bid with gas accounting", gasAccounting: true, wantSuccess: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.fixture.gasAccounting = test.gasAccounting
			strategy, input := newDecisionContractFixtureWith(t, test.fixture)
			if test.mutate != nil {
				test.mutate(&input)
			}
			beforeInput := contractCloneBidInput(input)
			var beforeGasRate *big.Int
			if input.Context.GasPrices != nil {
				beforeGasRate = input.Context.GasPrices.TokenOutPerNative(input.Adapter.Loan)
			}

			out, err := strategy.DecideBid(t.Context(), input)
			if err != nil {
				t.Fatalf("DecideBid: %v", err)
			}
			if !reflect.DeepEqual(input, beforeInput) {
				t.Fatalf("DecideBid mutated input\n got: %#v\nwant: %#v", input, beforeInput)
			}
			if input.Context.GasPrices != nil && input.Context.GasPrices.TokenOutPerNative(input.Adapter.Loan).Cmp(beforeGasRate) != 0 {
				t.Fatal("DecideBid mutated the gas-price snapshot")
			}

			got := normalizeDecision(t, out)
			if !test.wantSuccess {
				want := normalizedDecision{Decision: decision.DecisionSkip, Reason: test.wantReason}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("normalized output mismatch\n got: %+v\nwant: %+v", got, want)
				}
				return
			}
			assertSuccessfulDecisionEnvelope(t, test.gasAccounting, got)

			out.BidAmount.SetInt64(1)
			clear(out.OperationData)
			again, err := strategy.DecideBid(t.Context(), input)
			if err != nil {
				t.Fatalf("repeat DecideBid: %v", err)
			}
			if repeat := normalizeDecision(t, again); !reflect.DeepEqual(repeat, got) {
				t.Fatalf("repeat decision changed\n got: %+v\nwant: %+v", repeat, got)
			}
			if !reflect.DeepEqual(input, beforeInput) {
				t.Fatal("mutating BidOutput aliased immutable decision input")
			}
		})
	}
}

func assertSuccessfulDecisionEnvelope(t *testing.T, gasAccounting bool, got normalizedDecision) {
	t.Helper()
	want := normalizedDecision{
		Decision: decision.DecisionBid, BidAmount: "500000000000000",
		AuctionKey: crypto.Keccak256Hash([]byte("id:decision-fixture")), Deadline: "1781243400",
	}
	if gasAccounting {
		want.OperationData = contractGasData
		want.AuthSignature = contractGasSig
		want.MinBundleProfit = "2437500"
		want.Legs = []normalizedLeg{{
			MarketID: contractMarket, Borrower: contractBorrower,
			MaxSeizeAssets: "1000000000000000000", MinProfit: "750000",
		}}
	} else {
		want.OperationData = contractNoGasData
		want.AuthSignature = contractNoGasSig
		want.MinBundleProfit = "1"
		want.Legs = []normalizedLeg{{
			MarketID: contractMarket, Borrower: contractBorrower,
			MaxSeizeAssets: "1000000000000000000", MinProfit: "1",
		}}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized successful output mismatch\n got: %+v\nwant: %+v", got, want)
	}
	if recovered := contractRecoverSigner(t, got); recovered != common.HexToAddress("0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf") {
		t.Fatalf("recovered callback auth signer = %s", recovered)
	}
}

func normalizeDecision(tb testing.TB, out decision.BidOutput) normalizedDecision {
	tb.Helper()
	normalized := normalizedDecision{Decision: out.Decision, Reason: out.Reason}
	if out.Decision != decision.DecisionBid {
		return normalized
	}
	normalized.BidAmount = out.BidAmount.String()
	normalized.OperationData = hex.EncodeToString(out.OperationData)
	decoded := contractDecodeOperationData(tb, out.OperationData)
	normalized.AuctionKey = decoded.Auth.AuctionKey
	normalized.MinBundleProfit = decoded.Auth.MinBundleProfit.String()
	normalized.Deadline = decoded.Auth.Deadline.String()
	normalized.AuthSignature = hex.EncodeToString(decoded.AuthSig)
	normalized.Legs = make([]normalizedLeg, len(decoded.Legs))
	for i, leg := range decoded.Legs {
		normalized.Legs[i] = normalizedLeg{
			MarketID: leg.MarketId, Borrower: leg.Borrower,
			MaxSeizeAssets: leg.MaxSeizeAssets.String(), MinProfit: leg.MinProfit.String(),
		}
	}
	return normalized
}

func contractDecodeOperationData(tb testing.TB, data []byte) contractWireData {
	tb.Helper()
	values, err := contractOperationArguments().Unpack(data)
	if err != nil {
		tb.Fatalf("decode operationData: %v", err)
	}
	if len(values) != 1 {
		tb.Fatalf("decode operationData values = %d, want 1", len(values))
	}
	converted := abi.ConvertType(values[0], new(contractWireData))
	decoded, ok := converted.(*contractWireData)
	if !ok || decoded == nil {
		tb.Fatalf("decoded operationData type = %T", converted)
	}
	return *decoded
}

func contractOperationArguments() abi.Arguments {
	return abi.Arguments{{Type: contractABIType("tuple", []abi.ArgumentMarshaling{
		{Name: "auth", Type: "tuple", Components: []abi.ArgumentMarshaling{
			{Name: "auctionKey", Type: "bytes32"},
			{Name: "bidAmount", Type: "uint256"},
			{Name: "minBundleProfit", Type: "uint256"},
			{Name: "deadline", Type: "uint256"},
		}},
		{Name: "legs", Type: "tuple[]", Components: contractLegComponents()},
		{Name: "authSig", Type: "bytes"},
	})}}
}

func contractLegComponents() []abi.ArgumentMarshaling {
	return []abi.ArgumentMarshaling{
		{Name: "marketId", Type: "bytes32"},
		{Name: "borrower", Type: "address"},
		{Name: "maxSeizeAssets", Type: "uint256"},
		{Name: "minProfit", Type: "uint256"},
	}
}

func contractABIType(name string, components []abi.ArgumentMarshaling) abi.Type {
	typeOf, err := abi.NewType(name, "", components)
	if err != nil {
		panic("contract ABI schema: " + err.Error())
	}
	return typeOf
}

func contractRecoverSigner(tb testing.TB, decision normalizedDecision) common.Address {
	tb.Helper()
	legs := make([]contractWireLeg, len(decision.Legs))
	for i, leg := range decision.Legs {
		legs[i] = contractWireLeg{
			MarketId: leg.MarketID, Borrower: leg.Borrower,
			MaxSeizeAssets: mustBig(leg.MaxSeizeAssets), MinProfit: mustBig(leg.MinProfit),
		}
	}
	encodedLegs, err := (abi.Arguments{{Type: contractABIType("tuple[]", contractLegComponents())}}).Pack(legs)
	if err != nil {
		tb.Fatal(err)
	}
	digestArguments := abi.Arguments{
		{Type: contractABIType("bytes32", nil)},
		{Type: contractABIType("uint256", nil)},
		{Type: contractABIType("address", nil)},
		{Type: contractABIType("address", nil)},
		{Type: contractABIType("bytes32", nil)},
		{Type: contractABIType("uint256", nil)},
		{Type: contractABIType("uint256", nil)},
		{Type: contractABIType("uint256", nil)},
		{Type: contractABIType("bytes32", nil)},
	}
	encodedDigest, err := digestArguments.Pack(
		crypto.Keccak256Hash([]byte("SYMBIOTIC_OEV_AUTH_V1")), big.NewInt(11_155_111),
		contractCallback, contractExecutor, decision.AuctionKey, mustBig(decision.BidAmount),
		mustBig(decision.MinBundleProfit), mustBig(decision.Deadline), crypto.Keccak256Hash(encodedLegs),
	)
	if err != nil {
		tb.Fatal(err)
	}
	sig, err := hex.DecodeString(decision.AuthSignature)
	if err != nil {
		tb.Fatal(err)
	}
	if len(sig) != crypto.SignatureLength {
		tb.Fatalf("callback auth signature length = %d", len(sig))
	}
	if sig[64] >= 27 {
		sig[64] -= 27
	}
	pub, err := crypto.SigToPub(crypto.Keccak256(encodedDigest), sig)
	if err != nil {
		tb.Fatalf("recover callback auth signer: %v", err)
	}
	return crypto.PubkeyToAddress(*pub)
}

func TestDefaultStrategySeededCorpusOrderingReplayCapsAndReservations(t *testing.T) {
	strategy, input := newCorpusContractFixture(t)
	baselineInput := contractCloneBidInput(input)

	first, err := strategy.DecideBid(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	firstNormalized := normalizeDecision(t, first)
	if firstNormalized.Decision != decision.DecisionBid {
		t.Fatalf("first corpus decision = %+v", firstNormalized)
	}
	wantOrder := []normalizedLeg{
		{MarketID: common.BigToHash(big.NewInt(1)), Borrower: common.BigToAddress(big.NewInt(1)), MaxSeizeAssets: "1000000000000000000", MinProfit: "1"},
		{MarketID: common.BigToHash(big.NewInt(1)), Borrower: common.BigToAddress(big.NewInt(2)), MaxSeizeAssets: "1000000000000000000", MinProfit: "1"},
	}
	if !reflect.DeepEqual(firstNormalized.Legs, wantOrder) {
		t.Fatalf("ordered, replayed, capacity-capped legs = %+v, want %+v", firstNormalized.Legs, wantOrder)
	}
	if !reflect.DeepEqual(input, baselineInput) {
		t.Fatal("corpus decision mutated its input")
	}

	pending := contractCloneBidInput(input)
	pending.Auction.ID = "corpus-reserved"
	pending.Exposure = first.Exposure
	reserved, err := strategy.DecideBid(t.Context(), pending)
	if err != nil {
		t.Fatal(err)
	}
	reservedNormalized := normalizeDecision(t, reserved)
	if reservedNormalized.Decision != decision.DecisionBid {
		t.Fatalf("independent markets should remain live after reserving first bundle: %+v", reservedNormalized)
	}
	for _, leg := range reservedNormalized.Legs {
		if leg.MarketID == wantOrder[0].MarketID {
			t.Fatalf("reserved same-market positions were reused: %+v", leg)
		}
	}
}

func TestDefaultStrategyFundingAndGasBoundariesFromDecisions(t *testing.T) {
	t.Run("callback reservation", func(t *testing.T) {
		strategy, input := newCorpusContractFixtureWith(t, big.NewInt(500_000_000_000_000))
		first, err := strategy.DecideBid(t.Context(), input)
		if err != nil || first.Decision != decision.DecisionBid {
			t.Fatalf("first decision = %+v, err=%v", first, err)
		}
		input.Auction.ID = "corpus-callback-reservation"
		input.Exposure = first.Exposure
		out, err := strategy.DecideBid(t.Context(), input)
		if err != nil || out.Decision != decision.DecisionSkip || out.Reason != decision.SkipReasonCallbackBalance {
			t.Fatalf("callback reservation decision = %+v, err=%v", out, err)
		}
	})

	t.Run("deposit reservation", func(t *testing.T) {
		strategy, input := newCorpusContractFixture(t)
		input.Context.ExecutorDeposit = new(big.Int).Add(input.Context.ExecutorMinDeposit, big.NewInt(1_000_000_000_000_000))
		first, err := strategy.DecideBid(t.Context(), input)
		if err != nil || first.Decision != decision.DecisionBid {
			t.Fatalf("first decision = %+v, err=%v", first, err)
		}
		input.Auction.ID = "corpus-deposit-reservation"
		input.Exposure = first.Exposure
		out, err := strategy.DecideBid(t.Context(), input)
		if err != nil || out.Decision != decision.DecisionSkip || out.Reason != decision.SkipReasonDepositLow {
			t.Fatalf("deposit reservation decision = %+v, err=%v", out, err)
		}
	})

	t.Run("gas envelope changes wire depth", func(t *testing.T) {
		strategy, input := newCorpusContractFixture(t)
		input.Context.GasLimit = 700_000
		out, err := strategy.DecideBid(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		got := normalizeDecision(t, out)
		if got.Decision != decision.DecisionBid || len(got.Legs) != 1 {
			t.Fatalf("700k gas-limit decision = %+v, want one wire leg", got)
		}

		strategy, input = newCorpusContractFixture(t)
		input.Context.GasLimit = 650_000
		out, err = strategy.DecideBid(t.Context(), input)
		if err != nil || out.Decision != decision.DecisionSkip {
			t.Fatalf("650k gas-limit decision = %+v, err=%v", out, err)
		}
	})
}

func TestDefaultStrategyRepeatedAndConcurrentDecisionBytes(t *testing.T) {
	const calls = 8
	wantStrategy, wantInput := newDecisionContractFixture(t, true)
	want, err := wantStrategy.DecideBid(t.Context(), wantInput)
	if err != nil {
		t.Fatal(err)
	}

	strategy, input := newDecisionContractFixture(t, true)
	ctx := t.Context()
	results := make(chan decision.BidOutput, calls)
	errs := make(chan error, calls)
	var wg sync.WaitGroup
	for range calls {
		wg.Go(func() {
			out, err := strategy.DecideBid(ctx, input)
			results <- out
			errs <- err
		})
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent DecideBid: %v", err)
		}
	}
	for got := range results {
		if got.Decision != decision.DecisionBid || got.BidAmount.Cmp(want.BidAmount) != 0 || !bytes.Equal(got.OperationData, want.OperationData) {
			t.Fatalf("concurrent fixed-input bytes changed: decision=%s bid=%v dataEqual=%v", got.Decision, got.BidAmount, bytes.Equal(got.OperationData, want.OperationData))
		}
	}
}

func newCorpusContractFixture(tb testing.TB) (*Strategy, decision.BidInput) {
	tb.Helper()
	return newCorpusContractFixtureWith(tb, big.NewInt(1_000_000_000_000_000_000))
}

func newCorpusContractFixtureWith(tb testing.TB, callbackNative *big.Int) (*Strategy, decision.BidInput) {
	tb.Helper()
	markets := make(map[common.Hash]MarketInfo)
	positions := make(map[common.Hash]map[common.Address]morpho.PositionState)
	prices := make(map[common.Hash]*big.Int)
	for marketN := int64(3); marketN >= 1; marketN-- {
		id := common.BigToHash(big.NewInt(marketN))
		oracle := common.BigToAddress(big.NewInt(100 + marketN))
		markets[id] = contractMarketInfo(oracle)
		prices[id] = mustBig("1550000000000000000000000000")
		positions[id] = make(map[common.Address]morpho.PositionState)
		for borrowerN := int64(2); borrowerN >= 1; borrowerN-- {
			positions[id][common.BigToAddress(big.NewInt((marketN-1)*2+borrowerN))] = goldenBorrower()
		}
	}
	strategy, input := newDecisionContractFixtureWith(tb, contractFixtureOptions{
		callbackNative: callbackNative, markets: markets, prices: prices, positions: positions,
		adapterMaxAssets: big.NewInt(3_600_000_000),
	})
	input.Auction.ID = "corpus"
	input.Auction.RawPriceCount = 3
	return strategy, input
}

func contractMarketInfo(oracle common.Address) MarketInfo {
	return MarketInfo{
		Params: MarketParams{
			LoanToken: contractLoan, CollateralToken: contractColl, Oracle: oracle,
			Lltv: mustBig("860000000000000000"),
		},
		State: goldenMarket(),
	}
}

func contractBigCopy(value *big.Int) *big.Int {
	if value == nil {
		return nil
	}
	return new(big.Int).Set(value)
}

func contractCloneAdapter(in decision.AdapterSnapshot) decision.AdapterSnapshot {
	out := in
	out.FreeAssets = contractBigCopy(in.FreeAssets)
	out.Withdrawable = contractBigCopy(in.Withdrawable)
	out.Redeemable = slices.Clone(in.Redeemable)
	for i := range out.Redeemable {
		out.Redeemable[i].MaxRate = contractBigCopy(in.Redeemable[i].MaxRate)
		out.Redeemable[i].MaxAssets = contractBigCopy(in.Redeemable[i].MaxAssets)
		out.Redeemable[i].AcquireBalance = contractBigCopy(in.Redeemable[i].AcquireBalance)
	}
	return out
}

func contractCloneBidInput(in decision.BidInput) decision.BidInput {
	out := in
	out.Auction.Prices = slices.Clone(in.Auction.Prices)
	for i := range out.Auction.Prices {
		out.Auction.Prices[i].Price = contractBigCopy(in.Auction.Prices[i].Price)
	}
	out.Adapter = contractCloneAdapter(in.Adapter)
	out.Context.ChainID = contractBigCopy(in.Context.ChainID)
	out.Context.CallbackNative = contractBigCopy(in.Context.CallbackNative)
	out.Context.ExecutorDeposit = contractBigCopy(in.Context.ExecutorDeposit)
	out.Context.ExecutorMinDeposit = contractBigCopy(in.Context.ExecutorMinDeposit)
	out.Context.MaxTxGasPrice = contractBigCopy(in.Context.MaxTxGasPrice)
	out.PendingAuctions = slices.Clone(in.PendingAuctions)
	out.Exposure.BidNative = contractBigCopy(in.Exposure.BidNative)
	out.Exposure.GasNative = contractBigCopy(in.Exposure.GasNative)
	out.Exposure.Positions = slices.Clone(in.Exposure.Positions)
	return out
}

func contractAwaitCount(tb testing.TB, count *atomic.Int32, want int32, description string) {
	tb.Helper()
	deadline := time.Now().Add(time.Second)
	for count.Load() < want {
		if time.Now().After(deadline) {
			tb.Fatalf("timed out waiting for %s", description)
		}
		time.Sleep(time.Millisecond)
	}
}

func contractAwaitSignal(tb testing.TB, signal <-chan struct{}, description string) {
	tb.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		tb.Fatalf("timed out waiting for %s", description)
	}
}
