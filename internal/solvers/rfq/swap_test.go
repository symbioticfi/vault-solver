package rfq

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/default"
)

func TestSwapDirectLifecycleBuildsRouterBoundSignedCall(t *testing.T) {
	now := time.Unix(1_000, 0)
	candidate := directSwapCandidate()
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}, {candidate}}}
	state := newFakeSwapState(candidate.Route)
	signer := newSwapTestSigner(t)
	service := newTestSwapService(now, reader, state, signer)

	discovery := discoverySwapRequest()
	discoveryResponse, err := service.swap(t.Context(), &discovery)
	if err != nil {
		t.Fatalf("DISCOVERY: %v", err)
	}
	if discoveryResponse.Points == nil || len(*discoveryResponse.Points) != 2 ||
		(*discoveryResponse.Points)[0].AmountIn != "40" || (*discoveryResponse.Points)[0].AmountOut != "80" ||
		(*discoveryResponse.Points)[1].AmountIn != "100" || (*discoveryResponse.Points)[1].AmountOut != "200" {
		t.Fatalf("discovery response = %+v", discoveryResponse)
	}
	if reader.calls != 1 || reader.amounts[0].String() != "100" {
		t.Fatalf("discovery candidate reads = %d, amounts %v", reader.calls, reader.amounts)
	}

	confirm := confirmSwapRequest(discovery.RequestID, 1_020)
	confirmResponse, err := service.swap(t.Context(), &confirm)
	if err != nil {
		t.Fatalf("CONFIRM: %v", err)
	}
	if confirmResponse.SolverQuoteID != testSolverQuoteID || confirmResponse.AmountIn != "100" ||
		confirmResponse.AmountOut != "200" || confirmResponse.ValidUntil != 1_020 ||
		len(confirmResponse.LiquidityDomains) != 1 {
		t.Fatalf("confirm response = %+v", confirmResponse)
	}

	build := buildSwapRequest(confirmResponse, 1_020)
	buildResponse, err := service.swap(t.Context(), &build)
	if err != nil {
		t.Fatalf("BUILD: %v", err)
	}
	if buildResponse.Calls == nil || len(*buildResponse.Calls) != 1 {
		t.Fatalf("build response = %+v", buildResponse)
	}
	call := (*buildResponse.Calls)[0]
	if call.To != testAdapter || call.AmountIn != "100" || call.AmountOut != "200" ||
		call.TokenOut != testTokenOut || len(call.Data) <= 10 || call.Data[:10] != "0x9a4568b6" {
		t.Fatalf("signed call = %+v", call)
	}
	decoded, _ := unpackSignedCall(t, common.FromHex(call.Data))
	if decoded.Recipient != common.HexToAddress(testRouter) || decoded.Caller != common.HexToAddress(testRouter) ||
		decoded.Signer != signer.Address() || decoded.Deadline.Cmp(big.NewInt(1_020)) != 0 {
		t.Fatalf("decoded signed call = %+v", decoded)
	}
	if signer.calls != 1 || state.nonceReads != 1 || state.fillReads != 1 {
		t.Fatalf("build dependencies: signer=%d nonce=%d fill=%d", signer.calls, state.nonceReads, state.fillReads)
	}

	retry, err := service.swap(t.Context(), &build)
	if err != nil || (*retry.Calls)[0].Data != call.Data {
		t.Fatalf("idempotent BUILD = %+v, err %v", retry, err)
	}
	if signer.calls != 1 || state.nonceReads != 1 || state.fillReads != 1 {
		t.Fatal("cached BUILD repeated reads or signing")
	}
}

func TestSwapDiscoveryReturnsEmptyPointsWithoutLiquidity(t *testing.T) {
	now := time.Unix(1_000, 0)
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{nil}}
	service := newTestSwapService(now, reader, newFakeSwapState(directSwapCandidate().Route), newSwapTestSigner(t))
	request := discoverySwapRequest()
	response, err := service.swap(t.Context(), &request)
	if err != nil || response.Points == nil || len(*response.Points) != 0 {
		t.Fatalf("empty discovery = %+v, err %v", response, err)
	}
}

func TestSwapConfirmReturnsNoContentWhenExactInputCannotBeCovered(t *testing.T) {
	now := time.Unix(1_000, 0)
	candidate := directSwapCandidate()
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}, nil}}
	service := newTestSwapService(now, reader, newFakeSwapState(candidate.Route), newSwapTestSigner(t))
	discovery := discoverySwapRequest()
	if _, err := service.swap(t.Context(), &discovery); err != nil {
		t.Fatal(err)
	}
	confirm := confirmSwapRequest(discovery.RequestID, 1_020)
	if response, err := service.swap(t.Context(), &confirm); err != errSwapNoContent || response != nil {
		t.Fatalf("confirm response = %+v, err %v", response, err)
	}
}

