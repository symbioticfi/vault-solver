package defaultstrategy

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"math/big"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-logr/logr"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

var (
	characterizationAdapter  = common.HexToAddress("0x0000000000000000000000000000000000000a01")
	characterizationCallback = common.HexToAddress("0x0000000000000000000000000000000000000c01")
	characterizationExecutor = common.HexToAddress("0x0000000000000000000000000000000000000e01")
	characterizationLoan     = common.HexToAddress("0x0000000000000000000000000000000000000101")
	characterizationColl     = common.HexToAddress("0x0000000000000000000000000000000000000201")
	characterizationOracle   = common.HexToAddress("0x0000000000000000000000000000000000000301")
	characterizationMarket   = common.HexToHash("0x6209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5")
	characterizationBorrower = common.HexToAddress("0x629d764eC8563AFA701709B52c1a215e865632dE")
	characterizationNow      = time.Unix(1_781_243_340, 0).UTC()
)

const (
	characterizationPrivateKey = "0000000000000000000000000000000000000000000000000000000000000001"
	characterizationNoGasSig   = "69fdf90e8e8914d459b42c66766a30aa7d74f04ecc05348b3a100c4b100f99272767822c91fa1a0e5e9c8b117f941c79231381e66fa22e63e7ca5cfba3f5d4471c"
	characterizationGasSig     = "65dc35d10d1e6574d68780ad25e01b6756d0016081d1eac8904781498789cac453643cd40ba18dcd7c92775969b7514ff2ea1bf794aea1fadd917fd0f185b0221b"
	characterizationNoGasData  = "0000000000000000000000000000000000000000000000000000000000000020c9873a0705f8c269cbec0734d109d7ffad510958302d0f6dea2cef8e2ba264820000000000000000000000000000000000000000000000000001c6bf526340000000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000000006a2b9e0800000000000000000000000000000000000000000000000000000000000000c0000000000000000000000000000000000000000000000000000000000000016000000000000000000000000000000000000000000000000000000000000000016209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5000000000000000000000000629d764ec8563afa701709b52c1a215e865632de0000000000000000000000000000000000000000000000000de0b6b3a76400000000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000000000000004169fdf90e8e8914d459b42c66766a30aa7d74f04ecc05348b3a100c4b100f99272767822c91fa1a0e5e9c8b117f941c79231381e66fa22e63e7ca5cfba3f5d4471c00000000000000000000000000000000000000000000000000000000000000"
	characterizationGasData    = "0000000000000000000000000000000000000000000000000000000000000020c9873a0705f8c269cbec0734d109d7ffad510958302d0f6dea2cef8e2ba264820000000000000000000000000000000000000000000000000001c6bf52634000000000000000000000000000000000000000000000000000000000000025317c000000000000000000000000000000000000000000000000000000006a2b9e0800000000000000000000000000000000000000000000000000000000000000c0000000000000000000000000000000000000000000000000000000000000016000000000000000000000000000000000000000000000000000000000000000016209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5000000000000000000000000629d764ec8563afa701709b52c1a215e865632de0000000000000000000000000000000000000000000000000de0b6b3a764000000000000000000000000000000000000000000000000000000000000000b71b0000000000000000000000000000000000000000000000000000000000000004165dc35d10d1e6574d68780ad25e01b6756d0016081d1eac8904781498789cac453643cd40ba18dcd7c92775969b7514ff2ea1bf794aea1fadd917fd0f185b0221b00000000000000000000000000000000000000000000000000000000000000"
)

type characterizationSigner struct {
	key *ecdsa.PrivateKey
}

func newCharacterizationSigner(tb testing.TB) *characterizationSigner {
	tb.Helper()
	key, err := crypto.HexToECDSA(characterizationPrivateKey)
	if err != nil {
		tb.Fatal(err)
	}
	return &characterizationSigner{key: key}
}

func (s *characterizationSigner) SignHash(hash common.Hash) ([]byte, error) {
	sig, err := crypto.Sign(hash.Bytes(), s.key)
	if err == nil {
		sig[64] += 27
	}
	return sig, err
}

type normalizedDecision struct {
	Decision        types.Decision
	Reason          string
	BidAmount       string
	OperationData   string
	AuctionKey      common.Hash
	MinBundleProfit string
	Deadline        string
	AuthSignature   string
	RecoveredSigner common.Address
	Legs            []normalizedLeg
	GasUnits        uint64
	GasNative       string
	ExpectedLoanOut []string
}

