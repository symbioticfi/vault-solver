package defaultstrategy

import (
	"bytes"
	"context"
	"math/big"
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	callbackbinding "github.com/symbioticfi/vault-solver/api/bindings/oev/callback"
	irmbinding "github.com/symbioticfi/vault-solver/api/bindings/oev/irm"
	morphobinding "github.com/symbioticfi/vault-solver/api/bindings/oev/morpho"
	oraclebinding "github.com/symbioticfi/vault-solver/api/bindings/oev/oracle"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

var (
	callbackTestABI = mustParseABI(callbackbinding.SymbioticOevSolverMetaData.ABI)
	irmTestABI      = mustParseABI(irmbinding.AdaptiveCurveIrmMetaData.ABI)
	morphoTestABI   = mustParseABI(morphobinding.MorphoMetaData.ABI)
	oracleTestABI   = mustParseABI(oraclebinding.MorphoOracleMetaData.ABI)
)

type recordingMulticaller struct {
	batches       [][]chain.Call
	blocks        []*big.Int
	results       [][]chain.CallResult
	latestBatches [][]chain.Call
	latestResults [][]chain.CallResult
	err           error
}

func (r *recordingMulticaller) Multicall(
	_ context.Context,
	calls []chain.Call,
) ([]chain.CallResult, error) {
	r.latestBatches = append(r.latestBatches, slices.Clone(calls))
	if r.err != nil {
		return nil, r.err
	}
	if len(r.latestResults) == 0 {
		return nil, errors.New("unexpected latest-block multicall")
	}
	result := r.latestResults[0]
	r.latestResults = r.latestResults[1:]
	return result, nil
}

func (r *recordingMulticaller) MulticallAt(
	_ context.Context,
	calls []chain.Call,
	block *big.Int,
) ([]chain.CallResult, error) {
	r.batches = append(r.batches, slices.Clone(calls))
	var copiedBlock *big.Int
	if block != nil {
		copiedBlock = new(big.Int).Set(block)
	}
	r.blocks = append(r.blocks, copiedBlock)
	if r.err != nil {
		return nil, r.err
	}
	if len(r.results) == 0 {
		return nil, errors.New("unexpected extra multicall")
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result, nil
}

func TestCallbackMorphoUsesGeneratedBindingAndFailsClosed(t *testing.T) {
	callbackAddr := common.HexToAddress("0x00000000000000000000000000000000000000cb")
	morphoAddr := common.HexToAddress("0x00000000000000000000000000000000000000ff")
	tests := []struct {
		name    string
		result  chain.CallResult
		want    common.Address
		wantErr bool
	}{
		{name: "resolved", result: chain.CallResult{
			Success: true, ReturnData: packOut(t, callbackTestABI, "MORPHO", morphoAddr),
		}, want: morphoAddr},
		{name: "zero", result: chain.CallResult{
			Success: true, ReturnData: packOut(t, callbackTestABI, "MORPHO", common.Address{}),
		}, wantErr: true},
		{name: "reverted", result: chain.CallResult{Success: false}, wantErr: true},
		{name: "garbled", result: chain.CallResult{Success: true, ReturnData: []byte{0x01}}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &recordingMulticaller{latestResults: [][]chain.CallResult{{tc.result}}}
			r := &chainReader{calls: fake, log: logr.Discard()}
			got, err := r.ReadCallbackMorpho(t.Context(), callbackAddr)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("callbackMorpho = (%s, %v), want (%s, err=%v)", got, err, tc.want, tc.wantErr)
			}
			if len(fake.latestBatches) != 1 || len(fake.latestBatches[0]) != 1 {
				t.Fatalf("calls = %+v", fake.latestBatches)
			}
			call := fake.latestBatches[0][0]
			if call.Target != callbackAddr || !call.AllowFailure || !bytes.Equal(call.Data, callbackABI.PackMORPHO()) {
				t.Fatalf("MORPHO call = %+v", call)
			}
		})
	}
}

