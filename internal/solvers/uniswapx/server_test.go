package uniswapx

import (
	"bytes"
	"context"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

func TestQuoteHTTPServerRoutesHealth(t *testing.T) {
	solver := &Solver{
		cfg: &Config{QuoteServer: QuoteServerConfig{HTTPTimeout: time.Second}},
		log: logr.Discard(),
	}
	server := solver.newQuoteHTTPServer()

	for _, path := range []string{"/health", "/healthz"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
			response := httptest.NewRecorder()

			server.Handler.ServeHTTP(response, request)

			if response.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
			}
		})
	}
}

func TestQuoteDelegatesOneRequestedAmountToStrategy(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	strategy := &quoteTestStrategy{quote: &strategytypes.Quote{AmountIn: big.NewInt(100), AmountOut: big.NewInt(90)}}
	solver := newQuoteTestSolver(t, tokenIn, strategy)
	request := validQuoteRequest(tokenIn, tokenOut)

	response, err := solver.quote(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.AmountIn != "100" || response.AmountOut != "90" {
		t.Fatalf("response = %+v", response)
	}
	if len(strategy.inputs) != 1 || strategy.inputs[0].AmountIn.String() != "100" || strategy.inputs[0].AmountOut != nil {
		t.Fatalf("strategy inputs = %+v", strategy.inputs)
	}
	if strategy.inputs[0].TokenIn != tokenIn || strategy.inputs[0].TokenOut != tokenOut {
		t.Fatalf("strategy pair = %s -> %s", strategy.inputs[0].TokenIn, strategy.inputs[0].TokenOut)
	}

	request.Type = quoteTypeExactOutput
	request.Amount = "70"
	strategy.quote = &strategytypes.Quote{AmountIn: big.NewInt(80), AmountOut: big.NewInt(70)}
	response, err = solver.quote(t.Context(), request)
	if err != nil || response.AmountIn != "80" || response.AmountOut != "70" {
		t.Fatalf("exact-output response = %+v, err %v", response, err)
	}
	if strategy.inputs[1].AmountIn != nil || strategy.inputs[1].AmountOut.String() != "70" {
		t.Fatalf("exact-output strategy input = %+v", strategy.inputs[1])
	}
}

func TestQuoteRejectsStrategyThatChangesRequestedSide(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	strategy := &quoteTestStrategy{quote: &strategytypes.Quote{AmountIn: big.NewInt(99), AmountOut: big.NewInt(90)}}
	solver := newQuoteTestSolver(t, tokenIn, strategy)

	if _, err := solver.quote(t.Context(), validQuoteRequest(tokenIn, tokenOut)); err == nil {
		t.Fatal("quote error = nil, want changed exact-input rejection")
	}
}

func TestQuoteDoesNotSelfBlockIndicativeThenHardRound(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	strategy := &quoteTestStrategy{quote: &strategytypes.Quote{AmountIn: big.NewInt(100), AmountOut: big.NewInt(90)}}
	solver := newQuoteTestSolver(t, tokenIn, strategy)
	request := validQuoteRequest(tokenIn, tokenOut)

	for _, quoteID := range []string{"indicative", "indicative", "hard"} {
		request.QuoteID = quoteID
		response, err := solver.quote(t.Context(), request)
		if err != nil || response.AmountOut != "90" {
			t.Fatalf("quote %s = %+v, err %v", quoteID, response, err)
		}
	}
	if reservations := solver.capacity.Snapshot(); len(reservations) != 0 {
		t.Fatalf("quotes unexpectedly reserved capacity: %v", reservations)
	}
}

func TestQuoteDeclinesExpiredState(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	strategy := &quoteTestStrategy{quote: &strategytypes.Quote{AmountIn: big.NewInt(100), AmountOut: big.NewInt(90)}}
	solver := newQuoteTestSolver(t, tokenIn, strategy)
	state := solver.quoteState.Load()
	state.expiresAt = time.Now().Add(-time.Second)

	response, err := solver.quote(t.Context(), validQuoteRequest(tokenIn, tokenOut))
	if err != nil || response.AmountOut != "0" || response.declineReason != "quote-state-unavailable" ||
		len(strategy.inputs) != 0 {
		t.Fatalf("expired response = %+v, inputs = %d, err %v", response, len(strategy.inputs), err)
	}
}

func TestQuoteDeclinesStatePublishedForOldEpoch(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	strategy := &quoteTestStrategy{quote: &strategytypes.Quote{AmountIn: big.NewInt(100), AmountOut: big.NewInt(90)}}
	solver := newQuoteTestSolver(t, tokenIn, strategy)
	solver.quoteEpoch.Store(1)

	response, err := solver.quote(t.Context(), validQuoteRequest(tokenIn, tokenOut))

	if err != nil || response.AmountOut != "0" || len(strategy.inputs) != 0 {
		t.Fatalf("stale-epoch response = %+v, inputs = %d, err %v", response, len(strategy.inputs), err)
	}
}

func TestQuoteDeclinesWhenStateChangesDuringStrategy(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")

	tests := []struct {
		name       string
		invalidate func(*Solver)
	}{
		{
			name: "fill planning",
			invalidate: func(s *Solver) {
				s.beginFillPlanning()
				s.endFillPlanning()
			},
		},
		{
			name: "reservation",
			invalidate: func(s *Solver) {
				s.setPendingReservations(common.HexToHash("0x1"), liquidlane.CapacityReservations{
					"capacity-1": big.NewInt(1),
				})
			},
		},
		{
			name: "breaker notification",
			invalidate: func(s *Solver) {
				s.setBlockUntil(time.Now().Add(time.Minute).Unix())
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			strategy := &blockingQuoteStrategy{
				entered: make(chan struct{}), release: make(chan struct{}),
				quote: &strategytypes.Quote{AmountIn: big.NewInt(100), AmountOut: big.NewInt(90)},
			}
			solver := newBlockingQuoteTestSolver(t, tokenIn, strategy)
			result := make(chan quoteResponse, 1)
			errs := make(chan error, 1)
			go func() {
				response, err := solver.quote(t.Context(), validQuoteRequest(tokenIn, tokenOut))
				result <- response
				errs <- err
			}()

			<-strategy.entered
			tc.invalidate(solver)
			close(strategy.release)

			if err := <-errs; err != nil {
				t.Fatal(err)
			}
			if response := <-result; response.AmountOut != "0" {
				t.Fatalf("invalidated quote = %+v", response)
			}
		})
	}
}

func TestQuoteDeclinesNativeOutput(t *testing.T) {
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	strategy := &quoteTestStrategy{quote: &strategytypes.Quote{AmountIn: big.NewInt(10), AmountOut: big.NewInt(10)}}
	solver := newQuoteTestSolver(t, tokenIn, strategy)
	request := validQuoteRequest(tokenIn, common.Address{})
	response, err := solver.quote(t.Context(), request)
	if err != nil || response.AmountOut != "0" || len(strategy.inputs) != 0 {
		t.Fatalf("native quote = %+v, inputs = %d, err %v", response, len(strategy.inputs), err)
	}
}

func TestQuoteHandlerHonorsCircuitBreaker(t *testing.T) {
	solver := &Solver{cfg: &Config{Executor: common.HexToAddress("0x1111111111111111111111111111111111111111")}}
	solver.chainTime.Store(100)

	notification := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/quote", bytes.NewBufferString(`{"blockUntilTimestamp":4000000000}`),
	)
	notificationResponse := httptest.NewRecorder()
	solver.quoteHandler(notificationResponse, notification)
	if notificationResponse.Code != http.StatusNoContent || solver.blockUntil.Load() != 4_000_000_000 {
		t.Fatalf("notification response/block = %d/%d", notificationResponse.Code, solver.blockUntil.Load())
	}

	response, err := solver.quote(t.Context(), quoteRequest{RequestID: "request", QuoteID: "quote"})
	if err != nil || response.AmountOut != "0" {
		t.Fatalf("blocked quote = %+v, err %v", response, err)
	}

	clearRequest := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/quote", bytes.NewBufferString(`{"blockUntilTimestamp":0}`),
	)
	clearResponse := httptest.NewRecorder()
	solver.quoteHandler(clearResponse, clearRequest)
	if clearResponse.Code != http.StatusNoContent || solver.blockUntil.Load() != 0 {
		t.Fatalf("clear response/block = %d/%d", clearResponse.Code, solver.blockUntil.Load())
	}
}