type normalizedLeg struct {
	MarketID       common.Hash
	Borrower       common.Address
	MaxSeizeAssets string
	MinProfit      string
}

func TestDefaultStrategyDecideBidCharacterization(t *testing.T) {
	tests := []struct {
		name          string
		gasAccounting bool
		mutate        func(*Strategy, *types.BidInput)
		wantReason    string
		wantSuccess   bool
	}{
		{name: "adapter mismatch", mutate: func(_ *Strategy, in *types.BidInput) { in.Adapter.Address[19]++ }, wantReason: skipNoLegs},
		{name: "callback mismatch", mutate: func(_ *Strategy, in *types.BidInput) { in.Context.Callback[19]++ }, wantReason: skipNoLegs},
		{name: "filler denied", mutate: func(_ *Strategy, in *types.BidInput) { in.Adapter.Filler = false }, wantReason: skipNoLegs},
		{name: "adapter paused", mutate: func(_ *Strategy, in *types.BidInput) { in.Adapter.Paused = true }, wantReason: skipNoLegs},
		{name: "monitor state stale", mutate: func(s *Strategy, in *types.BidInput) {
			s.mon.(*apiMonitor).snap.Load().updatedAt = in.Now.Add(-2 * time.Minute)
		}, wantReason: skipStaleState},
		{name: "callback state stale", mutate: func(s *Strategy, in *types.BidInput) {
			s.state.store(decisionState{CallbackNative: big.NewInt(1_000_000_000_000_000_000), CallbackUpdatedAt: in.Now.Add(-2 * time.Minute)})
		}, wantReason: skipStaleState},
		{name: "snapshot epoch stale", mutate: func(s *Strategy, _ *types.BidInput) {
			s.mon.(*apiMonitor).snap.Load().blockTime -= uint64(snapshotMaxAuctionLag/time.Second) + 1
		}, wantReason: skipStaleEpoch},
		{name: "no auction price", mutate: func(_ *Strategy, in *types.BidInput) { in.Auction.Prices = nil }, wantReason: skipNoLegs},
		{name: "gas rate unavailable", gasAccounting: true, mutate: func(_ *Strategy, in *types.BidInput) { in.Context.GasPrices = nil }, wantReason: skipGasUnprofitable},
		{name: "deposit reservation unavailable", mutate: func(_ *Strategy, in *types.BidInput) {
			in.Context.ExecutorDeposit = new(big.Int).Set(in.Context.ExecutorMinDeposit)
		}, wantReason: types.SkipReasonDepositLow},
		{name: "callback funding unavailable", mutate: func(s *Strategy, in *types.BidInput) {
			s.state.store(decisionState{CallbackNative: new(big.Int).Sub(s.cfg.BidWei, big.NewInt(1)), CallbackUpdatedAt: in.Now})
		}, wantReason: types.SkipReasonCallbackBalance},
		{name: "direct bid without gas accounting", wantSuccess: true},
		{name: "direct bid with gas accounting", gasAccounting: true, wantSuccess: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			strategy, input := newDecisionCharacterizationFixture(t, test.gasAccounting)
			if test.mutate != nil {
				test.mutate(strategy, &input)
			}
			beforeInput := cloneBidInput(input)
			beforeSnapshot := cloneSnapshot(strategy.mon.snapshot())
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
			if !reflect.DeepEqual(strategy.mon.snapshot(), beforeSnapshot) {
				t.Fatal("DecideBid mutated the immutable monitor snapshot")
			}

			got := normalizeDecision(t, strategy, input, out)
			if !test.wantSuccess {
				want := normalizedDecision{Decision: types.DecisionSkip, Reason: test.wantReason}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("normalized output mismatch\n got: %+v\nwant: %+v", got, want)
				}
				return
			}
			assertSuccessfulDecisionEnvelope(t, test.gasAccounting, got)

			// BidOutput owns its mutable values: changing a caller-held result cannot alias the input,
			// monitor snapshot, or a later result for the same fixed decision.
			out.BidAmount.SetInt64(1)
			clear(out.OperationData)
			again, err := strategy.DecideBid(t.Context(), input)
			if err != nil {
				t.Fatalf("repeat DecideBid: %v", err)
			}
			if repeat := normalizeDecision(t, strategy, input, again); !reflect.DeepEqual(repeat, got) {
				t.Fatalf("repeat decision changed\n got: %+v\nwant: %+v", repeat, got)
			}
			if !reflect.DeepEqual(input, beforeInput) || !reflect.DeepEqual(strategy.mon.snapshot(), beforeSnapshot) {
				t.Fatal("mutating BidOutput aliased immutable decision inputs")
			}
		})
	}
}

