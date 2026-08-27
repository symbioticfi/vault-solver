//go:build e2e

package e2e

import (
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/go-errors/errors"

	rfqexecutor "github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
)

type rfqApprovalResponse struct {
	Approval *rfqApproval `json:"approval"`
}

type rfqApproval struct {
	To    common.Address `json:"to"`
	Data  string         `json:"data"`
	Value string         `json:"value"`
}

type rfqQuoteResponse struct {
	Quote         json.RawMessage  `json:"quote"`
	SignatureData rfqSignatureData `json:"signatureData"`
}

type rfqSignatureData struct {
	Domain      apitypes.TypedDataDomain  `json:"domain"`
	Types       apitypes.Types            `json:"types"`
	PrimaryType string                    `json:"primaryType"`
	Value       apitypes.TypedDataMessage `json:"value"`
}

type rfqQuotePayload struct {
	AggregatedOutputs []rfqAmount `json:"aggregatedOutputs"`
}

type rfqAmount struct {
	Amount string `json:"amount"`
}

type rfqOrderWire struct {
	OrderID        string            `json:"orderId"`
	OrderStatus    string            `json:"orderStatus"`
	TxHash         string            `json:"txHash"`
	Input          rfqAmount         `json:"input"`
	Outputs        []rfqAmount       `json:"outputs"`
	SettledAmounts []json.RawMessage `json:"settledAmounts"`
}