func TestQuoteHandlerRejectsMissingBreakerTimestamp(t *testing.T) {
	solver := &Solver{cfg: &Config{Executor: common.HexToAddress("0x1111111111111111111111111111111111111111")}}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/quote", bytes.NewBufferString(`{}`))
	response := httptest.NewRecorder()
	solver.quoteHandler(response, request)
	if response.Code != http.StatusBadRequest || solver.blockUntil.Load() != 0 {
		t.Fatalf("response/block = %d/%d, want 400/0", response.Code, solver.blockUntil.Load())
	}
}

func TestQuoteHandlerRejectsTrailingJSON(t *testing.T) {
	solver := &Solver{cfg: &Config{Executor: common.HexToAddress("0x1111111111111111111111111111111111111111")}}
	request := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/quote",
		bytes.NewBufferString(`{"blockUntilTimestamp":4000000000} {"blockUntilTimestamp":5000000000}`),
	)
	response := httptest.NewRecorder()
	solver.quoteHandler(response, request)
	if response.Code != http.StatusBadRequest || solver.blockUntil.Load() != 0 {
		t.Fatalf("response/block = %d/%d, want 400/0", response.Code, solver.blockUntil.Load())
	}
}

func newQuoteTestSolver(t *testing.T, tokenIn common.Address, strategy *quoteTestStrategy) *Solver {
	t.Helper()
	policy, err := tokenpolicy.New(tokenpolicy.All, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	solver := &Solver{
		chainID: 1,
		cfg: &Config{
			Executor:    common.HexToAddress("0x3333333333333333333333333333333333333333"),
			TokenPolicy: policy,
		},
		strategy: strategy,
	}
	solver.quoteState.Store(&quoteState{
		maxFeePerGas: big.NewInt(1), chainTime: now, expiresAt: now.Add(time.Minute),
		singleRouteFor: map[common.Address]bool{tokenIn: true},
	})
	return solver
}

func newBlockingQuoteTestSolver(t *testing.T, tokenIn common.Address, strategy strategytypes.Strategy) *Solver {
	t.Helper()
	policy, err := tokenpolicy.New(tokenpolicy.All, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	solver := &Solver{
		chainID: 1,
		cfg: &Config{
			Executor:    common.HexToAddress("0x3333333333333333333333333333333333333333"),
			TokenPolicy: policy,
		},
		strategy: strategy,
	}
	solver.quoteState.Store(&quoteState{
		maxFeePerGas: big.NewInt(1), chainTime: now, expiresAt: now.Add(time.Minute),
		singleRouteFor: map[common.Address]bool{tokenIn: true},
	})
	return solver
}

func validQuoteRequest(tokenIn, tokenOut common.Address) quoteRequest {
	return quoteRequest{
		RequestID: "request-1", QuoteID: "quote-1", TokenInChainID: 1, TokenOutChainID: 1,
		Swapper: common.HexToAddress("0x4444444444444444444444444444444444444444").Hex(),
		TokenIn: tokenIn.Hex(), TokenOut: tokenOut.Hex(), Amount: "100",
		Type: quoteTypeExactInput, NumOutputs: 1, Protocol: "v1",
	}
}

type quoteTestStrategy struct {
	quote  *strategytypes.Quote
	inputs []strategytypes.QuoteInput
}

func (s *quoteTestStrategy) DecideQuote(_ context.Context, input strategytypes.QuoteInput) (*strategytypes.Quote, error) {
	s.inputs = append(s.inputs, input)
	return s.quote, nil
}

func (s *quoteTestStrategy) DecideFill(context.Context, strategytypes.FillInput) (*strategytypes.FillPlan, error) {
	return nil, nil
}

type blockingQuoteStrategy struct {
	entered chan struct{}
	release chan struct{}
	quote   *strategytypes.Quote
}

func (s *blockingQuoteStrategy) DecideQuote(
	ctx context.Context,
	_ strategytypes.QuoteInput,
) (*strategytypes.Quote, error) {
	close(s.entered)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.release:
		return s.quote, nil
	}
}

func (s *blockingQuoteStrategy) DecideFill(
	context.Context,
	strategytypes.FillInput,
) (*strategytypes.FillPlan, error) {
	return nil, nil
}