func assertSuccessfulDecisionEnvelope(t *testing.T, gasAccounting bool, got normalizedDecision) {
	t.Helper()
	want := normalizedDecision{
		Decision:        types.DecisionBid,
		BidAmount:       "500000000000000",
		AuctionKey:      auctionKeyHash("decision-fixture"),
		Deadline:        "1781243400",
		RecoveredSigner: common.HexToAddress("0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf"),
		GasUnits:        475_000,
		GasNative:       "475000000000000",
		ExpectedLoanOut: []string{"1779999999"},
	}
	if gasAccounting {
		want.OperationData = characterizationGasData
		want.AuthSignature = characterizationGasSig
		want.MinBundleProfit = "2437500"
		want.Legs = []normalizedLeg{{
			MarketID: characterizationMarket, Borrower: characterizationBorrower,
			MaxSeizeAssets: "1000000000000000000", MinProfit: "750000",
		}}
	} else {
		want.OperationData = characterizationNoGasData
		want.AuthSignature = characterizationNoGasSig
		want.MinBundleProfit = "1"
		want.Legs = []normalizedLeg{{
			MarketID: characterizationMarket, Borrower: characterizationBorrower,
			MaxSeizeAssets: "1000000000000000000", MinProfit: "1",
		}}
	}
	// Signature and complete operationData bytes are deterministic fixed-input wire values.
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized successful output mismatch\n got: %+v\nwant: %+v", got, want)
	}
	expectedSig, err := hex.DecodeString(got.AuthSignature)
	if err != nil {
		t.Fatal(err)
	}
	auth := operationAuth{
		AuctionKey: got.AuctionKey, BidAmount: mustBig(got.BidAmount),
		MinBundleProfit: mustBig(got.MinBundleProfit), Deadline: mustBig(got.Deadline),
	}
	legs := make([]selectedLeg, len(got.Legs))
	for i, leg := range got.Legs {
		legs[i] = selectedLeg{MarketId: leg.MarketID, Borrower: leg.Borrower, MaxSeizeAssets: mustBig(leg.MaxSeizeAssets), MinProfit: mustBig(leg.MinProfit)}
	}
	expectedData, err := encodeOperationData(auth, legs, expectedSig)
	if err != nil {
		t.Fatal(err)
	}
	if got.OperationData != hex.EncodeToString(expectedData) {
		t.Fatal("operationData differs from the complete normalized auth, legs, and signature")
	}
}

func newDecisionCharacterizationFixture(tb testing.TB, gasAccounting bool) (*Strategy, types.BidInput) {
	tb.Helper()
	signer := newCharacterizationSigner(tb)
	snap := &snapshot{
		markets: map[common.Hash]MarketInfo{
			characterizationMarket: {
				Params: MarketParams{LoanToken: characterizationLoan, CollateralToken: characterizationColl, Oracle: characterizationOracle, Lltv: mustBig("860000000000000000")},
				State:  goldenMarket(),
			},
		},
		prices: map[common.Hash]*big.Int{characterizationMarket: mustBig("5000000000000000000000000000")},
		positions: map[common.Hash]map[common.Address]morpho.PositionState{
			characterizationMarket: {characterizationBorrower: goldenBorrower()},
		},
		block: 100, blockTime: uint64(characterizationNow.Unix()), updatedAt: characterizationNow,
	}
	mon := &apiMonitor{log: logr.Discard()}
	mon.snap.Store(snap)
	cfg := Config{
		BidWei: big.NewInt(500_000_000_000_000), CallbackAuthTTL: time.Minute,
		MaxStateAge: time.Minute, Sizing: SizingParams{AllowFullLiquidation: true},
	}
	strategy := &Strategy{
		cfg: cfg, adapter: characterizationAdapter, callback: characterizationCallback,
		gasAccounting: gasAccounting, signer: signer, chainID: big.NewInt(11_155_111),
		mon: mon, engine: newBundleEngine(cfg, logr.Discard()), log: logr.Discard(),
	}
	strategy.state.store(decisionState{CallbackNative: big.NewInt(1_000_000_000_000_000_000), CallbackUpdatedAt: characterizationNow})
	adapter := types.AdapterSnapshot{
		Address: characterizationAdapter, Loan: characterizationLoan, LoanDecimals: 6,
		FreeAssets: big.NewInt(100_000_000_000), Withdrawable: big.NewInt(100_000_000_000), Filler: true,
		Redeemable: []types.RedeemableSnapshot{{
			Asset: characterizationColl, Decimals: 18, MaxRate: mustBig("1780000000000000000000"),
			MaxAssets: big.NewInt(100_000_000_000), AcquireBalance: big.NewInt(100_000_000_000),
		}},
	}
	input := types.BidInput{
		Now: characterizationNow,
		Auction: types.AuctionSnapshot{
			ID: "decision-fixture", Timestamp: characterizationNow.UnixMilli(), RawPriceCount: 1,
			Prices: []types.AuctionPrice{{Oracle: characterizationOracle, Price: mustBig("1550000000000000000000000000")}},
		},
		Adapter: adapter,
		Context: types.BidContext{
			ChainID: big.NewInt(11_155_111), Executor: characterizationExecutor, Callback: characterizationCallback,
			ExecutorDeposit: big.NewInt(1_000_000_000_000_000_000), ExecutorMinDeposit: big.NewInt(10_000_000_000_000),
			MaxTxGasPrice: big.NewInt(1_000_000_000), GasLimit: 2_000_000,
		},
	}
	if gasAccounting {
		input.Context.GasPrices = liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{
			characterizationLoan: big.NewInt(2_500_000_000),
		})
	}
	return strategy, input
}