func testRFQ(t *testing.T, testEnv *testEnvironment) {
	t.Helper()
	if testEnv.variant != "external" && testEnv.variant != "internal" {
		t.Fatalf("rfq variant = %q", testEnv.variant)
	}
	manifest := testEnv.manifest
	if manifest.Contracts.Executor == (common.Address{}) || len(manifest.Vaults) == 0 {
		t.Fatal("RFQ deployment manifest is incomplete")
	}
	for _, vault := range manifest.Vaults {
		allowed := testEnv.adapterIsFiller(t, vault.Adapter, manifest.Participants.MarketMaker, manifest.Contracts.Executor)
		if allowed != (testEnv.variant == "external") {
			t.Fatalf("RFQ adapter %s authorization = %t in %s mode", vault.Adapter, allowed, testEnv.variant)
		}
	}

	adapters := make([]common.Address, 0, len(manifest.Vaults))
	for _, vault := range manifest.Vaults {
		adapters = append(adapters, vault.Adapter)
	}
	var discounts []advertisedDiscount
	if testEnv.variant == "internal" {
		discounts = loadAdvertisedDiscounts(t, testEnv, adapters, manifest.Tokens.DefaultInput, manifest.Tokens.DefaultOutput)
	}

	user := addressForKey(t, anvilDeployerKey)
	amount := new(big.Int).Mul(big.NewInt(23), pow10(18))
	balanceBefore := testEnv.balanceOf(t, manifest.Tokens.DefaultOutput, user)
	backendAPI := strings.TrimRight(testEnv.backendURL, "/") + "/api/v1"
	var approval rfqApprovalResponse
	status := testEnv.postJSON(t, backendAPI+"/check_approval", map[string]any{
		"walletAddress": user.Hex(),
		"chainId":       manifest.Chain.ID,
		"token":         manifest.Tokens.DefaultInput.Hex(),
		"amount":        amount.String(),
	}, &approval)
	if status != http.StatusOK {
		t.Fatalf("RFQ approval status = %d", status)
	}
	if approval.Approval != nil {
		data, err := hexutil.Decode(approval.Approval.Data)
		if err != nil {
			t.Fatalf("decode RFQ approval calldata: %v", err)
		}
		if approval.Approval.Value != "" && approval.Approval.Value != "0" {
			t.Fatalf("RFQ approval unexpectedly transfers value %s", approval.Approval.Value)
		}
		testEnv.send(t, anvilDeployerKey, approval.Approval.To, data)
	}

	var quote rfqQuoteResponse
	eventually(t, "RFQ backend quote", 30*time.Second, time.Second, func() error {
		quote = rfqQuoteResponse{}
		quoteStatus := testEnv.postJSON(t, backendAPI+"/quote", map[string]any{
			"tokenInChainId":    manifest.Chain.ID,
			"tokenOutChainId":   manifest.Chain.ID,
			"tokenIn":           manifest.Tokens.DefaultInput.Hex(),
			"tokenOut":          manifest.Tokens.DefaultOutput.Hex(),
			"type":              "EXACT_INPUT",
			"amount":            amount.String(),
			"swapper":           user.Hex(),
			"slippageTolerance": 0.5,
			"routingPreference": "BEST_PRICE",
			"permitAmount":      "EXACT",
			"outputs": []map[string]string{{
				"token":     manifest.Tokens.DefaultOutput.Hex(),
				"recipient": user.Hex(),
			}},
		}, &quote)
		if quoteStatus != http.StatusOK || len(quote.Quote) == 0 || quote.SignatureData.PrimaryType == "" {
			return errors.Errorf("quote status %d or incomplete response", quoteStatus)
		}
		return nil
	})

	quote.SignatureData.Types["EIP712Domain"] = []apitypes.Type{
		{Name: "name", Type: "string"},
		{Name: "version", Type: "string"},
		{Name: "chainId", Type: "uint256"},
		{Name: "verifyingContract", Type: "address"},
	}
	typedData := apitypes.TypedData{
		Types:       quote.SignatureData.Types,
		PrimaryType: quote.SignatureData.PrimaryType,
		Domain:      quote.SignatureData.Domain,
		Message:     quote.SignatureData.Value,
	}
	digest, _, err := apitypes.TypedDataAndHash(typedData)
	if err != nil {
		t.Fatalf("hash RFQ typed data: %v", err)
	}
	signature := signTypedData(t, anvilDeployerKey, digest)
	var created struct {
		OrderID string `json:"orderId"`
	}
	status = testEnv.postJSON(t, backendAPI+"/order", map[string]any{
		"quote":     quote.Quote,
		"signature": signature,
	}, &created)
	if !isHTTPSuccess(status) || created.OrderID == "" {
		t.Fatalf("RFQ order creation status = %d, order ID = %q", status, created.OrderID)
	}

	terminal := map[string]bool{
		"filled":             true,
		"expired":            true,
		"error":              true,
		"cancelled":          true,
		"unverified":         true,
		"insufficient-funds": true,
	}
	var order rfqOrderWire
	eventually(t, "indexed RFQ fill", 90*time.Second, time.Second, func() error {
		var response struct {
			Orders []rfqOrderWire `json:"orders"`
		}
		lookupStatus := testEnv.getJSON(t, backendAPI+"/orders?orderId="+created.OrderID, &response)
		if lookupStatus != http.StatusOK || len(response.Orders) == 0 {
			return errors.Errorf("order lookup status %d", lookupStatus)
		}
		order = response.Orders[0]
		if !terminal[order.OrderStatus] {
			return errors.Errorf("order status %q", order.OrderStatus)
		}
		if order.OrderStatus != "filled" {
			return errors.Errorf("order terminal status %q", order.OrderStatus)
		}
		return nil
	})
	if order.TxHash == "" || len(order.SettledAmounts) == 0 {
		t.Fatalf("RFQ filled order lacks settlement evidence: %+v", order)
	}

	settlement := decodeRFQFill(t, testEnv, manifest.Contracts.Executor, common.HexToHash(order.TxHash))
	mathResult := verifyRFQSettlementMath(t, testEnv, settlement, order, quote)
	balanceAfter := testEnv.balanceOf(t, manifest.Tokens.DefaultOutput, user)
	if new(big.Int).Sub(balanceAfter, balanceBefore).Cmp(mathResult.outputAmount) != 0 {
		t.Fatalf("RFQ payout delta = %s, want %s", new(big.Int).Sub(balanceAfter, balanceBefore), mathResult.outputAmount)
	}

	var discountID string
	if testEnv.variant == "external" {
		if len(settlement.direct) == 0 || len(settlement.discounts) != 0 {
			t.Fatalf("RFQ external routes = %d direct, %d discount", len(settlement.direct), len(settlement.discounts))
		}
	} else {
		if len(settlement.direct) != 0 || len(settlement.discounts) == 0 {
			t.Fatalf("RFQ internal routes = %d direct, %d discount", len(settlement.direct), len(settlement.discounts))
		}
		terms := settlement.discounts[0].DiscountSwap.Discount
		for _, advertised := range discounts {
			if advertised.Adapter == settlement.discounts[0].Adapter && advertised.Token == terms.TokenToRedeem &&
				advertised.Signer == terms.Signer && advertised.Discount == terms.Discount.String() &&
				advertised.Deadline == terms.Deadline.Uint64() {
				discountID = advertised.DiscountID
				break
			}
		}
		if discountID == "" {
			t.Fatal("RFQ fill used discount terms not advertised by backend")
		}
	}

	t.Logf(
		"RFQ %s fill order=%s tx=%s discount=%s output=%s headroom=%s",
		testEnv.variant,
		created.OrderID,
		order.TxHash,
		discountID,
		mathResult.outputAmount,
		mathResult.roundingHeadroom,
	)
}

type rfqSettlement struct {
	order     rfqexecutor.IReactorOrder
	direct    []rfqexecutor.IReactorSwapInput
	discounts []rfqexecutor.IReactorDiscountSwapInput
}

type rfqMathResult struct {
	outputAmount     *big.Int
	roundingHeadroom *big.Int
}

