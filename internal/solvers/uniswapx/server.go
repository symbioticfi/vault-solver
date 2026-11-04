package uniswapx

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

const maxQuoteRequestBytes = 32 << 10

const (
	quoteTypeExactInput  = "EXACT_INPUT"
	quoteTypeExactOutput = "EXACT_OUTPUT"
)

func (s *Solver) newQuoteHTTPServer(ctx context.Context) *http.Server {
	// Handler contexts inherit the solver logger from here. Cancellation is deliberately dropped so
	// shutdown keeps draining in-flight quotes through Shutdown instead of cutting them off.
	handlerCtx := context.WithoutCancel(ctx)
	mux := http.NewServeMux()
	healthHandler := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	mux.HandleFunc("POST /quote", s.quoteHandler)
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /healthz", healthHandler)
	mux.HandleFunc("GET /ready", s.readyHandler)
	// The server span is outermost so panic recovery and the handler both run under Uniswap's trace.
	handler := observability.TraceHandler(recoverQuoteServer(mux), uniswapxRoute)
	return &http.Server{
		Addr: s.cfg.QuoteServer.ListenAddress, Handler: handler,
		BaseContext:       func(net.Listener) context.Context { return handlerCtx },
		ReadHeaderTimeout: 2 * time.Second, ReadTimeout: s.cfg.QuoteServer.HTTPTimeout,
		WriteTimeout: s.cfg.QuoteServer.HTTPTimeout, IdleTimeout: 30 * time.Second,
	}
}