func TestSwapConfirmRequiresExactDiscoveryTupleAndFloor(t *testing.T) {
	now := time.Unix(1_000, 0)
	candidate := directSwapCandidate()
	service := newTestSwapService(
		now, &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}}},
		newFakeSwapState(candidate.Route), newSwapTestSigner(t),
	)
	discovery := discoverySwapRequest()
	if _, err := service.swap(t.Context(), &discovery); err != nil {
		t.Fatal(err)
	}
	confirm := confirmSwapRequest(discovery.RequestID, 1_020)
	confirm.MinAmountOut = stringPtr("199")
	if _, err := service.swap(t.Context(), &confirm); err == nil {
		t.Fatal("changed discovery floor was accepted")
	}
}

func TestSwapBuildRejectsChangedTupleAndSecondBuildID(t *testing.T) {
	now := time.Unix(1_000, 0)
	candidate := directSwapCandidate()
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}, {candidate}}}
	service := newTestSwapService(now, reader, newFakeSwapState(candidate.Route), newSwapTestSigner(t))
	discovery := discoverySwapRequest()
	if _, err := service.swap(t.Context(), &discovery); err != nil {
		t.Fatal(err)
	}
	confirm := confirmSwapRequest(discovery.RequestID, 1_020)
	confirmed, err := service.swap(t.Context(), &confirm)
	if err != nil {
		t.Fatal(err)
	}
	build := buildSwapRequest(confirmed, 1_020)
	changed := build
	changed.MinAmountOut = stringPtr("199")
	if _, err := service.swap(t.Context(), &changed); err == nil {
		t.Fatal("changed confirmed floor was accepted")
	}
	if _, err := service.swap(t.Context(), &build); err != nil {
		t.Fatal(err)
	}
	build.BuildID = stringPtr(uuid.NewString())
	if _, err := service.swap(t.Context(), &build); err == nil {
		t.Fatal("second build ID was accepted")
	}
}

func TestSwapBuildRejectsUsedNonceAndStaleLeg(t *testing.T) {
	now := time.Unix(1_000, 0)
	candidate := directSwapCandidate()
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}, {candidate}}}
	state := newFakeSwapState(candidate.Route)
	service := newTestSwapService(now, reader, state, newSwapTestSigner(t))
	discovery := discoverySwapRequest()
	_, _ = service.swap(t.Context(), &discovery)
	confirm := confirmSwapRequest(discovery.RequestID, 1_020)
	confirmed, _ := service.swap(t.Context(), &confirm)
	build := buildSwapRequest(confirmed, 1_020)
	state.used = []bool{true}
	if _, err := service.swap(t.Context(), &build); err == nil {
		t.Fatal("used deterministic nonce was accepted")
	}
}

type fakeSwapCandidateReader struct {
	responses [][]liquidlane.QuoteCandidate
	err       error
	calls     int
	amounts   []*big.Int
}

func (f *fakeSwapCandidateReader) readQuoteCandidates(
	context.Context,
	[]solverInventory,
	common.Address,
	common.Address,
	*big.Int,
) ([]liquidlane.QuoteCandidate, error) {
	panic("amount must be captured by named implementation")
}

func (f *fakeSwapCandidateReader) read(ctx context.Context, inventory []solverInventory, tokenIn, tokenOut common.Address, amount *big.Int) ([]liquidlane.QuoteCandidate, error) {
	_ = ctx
	_ = inventory
	_ = tokenIn
	_ = tokenOut
	f.amounts = append(f.amounts, new(big.Int).Set(amount))
	index := f.calls
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if index >= len(f.responses) {
		index = len(f.responses) - 1
	}
	if index < 0 {
		return nil, nil
	}
	return append([]liquidlane.QuoteCandidate(nil), f.responses[index]...), nil
}

type fakeSwapState struct {
	domains    map[common.Address]swapDomain
	fill       liquidlane.FillQuote
	used       []bool
	err        error
	fillReads  int
	nonceReads int
}

