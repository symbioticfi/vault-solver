package rfq

import (
	"context"
	"crypto/subtle"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// sharedSecretHeader authenticates the backend peer on /quote.
const sharedSecretHeader = "x-rfq-shared-secret" //nolint:gosec // header NAME, not a credential.

// badRequestError marks a client (4xx) error so the HTTP layer can distinguish it from an upstream
// (5xx) failure without string-matching.
type badRequestError struct{ err error }

func (e *badRequestError) Error() string { return e.err.Error() }
func (e *badRequestError) Unwrap() error { return e.err }

// server is the filler's HTTP surface. It uses Huma so the OpenAPI 3.1 spec is generated from the
// request/response types below and served at /openapi.json (+ /docs), with inbound validation done
// against that schema. /quote is gated by the x-rfq-shared-secret header (the backend peer); the
// filler is poll-only, so there is no push/notify endpoint.
type server struct {
	sharedSecret string
	quotes       *quoteService
	metrics      *rfqMetrics              // nil disables instrumentation (e.g. in tests)
	links        *observability.SpanLinks // shared with executionService; nil disables quote→fill links
}

/* ───────── Huma I/O types (drive both validation and the generated spec) ───────── */

type healthBody struct {
	Status    string `json:"status" example:"ok"`
	Timestamp string `json:"timestamp" format:"date-time"`
}

type healthOutput struct {
	Body healthBody
}

type quoteInput struct {
	Secret string       `header:"x-rfq-shared-secret" doc:"Backend shared secret"`
	Body   quoteRequest `contentType:"application/json"`
}

// quoteOutput uses a dynamic Status so the same operation can answer 200 (a quote) or 204 (no quote).
type quoteOutput struct {
	Status int
	Body   *quoteResponse
}

// handler builds the Huma API on a stdlib ServeMux and returns it. Huma serves /openapi.json,
// /openapi.yaml, and /docs automatically.
func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	config := huma.DefaultConfig("vault-solver RFQ filler", version1)
	config.Transformers = append(config.Transformers, quoteValidationReason)
	api := humago.New(mux, config)

	huma.Register(api, huma.Operation{
		OperationID: "health", Method: http.MethodGet, Path: "/health", Summary: "Health check",
	}, s.handleHealth)

	huma.Register(api, huma.Operation{
		OperationID: "quote", Method: http.MethodPost, Path: "/quote", Summary: "Request a filler quote",
		Description: "Returns a solver quote, or 204 when the filler cannot quote the request.",
	}, s.handleQuote)

	// Middleware chain (outer → inner): server span, body cap, access log + request-id, metrics,
	// panic recovery. The span is outermost so every inner layer runs under the backend's trace.
	var h = recoverPanics(mux)
	// The post-write response event is available even when Prometheus is disabled.
	h = s.metrics.instrument(h)
	h = logRequests(h)
	return observability.TraceHandler(
		http.MaxBytesHandler(h, maxRequestBytes),
		func(r *http.Request) string { return routeLabel(r.URL.Path) },
	)
}

const version1 = "1.0.0"

func (s *server) handleHealth(_ context.Context, _ *struct{}) (*healthOutput, error) {
	out := &healthOutput{}
	out.Body.Status = "ok"
	out.Body.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return out, nil
}