func (s *Solver) quoteHandler(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	observation := &quoteHTTPObservation{reason: "internal_error"}
	defer func() {
		s.observeQuoteHTTP(observation)
		if s.metrics != nil {
			s.metrics.observeQuoteLatency(time.Since(started))
		}
	}()
	body, err := io.ReadAll(io.LimitReader(r.Body, maxQuoteRequestBytes+1))
	if err != nil {
		observation.reason = "read-body"
		s.declineQuoteRequest(w, r, "read-body", "invalid request body", "error", err.Error())
		return
	}
	if len(body) > maxQuoteRequestBytes {
		observation.reason = "body-too-large"
		s.declineQuoteRequest(w, r, "body-too-large", "invalid request body", "bytes", len(body))
		return
	}
	var request quoteRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		observation.reason = "invalid-json"
		s.declineQuoteRequest(w, r, "invalid-json", "invalid request body")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		observation.reason = "trailing-json"
		s.declineQuoteRequest(w, r, "trailing-json", "invalid request body")
		return
	}
	pairInScope := s.quotePairInScope(request)
	if request.TokenInChainID == s.chainID && request.TokenOutChainID == s.chainID &&
		common.IsHexAddress(request.TokenIn) && common.IsHexAddress(request.TokenOut) {
		observation.tokenIn, observation.tokenOut = common.HexToAddress(request.TokenIn), common.HexToAddress(request.TokenOut)
		observation.pairKnown = s.metricPairSupported(observation.tokenIn, observation.tokenOut)
	}
	if request.RequestID == "" {
		if request.BlockUntilTimestamp == nil || *request.BlockUntilTimestamp < 0 {
			reason := "invalid-breaker-notification"
			if request.BlockUntilTimestamp == nil {
				reason = string(quoteDeclineMissingIDs)
			}
			observation.reason = reason
			s.declineQuoteRequest(w, r, reason, "invalid blockUntilTimestamp", s.quoteLogFields(request, pairInScope)...)
			return
		}
		observability.Log(r.Context()).V(1).Info(
			"quote breaker notification received",
			"blockUntilTimestamp", *request.BlockUntilTimestamp,
		)
		s.setBlockUntil(*request.BlockUntilTimestamp)
		observation.breaker = true
		s.observeQuote(quoteOutcomeBreakerNotification)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	observability.SetAttributes(r.Context(),
		observability.AttrRequestID.String(request.RequestID),
		observability.AttrQuoteID.String(request.QuoteID),
	)
	response, err := s.quote(r.Context(), request)
	if err == nil && response.AmountOut != "0" {
		// Only the actual decision snapshot can prove a positive quote's direction. A refresh
		// may replace the inventory after the preliminary scope check.
		pairInScope = response.pairInScope
	} else {
		pairInScope = response.pairInScope || pairInScope
	}
	// The decision may discover a newly trusted inventory direction after the initial check.
	observation.pairKnown = observation.pairKnown || pairInScope
	requestLog := observability.Log(r.Context()).WithValues(s.quoteLogFields(request, pairInScope)...)
	if err != nil {
		observation.reason = "strategy_error"
		s.observeQuote(quoteOutcomeError)
		loggedErr := err
		if !pairInScope {
			loggedErr = errors.New("quote strategy failed before directed pair validation")
		}
		requestLog.Error(loggedErr, "quote failed", "reason", "strategy_error")
		http.Error(w, "quote unavailable", http.StatusServiceUnavailable)
		return
	}
	if response.AmountOut == "0" {
		observation.reason = string(response.declineReason)
		s.observeQuoteDecline(response.declineReason)
		requestLog.V(1).Info("quote declined", "reason", response.declineReason)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// Publish the selection before writing: an awarded order can arrive as soon as
	// the peer receives the response. A failed write may leave a harmless bounded hint.
	if s.cfg.singleSource() {
		s.rememberSelection(response, time.Now())
	}
	s.observeQuote(quoteOutcomeQuoted)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(quoteResponseWriter{writer: w}).Encode(response); err != nil {
		observation.reason = "response_write_error"
		requestLog.Error(err, "write quote response", "reason", "response_write_error")
		return
	}
	if pairInScope {
		requestLog.V(1).Info("quote returned", "responseWritten", true,
			"amountIn", response.AmountIn, "amountOut", response.AmountOut)
	}
	observation.returned = true
	// Remember the served quote so the fill that wins it can link back to this trace (spec §12).
	// Indicative and hard quotes share a request shape, so both are remembered; the later wins.
	s.links.Remember(r.Context(), request.QuoteID, quoteLinkTTL)
}

// encoding/json propagates Write errors but not a nil-error short write. Preserve that distinction
// before recording a successful response, even if the underlying writer violates io.Writer's contract.
type quoteResponseWriter struct{ writer io.Writer }

func (w quoteResponseWriter) Write(body []byte) (int, error) {
	n, err := w.writer.Write(body)
	if err == nil && n != len(body) {
		err = io.ErrShortWrite
	}
	return n, err
}

// declineQuoteRequest answers a malformed body. The caller is at fault, so the server span records a
// declined event rather than an error, matching the V(1) logging rule.
func (s *Solver) declineQuoteRequest(
	w http.ResponseWriter, r *http.Request, reason, message string, keysAndValues ...any,
) {
	observability.Decline(r.Context(), "bad_request", reason)
	fields := append([]any{"reason", reason}, keysAndValues...)
	observability.Log(r.Context()).V(1).Info("quote request rejected", fields...)
	s.observeQuote(quoteOutcomeInvalid)
	http.Error(w, message, http.StatusBadRequest)
}

// quote answers one quote request as the uniswapx.quote stage. A decline is an expected outcome, so
// it is recorded as a declined event; only a real failure ends the span with an error.
func (s *Solver) quote(ctx context.Context, request quoteRequest) (response quoteResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.QuoteServer.HTTPTimeout)
	defer cancel()
	ctx, end := tracer.Start(ctx, "uniswapx.quote")
	// Deferred so the span still ends when the pipeline panics; the quote server recovers that into
	// a 500 without unwinding past here, and an unended span is never exported.
	defer func() {
		if err == nil && response.declineReason != "" {
			observability.Decline(ctx, quoteDeclineDecision(response.declineReason), string(response.declineReason))
		}
		end(err)
	}()
	return s.evaluateQuote(ctx, request)
}

// quoteDeclineDecision classifies a decline: a request we could not parse or price is the caller's
// fault, everything else is this filler choosing not to quote.
func quoteDeclineDecision(reason quoteDeclineReason) string {
	switch reason {
	case quoteDeclineInvalidAmount, quoteDeclineMissingIDs,
		quoteDeclineUnsupportedType, quoteDeclineInvalidOutputs, quoteDeclineUnsupportedProtocol,
		quoteDeclineUnsupportedChain, quoteDeclineInvalidAddress:
		return "bad_request"
	case quoteDeclineBlocked, quoteDeclinePairOutOfScope, quoteDeclineQuoteStateUnavailable,
		quoteDeclineStrategy, quoteDeclineStateChanged, quoteDeclineNoMatchingRoutes,
		quoteDeclineNoCandidates, quoteDeclineAmountBelowMinimum, quoteDeclineInsufficientCapacity,
		quoteDeclineNoAllocation, quoteDeclineGasExceedsOutput, quoteDeclineMinimumInput, quoteDeclineDexUnavailable:
		return "no_quote"
	}
	return "no_quote"
}

