package rfq

import (
	"context"
	"crypto/subtle"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
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
	log          logr.Logger
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
	api := humago.New(mux, config)

	huma.Register(api, huma.Operation{
		OperationID: "health", Method: http.MethodGet, Path: "/health", Summary: "Health check",
	}, s.handleHealth)

	huma.Register(api, huma.Operation{
		OperationID: "quote", Method: http.MethodPost, Path: "/quote", Summary: "Request a filler quote",
		Description: "Returns a solver quote, or 204 when the filler cannot quote the request.",
	}, s.handleQuote)

	return mux
}

const version1 = "1.0.0"

func (s *server) handleHealth(_ context.Context, _ *struct{}) (*healthOutput, error) {
	out := &healthOutput{}
	out.Body.Status = "ok"
	out.Body.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return out, nil
}

func (s *server) handleQuote(ctx context.Context, in *quoteInput) (*quoteOutput, error) {
	if !s.authorized(in.Secret) {
		// Log the denial (never the attempted secret) so credential scanning is observable.
		s.log.V(1).Info("rejected /quote: bad shared secret")
		return nil, huma.Error403Forbidden("forbidden")
	}
	resp, err := s.quotes.quote(ctx, &in.Body)
	if err != nil {
		var bad *badRequestError
		if errors.As(err, &bad) {
			return nil, huma.Error400BadRequest(bad.Error())
		}
		s.log.Error(err, "quote failed", "quoteId", in.Body.QuoteID)
		return nil, huma.Error502BadGateway("quote failed")
	}
	if resp == nil {
		return &quoteOutput{Status: http.StatusNoContent}, nil // well-formed, nothing to quote
	}
	return &quoteOutput{Status: http.StatusOK, Body: resp}, nil
}

// authorized constant-time-compares the shared secret header.
func (s *server) authorized(got string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.sharedSecret)) == 1
}
