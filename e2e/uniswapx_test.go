//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	adapterbinding "github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	uniswapexecutor "github.com/symbioticfi/vault-solver/api/bindings/uniswapx/executor"
)

const uniswapLocalAPIKey = "local-uniswapx-order-key" //nolint:gosec // Public local-fixture credential.

type uniswapQuoteWire struct {
	AmountIn  string `json:"amountIn"`
	AmountOut string `json:"amountOut"`
}

type uniswapCreatedOrder struct {
	RFQ   uniswapRFQ   `json:"rfq"`
	Order uniswapOrder `json:"order"`
}

type uniswapRFQ struct {
	Indicative    uniswapQuoteWire `json:"indicative"`
	Hard          uniswapQuoteWire `json:"hard"`
	Opposing      uniswapQuoteWire `json:"opposing"`
	RequestID     string           `json:"requestId"`
	WireRequestID string           `json:"wireRequestId"`
}

type uniswapOrder struct {
	OrderHash string               `json:"orderHash"`
	Input     uniswapOrderAmount   `json:"input"`
	Outputs   []uniswapOrderAmount `json:"outputs"`
}

type uniswapOrderAmount struct {
	StartAmount string `json:"startAmount"`
}

type uniswapFilledOrder struct {
	OrderHash string `json:"orderHash"`
	TxHash    string `json:"txHash"`
}

type advertisedDiscount struct {
	DiscountID string         `json:"discountId"`
	Adapter    common.Address `json:"adapter"`
	Token      common.Address `json:"tokenToRedeem"`
	Signer     common.Address `json:"signer"`
	Discount   string         `json:"discount"`
	Deadline   uint64         `json:"deadline"`
	Collateral common.Address `json:"collateral"`
}

func advertisedDiscountID(
	discounts []advertisedDiscount,
	adapter, token, signer common.Address,
	discount string,
	deadline uint64,
) string {
	for _, advertised := range discounts {
		if advertised.Adapter == adapter && advertised.Token == token && advertised.Signer == signer &&
			advertised.Discount == discount && advertised.Deadline == deadline {
			return advertised.DiscountID
		}
	}
	return ""
}

func TestAdvertisedDiscountID(t *testing.T) {
	match := advertisedDiscount{
		DiscountID: "match",
		Adapter:    common.HexToAddress("0x1"),
		Token:      common.HexToAddress("0x2"),
		Signer:     common.HexToAddress("0x3"),
		Discount:   "50000",
		Deadline:   42,
	}
	wrongAdapter := match
	wrongAdapter.Adapter = common.HexToAddress("0x4")
	wrongToken := match
	wrongToken.Token = common.HexToAddress("0x4")
	wrongSigner := match
	wrongSigner.Signer = common.HexToAddress("0x4")
	wrongDiscount := match
	wrongDiscount.Discount = "50001"
	wrongDeadline := match
	wrongDeadline.Deadline++

	for _, test := range []struct {
		name      string
		candidate advertisedDiscount
		want      string
	}{
		{name: "match", candidate: match, want: match.DiscountID},
		{name: "adapter mismatch", candidate: wrongAdapter},
		{name: "token mismatch", candidate: wrongToken},
		{name: "signer mismatch", candidate: wrongSigner},
		{name: "discount mismatch", candidate: wrongDiscount},
		{name: "deadline mismatch", candidate: wrongDeadline},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := advertisedDiscountID(
				[]advertisedDiscount{test.candidate},
				match.Adapter,
				match.Token,
				match.Signer,
				match.Discount,
				match.Deadline,
			)
			if got != test.want {
				t.Fatalf("discount ID = %q, want %q", got, test.want)
			}
		})
	}

	second := match
	second.DiscountID = "second"
	if got := advertisedDiscountID(
		[]advertisedDiscount{match, second},
		match.Adapter,
		match.Token,
		match.Signer,
		match.Discount,
		match.Deadline,
	); got != match.DiscountID {
		t.Fatalf("first matching discount ID = %q, want %q", got, match.DiscountID)
	}
}