// evaluateQuote is the quote pipeline; quote wraps it in the uniswapx.quote span.
func (s *Solver) evaluateQuote(ctx context.Context, request quoteRequest) (quoteResponse, error) {
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
	if reason := s.validateQuoteRequest(request); reason != "" {
		return declinedQuote(response, reason), nil
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
	reservationRevision := s.capacity.Revision()
	state := s.quoteState.Load()
	if state == nil || state.epoch != epoch || !state.expiresAt.After(time.Unix(now, 0)) {
		return declinedQuote(response, quoteDeclineQuoteStateUnavailable), nil
	}
	// Wall expiry and a leading observed chain clock can each shorten the usable snapshot lifetime.
	// Neither the handler socket timeout nor a cooperative strategy replaces the publication check.
	lifetime := min(time.Until(state.expiresAt), state.expiresAt.Sub(time.Unix(s.currentTime(), 0)))
	ctx, cancel := context.WithTimeout(ctx, lifetime)
	defer cancel()
	response.pairInScope = s.inventoryPairInScope(tokenIn, tokenOut, state)
	declines := &quoteStrategyDeclines{}
	decisionTrace := s.decisionTrace(ctx, "requestId", request.RequestID,
		"quoteId", request.QuoteID, "quoteType", request.Type)
	if !response.pairInScope {
		// Preserve bounded decline classification, but do not forward arbitrary unsupported
		// pair details from the strategy's diagnostic trace.
		decisionTrace = nil
	}
	input := types.QuoteInput{
		RequestID: request.RequestID, QuoteID: request.QuoteID,
		TokenIn: tokenIn, TokenOut: tokenOut,
		RequireSingleRoute: state.singleRouteFor[tokenIn],
		Inventory:          state.inventory,
		Reservations:       s.capacity.Snapshot(),
		GasSnapshot:        state.gasSnapshot, GasPrices: state.gasPrices,
		MaxFeePerGas: state.maxFeePerGas, ChainTime: state.chainTime, QuoteExpiresAt: state.expiresAt,
		Trace: declines.trace(decisionTrace),
	}
	if request.Type == quoteTypeExactInput {
		input.AmountIn = requestAmount
	} else {
		input.AmountOut = requestAmount
	}
	var quote *types.Quote
	var err error
	if s.cfg.Dex.matches(tokenIn, tokenOut) {
		quote, err = s.decideDexQuote(ctx, input)
	} else {
		quote, err = s.decideQuote(ctx, input)
	}
	if ctx.Err() != nil {
		return declinedQuote(response, quoteDeclineStateChanged), nil
	}
	if errors.Is(err, errDexUnavailable) {
		return declinedQuote(response, quoteDeclineDexUnavailable), nil
	}
	if err != nil {
		return response, err
	}
	if quote == nil {
		return declinedQuote(response, declines.reason()), nil
	}
	if err := validateStrategyQuote(input, quote); err != nil {
		return response, err
	}
	now = s.currentTime()
	if ctx.Err() != nil || s.quoteEpoch.Load() != epoch || s.quoteState.Load() != state ||
		s.capacity.Revision() != reservationRevision || !state.expiresAt.After(time.Now()) ||
		!state.expiresAt.After(time.Unix(now, 0)) || s.quoteBlocked(now) {
		return declinedQuote(response, quoteDeclineStateChanged), nil
	}
	response.selectedCandidate = quote.CandidateID
	response.AmountIn = quote.AmountIn.String()
	response.AmountOut = quote.AmountOut.String()
	return response, nil
}

// decideQuote runs the strategy as the uniswapx.quote.decide stage, so a webhook strategy's HTTP
// call nests under a named decision span.
func (s *Solver) decideQuote(
	ctx context.Context, input types.QuoteInput,
) (quote *types.Quote, err error) {
	ctx, end := tracer.Start(ctx, "uniswapx.quote.decide", observability.AttrStrategy.String(s.cfg.Strategy.Name))
	defer func() {
		if ctx.Err() != nil {
			observability.Decline(ctx, "no_quote", string(quoteDeclineStateChanged))
			end(nil)
			return
		}
		end(err)
	}()
	return s.strategy.DecideQuote(ctx, input)
}

func declinedQuote(response quoteResponse, reason quoteDeclineReason) quoteResponse {
	response.declineReason = reason
	return response
}

func (s *Solver) quoteBlocked(now int64) bool {
	return s.timeBasedBlockUntil() > now ||
		(s.txm != nil && !s.txm.Available()) ||
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

// validateQuoteRequest splits the existing semantic predicate without changing acceptance.
// Precedence follows its original left-to-right order. Runtime blocking still precedes this
// check; pair policy and amount validation still follow it. Zero swapper is valid upstream.
func (s *Solver) validateQuoteRequest(request quoteRequest) quoteDeclineReason {
	switch {
	case request.RequestID == "" || request.QuoteID == "":
		return quoteDeclineMissingIDs
	case !supportedQuoteType(request.Type):
		return quoteDeclineUnsupportedType
	case request.NumOutputs < 1:
		return quoteDeclineInvalidOutputs
	case !supportedQuoteProtocol(request.Protocol):
		return quoteDeclineUnsupportedProtocol
	case request.TokenInChainID != s.chainID || request.TokenOutChainID != s.chainID:
		return quoteDeclineUnsupportedChain
	case !common.IsHexAddress(request.Swapper) || !common.IsHexAddress(request.TokenIn) ||
		!common.IsHexAddress(request.TokenOut):
		return quoteDeclineInvalidAddress
	default:
		return ""
	}
}

func supportedQuoteType(value string) bool {
	return value == quoteTypeExactInput || value == quoteTypeExactOutput
}

func supportedQuoteProtocol(value string) bool {
	return value == "v1" || value == "v2"
}

func validateStrategyQuote(input types.QuoteInput, quote *types.Quote) error {
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

// Diagnostic scope is an exact validated route direction admitted by input policy.
// It is deliberately not a claim of available pair capacity or a priceable request amount.
// quoteLogFields records details only for a direction proven by validated routes/inventory. Metadata
// enriches display, never decides support. Arbitrary unsupported request fields are not copied.
func (s *Solver) quoteLogFields(request quoteRequest, inScope bool) []any {
	fields := []any{"pairInScope", inScope}
	if !inScope {
		return fields
	}
	fields = append(fields, "pairScope", "directed_route", "pairScopeSource", "verified_quote_routes",
		"requestId", request.RequestID, "quoteId", request.QuoteID,
		"type", request.Type, "protocol", request.Protocol,
		"tokenInChainId", request.TokenInChainID, "tokenOutChainId", request.TokenOutChainID,
		"tokenIn", request.TokenIn, "tokenOut", request.TokenOut, "amount", request.Amount)
	return append(fields, s.tokenMetadata.PairLogFields(common.HexToAddress(request.TokenIn), common.HexToAddress(request.TokenOut))...)
}

func (s *Solver) quotePairInScope(request quoteRequest) bool {
	if request.TokenInChainID != s.chainID || request.TokenOutChainID != s.chainID ||
		!common.IsHexAddress(request.TokenIn) || !common.IsHexAddress(request.TokenOut) {
		return false
	}
	return s.metricPairSupported(common.HexToAddress(request.TokenIn), common.HexToAddress(request.TokenOut))
}

func (s *Solver) inventoryPairInScope(in, out common.Address, state *quoteState) bool {
	if s.cfg.Dex.matches(in, out) {
		out = s.cfg.Dex.BridgeToken
	}
	if in == out || in == (common.Address{}) || out == (common.Address{}) || !s.cfg.TokenPolicy.Allows(in) || state == nil {
		return false
	}
	for _, item := range state.inventory {
		if item.TokenIn == in && item.TokenOut == out {
			return true
		}
	}
	return false
}

// metricPairSupported reuses configured adapter directions and this refresh's validated scope.
// Dynamic proof expires/replaces with quoteState; temporary capacity and metadata do not decide it.
func (s *Solver) metricPairSupported(in, out common.Address) bool {
	if s.cfg.Dex.matches(in, out) {
		return s.metricPairSupported(in, s.cfg.Dex.BridgeToken)
	}
	if in == out || in == (common.Address{}) || out == (common.Address{}) || !s.cfg.TokenPolicy.Allows(in) {
		return false
	}
	for _, route := range s.metricRoutes {
		if route.TokenIn == in && route.TokenOut == out {
			return true
		}
	}
	state := s.quoteState.Load()
	if state == nil || state.epoch != s.quoteEpoch.Load() || !state.expiresAt.After(time.Unix(s.currentTime(), 0)) {
		return false
	}
	for _, route := range state.routes {
		if route.TokenIn == in && route.TokenOut == out {
			return true
		}
	}
	return s.inventoryPairInScope(in, out, state)
}
