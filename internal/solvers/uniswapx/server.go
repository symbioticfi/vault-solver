package uniswapx

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

const maxQuoteRequestBytes = 32 << 10

const (
	quoteTypeExactInput  = "EXACT_INPUT"
	quoteTypeExactOutput = "EXACT_OUTPUT"
)

func (s *Solver) newQuoteHTTPServer() *http.Server {
	mux := http.NewServeMux()
	healthHandler := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	mux.HandleFunc("POST /quote", s.quoteHandler)
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /healthz", healthHandler)
	mux.HandleFunc("GET /ready", s.readyHandler)
	return &http.Server{
		Addr: s.cfg.QuoteServer.ListenAddress, Handler: recoverQuoteServer(mux, s.log),
		ReadHeaderTimeout: 2 * time.Second, ReadTimeout: s.cfg.QuoteServer.HTTPTimeout,
		WriteTimeout: s.cfg.QuoteServer.HTTPTimeout, IdleTimeout: 30 * time.Second,
	}
}

func (s *Solver) quoteHandler(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	defer func() {
		if s.metrics != nil {
			s.metrics.observeQuoteLatency(time.Since(started))
		}
	}()
	body, err := io.ReadAll(io.LimitReader(r.Body, maxQuoteRequestBytes+1))
	if err != nil {
		s.log.V(1).Info("quote request rejected", "reason", "read-body", "error", err.Error())
		s.observeQuote(quoteOutcomeInvalid)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(body) > maxQuoteRequestBytes {
		s.log.V(1).Info("quote request rejected", "reason", "body-too-large", "bytes", len(body))
		s.observeQuote(quoteOutcomeInvalid)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	var request quoteRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		s.log.V(1).Info("quote request rejected", "reason", "invalid-json", "error", err.Error())
		s.observeQuote(quoteOutcomeInvalid)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		s.log.V(1).Info(
			"quote request rejected",
			"reason", "trailing-json",
			"requestId", request.RequestID,
			"quoteId", request.QuoteID,
		)
		s.observeQuote(quoteOutcomeInvalid)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if request.RequestID == "" {
		if request.BlockUntilTimestamp == nil || *request.BlockUntilTimestamp < 0 {
			s.log.V(1).Info("quote request rejected", "reason", "invalid-breaker-notification")
			s.observeQuote(quoteOutcomeInvalid)
			http.Error(w, "invalid blockUntilTimestamp", http.StatusBadRequest)
			return
		}
		s.log.V(1).Info(
			"quote breaker notification received",
			"blockUntilTimestamp", *request.BlockUntilTimestamp,
		)
		s.setBlockUntil(*request.BlockUntilTimestamp)
		s.observeQuote(quoteOutcomeBreakerNotification)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.log.V(1).Info(
		"quote request received",
		"requestId", request.RequestID,
		"quoteId", request.QuoteID,
		"type", request.Type,
		"protocol", request.Protocol,
		"tokenIn", request.TokenIn,
		"tokenOut", request.TokenOut,
		"amount", request.Amount,
	)
	response, err := s.quote(r.Context(), request)
	if err != nil {
		s.observeQuote(quoteOutcomeError)
		s.log.Error(err, "quote failed", "requestId", request.RequestID, "quoteId", request.QuoteID)
		http.Error(w, "quote unavailable", http.StatusServiceUnavailable)
		return
	}
	if response.AmountOut == "0" {
		s.observeQuoteDecline(response.declineReason)
		s.log.V(1).Info(
			"quote declined",
			"requestId", request.RequestID,
			"quoteId", request.QuoteID,
			"type", request.Type,
			"reason", response.declineReason,
			"blockUntil", s.blockUntil.Load(),
			"localBlockUntil", s.localBlockUntil.Load(),
			"exclusiveBlockUntil", s.exclusiveBlockUntil.Load(),
			"warmupUntil", s.warmupUntil.Load(),
			"planningFills", s.planningFills.Load(),
		)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.observeQuote(quoteOutcomeQuoted)
	s.observeQuotedAmounts(response)
	s.log.V(1).Info(
		"quote returned",
		"requestId", request.RequestID,
		"quoteId", request.QuoteID,
		"type", request.Type,
		"amountIn", response.AmountIn,
		"amountOut", response.AmountOut,
	)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.log.Error(err, "write quote response", "requestId", request.RequestID, "quoteId", request.QuoteID)
	}
}

func (s *Solver) quote(ctx context.Context, request quoteRequest) (quoteResponse, error) {
	response := quoteResponse{
		ChainID: s.chainID, RequestID: request.RequestID, Swapper: request.Swapper, TokenIn: request.TokenIn,
		AmountIn: "0", TokenOut: request.TokenOut, AmountOut: "0",
		Filler: s.cfg.Executor.Hex(), QuoteID: request.QuoteID,
	}
	if request.Type == quoteTypeExactInput {
		response.AmountIn = request.Amount
	}
	now := s.currentTime()
	if s.quoteBlocked(now) {
		return declinedQuote(response, quoteDeclineBlocked), nil
	}
	if request.RequestID == "" || request.QuoteID == "" || !supportedQuoteType(request.Type) || request.NumOutputs < 1 ||
		!supportedQuoteProtocol(request.Protocol) || request.TokenInChainID != s.chainID || request.TokenOutChainID != s.chainID ||
		!common.IsHexAddress(request.Swapper) ||
		!common.IsHexAddress(request.TokenIn) || !common.IsHexAddress(request.TokenOut) {
		return declinedQuote(response, quoteDeclineInvalidRequest), nil
	}
	tokenIn := common.HexToAddress(request.TokenIn)
	tokenOut := common.HexToAddress(request.TokenOut)
	if tokenIn == tokenOut || tokenOut == (common.Address{}) || !s.cfg.TokenPolicy.Allows(tokenIn) {
		return declinedQuote(response, quoteDeclinePairOutOfScope), nil
	}
	requestAmount, amountOK := new(big.Int).SetString(request.Amount, 10)
	if !amountOK || requestAmount.Sign() <= 0 {
		return declinedQuote(response, quoteDeclineInvalidAmount), nil
	}
	epoch := s.quoteEpoch.Load()
	state := s.quoteState.Load()
	if state == nil || state.epoch != epoch || !state.expiresAt.After(time.Unix(now, 0)) {
		return declinedQuote(response, quoteDeclineQuoteStateUnavailable), nil
	}
	input := strategytypes.QuoteInput{
		RequestID: request.RequestID, QuoteID: request.QuoteID,
		TokenIn: tokenIn, TokenOut: tokenOut,
		RequireSingleRoute: state.singleRouteFor[tokenIn],
		Inventory:          state.inventory,
		Reservations:       s.capacity.Snapshot(),
		GasSnapshot:        state.gasSnapshot, GasPrices: state.gasPrices,
		MaxFeePerGas: state.maxFeePerGas, ChainTime: state.chainTime, QuoteExpiresAt: state.expiresAt,
		Trace: s.decisionTrace(
			"requestId", request.RequestID,
			"quoteId", request.QuoteID,
			"quoteType", request.Type,
		),
	}
	if request.Type == quoteTypeExactInput {
		input.AmountIn = requestAmount
	} else {
		input.AmountOut = requestAmount
	}
	quote, err := s.strategy.DecideQuote(ctx, input)
	if err != nil {
		return response, err
	}
	if quote == nil {
		return declinedQuote(response, quoteDeclineStrategy), nil
	}
	if err := validateStrategyQuote(input, quote); err != nil {
		return response, err
	}
	if s.quoteEpoch.Load() != epoch || s.quoteState.Load() != state || s.quoteBlocked(s.currentTime()) {
		return declinedQuote(response, quoteDeclineStateChanged), nil
	}
	response.AmountIn = quote.AmountIn.String()
	response.AmountOut = quote.AmountOut.String()
	return response, nil
}

func declinedQuote(response quoteResponse, reason quoteDeclineReason) quoteResponse {
	response.declineReason = reason
	return response
}

func (s *Solver) quoteBlocked(now int64) bool {
	return s.timeBasedBlockUntil() > now ||
		s.planningFills.Load() != 0 ||
		(s.txm != nil && !s.txm.LaneReady()) ||
		!s.exclusiveDeliveryHealthy()
}

func (s *Solver) timeBasedBlockUntil() int64 {
	return max(
		s.blockUntil.Load(),
		s.localBlockUntil.Load(),
		s.exclusiveBlockUntil.Load(),
		s.warmupUntil.Load(),
	)
}

func supportedQuoteType(value string) bool {
	return value == quoteTypeExactInput || value == quoteTypeExactOutput
}

func supportedQuoteProtocol(value string) bool {
	return value == "v1" || value == "v2"
}

func validateStrategyQuote(input strategytypes.QuoteInput, quote *strategytypes.Quote) error {
	if quote.AmountIn == nil || quote.AmountIn.Sign() <= 0 || quote.AmountOut == nil || quote.AmountOut.Sign() <= 0 {
		return errors.New("strategy returned invalid quote amounts")
	}
	if input.AmountIn != nil && quote.AmountIn.Cmp(input.AmountIn) != 0 {
		return errors.New("strategy changed exact-input amount")
	}
	if input.AmountOut != nil && quote.AmountOut.Cmp(input.AmountOut) != 0 {
		return errors.New("strategy changed exact-output amount")
	}
	return nil
}

func (s *Solver) currentTime() int64 {
	now := time.Now().Unix()
	if chainTime := s.chainTime.Load(); chainTime > now {
		return chainTime
	}
	return now
}

func (s *Solver) setBlockUntil(timestamp int64) {
	s.blockUntil.Store(timestamp)
	if timestamp > s.currentTime() {
		s.invalidateQuotes()
	}
	s.requestQuoteRefresh()
}
