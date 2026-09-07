package uniswapx

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/planning"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/parse"
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
	var request quoteRequest
	if err := parse.JSON(http.MaxBytesReader(w, r.Body, maxQuoteRequestBytes), &request); err != nil {
		s.log.V(1).Info("quote request rejected", "reason", "invalid-json", "error", err.Error())
		s.observeQuote(quoteOutcomeInvalid)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if request.RequestID == "" {
		if request.BlockUntilTimestamp == nil || *request.BlockUntilTimestamp < 0 {
			s.observeQuote(quoteOutcomeInvalid)
			http.Error(w, "invalid blockUntilTimestamp", http.StatusBadRequest)
			return
		}
		s.setBlockUntil(*request.BlockUntilTimestamp)
		s.log.V(1).Info("quote breaker notification received", "blockUntilTimestamp", *request.BlockUntilTimestamp)
		s.observeQuote(quoteOutcomeBreakerNotification)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	log := s.log.WithValues("requestId", request.RequestID, "quoteId", request.QuoteID, "type", request.Type)
	log.V(1).Info("quote request received", "protocol", request.Protocol,
		"tokenIn", request.TokenIn, "tokenOut", request.TokenOut, "amount", request.Amount)
	response, err := s.quote(r.Context(), request)
	if err != nil {
		s.observeQuote(quoteOutcomeError)
		log.Error(err, "quote failed")
		http.Error(w, "quote unavailable", http.StatusServiceUnavailable)
		return
	}
	if response.AmountOut == "0" {
		s.observeQuoteDecline(response.declineReason)
		log.V(1).Info("quote declined", "reason", response.declineReason,
			"blockUntil", s.timeBasedBlockUntil(), "planningFills", s.quotes.planningCount())
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.observeQuote(quoteOutcomeQuoted)
	s.observeQuotedAmounts(response)
	log.V(1).Info("quote returned", "amountIn", response.AmountIn, "amountOut", response.AmountOut)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error(err, "write quote response")
	}
}

func (s *Solver) quote(ctx context.Context, request quoteRequest) (quoteResponse, error) {
	response := quoteResponse{ChainID: s.chainID, RequestID: request.RequestID, QuoteID: request.QuoteID,
		Swapper: request.Swapper, TokenIn: request.TokenIn, TokenOut: request.TokenOut,
		Filler: s.cfg.Executor.Hex(), AmountIn: "0", AmountOut: "0"}
	if request.Type == quoteTypeExactInput {
		response.AmountIn = request.Amount
	}
	now := s.quoteClock()
	if s.quoteBlocked(now.Unix()) {
		return declinedQuote(response, quoteDeclineBlocked), nil
	}
	input, decline := request.strategyInput(s.chainID, s.cfg.TokenPolicy)
	if decline != "" {
		return declinedQuote(response, decline), nil
	}
	snapshot := s.quotes.current()
	if !s.currentQuoteSnapshot(snapshot, now) {
		return declinedQuote(response, quoteDeclineQuoteStateUnavailable), nil
	}
	input.RequireSingleRoute, input.Inventory = snapshot.singleRouteFor[input.TokenIn], snapshot.inventory
	input.Reservations = s.capacity.Snapshot()
	input.GasSnapshot, input.GasPrices, input.MaxFeePerGas = snapshot.gasSnapshot, snapshot.gasPrices, snapshot.maxFeePerGas
	input.ChainTime, input.QuoteExpiresAt = snapshot.chainTime, snapshot.expiresAt
	input.Trace = planning.NewDecisionTrace(s.log, "requestId", request.RequestID, "quoteId", request.QuoteID, "quoteType", request.Type)
	decision, err := s.strategy.DecideQuote(ctx, input)
	if err != nil {
		return response, err
	}
	if decision == nil {
		return declinedQuote(response, quoteDeclineStrategy), nil
	}
	if err := validateStrategyQuote(input, decision); err != nil {
		return response, err
	}
	// A slow strategy consumes the snapshot's lifetime even if no refresh replaced it.
	now = s.quoteClock()
	if !s.currentQuoteSnapshot(snapshot, now) || s.quoteBlocked(now.Unix()) {
		return declinedQuote(response, quoteDeclineStateChanged), nil
	}
	response.AmountIn, response.AmountOut = decision.AmountIn.String(), decision.AmountOut.String()
	response.quotedPairBounded = quotePairIsBounded(snapshot, input.TokenIn, input.TokenOut)
	return response, nil
}

func declinedQuote(response quoteResponse, reason quoteDeclineReason) quoteResponse {
	response.declineReason = reason
	return response
}

func (s *Solver) quoteBlocked(now int64) bool {
	return s.timeBasedBlockUntil() > now ||
		s.quotes.planningCount() != 0 ||
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

func (s *Solver) currentTime() int64 { return s.quoteClock().Unix() }

func (s *Solver) quoteClock() time.Time {
	now := time.Now()
	observed := time.Unix(s.chainTime.Load(), 0)
	if observed.After(now) {
		return observed
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