func normalizeDecision(tb testing.TB, strategy *Strategy, input types.BidInput, out types.BidOutput) normalizedDecision {
	tb.Helper()
	normalized := normalizedDecision{Decision: out.Decision, Reason: out.Reason}
	if out.Decision != types.DecisionBid {
		return normalized
	}
	normalized.BidAmount = out.BidAmount.String()
	normalized.OperationData = hex.EncodeToString(out.OperationData)
	decoded, err := decodeOperationData(out.OperationData)
	if err != nil {
		tb.Fatalf("decode operationData: %v", err)
	}
	normalized.AuctionKey = decoded.Auth.AuctionKey
	normalized.MinBundleProfit = decoded.Auth.MinBundleProfit.String()
	normalized.Deadline = decoded.Auth.Deadline.String()
	normalized.AuthSignature = hex.EncodeToString(decoded.AuthSig)
	normalized.Legs = make([]normalizedLeg, len(decoded.Legs))
	for i, leg := range decoded.Legs {
		normalized.Legs[i] = normalizedLeg{MarketID: leg.MarketId, Borrower: leg.Borrower, MaxSeizeAssets: leg.MaxSeizeAssets.String(), MinProfit: leg.MinProfit.String()}
	}
	digest, err := callbackAuthDigest(input.Context.ChainID, input.Context.Callback, input.Context.Executor, decoded.Auth, decoded.Legs)
	if err != nil {
		tb.Fatal(err)
	}
	sig := slices.Clone(decoded.AuthSig)
	if len(sig) != crypto.SignatureLength {
		tb.Fatalf("callback auth signature length = %d, want %d", len(sig), crypto.SignatureLength)
	}
	if sig[64] >= 27 {
		sig[64] -= 27
	}
	pub, err := crypto.SigToPub(digest.Bytes(), sig)
	if err != nil {
		tb.Fatalf("recover callback auth signer: %v", err)
	}
	normalized.RecoveredSigner = crypto.PubkeyToAddress(*pub)

	scored := strategy.scoredLegs(input.Auction, input.Now, input.Adapter)
	bundle, skip := strategy.engine.selectBundleWithGas(scored, liquidLaneStateFromAdapter(input.Adapter), input.Context.GasLimit, auctionFeedCount(input.Auction))
	if strategy.gasAccounting {
		rate := input.Context.GasPrices.TokenOutPerNative(input.Adapter.Loan)
		bundle, skip = strategy.engine.selectNetBundle(scored, rate, liquidLaneStateFromAdapter(input.Adapter), input.Context.MaxTxGasPrice, input.Context.GasLimit, auctionFeedCount(input.Auction))
	}
	if skip != "" {
		tb.Fatalf("normalizing successful decision selected skip %q", skip)
	}
	prediction := predictGasForFeeds(legHints(bundle.legs), liquidLaneStateFromAdapter(input.Adapter), auctionFeedCount(input.Auction))
	normalized.GasUnits = prediction.Units
	normalized.GasNative = gasCostNative(prediction.Units, input.Context.MaxTxGasPrice).String()
	normalized.ExpectedLoanOut = make([]string, len(bundle.legs))
	for i, leg := range bundle.legs {
		normalized.ExpectedLoanOut[i] = leg.expectedLoanOut.String()
	}
	return normalized
}

