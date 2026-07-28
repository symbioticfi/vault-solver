package lifi

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestParseSubmittedOrder(t *testing.T) {
	cfg := testLifiConfig()
	tokenIn := common.HexToAddress("0x6666666666666666666666666666666666666666")
	tokenOut := common.HexToAddress("0x7777777777777777777777777777777777777777")

	order, err := parseSubmittedOrder(testOrderJSON(t, cfg, tokenIn, tokenOut), cfg, 11155111)
	if err != nil {
		t.Fatalf("parseSubmittedOrder: %v", err)
	}
	if order.QuoteID != "quote-1" {
		t.Fatalf("quote id = %q", order.QuoteID)
	}
	if order.TokenIn != tokenIn || order.TokenOut != tokenOut {
		t.Fatalf("tokens = %s/%s", order.TokenIn, order.TokenOut)
	}
	if got := order.AmountIn.String(); got != "1000000" {
		t.Fatalf("amount in = %s", got)
	}
	if got := order.OutputAmount.String(); got != "990000" {
		t.Fatalf("amount out = %s", got)
	}
	if order.Output.Oracle != addressIdentifier(cfg.OutputSettler) {
		t.Fatal("output oracle was not parsed as output settler identifier")
	}
	order.OutputAmount.SetInt64(1)
	if order.Output.Amount.String() != "990000" {
		t.Fatalf("output amount aliases mandate output: %s", order.Output.Amount)
	}
}