func testUniswapX(t *testing.T, testEnv *testEnvironment) {
	t.Helper()
	if testEnv.variant != "external" && testEnv.variant != "internal" {
		t.Fatalf("uniswapx variant = %q", testEnv.variant)
	}
	manifest := testEnv.manifest.UniswapX
	if manifest.Executor == (common.Address{}) || len(manifest.Adapters) == 0 {
		t.Fatal("UniswapX deployment manifest is incomplete")
	}
	for _, adapter := range manifest.Adapters {
		allowed := testEnv.adapterIsFiller(t, adapter, testEnv.manifest.Participants.MarketMaker, manifest.Executor)
		if allowed != (testEnv.variant == "external") {
			t.Fatalf("UniswapX adapter %s authorization = %t in %s mode", adapter, allowed, testEnv.variant)
		}
	}

	var discounts []advertisedDiscount
	if testEnv.variant == "internal" {
		discounts = loadAdvertisedDiscounts(t, testEnv, manifest.Adapters, manifest.TokenIn, manifest.TokenOut)
	}

	inputUnit := pow10(testEnv.tokenDecimals(t, manifest.TokenIn))
	minimumStatus, minimumQuote := requestUniswapQuote(t, testEnv, manifest.TokenIn, manifest.TokenOut, inputUnit)
	if minimumStatus != http.StatusOK {
		t.Fatalf("UniswapX minimum quote status = %d, want 200", minimumStatus)
	}
	if minimumQuote.AmountIn != inputUnit.String() || parseBig(t, minimumQuote.AmountOut).Sign() <= 0 {
		t.Fatalf("UniswapX minimum quote = %+v, want input %s and nonzero output", minimumQuote, inputUnit)
	}

	belowMinimum := new(big.Int).Sub(inputUnit, big.NewInt(1))
	belowMinimumStatus, _ := requestUniswapQuote(t, testEnv, manifest.TokenIn, manifest.TokenOut, belowMinimum)
	if belowMinimumStatus != http.StatusNoContent {
		t.Fatalf("UniswapX below-minimum quote status = %d, want 204", belowMinimumStatus)
	}

	var restricted *manifestToken
	for index := range testEnv.manifest.Tokens.Input {
		candidate := &testEnv.manifest.Tokens.Input[index]
		if candidate.Address != manifest.TokenIn {
			restricted = candidate
			break
		}
	}
	if restricted == nil {
		t.Fatal("UniswapX restricted-token fixture is missing")
	}
	restrictedAmount := new(big.Int).Mul(big.NewInt(10), pow10(restricted.Decimals))
	restrictedStatus, _ := requestUniswapQuote(t, testEnv, restricted.Address, manifest.TokenOut, restrictedAmount)
	if restrictedStatus != http.StatusNoContent {
		t.Fatalf("UniswapX restricted quote status = %d, want 204", restrictedStatus)
	}

	user := addressForKey(t, anvilDeployerKey)
	balanceBefore := testEnv.balanceOf(t, manifest.TokenOut, user)
	inputAmount := new(big.Int).Add(inputUnit, big.NewInt(17))
	var created uniswapCreatedOrder
	status := testEnv.requestJSON(
		t,
		http.MethodPost,
		testEnv.fixtureURL+"/local/orders",
		map[string]string{"x-api-key": uniswapLocalAPIKey},
		map[string]string{"amountIn": inputAmount.String()},
		&created,
	)
	if !isHTTPSuccess(status) {
		t.Fatalf("create UniswapX order status = %d", status)
	}
	if created.RFQ.Indicative.AmountOut == "0" || created.RFQ.Hard.AmountOut == "0" || created.RFQ.Opposing.AmountOut != "0" {
		t.Fatalf("unexpected UniswapX RFQ responses: %+v", created.RFQ)
	}
	if created.RFQ.WireRequestID == created.RFQ.RequestID {
		t.Fatal("UniswapX wire and posted-order request IDs match")
	}

	var filled uniswapFilledOrder
	eventually(t, "UniswapX fill", 45*time.Second, time.Second, func() error {
		var response struct {
			Orders []uniswapFilledOrder `json:"orders"`
		}
		lookupStatus := testEnv.requestJSON(
			t,
			http.MethodGet,
			testEnv.fixtureURL+"/v2/orders?orderStatus=filled",
			map[string]string{"x-api-key": uniswapLocalAPIKey},
			nil,
			&response,
		)
		if lookupStatus != http.StatusOK {
			return errors.Errorf("order lookup status %d", lookupStatus)
		}
		for _, order := range response.Orders {
			if strings.EqualFold(order.OrderHash, created.Order.OrderHash) {
				filled = order
				return nil
			}
		}
		return errors.New("order is not filled")
	})

	fill := decodeUniswapFill(t, testEnv, manifest.Executor, common.HexToHash(filled.TxHash))
	balanceAfter := testEnv.balanceOf(t, manifest.TokenOut, user)
	hardOutput := parseBig(t, created.RFQ.Hard.AmountOut)
	if new(big.Int).Sub(balanceAfter, balanceBefore).Cmp(hardOutput) != 0 {
		t.Fatalf("UniswapX payout delta = %s, want %s", new(big.Int).Sub(balanceAfter, balanceBefore), hardOutput)
	}
	if created.Order.Input.StartAmount != inputAmount.String() || len(created.Order.Outputs) == 0 || created.Order.Outputs[0].StartAmount != hardOutput.String() {
		t.Fatalf("UniswapX posted order does not match hard RFQ: %+v", created.Order)
	}

	var routeAdapter common.Address
	var routeInput *big.Int
	var directOutput *big.Int
	var discount *big.Int
	var discountID string
	if testEnv.variant == "external" {
		if len(fill.Routes) != 1 || len(fill.DiscountRoutes) != 0 {
			t.Fatalf("UniswapX external routes = %d direct, %d discount", len(fill.Routes), len(fill.DiscountRoutes))
		}
		route := fill.Routes[0]
		routeAdapter, routeInput, directOutput = route.Adapter, route.AmountIn, route.AmountOut
		discount = testEnv.adapterMinDiscount(t, routeAdapter, manifest.TokenIn)
	} else {
		if len(fill.Routes) != 0 || len(fill.DiscountRoutes) != 1 {
			t.Fatalf("UniswapX internal routes = %d direct, %d discount", len(fill.Routes), len(fill.DiscountRoutes))
		}
		route := fill.DiscountRoutes[0]
		routeAdapter, routeInput = route.Adapter, route.AmountIn
		terms := route.DiscountSwap.Discount
		discount = terms.Discount
		discountID = advertisedDiscountID(
			discounts,
			route.Adapter,
			terms.TokenToRedeem,
			terms.Signer,
			terms.Discount.String(),
			terms.Deadline.Uint64(),
		)
		if discountID == "" {
			t.Fatal("UniswapX fill used discount terms not advertised by backend")
		}
	}
	if routeInput.Cmp(inputAmount) != 0 {
		t.Fatalf("UniswapX route input = %s, want %s", routeInput, inputAmount)
	}

	inputDecimals := testEnv.tokenDecimals(t, manifest.TokenIn)
	outputDecimals := testEnv.tokenDecimals(t, manifest.TokenOut)
	grossUnit := testEnv.adapterAmountOut(t, routeAdapter, manifest.TokenIn, pow10(inputDecimals))
	var advertisedRate *big.Int
	if testEnv.variant == "internal" {
		advertisedRate = advertisedDiscountRate(grossUnit, discount, outputDecimals)
	} else {
		advertisedRate = testEnv.adapterMaxRate(t, routeAdapter, manifest.TokenIn)
	}
	expectedQuote := quoteBufferedAmount(
		amountOutForRate(inputAmount, advertisedRate, inputDecimals, outputDecimals),
		20,
	)
	if hardOutput.Cmp(expectedQuote) != 0 || parseBig(t, created.RFQ.Indicative.AmountOut).Cmp(expectedQuote) != 0 {
		t.Fatalf("UniswapX quote = %s, indicative = %s, want %s", hardOutput, created.RFQ.Indicative.AmountOut, expectedQuote)
	}
	grossOutput := testEnv.adapterAmountOut(t, routeAdapter, manifest.TokenIn, inputAmount)
	executableOutput := discountedAmountOut(grossOutput, discount)
	if hardOutput.Cmp(fillBufferedAmount(executableOutput, 20)) > 0 {
		t.Fatalf("UniswapX quote %s exceeds fill ceiling for %s", hardOutput, executableOutput)
	}

	fillOutput := uniswapSwapOutput(t, testEnv, common.HexToHash(filled.TxHash), routeAdapter)
	expectedRouteOutput := executableOutput
	if testEnv.variant == "external" {
		expectedRouteOutput = hardOutput
		if directOutput.Cmp(fillOutput) != 0 {
			t.Fatalf("UniswapX direct calldata output = %s, event = %s", directOutput, fillOutput)
		}
	}
	if fillOutput.Cmp(expectedRouteOutput) != 0 {
		t.Fatalf("UniswapX route output = %s, want %s", fillOutput, expectedRouteOutput)
	}
	executionSurplus := new(big.Int).Sub(fillOutput, hardOutput)

	t.Logf(
		"UniswapX %s fill order=%s tx=%s adapter=%s discount=%s output=%s surplus=%s",
		testEnv.variant,
		created.Order.OrderHash,
		filled.TxHash,
		routeAdapter,
		discountID,
		fillOutput,
		executionSurplus,
	)
}