func (s *server) handleQuote(ctx context.Context, in *quoteInput) (*quoteOutput, error) {
	observability.SetAttributes(ctx,
		observability.AttrRequestID.String(requestID(ctx)),
		observability.AttrQuoteID.String(in.Body.QuoteID),
	)
	observation, _ := ctx.Value(quoteMetricsKey).(*quoteHTTPObservation)
	if !s.authorized(in.Secret) {
		if observation != nil {
			observation.reason = "unauthorized"
		}
		// Log the denial (never the attempted secret) so credential scanning is observable.
		observability.Log(ctx).V(1).Info("rejected /quote: bad shared secret", "requestId", requestID(ctx))
		return nil, huma.Error403Forbidden("forbidden")
	}
	// Preserve existing structural support before optional strategy work can panic. A later
	// candidate proof may extend this, but raw request tokens alone never establish support.
	if observation != nil && common.IsHexAddress(in.Body.TokenIn) && common.IsHexAddress(in.Body.TokenOut) &&
		s.quotes.configuredPairInScope(&in.Body) {
		observation.pairInScope = true
		observation.chainID = s.quotes.chainID
		observation.tokenIn, observation.tokenOut = common.HexToAddress(in.Body.TokenIn), common.HexToAddress(in.Body.TokenOut)
	}
	decision, err := s.quotes.quote(ctx, &in.Body)
	// Only a positive, on-chain-validated decision may extend optional log metadata.
	if err == nil && decision.response != nil && s.quotes.warmMetadata != nil {
		s.quotes.warmMetadata([]common.Address{
			common.HexToAddress(in.Body.TokenIn), common.HexToAddress(in.Body.TokenOut),
		})
	}
	var bad *badRequestError
	if errors.As(err, &bad) {
		decision.outcome = quoteDecisionBadRequest
	}
	if observation != nil {
		if decision.metricPairInScope {
			observation.pairInScope = true
			observation.chainID = s.quotes.chainID
			observation.tokenIn, observation.tokenOut = common.HexToAddress(in.Body.TokenIn), common.HexToAddress(in.Body.TokenOut)
		}
		observation.reason = string(decision.outcome)
		if bad != nil {
			observation.reason = quoteBadRequestReason(&in.Body)
		}
		if decision.outcome == quoteDecisionNotQuotable {
			if in.Body.TokenInChainID != s.quotes.chainID || in.Body.TokenOutChainID != s.quotes.chainID {
				observation.reason = "unsupported_chain"
			} else if len(in.Body.Adapters) == 0 {
				observation.reason = "no_candidates"
			} else {
				observation.reason = "pair_out_of_scope"
			}
		}
		quoteLog := observability.Log(ctx)
		pairScope, pairScopeSource := "unknown", "unknown"
		if decision.pairInScope {
			pairScope, pairScopeSource = "directed_route", "validated_candidates"
			quoteLog = quoteLog.WithValues(s.quotes.tokenMetadata.PairLogFields(common.HexToAddress(in.Body.TokenIn), common.HexToAddress(in.Body.TokenOut))...)
		}
		fields := []any{"reason", observation.reason, "requestId", requestID(ctx), "quoteId", in.Body.QuoteID,
			"pairInScope", decision.pairInScope, "pairScope", pairScope, "pairScopeSource", pairScopeSource}
		if decision.pairInScope {
			fields = append(fields, "protocol", in.Body.Protocol, "tokenIn", in.Body.TokenIn, "tokenOut", in.Body.TokenOut,
				"tokenInChainId", in.Body.TokenInChainID, "tokenOutChainId", in.Body.TokenOutChainID,
				"type", in.Body.Type, "amount", in.Body.Amount)
		}
		if decision.response != nil {
			fields = append(fields, "amountIn", decision.response.AmountIn, "amountOut", decision.response.AmountOut)
			observation.response = decision.response
			observation.fields = append(s.quotes.tokenMetadata.PairLogFields(common.HexToAddress(in.Body.TokenIn), common.HexToAddress(in.Body.TokenOut)), fields...)
		}
		if decision.response != nil {
			// The transport wrapper emits the only detailed terminal event after writing.
			observability.Log(ctx).V(1).Info("quote result", "reason", observation.reason,
				"requestId", requestID(ctx), "quoteId", in.Body.QuoteID,
				"pairInScope", decision.pairInScope, "pairScope", pairScope, "pairScopeSource", pairScopeSource)
		} else {
			quoteLog.V(1).Info("quote result", fields...)
		}
	}
	s.metrics.observeQuoteDecision(decision.outcome)
	if err != nil {
		if bad != nil {
			return nil, huma.Error400BadRequest(bad.Error())
		}
		if decision.pairInScope {
			backendErrorLogger(observability.Log(ctx), err).
				Error(err, "quote failed", "quoteId", in.Body.QuoteID, "requestId", requestID(ctx))
		} else {
			observability.Log(ctx).Error(errors.New("quote dependency failed before directed pair validation"),
				"quote failed", "quoteId", in.Body.QuoteID, "requestId", requestID(ctx), "pairInScope", false)
		}
		return nil, huma.Error502BadGateway("quote failed")
	}
	if decision.response == nil {
		return &quoteOutput{Status: http.StatusNoContent}, nil // well-formed, nothing to quote
	}
	// Remember the served quote so the fill that wins it can link back to this trace (spec §12).
	s.links.Remember(ctx, in.Body.QuoteID, quoteLinkTTL)
	return &quoteOutput{Status: http.StatusOK, Body: decision.response}, nil
}

// authorized constant-time-compares the shared secret header.
func (s *server) authorized(got string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.sharedSecret)) == 1
}
