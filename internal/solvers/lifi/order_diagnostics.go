package lifi

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/lifiorder"
)

// Only allowlisted scalar fields are emitted. In particular, nested objects, full orders,
// signatures, callback data and auction context must never be copied into a diagnostic.
const orderDiagnosticLimit = 160

type orderParseError struct {
	reason, field string
	message       string
	cause         error
}

func (e *orderParseError) Error() string {
	if strings.HasPrefix(e.message, e.field) {
		return e.message
	}
	return e.field + ": " + e.message
}
func (e *orderParseError) Unwrap() error { return e.cause }

func orderFieldError(reason, field, message string, cause error) error {
	return &orderParseError{reason: reason, field: field, message: message, cause: cause}
}

func decodeOrderError(order map[string]json.RawMessage, err error) error {
	field := "payload"
	var mismatch *json.UnmarshalTypeError
	if errors.As(err, &mismatch) {
		field = mismatch.Field
		// Generated nested UnmarshalJSON methods can reset the path at their own struct.
		switch mismatch.Struct {
		case "_SubmitOrderDtoOrder":
			field = "order." + strings.TrimPrefix(field, "order.")
		case "_SubmitOrderDtoOrderOutputsInner":
			// The generated output decoder loses the array index. Re-read each raw output only
			// for diagnostics so a failure in output N is never attributed to output zero.
			var outputs []json.RawMessage
			if json.Unmarshal(order["outputs"], &outputs) == nil {
				for i, raw := range outputs {
					var output lifiorder.SubmitOrderDtoOrderOutputsInner
					if json.Unmarshal(raw, &output) != nil {
						field = fmt.Sprintf("order.outputs[%d].%s", i, strings.TrimPrefix(field, "order.outputs."))
						break
					}
				}
			}
		}
	}
	return orderFieldError("lifi_decode_error", field, "decode submit order dto: invalid JSON shape", err)
}

func orderDiagnosticValue(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return "<unavailable>"
	}
	var text string
	switch v := value.(type) {
	case string:
		text = v
	case float64, bool, nil:
		text = string(raw)
	case []any:
		return fmt.Sprintf("<array length=%d>", len(v))
	default:
		return "<object>"
	}
	// Bound UTF-8 bytes before the logger applies JSON escaping.
	text = strings.ToValidUTF8(text, "�")
	if len(text) > orderDiagnosticLimit {
		text = strings.ToValidUTF8(text[:orderDiagnosticLimit-3], "") + "..."
	}
	return text
}

// Decode each routing/diagnostic object once. The parser validates the shape;
// missing raw fields stay unavailable in failure context.
func orderRawObject(raw json.RawMessage) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	return fields
}

func orderRawField(fields map[string]json.RawMessage, path string) json.RawMessage {
	parts := strings.Split(strings.NewReplacer("[", ".", "]", "").Replace(path), ".")
	raw := fields[parts[0]]
	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		if i, err := strconv.Atoi(part); err == nil && i >= 0 {
			var values []json.RawMessage
			if json.Unmarshal(raw, &values) != nil || i >= len(values) {
				return nil
			}
			raw = values[i]
		} else {
			raw = orderRawObject(raw)[part]
		}
	}
	return raw
}

func orderDiagnosticFields(data []byte, err error) []any {
	reason, field := "lifi_invalid_order", "payload"
	var parseErr *orderParseError
	if errors.As(err, &parseErr) {
		reason, field = parseErr.reason, parseErr.field
	} else if errors.Is(err, errOrderForDifferentChain) {
		reason, field = "unsupported_chain", "order.originChainId"
	}
	envelope := orderRawObject(data)
	meta := orderRawObject(envelope["meta"])
	order := orderRawObject(envelope["order"])

	value := "<redacted>"
	normalizedField := field
	if strings.HasPrefix(field, "order.outputs[") {
		if _, tail, ok := strings.Cut(field, "]"); ok {
			normalizedField = "order.outputs[0]" + tail
		}
	}
	switch normalizedField {
	case "orderType", "inputSettler", "meta.orderStatus", "order.user", "order.inputOracle",
		"order.originChainId", "order.nonce", "order.expires", "order.fillDeadline", "order.inputs", "order.outputs",
		"order.inputs[0]", "order.inputs[0][0]", "order.inputs[0][1]", "order.outputs[0].oracle",
		"order.outputs[0].settler", "order.outputs[0].token", "order.outputs[0].recipient",
		"order.outputs[0].amount", "order.outputs[0].chainId":
		switch {
		case strings.HasPrefix(field, "order."):
			value = orderDiagnosticValue(orderRawField(order, strings.TrimPrefix(field, "order.")))
		case strings.HasPrefix(field, "meta."):
			value = orderDiagnosticValue(orderRawField(meta, strings.TrimPrefix(field, "meta.")))
		default:
			value = orderDiagnosticValue(envelope[field])
		}
	}
	return []any{"reason_code", reason, "field", field,
		"orderId", orderDiagnosticValue(meta["orderIdentifier"]),
		"onChainOrderId", orderDiagnosticValue(meta["onChainOrderId"]),
		"orderType", orderDiagnosticValue(envelope["orderType"]),
		"inputSettler", orderDiagnosticValue(envelope["inputSettler"]),
		"originChainId", orderDiagnosticValue(order["originChainId"]),
		"field_value", value,
	}
}