func cloneBidInput(in types.BidInput) types.BidInput {
	out := in
	out.Auction.Prices = slices.Clone(in.Auction.Prices)
	for i := range out.Auction.Prices {
		out.Auction.Prices[i].Price = cloneBig(out.Auction.Prices[i].Price)
	}
	out.Adapter.FreeAssets = cloneBig(in.Adapter.FreeAssets)
	out.Adapter.Withdrawable = cloneBig(in.Adapter.Withdrawable)
	out.Adapter.Redeemable = slices.Clone(in.Adapter.Redeemable)
	for i := range out.Adapter.Redeemable {
		out.Adapter.Redeemable[i].MaxRate = cloneBig(out.Adapter.Redeemable[i].MaxRate)
		out.Adapter.Redeemable[i].MaxAssets = cloneBig(out.Adapter.Redeemable[i].MaxAssets)
		out.Adapter.Redeemable[i].AcquireBalance = cloneBig(out.Adapter.Redeemable[i].AcquireBalance)
	}
	out.Context.ChainID = cloneBig(in.Context.ChainID)
	out.Context.ExecutorDeposit = cloneBig(in.Context.ExecutorDeposit)
	out.Context.ExecutorMinDeposit = cloneBig(in.Context.ExecutorMinDeposit)
	out.Context.MaxTxGasPrice = cloneBig(in.Context.MaxTxGasPrice)
	out.PendingAuctions = slices.Clone(in.PendingAuctions)
	return out
}

func cloneSnapshot(in *snapshot) *snapshot {
	if in == nil {
		return nil
	}
	out := &snapshot{block: in.block, blockTime: in.blockTime, updatedAt: in.updatedAt}
	if in.markets != nil {
		out.markets = make(map[common.Hash]MarketInfo, len(in.markets))
	}
	if in.prices != nil {
		out.prices = make(map[common.Hash]*big.Int, len(in.prices))
	}
	if in.quotes != nil {
		out.quotes = make(map[common.Hash]AdapterQuote, len(in.quotes))
	}
	if in.positions != nil {
		out.positions = make(map[common.Hash]map[common.Address]morpho.PositionState, len(in.positions))
	}
	for id, market := range in.markets {
		market.Params.Lltv = cloneBig(market.Params.Lltv)
		market.State = morpho.CloneMarketState(market.State)
		out.markets[id] = market
	}
	for id, price := range in.prices {
		out.prices[id] = cloneBig(price)
	}
	for id, quote := range in.quotes {
		quote.MaxRate, quote.MaxAssets = cloneBig(quote.MaxRate), cloneBig(quote.MaxAssets)
		quote.LoanScale, quote.CollScale = cloneBig(quote.LoanScale), cloneBig(quote.CollScale)
		out.quotes[id] = quote
	}
	for id, positions := range in.positions {
		out.positions[id] = make(map[common.Address]morpho.PositionState, len(positions))
		for borrower, position := range positions {
			out.positions[id][borrower] = morpho.ClonePositionState(position)
		}
	}
	return out
}