func requestUniswapQuote(
	t *testing.T,
	testEnv *testEnvironment,
	tokenIn, tokenOut common.Address,
	amount *big.Int,
) (int, uniswapQuoteWire) {
	t.Helper()
	var quote uniswapQuoteWire
	status := testEnv.postJSON(t, testEnv.quoteURL, map[string]any{
		"requestId":       randomUUID(t),
		"quoteId":         randomUUID(t),
		"tokenInChainId":  testEnv.manifest.Chain.ID,
		"tokenOutChainId": testEnv.manifest.Chain.ID,
		"swapper":         common.Address{}.Hex(),
		"tokenIn":         tokenIn.Hex(),
		"tokenOut":        tokenOut.Hex(),
		"amount":          amount.String(),
		"type":            "EXACT_INPUT",
		"numOutputs":      1,
		"protocol":        "v1",
	}, &quote)
	return status, quote
}

func decodeUniswapFill(
	t *testing.T,
	testEnv *testEnvironment,
	executorAddress common.Address,
	hash common.Hash,
) uniswapexecutor.ILiquidLaneUniswapXExecutorFillCall {
	t.Helper()
	transaction := testEnv.transaction(t, hash)
	if transaction.To() == nil || *transaction.To() != executorAddress {
		t.Fatalf("UniswapX fill target = %v, want %s", transaction.To(), executorAddress)
	}
	values := decodeMethodInput(t, &uniswapexecutor.LiquidLaneUniswapXExecutorMetaData, transaction.Data(), "execute")
	if len(values) != 2 {
		t.Fatalf("UniswapX execute has %d arguments", len(values))
	}
	return convertABIValue[uniswapexecutor.ILiquidLaneUniswapXExecutorFillCall](t, values[1])
}