func decodeRFQFill(
	t *testing.T,
	testEnv *testEnvironment,
	executorAddress common.Address,
	hash common.Hash,
) rfqSettlement {
	t.Helper()
	transaction := testEnv.transaction(t, hash)
	if transaction.To() == nil || *transaction.To() != executorAddress {
		t.Fatalf("RFQ fill target = %v, want %s", transaction.To(), executorAddress)
	}
	values := decodeMethodInput(t, &rfqexecutor.ExecutorMetaData, transaction.Data(), "fill")
	if len(values) != 5 {
		t.Fatalf("RFQ fill has %d arguments, want 5", len(values))
	}
	return rfqSettlement{
		order:     convertABIValue[rfqexecutor.IReactorOrder](t, values[0]),
		direct:    convertABIValue[[]rfqexecutor.IReactorSwapInput](t, values[2]),
		discounts: convertABIValue[[]rfqexecutor.IReactorDiscountSwapInput](t, values[3]),
	}
}

func verifyRFQSettlementMath(
	t *testing.T,
	testEnv *testEnvironment,
	settlement rfqSettlement,
	order rfqOrderWire,
	quote rfqQuoteResponse,
) rfqMathResult {
	t.Helper()
	amountIn := parseBig(t, order.Input.Amount)
	outputAmount := new(big.Int)
	for _, output := range order.Outputs {
		outputAmount.Add(outputAmount, parseBig(t, output.Amount))
	}
	signedOutput := new(big.Int)
	for _, output := range settlement.order.Request.Outputs {
		signedOutput.Add(signedOutput, output.Amount)
	}
	finalOutput := new(big.Int)
	for _, output := range settlement.order.Outputs {
		finalOutput.Add(finalOutput, output.Amount)
	}
	var quotePayload rfqQuotePayload
	if err := json.Unmarshal(quote.Quote, &quotePayload); err != nil || len(quotePayload.AggregatedOutputs) == 0 {
		t.Fatalf("decode RFQ quote payload: %v", err)
	}
	quotedOutput := parseBig(t, quotePayload.AggregatedOutputs[0].Amount)
	if signedOutput.Cmp(quotedOutput) != 0 || finalOutput.Cmp(outputAmount) != 0 || finalOutput.Cmp(signedOutput) < 0 {
		t.Fatalf("RFQ output accounting signed=%s quoted=%s final=%s backend=%s", signedOutput, quotedOutput, finalOutput, outputAmount)
	}

	routedInput := new(big.Int)
	for _, route := range settlement.direct {
		routedInput.Add(routedInput, route.Swap.AmountIn)
	}
	for _, route := range settlement.discounts {
		routedInput.Add(routedInput, route.AmountIn)
	}
	if routedInput.Cmp(amountIn) != 0 {
		t.Fatalf("RFQ routed input = %s, want %s", routedInput, amountIn)
	}

	if testEnv.variant == "external" {
		executableOutput := new(big.Int)
		routedOutput := new(big.Int)
		for _, route := range settlement.direct {
			gross := testEnv.adapterAmountOut(t, route.Adapter, testEnv.manifest.Tokens.DefaultInput, route.Swap.AmountIn)
			discount := testEnv.adapterMinDiscount(t, route.Adapter, testEnv.manifest.Tokens.DefaultInput)
			executable := discountedAmountOut(gross, discount)
			if route.Swap.AmountOut.Cmp(executable) != 0 {
				t.Fatalf("RFQ route %s output = %s, want %s", route.Adapter, route.Swap.AmountOut, executable)
			}
			executableOutput.Add(executableOutput, executable)
			routedOutput.Add(routedOutput, route.Swap.AmountOut)
		}
		if routedOutput.Cmp(finalOutput) != 0 {
			t.Fatalf("RFQ routed output = %s, want %s", routedOutput, finalOutput)
		}
		return rfqMathResult{outputAmount: outputAmount, roundingHeadroom: new(big.Int).Sub(executableOutput, finalOutput)}
	}

	if len(settlement.discounts) != 1 {
		t.Fatalf("RFQ internal arithmetic requires one route, got %d", len(settlement.discounts))
	}
	route := settlement.discounts[0]
	inputDecimals := testEnv.tokenDecimals(t, testEnv.manifest.Tokens.DefaultInput)
	outputDecimals := testEnv.tokenDecimals(t, testEnv.manifest.Tokens.DefaultOutput)
	gross := testEnv.adapterAmountOut(t, route.Adapter, testEnv.manifest.Tokens.DefaultInput, route.AmountIn)
	grossUnit := testEnv.adapterAmountOut(t, route.Adapter, testEnv.manifest.Tokens.DefaultInput, pow10(inputDecimals))
	discount := route.DiscountSwap.Discount.Discount
	executableOutput := discountedAmountOut(gross, discount)
	advertisedRate := advertisedDiscountRate(grossUnit, discount, outputDecimals)
	conservativeOutput := conservativeAdvertisedAmountOut(route.AmountIn, advertisedRate, inputDecimals, outputDecimals)
	if finalOutput.Cmp(conservativeOutput) != 0 {
		t.Fatalf("RFQ discount output = %s, want conservative %s (executable %s)", finalOutput, conservativeOutput, executableOutput)
	}
	return rfqMathResult{outputAmount: outputAmount, roundingHeadroom: new(big.Int).Sub(executableOutput, finalOutput)}
}