func TestReadMarketStatesAtPinsBlockAndDecodesFeeRate(t *testing.T) {
	block := big.NewInt(123)
	morphoAddr := common.HexToAddress("0x00000000000000000000000000000000000000ff")
	nonzeroIRM := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	marketA := common.HexToHash("0x01")
	marketB := common.HexToHash("0x02")
	params := map[common.Hash]abiMarketParams{
		marketA: {
			LoanToken:       common.HexToAddress("0x0000000000000000000000000000000000000011"),
			CollateralToken: common.HexToAddress("0x0000000000000000000000000000000000000012"),
			Oracle:          common.HexToAddress("0x0000000000000000000000000000000000000013"),
			Irm:             nonzeroIRM, Lltv: mustBig("860000000000000000"),
		},
		marketB: {
			LoanToken:       common.HexToAddress("0x0000000000000000000000000000000000000021"),
			CollateralToken: common.HexToAddress("0x0000000000000000000000000000000000000022"),
			Oracle:          common.HexToAddress("0x0000000000000000000000000000000000000023"),
			Lltv:            mustBig("770000000000000000"),
		},
	}
	stateA := morphobinding.MarketOutput{
		TotalSupplyAssets: big.NewInt(1000), TotalSupplyShares: big.NewInt(900),
		TotalBorrowAssets: big.NewInt(500), TotalBorrowShares: big.NewInt(450),
		LastUpdate: big.NewInt(100), Fee: mustBig("100000000000000000"),
	}
	stateB := morphobinding.MarketOutput{
		TotalSupplyAssets: big.NewInt(2000), TotalSupplyShares: big.NewInt(1800),
		TotalBorrowAssets: big.NewInt(0), TotalBorrowShares: big.NewInt(0),
		LastUpdate: big.NewInt(101), Fee: big.NewInt(0),
	}
	fake := &recordingMulticaller{results: [][]chain.CallResult{
		{
			{Success: true, ReturnData: packOut(t, morphoTestABI, "market", stateA.TotalSupplyAssets, stateA.TotalSupplyShares, stateA.TotalBorrowAssets, stateA.TotalBorrowShares, stateA.LastUpdate, stateA.Fee)},
			{Success: true, ReturnData: packOut(t, morphoTestABI, "market", stateB.TotalSupplyAssets, stateB.TotalSupplyShares, stateB.TotalBorrowAssets, stateB.TotalBorrowShares, stateB.LastUpdate, stateB.Fee)},
		},
		{{Success: true, ReturnData: packOut(t, irmTestABI, "borrowRateView", big.NewInt(182418302))}},
	}}
	r := &chainReader{calls: fake, log: logr.Discard()}
	got, err := r.ReadMarketStatesAt(t.Context(), morphoAddr, params, block)
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.blocks) != 2 || fake.blocks[0].Cmp(block) != 0 || fake.blocks[1].Cmp(block) != 0 {
		t.Fatalf("blocks = %v, want two calls at %s", fake.blocks, block)
	}
	if got[marketA].Fee.Cmp(stateA.Fee) != 0 || got[marketA].BorrowRatePerSec.Cmp(big.NewInt(182418302)) != 0 {
		t.Fatalf("market A state = %+v", got[marketA])
	}
	if got[marketB].BorrowRatePerSec.Sign() != 0 {
		t.Fatalf("zero IRM rate = %s, want 0", got[marketB].BorrowRatePerSec)
	}
	if len(fake.batches[0]) != 2 || fake.batches[0][0].Target != morphoAddr || fake.batches[0][1].Target != morphoAddr {
		t.Fatalf("market batch = %+v", fake.batches[0])
	}
	if !bytes.Equal(fake.batches[0][0].Data, morphoABI.PackMarket(marketA)) ||
		!bytes.Equal(fake.batches[0][1].Data, morphoABI.PackMarket(marketB)) {
		t.Fatalf("market selectors/order = %x / %x", fake.batches[0][0].Data, fake.batches[0][1].Data)
	}
	if len(fake.batches[1]) != 1 || fake.batches[1][0].Target != nonzeroIRM {
		t.Fatalf("IRM batch = %+v", fake.batches[1])
	}
	expectedIRMCall := irmABI.PackBorrowRateView(irmParams(params[marketA]), irmMarket(got[marketA]))
	if recorded := fake.batches[1][0].Data; !bytes.Equal(recorded, expectedIRMCall) {
		t.Fatalf("borrowRateView calldata = %x, want %x", recorded, expectedIRMCall)
	}
}