func uniswapSwapOutput(t *testing.T, testEnv *testEnvironment, hash common.Hash, routeAdapter common.Address) *big.Int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	receipt, err := testEnv.client.TransactionReceipt(ctx, hash)
	if err != nil {
		t.Fatalf("read UniswapX receipt: %v", err)
	}
	binding := adapterbinding.NewLiquidLaneAdapter()
	for _, entry := range receipt.Logs {
		if entry.Address != routeAdapter || len(entry.Topics) == 0 {
			continue
		}
		event, unpackErr := binding.UnpackDoSwapEvent(entry)
		if unpackErr == nil {
			return event.Swap.AmountOut
		}
	}
	t.Fatal("UniswapX fill emitted no adapter DoSwap event")
	return nil
}

func loadAdvertisedDiscounts(
	t *testing.T,
	testEnv *testEnvironment,
	adapters []common.Address,
	tokenIn, tokenOut common.Address,
) []advertisedDiscount {
	t.Helper()
	adapterSet := make(map[common.Address]struct{}, len(adapters))
	for _, adapter := range adapters {
		adapterSet[adapter] = struct{}{}
	}
	var matching []advertisedDiscount
	eventually(t, "advertised discounts", 30*time.Second, 500*time.Millisecond, func() error {
		var response struct {
			Discounts []advertisedDiscount `json:"discounts"`
		}
		status := testEnv.getJSON(t, strings.TrimRight(testEnv.backendURL, "/")+"/api-internal/v1/discounts", &response)
		if status != http.StatusOK {
			return errors.Errorf("discount status %d", status)
		}
		matching = matching[:0]
		for _, discount := range response.Discounts {
			if _, ok := adapterSet[discount.Adapter]; ok && discount.Token == tokenIn && discount.Collateral == tokenOut {
				matching = append(matching, discount)
			}
		}
		if len(matching) == 0 {
			return errors.New("no matching discounts")
		}
		return nil
	})
	return matching
}

func randomUUID(t *testing.T) string {
	t.Helper()
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatalf("generate UUID: %v", err)
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
