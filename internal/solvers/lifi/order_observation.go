package lifi

import (
	"context"
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// Optional quote ownership never rejects or authorizes an order. Malformed/absent metadata is
// unknown; an explicit different owner cannot become ours through recovery provenance.
func matchedQuoteSource(raw json.RawMessage, executor common.Address) string {
	var quote struct {
		ExclusiveFor string `json:"exclusiveFor"`
	}
	if json.Unmarshal(raw, &quote) != nil || !common.IsHexAddress(quote.ExclusiveFor) || executor == (common.Address{}) {
		return ""
	}
	owner := common.HexToAddress(quote.ExclusiveFor)
	if owner == executor {
		return "quote_exclusive_for"
	}
	if owner != (common.Address{}) {
		return "other_quote"
	}
	return ""
}

// This is an admission observation, not a lifetime-unique win. Existing inbox/recovery dedup
// suppresses overlapping copies; later lifecycles/reconnections/replicas may observe it again.
func (s *Solver) observeMatchedOrderAdmission(ctx context.Context, order *submittedOrder) {
	if order.matchSource != "quote_exclusive_for" && order.matchSource != "recovery_exclusive_for" {
		return
	}
	if s.metrics != nil {
		s.metrics.workflow.ObserveEvent("matched_order", "admitted")
		if s.pairScopeSource(order.TokenIn, order.TokenOut, nil) != "" {
			s.metrics.pairs.Assignment(s.chainID, order.TokenIn, order.TokenOut, "admitted")
		}
	}
	ctx = trace.ContextWithSpanContext(ctx, order.span)
	observability.Log(ctx).WithValues(s.tokenMetadata.PairLogFields(order.TokenIn, order.TokenOut)...).
		Info("matched order admitted", "chainId", s.chainID, "assignmentSource", order.matchSource,
			"orderKey", orderInboxKey(order), "orderId", order.OrderID, "onChainOrderId", order.OnChainOrderID,
			"quoteId", order.QuoteID, "tokenIn", order.TokenIn.Hex(), "tokenOut", order.TokenOut.Hex(),
			"amountIn", bigString(order.AmountIn), "requiredAmountOut", bigString(order.OutputAmount),
			"orderStatus", order.OrderStatus, "inputSettler", order.InputSettler.Hex(), "user", order.Order.User.Hex(),
			"expires", order.Order.Expires, "fillDeadline", order.Order.FillDeadline)
}