func TestReadMarketStatesAtDropsFailedNonzeroIRM(t *testing.T) {
	block := big.NewInt(123)
	marketID := common.HexToHash("0x01")
	morphoAddr := common.HexToAddress("0x00000000000000000000000000000000000000ff")
	params := map[common.Hash]abiMarketParams{
		marketID: {
			Irm:  common.HexToAddress("0x00000000000000000000000000000000000000a1"),
			Lltv: mustBig("860000000000000000"),
		},
	}
	marketResult := chain.CallResult{Success: true, ReturnData: packOut(
		t, morphoTestABI, "market",
		big.NewInt(1000), big.NewInt(900), big.NewInt(500), big.NewInt(450),
		big.NewInt(100), mustBig("100000000000000000"),
	)}
	fake := &recordingMulticaller{results: [][]chain.CallResult{
		{marketResult},
		{{Success: false}},
	}}
	r := &chainReader{calls: fake, log: logr.Discard()}
	got, err := r.ReadMarketStatesAt(t.Context(), morphoAddr, params, block)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got[marketID]; ok {
		t.Fatal("market with reverted non-zero IRM was retained with a zero-rate fallback")
	}
}

func TestReadMarketStatesAtDropsUninitializedZeroMarket(t *testing.T) {
	block := big.NewInt(123)
	marketID := common.HexToHash("0x01")
	morphoAddr := common.HexToAddress("0x00000000000000000000000000000000000000ff")
	params := map[common.Hash]abiMarketParams{
		marketID: {
			Irm:  common.HexToAddress("0x00000000000000000000000000000000000000a1"),
			Lltv: mustBig("860000000000000000"),
		},
	}
	fake := &recordingMulticaller{results: [][]chain.CallResult{{{
		Success: true,
		ReturnData: packOut(t, morphoTestABI, "market",
			new(big.Int), new(big.Int), new(big.Int), new(big.Int), new(big.Int), new(big.Int)),
	}}}}
	r := &chainReader{calls: fake, log: logr.Discard()}
	got, err := r.ReadMarketStatesAt(t.Context(), morphoAddr, params, block)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("uninitialized market retained: %+v", got)
	}
	if len(fake.batches) != 1 {
		t.Fatalf("uninitialized market issued %d batches, want 1", len(fake.batches))
	}
}

func TestReadMarketStatesAtRejectsInvalidBoundary(t *testing.T) {
	validAddress := common.HexToAddress("0x00000000000000000000000000000000000000ff")
	tests := []struct {
		name   string
		morpho common.Address
		block  *big.Int
	}{
		{name: "zero Morpho", block: big.NewInt(1)},
		{name: "nil block", morpho: validAddress},
		{name: "negative block", morpho: validAddress, block: big.NewInt(-1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &recordingMulticaller{}
			r := &chainReader{calls: fake, log: logr.Discard()}
			if _, err := r.ReadMarketStatesAt(t.Context(), tc.morpho, nil, tc.block); err == nil {
				t.Fatal("invalid pinned-state boundary accepted")
			}
			if len(fake.batches) != 0 {
				t.Fatalf("invalid input issued %d batches", len(fake.batches))
			}
		})
	}
}

func TestReadMarketStatesAtRejectsResultLengthMismatch(t *testing.T) {
	block := big.NewInt(123)
	morphoAddr := common.HexToAddress("0x00000000000000000000000000000000000000ff")
	marketID := common.HexToHash("0x01")
	params := map[common.Hash]abiMarketParams{marketID: {Lltv: big.NewInt(1)}}
	fake := &recordingMulticaller{results: [][]chain.CallResult{{}}}
	r := &chainReader{calls: fake, log: logr.Discard()}
	if _, err := r.ReadMarketStatesAt(t.Context(), morphoAddr, params, block); err == nil {
		t.Fatal("short market result vector accepted")
	}
}