func newFakeSwapState(route liquidlane.Route) *fakeSwapState {
	domain := sampleSwapDomain()
	domain.VerifyingContract = route.Adapter
	return &fakeSwapState{
		domains: map[common.Address]swapDomain{route.Adapter: domain},
		fill: liquidlane.FillQuote{
			Inventory: liquidlane.DirectInventory(route, big.NewInt(1_000), big.NewInt(2_000_000_000_000_000_000)),
			AmountIn:  big.NewInt(100), GrossAmountOut: big.NewInt(200), MaxAmountOut: big.NewInt(200),
		},
	}
}

func (f *fakeSwapState) validateRouter(context.Context, common.Address) error { return f.err }

func (f *fakeSwapState) validateAdapters(context.Context, []common.Address, common.Address) (map[common.Address]swapDomain, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.domains, nil
}

func (f *fakeSwapState) readFillQuote(context.Context, liquidlane.Route, *big.Int) (liquidlane.FillQuote, error) {
	f.fillReads++
	if f.err != nil {
		return liquidlane.FillQuote{}, f.err
	}
	return cloneFillQuote(f.fill), nil
}

func (f *fakeSwapState) readUsedNonces(context.Context, []swapNonceCheck) ([]bool, error) {
	f.nonceReads++
	if f.err != nil {
		return nil, f.err
	}
	if f.used != nil {
		return append([]bool(nil), f.used...), nil
	}
	return []bool{false}, nil
}

func newTestSwapService(
	now time.Time,
	reader *fakeSwapCandidateReader,
	state swapStateReader,
	signer *swapTestSigner,
) *swapService {
	return &swapService{
		chainID: 1, executor: common.HexToAddress("0x9999999999999999999999999999999999999999"),
		router: common.HexToAddress(testRouter), quoteTTL: 30 * time.Second, reader: quoteCandidateReaderFunc(reader.read),
		state: state, strategy: defaultstrategy.New(), store: newSwapStore(func() time.Time { return now }),
		signer: signer, now: func() time.Time { return now }, newID: func() uuid.UUID { return uuid.MustParse(testSolverQuoteID) },
		log: logr.Discard(),
	}
}

type quoteCandidateReaderFunc func(context.Context, []solverInventory, common.Address, common.Address, *big.Int) ([]liquidlane.QuoteCandidate, error)

func (f quoteCandidateReaderFunc) readQuoteCandidates(
	ctx context.Context,
	inv []solverInventory,
	tokenIn common.Address,
	tokenOut common.Address,
	amount *big.Int,
) ([]liquidlane.QuoteCandidate, error) {
	return f(ctx, inv, tokenIn, tokenOut, amount)
}

func directSwapCandidate() liquidlane.QuoteCandidate {
	route := liquidlane.NewRoute(
		1, common.HexToAddress(testAdapter), common.HexToAddress(testVault), common.HexToAddress(testTokenIn),
		common.HexToAddress(testTokenOut), 0, 0,
	)
	return liquidlane.QuoteCandidate{
		ID: liquidlane.NewCandidateID(route, nil), Route: route, Rate: big.NewInt(2_000_000_000_000_000_000),
		MaxAmountIn: big.NewInt(100), MaxAmountOut: big.NewInt(200),
	}
}

func discoverySwapRequest() swapRequest {
	r := baseSwapRequest(swapPhaseDiscovery, testDiscoveryRequestID)
	r.SampleAmountsIn = []string{"40", "100"}
	r.Adapters = []quoteAdapter{{
		Adapter: testAdapter, Asset: testTokenOut, AssetDecimals: 0, MaxAssets: "1000",
		MaxRate: "2000000000000000000",
	}}
	return r
}

func confirmSwapRequest(discoveryID string, deadline int64) swapRequest {
	r := baseSwapRequest(swapPhaseConfirm, testConfirmRequestID)
	r.DiscoveryRequestID, r.AmountIn, r.MinAmountOut = stringPtr(discoveryID), stringPtr("100"), stringPtr("200")
	r.Deadline = int64Ptr(deadline)
	r.Adapters = discoverySwapRequest().Adapters
	return r
}

func buildSwapRequest(confirmed *swapResponse, deadline int64) swapRequest {
	r := baseSwapRequest(swapPhaseBuild, testBuildRequestID)
	r.SolverQuoteID, r.BuildID = stringPtr(confirmed.SolverQuoteID), stringPtr(testBuildID)
	r.AmountIn, r.MinAmountOut, r.Deadline = stringPtr(confirmed.AmountIn), stringPtr(confirmed.AmountOut), int64Ptr(deadline)
	r.LiquidityDomains = append([]string(nil), confirmed.LiquidityDomains...)
	r.Router = stringPtr(testRouter)
	return r
}

func stringPtr(value string) *string { return &value }
