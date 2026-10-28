package rfq

import (
	"math/big"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type quoteHTTPObservation struct {
	request liquidlane.QuoteMetricRequest
	reason  string
}

func quoteHTTPReason(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_json"
	case http.StatusForbidden:
		return "unauthorized"
	case http.StatusRequestEntityTooLarge:
		return "body_too_large"
	case http.StatusUnprocessableEntity:
		return "invalid_request"
	case http.StatusUnsupportedMediaType, http.StatusNotAcceptable:
		return "unsupported_media_type"
	case http.StatusInternalServerError:
		return "internal_error"
	default:
		return "unknown"
	}
}

// Observe fixed schema locations before Huma's schema-link transformer. Never inspect free-form
// validation messages, log credentials, or modify the response or validation order.
func quoteValidationReason(ctx huma.Context, _ string, body any) (any, error) {
	observation, _ := ctx.Context().Value(quoteMetricsKey).(*quoteHTTPObservation)
	problem, ok := body.(*huma.ErrorModel)
	if observation == nil || !ok || observation.reason != "" {
		return body, nil
	}
	fields := []struct{ location, reason string }{
		{"type", "unsupported_type"}, {"protocol", "unsupported_protocol"}, {"numOutputs", "invalid_outputs"},
		{"requestId", "invalid_ids"}, {"quoteId", "invalid_ids"}, {"swapper", "invalid_address"}, {"tokenIn", "invalid_address"}, {"tokenOut", "invalid_address"},
		{"tokenInChainId", "unsupported_chain"}, {"tokenOutChainId", "unsupported_chain"}, {"amount", "invalid_amount"}, {"adapters", "invalid_inventory"},
	}
	for _, field := range fields {
		for _, detail := range problem.Errors {
			if detail == nil {
				continue
			}
			// Huma reports omitted required keys at body with the decoded object as Value.
			// Inspect only key presence, using the same fixed field precedence as other errors.
			missing := false
			if detail.Location == "body" {
				if object, isObject := detail.Value.(map[string]any); isObject {
					_, present := object[field.location]
					missing = !present
				}
			}
			if missing || detail.Location == "body."+field.location || strings.HasPrefix(detail.Location, "body."+field.location+"[") {
				observation.reason = field.reason
				if field.reason == "invalid_ids" {
					if missing || detail.Value == nil {
						observation.reason = "missing_ids"
					}
					if value, isString := detail.Value.(string); isString && value == "" {
						observation.reason = "missing_ids"
					}
				}
				return body, nil
			}
		}
	}
	return body, nil
}

func quoteBadRequestReason(q *quoteRequest) string {
	switch {
	case q.Type != quoteTypeExactInput:
		return "unsupported_type"
	case q.Protocol != quoteProtocolV1:
		return "unsupported_protocol"
	case q.NumOutputs <= 0:
		return "invalid_outputs"
	case q.RequestID == "" || q.QuoteID == "":
		return "missing_ids"
	case !common.IsHexAddress(q.Swapper) || !common.IsHexAddress(q.TokenIn) || !common.IsHexAddress(q.TokenOut):
		return "invalid_address"
	}
	amount, ok := new(big.Int).SetString(q.Amount, 10)
	if !ok || amount.Sign() < 0 {
		return "invalid_amount"
	}
	return "invalid_inventory"
}

func boundedQuoteReason(reason string) string {
	switch reason {
	case "lane_unavailable", "not_quotable", "below_minimum", "no_candidates", "strategy_declined", "bad_request", "error",
		"unsupported_type", "unsupported_protocol", "invalid_outputs", "missing_ids", "invalid_ids", "invalid_address", "unsupported_chain", "invalid_amount", "invalid_inventory", "pair_out_of_scope",
		"invalid_json", "unauthorized", "body_too_large", "invalid_request", "unsupported_media_type", "internal_error", "response_write_error":
		return reason
	default:
		return "unknown"
	}
}