func TestReadMarketStatesAtRejectsRateResultLengthMismatch(t *testing.T) {
	block := big.NewInt(123)
	morphoAddr := common.HexToAddress("0x00000000000000000000000000000000000000ff")
	marketID := common.HexToHash("0x01")
	params := map[common.Hash]abiMarketParams{marketID: {
		Irm:  common.HexToAddress("0x00000000000000000000000000000000000000a1"),
		Lltv: big.NewInt(1),
	}}
	fake := &recordingMulticaller{results: [][]chain.CallResult{
		{{Success: true, ReturnData: packOut(t, morphoTestABI, "market",
			big.NewInt(1), big.NewInt(1), big.NewInt(1), big.NewInt(1), big.NewInt(1), big.NewInt(0))}},
		{},
	}}
	r := &chainReader{calls: fake, log: logr.Discard()}
	if _, err := r.ReadMarketStatesAt(t.Context(), morphoAddr, params, block); err == nil {
		t.Fatal("short IRM result vector accepted")
	}
}

func TestReadMarketStatesAtDropsUndecodableNonzeroIRM(t *testing.T) {
	block := big.NewInt(123)
	morphoAddr := common.HexToAddress("0x00000000000000000000000000000000000000ff")
	marketID := common.HexToHash("0x01")
	params := map[common.Hash]abiMarketParams{marketID: {
		Irm:  common.HexToAddress("0x00000000000000000000000000000000000000a1"),
		Lltv: big.NewInt(1),
	}}
	fake := &recordingMulticaller{results: [][]chain.CallResult{
		{{Success: true, ReturnData: packOut(t, morphoTestABI, "market",
			big.NewInt(1), big.NewInt(1), big.NewInt(1), big.NewInt(1), big.NewInt(1), big.NewInt(0))}},
		{{Success: true, ReturnData: []byte{0x01}}},
	}}
	r := &chainReader{calls: fake, log: logr.Discard()}
	got, err := r.ReadMarketStatesAt(t.Context(), morphoAddr, params, block)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got[marketID]; ok {
		t.Fatal("market with undecodable non-zero IRM was retained")
	}
}

func TestTestMonitorReadMarketsPinsOracleToStateBlock(t *testing.T) {
	block := big.NewInt(123)
	morphoAddr := common.HexToAddress("0x00000000000000000000000000000000000000ff")
	oracleAddr := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	marketID := common.HexToHash("0x01")
	params := map[common.Hash]abiMarketParams{
		marketID: {Oracle: oracleAddr, Lltv: mustBig("860000000000000000")},
	}
	fake := &recordingMulticaller{results: [][]chain.CallResult{
		{{Success: true, ReturnData: packOut(t, morphoTestABI, "market",
			big.NewInt(1000), big.NewInt(900), big.NewInt(500), big.NewInt(450),
			big.NewInt(100), big.NewInt(0))}},
		{{Success: true, ReturnData: packOut(t, oracleTestABI, "price", big.NewInt(42))}},
	}}
	r := &chainReader{calls: fake, log: logr.Discard()}
	markets, prices, err := r.ReadTestMarketStates(t.Context(), morphoAddr, params, block)
	if err != nil {
		t.Fatal(err)
	}
	if len(markets) != 1 || prices[marketID].Cmp(big.NewInt(42)) != 0 {
		t.Fatalf("markets=%+v prices=%+v", markets, prices)
	}
	if len(fake.blocks) != 2 || fake.blocks[0].Cmp(block) != 0 || fake.blocks[1].Cmp(block) != 0 {
		t.Fatalf("blocks = %v, want market and oracle at %s", fake.blocks, block)
	}
	if len(fake.batches[1]) != 1 || fake.batches[1][0].Target != oracleAddr ||
		!bytes.Equal(fake.batches[1][0].Data, oracleABI.PackPrice()) {
		t.Fatalf("oracle batch = %+v", fake.batches[1])
	}
}
