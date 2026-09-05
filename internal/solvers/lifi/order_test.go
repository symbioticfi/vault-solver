package lifi

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
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

func TestParseSubmittedOrderClassifiesDifferentChain(t *testing.T) {
	foreignSettler := common.HexToAddress("0x008C3800F3Ad9b3B662d002E90Cc00000000eE17")
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "origin",
			mutate: func(body map[string]any) {
				order := mapField(t, body, "order")
				order["originChainId"] = "1"
				order["inputOracle"] = foreignSettler.Hex()
			},
		},
		{
			name: "output",
			mutate: func(body map[string]any) {
				output := sliceField(t, mapField(t, body, "order"), "outputs")[0].(map[string]any)
				output["chainId"] = "1"
				output["oracle"] = hexID(foreignSettler)
				output["settler"] = hexID(foreignSettler)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testLifiConfig()
			raw := mutatedTestOrderJSON(t, cfg, tt.mutate)
			_, err := parseSubmittedOrder(raw, cfg, 11155111)
			if !errors.Is(err, errOrderForDifferentChain) {
				t.Fatalf("error = %v, want %v", err, errOrderForDifferentChain)
			}
		})
	}
}

func TestParseSubmittedOrderKeepsInvalidOrdersActionable(t *testing.T) {
	foreignOracle := common.HexToAddress("0x008C3800F3Ad9b3B662d002E90Cc00000000eE17")
	tests := []struct {
		name       string
		mutate     func(map[string]any)
		wantReason string
	}{
		{
			name: "target chain input settler mismatch",
			mutate: func(body map[string]any) {
				body["inputSettler"] = foreignOracle.Hex()
			},
			wantReason: "does not match configured",
		},
		{
			name: "target chain oracle mismatch",
			mutate: func(body map[string]any) {
				mapField(t, body, "order")["inputOracle"] = foreignOracle.Hex()
			},
			wantReason: "does not match outputSettler",
		},
		{
			name: "target chain output oracle mismatch",
			mutate: func(body map[string]any) {
				output := sliceField(t, mapField(t, body, "order"), "outputs")[0].(map[string]any)
				output["oracle"] = hexID(foreignOracle)
			},
			wantReason: "outputs[0].oracle does not match outputSettler",
		},
		{
			name: "target chain output settler mismatch",
			mutate: func(body map[string]any) {
				output := sliceField(t, mapField(t, body, "order"), "outputs")[0].(map[string]any)
				output["settler"] = hexID(foreignOracle)
			},
			wantReason: "outputs[0].settler does not match outputSettler",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testLifiConfig()
			_, err := parseSubmittedOrder(mutatedTestOrderJSON(t, cfg, tt.mutate), cfg, 11155111)
			if err == nil || errors.Is(err, errOrderForDifferentChain) || !strings.Contains(err.Error(), tt.wantReason) {
				t.Fatalf("error = %v, want actionable %q error", err, tt.wantReason)
			}
		})
	}
}

func TestOrderInboxKeyUsesOrderPayloadInsteadOfMetadata(t *testing.T) {
	cfg := testLifiConfig()
	tokenIn := common.HexToAddress("0x6666666666666666666666666666666666666666")
	tokenOut := common.HexToAddress("0x7777777777777777777777777777777777777777")
	first, err := parseSubmittedOrder(testOrderJSON(t, cfg, tokenIn, tokenOut), cfg, 11155111)
	if err != nil {
		t.Fatalf("parse first order: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(testOrderJSON(t, cfg, tokenIn, tokenOut), &body); err != nil {
		t.Fatalf("unmarshal second order: %v", err)
	}
	mapField(t, body, "order")["nonce"] = "8"
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal second order: %v", err)
	}
	second, err := parseSubmittedOrder(raw, cfg, 11155111)
	if err != nil {
		t.Fatalf("parse second order: %v", err)
	}

	if first.OnChainOrderID != second.OnChainOrderID {
		t.Fatal("test orders do not share metadata id")
	}
	if orderInboxKey(first) == orderInboxKey(second) {
		t.Fatal("different order payloads were deduplicated by shared metadata id")
	}
	mapField(t, body, "order")["nonce"] = "7"
	mapField(t, body, "meta")["onChainOrderId"] =
		"0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	raw, err = json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal replay order: %v", err)
	}
	replay, err := parseSubmittedOrder(raw, cfg, 11155111)
	if err != nil {
		t.Fatalf("parse replay order: %v", err)
	}
	if orderInboxKey(first) != orderInboxKey(replay) {
		t.Fatal("same order payload received different keys after metadata changed")
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
		Gas:           &liquidlanegas.OracleConfig{},
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

func mutatedTestOrderJSON(t *testing.T, cfg *Config, mutate func(map[string]any)) []byte {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(testOrderJSON(
		t,
		cfg,
		common.HexToAddress("0x6666666666666666666666666666666666666666"),
		common.HexToAddress("0x7777777777777777777777777777777777777777"),
	), &body); err != nil {
		t.Fatalf("unmarshal order: %v", err)
	}
	mutate(body)
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal order: %v", err)
	}
	return raw
}

func testListedOrderJSON(
	t *testing.T,
	cfg *Config,
	tokenIn, tokenOut common.Address,
	status string,
) json.RawMessage {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(testOrderJSON(t, cfg, tokenIn, tokenOut), &body); err != nil {
		t.Fatalf("unmarshal order: %v", err)
	}
	delete(body, "orderType")
	delete(body, "quoteId")
	body["quote"] = nil
	meta := mapField(t, body, "meta")
	meta["orderStatus"] = status
	meta["submitTime"] = float64(1_700_000_000)
	meta["destinationAddress"] = common.HexToAddress("0x8888888888888888888888888888888888888888").Hex()
	for _, field := range []string{
		"orderInitiatedTxHash",
		"orderDeliveredTxHash",
		"orderVerifiedTxHash",
		"orderSettledTxHash",
		"refundTxHash",
		"signedAt",
		"expiredAt",
		"deliveredAt",
		"settledAt",
		"refundedAt",
		"lastCompactDepositBlockNumber",
	} {
		meta[field] = nil
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal listed order: %v", err)
	}
	return raw
}

func testListedOrdersPageJSON(
	t *testing.T,
	orders []json.RawMessage,
	total, offset int,
) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"data": orders,
		"meta": map[string]any{
			"total":  total,
			"limit":  orderRecoveryPageLimit,
			"offset": offset,
		},
	})
	if err != nil {
		t.Fatalf("marshal listed orders page: %v", err)
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

// LI.FI's feed carries every network it serves. An order for another chain is classified by its
// originChainId before any address is read, so a Solana submission (base58 settler, chain
// 1151111081099710) or a malformed payload for a foreign chain is expected traffic, not an error.
func TestParseSubmittedOrderClassifiesForeignChainBeforeAddresses(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"solana submission": func(body map[string]any) {
			body["inputSettler"] = "LiFiRp8RM7nJUZyUYC9FPPpDr7sAy5XPfBN6ABzBgT7"
			mapField(t, body, "order")["originChainId"] = "1151111081099710"
		},
		"malformed foreign order": func(body map[string]any) {
			order := mapField(t, body, "order")
			order["originChainId"] = "1"
			sliceField(t, order, "inputs")[0].([]any)[1] = float64(1_000_000)
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := testLifiConfig()
			_, err := parseSubmittedOrder(mutatedTestOrderJSON(t, cfg, mutate), cfg, 11155111)
			if !errors.Is(err, errOrderForDifferentChain) {
				t.Fatalf("error = %v, want errOrderForDifferentChain", err)
			}
		})
	}
}