func TestDefaultStrategySeededCorpusOrderingReservationsAndReplay(t *testing.T) {
	strategy, input := newCorpusCharacterizationFixture(t)
	baselineInput := cloneBidInput(input)

	first, err := strategy.DecideBid(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	firstNormalized := normalizeDecision(t, strategy, input, first)
	if firstNormalized.Decision != types.DecisionBid {
		t.Fatalf("first corpus decision = %+v", firstNormalized)
	}
	wantOrder := []struct {
		market   common.Hash
		borrower common.Address
	}{
		{common.BigToHash(big.NewInt(1)), common.BigToAddress(big.NewInt(1))},
		{common.BigToHash(big.NewInt(1)), common.BigToAddress(big.NewInt(2))},
	}
	if len(firstNormalized.Legs) != len(wantOrder) {
		t.Fatalf("selected legs = %+v, want %d shared-capacity legs", firstNormalized.Legs, len(wantOrder))
	}
	for i, want := range wantOrder {
		if firstNormalized.Legs[i].MarketID != want.market || firstNormalized.Legs[i].Borrower != want.borrower {
			t.Fatalf("leg %d identity = %s/%s, want %s/%s", i, firstNormalized.Legs[i].MarketID, firstNormalized.Legs[i].Borrower, want.market, want.borrower)
		}
	}
	assertCorpusReplaysSameMarket(t, strategy, input)
	if !reflect.DeepEqual(input, baselineInput) {
		t.Fatal("corpus decision mutated its input")
	}

	pending := cloneBidInput(input)
	pending.Auction.ID = "corpus-reserved"
	pending.PendingAuctions = []types.PendingAuction{{ID: input.Auction.ID, ExpiresAt: input.Now.Add(time.Minute)}}
	reserved, err := strategy.DecideBid(t.Context(), pending)
	if err != nil {
		t.Fatal(err)
	}
	if reserved.Decision != types.DecisionBid {
		t.Fatalf("independent markets should remain live after reserving first bundle: %+v", reserved)
	}
	reservedNormalized := normalizeDecision(t, strategy, pending, reserved)
	for _, leg := range reservedNormalized.Legs {
		for _, used := range wantOrder {
			if leg.MarketID == used.market && leg.Borrower == used.borrower {
				t.Fatalf("reserved position was reused: %+v", leg)
			}
		}
	}

	callbackTight, callbackInput := newCorpusCharacterizationFixture(t)
	callbackTight.state.store(decisionState{CallbackNative: new(big.Int).Set(callbackTight.cfg.BidWei), CallbackUpdatedAt: callbackInput.Now})
	if _, err := callbackTight.DecideBid(t.Context(), callbackInput); err != nil {
		t.Fatal(err)
	}
	callbackInput.Auction.ID = "corpus-callback-reservation"
	callbackInput.PendingAuctions = []types.PendingAuction{{ID: "corpus", ExpiresAt: callbackInput.Now.Add(time.Minute)}}
	callbackOut, err := callbackTight.DecideBid(t.Context(), callbackInput)
	if err != nil || callbackOut.Reason != types.SkipReasonCallbackBalance {
		t.Fatalf("callback reservation gate = %+v, err=%v", callbackOut, err)
	}

	depositTight, depositInput := newCorpusCharacterizationFixture(t)
	oneGas := gasCostNative(firstNormalized.GasUnits, depositInput.Context.MaxTxGasPrice)
	depositInput.Context.ExecutorDeposit = new(big.Int).Sub(
		new(big.Int).Add(depositInput.Context.ExecutorMinDeposit, new(big.Int).Mul(oneGas, big.NewInt(2))),
		big.NewInt(1),
	)
	if _, err := depositTight.DecideBid(t.Context(), depositInput); err != nil {
		t.Fatal(err)
	}
	depositInput.Auction.ID = "corpus-deposit-reservation"
	depositInput.PendingAuctions = []types.PendingAuction{{ID: "corpus", ExpiresAt: depositInput.Now.Add(time.Minute)}}
	depositOut, err := depositTight.DecideBid(t.Context(), depositInput)
	if err != nil || depositOut.Reason != types.SkipReasonDepositLow {
		t.Fatalf("deposit reservation gate = %+v, err=%v", depositOut, err)
	}
}

func TestDefaultStrategyFramePriceOverridesAPICacheAndTestMonitorCache(t *testing.T) {
	for _, monitorType := range []string{"api", "test"} {
		t.Run(monitorType, func(t *testing.T) {
			strategy, input := newCorpusCharacterizationFixture(t)
			if monitorType == "test" {
				monitor := &testMonitor{log: logr.Discard()}
				monitor.snap.Store(cloneSnapshot(strategy.mon.snapshot()))
				strategy.mon = monitor
			}
			out, err := strategy.DecideBid(t.Context(), input)
			if err != nil || out.Decision != types.DecisionBid {
				t.Fatalf("frame-price decision = %+v, err=%v", out, err)
			}
			input.Auction.ID += "-without-frame"
			input.Auction.Prices = nil
			out, err = strategy.DecideBid(t.Context(), input)
			if err != nil || out.Decision != types.DecisionSkip || out.Reason != skipNoLegs {
				t.Fatalf("cached monitor price substituted for absent frame price: out=%+v err=%v", out, err)
			}
		})
	}
}

func assertCorpusReplaysSameMarket(t *testing.T, strategy *Strategy, input types.BidInput) {
	t.Helper()
	scored := strategy.scoredLegs(input.Auction, input.Now, input.Adapter)
	replayMarket := common.BigToHash(big.NewInt(1))
	poisoned := make([]scoredLeg, 0, 2)
	for _, candidate := range scored {
		if candidate.MarketId != replayMarket {
			continue
		}
		candidate.MaxSeizeAssets = big.NewInt(1)
		candidate.expectedLoanOut = big.NewInt(1)
		candidate.profit = mustBig("999999999999999999")
		poisoned = append(poisoned, candidate)
	}
	replayed, skip := strategy.engine.selectBundleWithGas(poisoned, liquidLaneStateFromAdapter(input.Adapter), input.Context.GasLimit, auctionFeedCount(input.Auction))
	if skip != "" || len(replayed.legs) != 2 {
		t.Fatalf("same-market replay selection = %d legs / skip %q, want two legs", len(replayed.legs), skip)
	}
	for i, leg := range replayed.legs {
		if leg.MaxSeizeAssets.Cmp(big.NewInt(1)) == 0 || leg.expectedLoanOut.Cmp(big.NewInt(1)) == 0 {
			t.Fatalf("same-market replay copied stale precomputed leg %d: %+v", i, leg)
		}
	}
	if replayed.grossLoan.Cmp(mustBig("999999999999999999")) >= 0 {
		t.Fatalf("same-market replay copied stale precomputed profit: %s", replayed.grossLoan)
	}
}

func TestDefaultStrategyRepeatedAndConcurrentDecisionBytes(t *testing.T) {
	const calls = 8
	wantStrategy, wantInput := newDecisionCharacterizationFixture(t, true)
	want, err := wantStrategy.DecideBid(t.Context(), wantInput)
	if err != nil {
		t.Fatal(err)
	}

	strategy, input := newDecisionCharacterizationFixture(t, true)
	ctx := t.Context()
	results := make(chan types.BidOutput, calls)
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
		if got.Decision != types.DecisionBid || got.BidAmount.Cmp(want.BidAmount) != 0 || !bytes.Equal(got.OperationData, want.OperationData) {
			t.Fatalf("concurrent fixed-input bytes changed: decision=%s bid=%v dataEqual=%v", got.Decision, got.BidAmount, bytes.Equal(got.OperationData, want.OperationData))
		}
	}
}

