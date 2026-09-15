package lifi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-logr/logr/funcr"
	"github.com/prometheus/client_golang/prometheus"
)

func TestOrderRejectionDiagnostics(t *testing.T) {
	cfg := testLifiConfig()
	for _, tc := range []struct {
		name, reason, field, value, outcome string
		mutate                              func(map[string]any)
		errorLevel                          bool
	}{
		{"invalid address", "lifi_invalid_address", "inputSettler", strings.Repeat("x", 157) + "...", "invalid", func(b map[string]any) { b["inputSettler"] = strings.Repeat("x", 1000) }, true},
		{"strict integer type", "lifi_decode_error", "order.originChainId", "42", "invalid", func(b map[string]any) { mapField(t, b, "order")["originChainId"] = 42 }, true},
		{"strict output type", "lifi_decode_error", "order.outputs[0].token", "42", "invalid", func(b map[string]any) {
			sliceField(t, mapField(t, b, "order"), "outputs")[0].(map[string]any)["token"] = 42
		}, true},
		{"strict second output", "lifi_decode_error", "order.outputs[1].token", "42", "invalid", func(b map[string]any) {
			order := mapField(t, b, "order")
			outputs := sliceField(t, order, "outputs")
			second := map[string]any{}
			for key, value := range outputs[0].(map[string]any) {
				second[key] = value
			}
			second["token"] = 42
			order["outputs"] = append(outputs, second)
		}, true},
		{"foreign huge chain", "unsupported_chain", "order.originChainId", strings.Repeat("9", 157) + "...", "other_chain", func(b map[string]any) { mapField(t, b, "order")["originChainId"] = strings.Repeat("9", 1000) }, false},
		{"foreign output chain", "unsupported_chain", "order.outputs[0].chainId", "1", "other_chain", func(b map[string]any) {
			sliceField(t, mapField(t, b, "order"), "outputs")[0].(map[string]any)["chainId"] = "1"
		}, false},
		{"nested invalid address", "lifi_decode_error", "inputSettler", "<object>", "invalid", func(b map[string]any) { b["inputSettler"] = map[string]any{"signature": "SECRET_SIGNATURE"} }, true},
		{"redacted callback", "lifi_invalid_hex", "order.outputs[0].callbackData", "<redacted>", "invalid", func(b map[string]any) {
			sliceField(t, mapField(t, b, "order"), "outputs")[0].(map[string]any)["callbackData"] = "SECRET_SIGNATURE"
		}, true},
		{"native input", "unsupported_native_input", "order.inputs[0][0]", "0", "unsupported", func(b map[string]any) { sliceField(t, mapField(t, b, "order"), "inputs")[0].([]any)[0] = "0" }, false},
		{"dirty input high bits", "invalid_token_identifier", "order.inputs[0][0]", "1461501637330902918203684832716283019655932542977", "invalid", func(b map[string]any) {
			sliceField(t, mapField(t, b, "order"), "inputs")[0].([]any)[0] = "1461501637330902918203684832716283019655932542977"
		}, true},
		{"invalid input decimal", "lifi_invalid_integer", "order.inputs[0][0]", "bad-token", "invalid", func(b map[string]any) {
			sliceField(t, mapField(t, b, "order"), "inputs")[0].([]any)[0] = "bad-token"
		}, true},
		{"foreign settler zero input", "unsupported_settler", "inputSettler", "0x9999999999999999999999999999999999999999", "unsupported", func(b map[string]any) {
			b["inputSettler"] = "0x9999999999999999999999999999999999999999"
			sliceField(t, mapField(t, b, "order"), "inputs")[0].([]any)[0] = "0"
		}, false},
		{"foreign settler different payload shape", "unsupported_settler", "inputSettler", "0x9999999999999999999999999999999999999999", "unsupported", func(b map[string]any) {
			b["inputSettler"] = "0x9999999999999999999999999999999999999999"
			mapField(t, b, "order")["inputs"] = "not our format"
		}, false},
		{"foreign chain different payload shape", "unsupported_chain", "order.originChainId", "1", "other_chain", func(b map[string]any) {
			mapField(t, b, "order")["originChainId"] = "1"
			mapField(t, b, "order")["inputs"] = "not our format"
		}, false},
		{"foreign output chain different payload shape", "unsupported_chain", "order.outputs[0].chainId", "1", "other_chain", func(b map[string]any) {
			order := mapField(t, b, "order")
			sliceField(t, order, "outputs")[0].(map[string]any)["chainId"] = "1"
			order["inputs"] = "not our format"
		}, false},
		{"foreign output chain non-EVM settler", "unsupported_chain", "order.outputs[0].chainId", "1", "other_chain", func(b map[string]any) {
			b["inputSettler"] = "non-EVM-settler"
			sliceField(t, mapField(t, b, "order"), "outputs")[0].(map[string]any)["chainId"] = "1"
		}, false},
		{"foreign settler different output shape", "unsupported_settler", "inputSettler", "0x9999999999999999999999999999999999999999", "unsupported", func(b map[string]any) {
			b["inputSettler"] = "0x9999999999999999999999999999999999999999"
			mapField(t, b, "order")["outputs"] = "not our format"
		}, false},
		{"multiple inputs", "unsupported_multiple_inputs", "order.inputs", "<array length=2>", "unsupported", func(b map[string]any) {
			order := mapField(t, b, "order")
			inputs := sliceField(t, order, "inputs")
			order["inputs"] = append(inputs, inputs[0])
		}, false},
		{"multiple outputs", "unsupported_multiple_outputs", "order.outputs", "<array length=2>", "unsupported", func(b map[string]any) {
			order := mapField(t, b, "order")
			outputs := sliceField(t, order, "outputs")
			order["outputs"] = append(outputs, outputs[0])
		}, false},
		{"missing inputs", "lifi_input_count", "order.inputs", "<array length=0>", "invalid", func(b map[string]any) { mapField(t, b, "order")["inputs"] = []any{} }, true},
		{"callback unsupported", "unsupported_callback_data", "order.outputs[0].callbackData", "<redacted>", "unsupported", func(b map[string]any) {
			sliceField(t, mapField(t, b, "order"), "outputs")[0].(map[string]any)["callbackData"] = "0x1234"
		}, false},
		{"zero output oracle", "lifi_zero_identifier", "order.outputs[0].oracle", "0x" + strings.Repeat("0", 64), "invalid", func(b map[string]any) {
			sliceField(t, mapField(t, b, "order"), "outputs")[0].(map[string]any)["oracle"] = "0x" + strings.Repeat("0", 64)
		}, true},
		{"zero output", "lifi_zero_output_identifier", "order.outputs[0].token", "0x" + strings.Repeat("0", 64), "unsupported", func(b map[string]any) {
			sliceField(t, mapField(t, b, "order"), "outputs")[0].(map[string]any)["token"] = "0x" + strings.Repeat("0", 64)
		}, false},
		{"unsupported type", "lifi_unsupported_order_type", "orderType", "GaslessCrosschainOrder", "unsupported", func(b map[string]any) { b["orderType"] = "GaslessCrosschainOrder" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			metrics, err := newLIFIMetrics(reg, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			raw := mutatedTestOrderJSON(t, cfg, func(b map[string]any) {
				meta := mapField(t, b, "meta")
				meta["orderIdentifier"], meta["onChainOrderId"] = "api-order-42", "chain-order-42"
				b["signature"] = "SECRET_SIGNATURE"
				tc.mutate(b)
			})
			var logs []string
			s := &Solver{cfg: cfg, chainID: 11155111, metrics: metrics, log: funcr.NewJSON(func(line string) { logs = append(logs, line) }, funcr.Options{Verbosity: 1})}
			for range 2 {
				if s.parseOrderMessage(orderMessage{Event: orderSubmitEvent, Data: raw}) != nil {
					t.Fatal("accepted invalid order")
				}
			}
			if len(logs) != 2 {
				t.Fatalf("log frequency changed: %v", logs)
			}
			for _, line := range logs {
				var fields map[string]any
				if err := json.Unmarshal([]byte(line), &fields); err != nil {
					t.Fatal(err)
				}
				for k, want := range map[string]string{"reason_code": tc.reason, "field": tc.field, "field_value": tc.value, "orderId": "api-order-42", "onChainOrderId": "chain-order-42"} {
					if fields[k] != want {
						t.Errorf("%s = %v, want %s; %s", k, fields[k], want, line)
					}
				}
				for _, k := range []string{"orderType", "inputSettler", "originChainId"} {
					if fields[k] == nil {
						t.Errorf("missing %s", k)
					}
				}
				if tc.reason == "unsupported_native_input" && fields["level"] != float64(0) {
					t.Errorf("native input should log at Info: %s", line)
				}
				_, isError := fields["error"]
				if isError != tc.errorLevel {
					t.Errorf("error level = %v, want %v", isError, tc.errorLevel)
				}
				if strings.Contains(line, "SECRET_SIGNATURE") || (strings.Contains(line, "native") && tc.reason != "unsupported_native_input") || strings.Contains(line, strings.Repeat("x", 161)) || strings.Contains(line, strings.Repeat("9", 161)) {
					t.Fatalf("unsafe log: %s", line)
				}
			}
			families, err := reg.Gather()
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, family := range families {
				if family.GetName() != "solver_bot_workflow_events_total" {
					continue
				}
				for _, metric := range family.GetMetric() {
					labels := map[string]string{}
					for _, label := range metric.GetLabel() {
						labels[label.GetName()] = label.GetValue()
					}
					if labels["event"] == "order_parse" && labels["outcome"] == tc.outcome {
						found = true
						if metric.GetCounter().GetValue() != 2 {
							t.Errorf("count = %v", metric.GetCounter().GetValue())
						}
					}
					for k := range labels {
						if k != "solver" && k != "strategy" && k != "event" && k != "outcome" {
							t.Errorf("unexpected label %s", k)
						}
					}
				}
			}
			if !found {
				t.Fatal("parse rejection metric missing")
			}
		})
	}
}