func TestParseSubmittedOrderRejectsNonStringInputTuple(t *testing.T) {
	cfg := testLifiConfig()
	var body map[string]any
	if err := json.Unmarshal(testOrderJSON(
		t,
		cfg,
		common.HexToAddress("0x6666666666666666666666666666666666666666"),
		common.HexToAddress("0x7777777777777777777777777777777777777777"),
	), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	inputs := sliceField(t, mapField(t, body, "order"), "inputs")
	inputs[0].([]any)[1] = float64(1_000_000)
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	_, err = parseSubmittedOrder(raw, cfg, 11155111)
	if err == nil || !strings.Contains(err.Error(), "expected decimal string") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseSubmittedOrderAllowsMissingQuoteID(t *testing.T) {
	cfg := testLifiConfig()
	var body map[string]any
	if err := json.Unmarshal(testOrderJSON(
		t,
		cfg,
		common.HexToAddress("0x6666666666666666666666666666666666666666"),
		common.HexToAddress("0x7777777777777777777777777777777777777777"),
	), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	delete(body, "quoteId")
	mapField(t, body, "meta")["quoteId"] = nil
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	order, err := parseSubmittedOrder(raw, cfg, 11155111)
	if err != nil {
		t.Fatalf("parseSubmittedOrder: %v", err)
	}
	if order.QuoteID != "" {
		t.Fatalf("quote id = %q", order.QuoteID)
	}
}

func TestParseSubmittedOrderInfersOnChainOrderWhenTypeMissing(t *testing.T) {
	cfg := testLifiConfig()
	var body map[string]any
	if err := json.Unmarshal(testOrderJSON(
		t,
		cfg,
		common.HexToAddress("0x6666666666666666666666666666666666666666"),
		common.HexToAddress("0x7777777777777777777777777777777777777777"),
	), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	delete(body, "orderType")
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	order, err := parseSubmittedOrder(raw, cfg, 11155111)
	if err != nil {
		t.Fatalf("parseSubmittedOrder: %v", err)
	}
	if order.OnChainOrderID == "" {
		t.Fatal("on-chain order id was not parsed")
	}
}

func TestParseSubmittedOrderRejectsMissingTypeWithoutOnChainMetadata(t *testing.T) {
	cfg := testLifiConfig()
	var body map[string]any
	if err := json.Unmarshal(testOrderJSON(
		t,
		cfg,
		common.HexToAddress("0x6666666666666666666666666666666666666666"),
		common.HexToAddress("0x7777777777777777777777777777777777777777"),
	), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	delete(body, "orderType")
	meta := body["meta"].(map[string]any)
	delete(meta, "onChainOrderId")
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if _, err = parseSubmittedOrder(raw, cfg, 11155111); err == nil ||
		!strings.Contains(err.Error(), "missing orderType requires onChainOrderId") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseSubmittedOrderPreservesOutputContext(t *testing.T) {
	cfg := testLifiConfig()
	var body map[string]any
	if err := json.Unmarshal(testOrderJSON(
		t,
		cfg,
		common.HexToAddress("0x6666666666666666666666666666666666666666"),
		common.HexToAddress("0x7777777777777777777777777777777777777777"),
	), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	outputs := sliceField(t, mapField(t, body, "order"), "outputs")
	output := outputs[0].(map[string]any)
	output["context"] = "0x01000000010000000200000000000000000000000000000000000000000000000000000000000003"
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	order, err := parseSubmittedOrder(raw, cfg, 11155111)
	if err != nil {
		t.Fatalf("parseSubmittedOrder: %v", err)
	}
	if got := hexutil.Encode(order.Output.Context); got != output["context"] {
		t.Fatalf("context = %s", got)
	}
}

func TestParseSubmittedOrderRejectsDirtyOutputIdentifier(t *testing.T) {
	cfg := testLifiConfig()
	var body map[string]any
	if err := json.Unmarshal(testOrderJSON(
		t,
		cfg,
		common.HexToAddress("0x6666666666666666666666666666666666666666"),
		common.HexToAddress("0x7777777777777777777777777777777777777777"),
	), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	outputs := sliceField(t, mapField(t, body, "order"), "outputs")
	output, ok := outputs[0].(map[string]any)
	if !ok {
		t.Fatalf("output type = %T", outputs[0])
	}
	dirty := addressIdentifier(common.HexToAddress("0x7777777777777777777777777777777777777777"))
	dirty[0] = 1
	output["token"] = hexutil.Encode(dirty[:])
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	_, err = parseSubmittedOrder(raw, cfg, 11155111)
	if err == nil || !strings.Contains(err.Error(), "clean address identifier") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseSubmittedOrderRejectsNonOnChainOrderType(t *testing.T) {
	cfg := testLifiConfig()
	var body map[string]any
	if err := json.Unmarshal(testOrderJSON(
		t,
		cfg,
		common.HexToAddress("0x6666666666666666666666666666666666666666"),
		common.HexToAddress("0x7777777777777777777777777777777777777777"),
	), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	body["orderType"] = "GaslessCrosschainOrder"
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	_, err = parseSubmittedOrder(raw, cfg, 11155111)
	if err == nil || !strings.Contains(err.Error(), "unsupported non-onchain order type") {
		t.Fatalf("err = %v", err)
	}

	body["orderType"] = "NonOnChainOrder"
	raw, err = json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	_, err = parseSubmittedOrder(raw, cfg, 11155111)
	if err == nil || !strings.Contains(err.Error(), "unsupported non-onchain order type") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseSubmittedOrderAcceptsOIFUserOpenOrderType(t *testing.T) {
	cfg := testLifiConfig()
	var body map[string]any
	if err := json.Unmarshal(testOrderJSON(
		t,
		cfg,
		common.HexToAddress("0x6666666666666666666666666666666666666666"),
		common.HexToAddress("0x7777777777777777777777777777777777777777"),
	), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	body["orderType"] = "oif-user-open-v0"
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if _, err = parseSubmittedOrder(raw, cfg, 11155111); err != nil {
		t.Fatalf("parseSubmittedOrder: %v", err)
	}
}

func TestParseSubmittedOrderRejectsMissingOrderStatus(t *testing.T) {
	cfg := testLifiConfig()
	var body map[string]any
	if err := json.Unmarshal(testOrderJSON(
		t,
		cfg,
		common.HexToAddress("0x6666666666666666666666666666666666666666"),
		common.HexToAddress("0x7777777777777777777777777777777777777777"),
	), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	delete(mapField(t, body, "meta"), "orderStatus")
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	_, err = parseSubmittedOrder(raw, cfg, 11155111)
	if err == nil || !strings.Contains(err.Error(), "unsupported order status") {
		t.Fatalf("err = %v", err)
	}
}

func testLifiConfig() *Config {
	return &Config{
		InputSettler:  common.HexToAddress("0x2222222222222222222222222222222222222222"),
		OutputSettler: common.HexToAddress("0x3333333333333333333333333333333333333333"),
		Executor:      common.HexToAddress("0x4444444444444444444444444444444444444444"),
	}
}

func testOrderJSON(t *testing.T, cfg *Config, tokenIn, tokenOut common.Address) []byte {
	t.Helper()
	user := common.HexToAddress("0x1111111111111111111111111111111111111111")
	recipient := common.HexToAddress("0x8888888888888888888888888888888888888888")
	body := map[string]any{
		"orderType":    "OnChainOrder",
		"quoteId":      "quote-1",
		"inputSettler": cfg.InputSettler.Hex(),
		"order": map[string]any{
			"user":          user.Hex(),
			"nonce":         "7",
			"originChainId": "11155111",
			"expires":       "1800000000",
			"fillDeadline":  "1800000300",
			"inputOracle":   cfg.OutputSettler.Hex(),
			"inputs": [][]string{
				{new(big.Int).SetBytes(tokenIn.Bytes()).String(), "1000000"},
			},
			"outputs": []map[string]any{{
				"oracle":       hexID(cfg.OutputSettler),
				"settler":      hexID(cfg.OutputSettler),
				"chainId":      "11155111",
				"token":        hexID(tokenOut),
				"amount":       "990000",
				"recipient":    hexID(recipient),
				"callbackData": "0x",
				"context":      "0x",
			}},
		},
		"meta": map[string]any{
			"orderStatus":     "Signed",
			"orderIdentifier": "intent-1",
			"onChainOrderId":  "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"quoteId":         "quote-from-meta",
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal order: %v", err)
	}
	return raw
}

func hexID(addr common.Address) string {
	id := addressIdentifier(addr)
	return hexutil.Encode(id[:])
}

func mapField(t *testing.T, m map[string]any, field string) map[string]any {
	t.Helper()
	out, ok := m[field].(map[string]any)
	if !ok {
		t.Fatalf("%s type = %T", field, m[field])
	}
	return out
}

func sliceField(t *testing.T, m map[string]any, field string) []any {
	t.Helper()
	out, ok := m[field].([]any)
	if !ok {
		t.Fatalf("%s type = %T", field, m[field])
	}
	return out
}