func newCorpusCharacterizationFixture(tb testing.TB) (*Strategy, types.BidInput) {
	tb.Helper()
	strategy, input := newDecisionCharacterizationFixture(tb, false)
	markets := make(map[common.Hash]MarketInfo)
	positions := make(map[common.Hash]map[common.Address]morpho.PositionState)
	prices := make(map[common.Hash]*big.Int)
	input.Auction.ID = "corpus"
	input.Auction.Prices = nil
	for marketN := int64(3); marketN >= 1; marketN-- {
		id := common.BigToHash(big.NewInt(marketN))
		oracle := common.BigToAddress(big.NewInt(100 + marketN))
		info := MarketInfo{
			Params: MarketParams{LoanToken: characterizationLoan, CollateralToken: characterizationColl, Oracle: oracle, Lltv: mustBig("860000000000000000")},
			State:  goldenMarket(),
		}
		markets[id] = info
		prices[id] = mustBig("5000000000000000000000000000")
		positions[id] = make(map[common.Address]morpho.PositionState)
		for borrowerN := int64(2); borrowerN >= 1; borrowerN-- {
			positions[id][common.BigToAddress(big.NewInt((marketN-1)*2+borrowerN))] = goldenBorrower()
		}
		input.Auction.Prices = append(input.Auction.Prices, types.AuctionPrice{Oracle: oracle, Price: mustBig("1550000000000000000000000000")})
	}
	input.Auction.RawPriceCount = len(input.Auction.Prices)
	strategy.mon.(*apiMonitor).snap.Store(&snapshot{
		markets: markets, prices: prices, positions: positions,
		block: 100, blockTime: uint64(input.Now.Unix()), updatedAt: input.Now,
	})
	// Two legs fit this shared per-collateral redemption budget; the third would over-reserve it.
	input.Adapter.Redeemable[0].MaxAssets = big.NewInt(3_600_000_000)
	input.Adapter.Redeemable[0].AcquireBalance = big.NewInt(100_000_000_000)
	return strategy, input
}
