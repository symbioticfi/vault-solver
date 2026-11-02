package rfq

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
)

type returnedEventWriter struct {
	*httptest.ResponseRecorder

	mode        string
	beforeWrite func()
}

func (w returnedEventWriter) Write(body []byte) (int, error) {
	if w.beforeWrite != nil {
		w.beforeWrite()
	}
	switch w.mode {
	case "failure":
		return 0, errors.New("write unavailable")
	case "short":
		return w.ResponseRecorder.Write(body[:len(body)-1])
	case "panic":
		panic("response write panic")
	default:
		return w.ResponseRecorder.Write(body)
	}
}

func TestQuoteResponseReturnedRequiresSuccessfulWrite(t *testing.T) {
	for _, mode := range []string{"success", "metrics disabled", "failure", "short", "panic", "declined", "unauthorized"} {
		t.Run(mode, func(t *testing.T) {
			var events []map[string]any
			log := funcr.NewJSON(func(line string) {
				var event map[string]any
				if err := json.Unmarshal([]byte(line), &event); err != nil {
					t.Error(err)
				}
				events = append(events, event)
			}, funcr.Options{Verbosity: 1})
			srv := testServer()
			if mode != "metrics disabled" {
				var err error
				srv.metrics, err = newRFQMetrics(prometheus.NewRegistry(), newStore(time.Now), "")
				if err != nil {
					t.Fatal(err)
				}
			}
			body := validQuoteBody()
			if mode == "declined" {
				body.Adapters = nil
			}
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequestWithContext(observability.WithLogger(t.Context(), log), http.MethodPost, "/quote", bytes.NewReader(raw))
			r.Header.Set(sharedSecretHeader, testSecret)
			r.Header.Set(requestIDHeader, "returned-event-request")
			if mode == "unauthorized" {
				r.Header.Set(sharedSecretHeader, "incorrect")
			}
			writer := returnedEventWriter{ResponseRecorder: httptest.NewRecorder(), mode: mode, beforeWrite: func() {
				for _, event := range events {
					if event["msg"] == "quote response returned" {
						t.Error("returned event emitted before response write")
					}
				}
			}}
			srv.handler().ServeHTTP(writer, r)
			var returned []map[string]any
			for _, event := range events {
				if event["msg"] == "quote result" && event["requestId"] != "returned-event-request" {
					t.Errorf("decision requestId=%v", event["requestId"])
				}
				if event["msg"] == "quote result" && event["reason"] == "quoted" {
					for _, field := range []string{"tokenIn", "tokenOut", "amountIn", "amountOut", "amount", "tokenInDecimals", "tokenOutDecimals"} {
						if _, present := event[field]; present {
							t.Errorf("pre-write success duplicated detailed %s: %v", field, event)
						}
					}
				}
				if event["msg"] == "quote response returned" {
					returned = append(returned, event)
				}
			}
			want := 0
			if mode == "success" || mode == "metrics disabled" {
				want = 1
			}
			if len(returned) != want {
				t.Fatalf("returned events=%v want=%d", returned, want)
			}
			if mode == "failure" || mode == "short" || mode == "panic" {
				terminal := 0
				for _, event := range events {
					written, writeKnown := event["responseWritten"].(bool)
					if event["msg"] == "quote result" && writeKnown && !written {
						terminal++
						wantReason := "response_write_error"
						if mode == "panic" {
							wantReason = "internal_error"
						}
						scoped, scopeKnown := event["pairInScope"].(bool)
						if !scopeKnown || !scoped || event["reason"] != wantReason || event["tokenIn"] != body.TokenIn || event["tokenOut"] != body.TokenOut {
							t.Errorf("terminal refusal=%v", event)
						}
					}
				}
				if terminal != 1 {
					t.Errorf("terminal known-pair refusals=%d want1", terminal)
				}
			}
			if want == 1 {
				e := returned[0]
				for key, expected := range map[string]any{
					"requestId": "returned-event-request", "quoteId": body.QuoteID, "protocol": "v1", "type": "EXACT_INPUT",
					"tokenIn": body.TokenIn, "tokenOut": body.TokenOut, "amountIn": body.Amount, "amountOut": "1000000",
					"responseWritten": true, "pairInScope": true, "pairScope": "directed_route", "pairScopeSource": "validated_candidates",
				} {
					if e[key] != expected {
						t.Errorf("%s=%v want %v", key, e[key], expected)
					}
				}
				if e["httpStatus"] != float64(200) || e["responseBytes"].(float64) <= 0 {
					t.Fatalf("response outcome fields=%v", e)
				}
			}
		})
	}
}

func TestQuoteDirectionScopeRequiresValidatedCandidates(t *testing.T) {
	for _, mode := range []string{"quoted", "strategy declined", "no inventory", "candidate error"} {
		t.Run(mode, func(t *testing.T) {
			srv := testServer()
			body := validQuoteBody()
			switch mode {
			case "strategy declined":
				srv.quotes.strategy = &inputRecordingStrategy{quoteOut: types.QuoteOutput{Decision: types.DecisionDecline}}
			case "no inventory":
				body.Adapters = nil
			case "candidate error":
				srv.quotes.reader = failingQuoteReader{}
			}
			decision, err := srv.quotes.quote(t.Context(), &body)
			if (err != nil) != (mode == "candidate error") {
				t.Fatalf("quote error=%v", err)
			}
			want := mode == "quoted" || mode == "strategy declined"
			if decision.pairInScope != want {
				t.Fatalf("direction scope=%v want=%v", decision.pairInScope, want)
			}
		})
	}
}

func TestQuoteResponseReturnedExcludesEmptyBodyAndPanicAfterWrite(t *testing.T) {
	for _, mode := range []string{"empty", "panic after write"} {
		t.Run(mode, func(t *testing.T) {
			returned := 0
			log := funcr.NewJSON(func(line string) {
				var event map[string]any
				if err := json.Unmarshal([]byte(line), &event); err != nil {
					t.Error(err)
				}
				if event["msg"] == "quote response returned" {
					returned++
				}
			}, funcr.Options{Verbosity: 1})
			var metrics *rfqMetrics
			h := metrics.instrument(recoverPanics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				observation := r.Context().Value(quoteMetricsKey).(*quoteHTTPObservation)
				observation.response = &quoteResponse{AmountIn: "1", AmountOut: "1"}
				w.WriteHeader(http.StatusOK)
				if mode == "panic after write" {
					_, _ = w.Write([]byte(`{"amountOut":"1"}`))
					panic("after complete response write")
				}
			})))
			r := httptest.NewRequestWithContext(observability.WithLogger(t.Context(), log), http.MethodPost, "/quote", nil)
			h.ServeHTTP(httptest.NewRecorder(), r)
			if returned != 0 {
				t.Fatalf("false returned event after %s", mode)
			}
		})
	}
}
