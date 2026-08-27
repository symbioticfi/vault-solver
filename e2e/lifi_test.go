//go:build e2e

package e2e

import (
	"context"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	ethmath "github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/go-errors/errors"

	erc4626binding "github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	lifiexecutor "github.com/symbioticfi/vault-solver/api/bindings/lifi/executor"
	"github.com/symbioticfi/vault-solver/api/bindings/lifi/inputsettler"
	adapterbinding "github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
)

const lifiPriceBufferBPS = int64(20)

type lifiQuote struct {
	FromAsset    common.Address   `json:"fromAsset"`
	ToAsset      common.Address   `json:"toAsset"`
	FromDecimals uint8            `json:"fromDecimals"`
	ToDecimals   uint8            `json:"toDecimals"`
	Ranges       []lifiQuoteRange `json:"ranges"`
}

type lifiQuoteRange struct {
	MinAmount string `json:"minAmount"`
	MaxAmount string `json:"maxAmount"`
	Quote     string `json:"quote"`
}

type lifiRedemption struct {
	adapter   common.Address
	kind      string
	amountIn  *big.Int
	amountOut *big.Int
	discount  *lifiexecutor.ILiquidLaneAdapterDiscount
}

type lifiFillEvidence struct {
	transaction common.Hash
	redemptions []lifiRedemption
}

func testLifi(t *testing.T, testEnv *testEnvironment) {
	t.Helper()
	if testEnv.variant != "external" && testEnv.variant != "internal" {
		t.Fatalf("lifi variant = %q", testEnv.variant)
	}
	manifest := testEnv.manifest.Lifi
	if manifest.Executor == (common.Address{}) || manifest.InputSettler == (common.Address{}) || len(manifest.Adapters) < 2 {
		t.Fatal("LI.FI deployment manifest is incomplete")
	}
	if !containsAddress(manifest.Permissioned, manifest.TokenIn) {
		t.Fatalf("LI.FI input %s is not permissioned", manifest.TokenIn)
	}
	verifyLifiRegistration(t, testEnv, manifest.Executor)
	for _, adapter := range manifest.Adapters {
		allowed := testEnv.adapterIsFiller(t, adapter, testEnv.manifest.Participants.MarketMaker, manifest.Executor)
		if allowed != (testEnv.variant == "external") {
			t.Fatalf("LI.FI adapter %s authorization = %t in %s mode", adapter, allowed, testEnv.variant)
		}
	}

	var discounts []advertisedDiscount
	if testEnv.variant == "internal" {
		discounts = loadAdvertisedDiscounts(t, testEnv, manifest.Adapters, manifest.TokenIn, manifest.TokenOut)
	}

	var quote lifiQuote
	eventually(t, "LI.FI quote", 30*time.Second, 500*time.Millisecond, func() error {
		var response struct {
			Quotes []lifiQuote `json:"quotes"`
		}
		status := testEnv.getJSON(t, testEnv.fixtureURL+"/quotes", &response)
		if status != http.StatusOK {
			return errors.Errorf("quote status %d", status)
		}
		for _, candidate := range response.Quotes {
			if candidate.FromAsset == manifest.TokenIn && candidate.ToAsset == manifest.TokenOut && len(candidate.Ranges) > 0 {
				quote = candidate
				return nil
			}
		}
		return errors.New("matching quote not published")
	})

	rangeQuote := quote.Ranges[0]
	amountIn := parseBig(t, rangeQuote.MaxAmount)
	amountOut := decimalRateAmountOut(t, amountIn, rangeQuote.Quote, quote.FromDecimals, quote.ToDecimals)
	if amountIn.Sign() <= 0 || amountOut.Sign() <= 0 {
		t.Fatalf("LI.FI quote produced input=%s output=%s", amountIn, amountOut)
	}

	user := addressForKey(t, anvilOrderUserKey)
	balanceBefore := testEnv.balanceOf(t, manifest.TokenOut, user)
	opened := openLifiOrder(t, testEnv, manifest, user, amountIn, amountOut)
	settler := inputsettler.NewILifiInputSettler()
	eventually(t, "LI.FI on-chain fill", 30*time.Second, 500*time.Millisecond, func() error {
		status, err := settler.UnpackOrderStatus(
			testEnv.call(t, manifest.InputSettler, settler.PackOrderStatus(opened.orderID)),
		)
		if err != nil {
			return errors.Errorf("decode order status: %w", err)
		}
		if status != 2 {
			return errors.Errorf("order status is %d", status)
		}
		return nil
	})
	balanceAfter := testEnv.balanceOf(t, manifest.TokenOut, user)
	if new(big.Int).Sub(balanceAfter, balanceBefore).Cmp(amountOut) != 0 {
		t.Fatalf("LI.FI payout delta = %s, want %s", new(big.Int).Sub(balanceAfter, balanceBefore), amountOut)
	}

	evidence := findLifiFill(t, testEnv, manifest, opened)
	if len(evidence.redemptions) != 1 {
		t.Fatalf("permissioned LI.FI fill used %d routes, want 1", len(evidence.redemptions))
	}
	redemption := evidence.redemptions[0]
	expectedKind := "direct"
	if testEnv.variant == "internal" {
		expectedKind = "discount"
	}
	if redemption.kind != expectedKind || !containsAddress(manifest.Adapters, redemption.adapter) {
		t.Fatalf("LI.FI redemption kind=%s adapter=%s", redemption.kind, redemption.adapter)
	}
	mathResult := verifyLifiMath(t, testEnv, quote, amountIn, amountOut, redemption)

	var discountID string
	if testEnv.variant == "internal" {
		for _, advertised := range discounts {
			terms := redemption.discount
			if advertised.Adapter == redemption.adapter && advertised.Token == terms.TokenToRedeem &&
				advertised.Signer == terms.Signer && advertised.Discount == terms.Discount.String() &&
				advertised.Deadline == terms.Deadline.Uint64() {
				discountID = advertised.DiscountID
				break
			}
		}
		if discountID == "" {
			t.Fatal("LI.FI fill used discount terms not advertised by backend")
		}
	}

	t.Logf(
		"LI.FI %s fill order=%s tx=%s adapter=%s discount=%s output=%s headroom=%s",
		testEnv.variant,
		common.Hash(opened.orderID),
		evidence.transaction,
		redemption.adapter,
		discountID,
		amountOut,
		mathResult,
	)
}

func verifyLifiRegistration(t *testing.T, testEnv *testEnvironment, executorAddress common.Address) {
	t.Helper()
	binding := lifiexecutor.NewLiquidLaneLifiExecutor()
	caller := addressForKey(t, anvilSolverKey)
	allowed, err := binding.UnpackIsCaller(testEnv.call(t, executorAddress, binding.PackIsCaller(caller)))
	if err != nil {
		t.Fatalf("decode LI.FI caller authorization: %v", err)
	}
	if !allowed {
		t.Fatalf("LI.FI executor %s does not authorize caller %s", executorAddress, caller)
	}

	messageHash := common.BytesToHash(accounts.TextHash([]byte("local LI.FI registration")))
	chainID := big.NewInt(testEnv.manifest.Chain.ID)
	typedData := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"LifiRegistration": {{Name: "messageHash", Type: "bytes32"}},
		},
		PrimaryType: "LifiRegistration",
		Domain: apitypes.TypedDataDomain{
			Name:              "LiquidLaneLifiExecutor",
			Version:           "1",
			ChainId:           (*ethmath.HexOrDecimal256)(chainID),
			VerifyingContract: executorAddress.Hex(),
		},
		Message: apitypes.TypedDataMessage{"messageHash": messageHash.Hex()},
	}
	digest, _, err := apitypes.TypedDataAndHash(typedData)
	if err != nil {
		t.Fatalf("hash LI.FI registration: %v", err)
	}
	signatureHex := signTypedData(t, anvilSolverKey, digest)
	signature := common.FromHex(signatureHex)
	magic, err := binding.UnpackIsValidSignature(
		testEnv.call(t, executorAddress, binding.PackIsValidSignature(messageHash, signature)),
	)
	if err != nil {
		t.Fatalf("decode LI.FI registration signature result: %v", err)
	}
	if magic != [4]byte{0x16, 0x26, 0xba, 0x7e} {
		t.Fatalf("LI.FI registration magic = %#x", magic)
	}
}

type openedLifiOrder struct {
	order            inputsettler.StandardOrder
	orderID          [32]byte
	openReceiptBlock *big.Int
}

func openLifiOrder(
	t *testing.T,
	testEnv *testEnvironment,
	manifest lifiManifest,
	user common.Address,
	amountIn, amountOut *big.Int,
) openedLifiOrder {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	header, err := testEnv.client.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatalf("read LI.FI chain time: %v", err)
	}
	deadline := uint32(header.Time + 300)
	order := inputsettler.StandardOrder{
		User:          user,
		Nonce:         big.NewInt(time.Now().UnixNano()),
		OriginChainId: big.NewInt(testEnv.manifest.Chain.ID),
		Expires:       deadline,
		FillDeadline:  deadline,
		InputOracle:   manifest.OutputSettler,
		Inputs:        [][2]*big.Int{{new(big.Int).SetBytes(manifest.TokenIn.Bytes()), amountIn}},
		Outputs: []inputsettler.MandateOutput{{
			Oracle:       addressID(manifest.OutputSettler),
			Settler:      addressID(manifest.OutputSettler),
			ChainId:      big.NewInt(testEnv.manifest.Chain.ID),
			Token:        addressID(manifest.TokenOut),
			Amount:       amountOut,
			Recipient:    addressID(user),
			CallbackData: []byte{},
			Context:      []byte{0},
		}},
	}
	token := erc4626binding.NewIERC4626()
	testEnv.send(t, anvilOrderUserKey, manifest.TokenIn, token.PackApprove(manifest.InputSettler, amountIn))
	settler := inputsettler.NewILifiInputSettler()
	openReceipt := testEnv.send(t, anvilOrderUserKey, manifest.InputSettler, settler.PackOpen(order))
	orderID, err := settler.UnpackOrderIdentifier(
		testEnv.call(t, manifest.InputSettler, settler.PackOrderIdentifier(order)),
	)
	if err != nil {
		t.Fatalf("decode LI.FI order ID: %v", err)
	}
	quoteID := "local-" + common.Hash(orderID).Hex()[2:10]
	var delivery struct {
		Delivered int `json:"delivered"`
	}
	status := testEnv.postJSON(t, testEnv.fixtureURL+"/orders", map[string]any{
		"orderType":    "oif-user-open-v0",
		"quoteId":      quoteID,
		"inputSettler": manifest.InputSettler.Hex(),
		"order":        lifiWireOrder(order),
		"meta": map[string]any{
			"orderStatus":     "Signed",
			"orderIdentifier": "intent_" + quoteID,
			"onChainOrderId":  common.Hash(orderID).Hex(),
			"quoteId":         quoteID,
		},
	}, &delivery)
	if !isHTTPSuccess(status) || delivery.Delivered <= 0 {
		t.Fatalf("LI.FI order delivery status = %d, delivered = %d", status, delivery.Delivered)
	}
	return openedLifiOrder{order: order, orderID: orderID, openReceiptBlock: openReceipt.BlockNumber}
}

func findLifiFill(
	t *testing.T,
	testEnv *testEnvironment,
	manifest lifiManifest,
	opened openedLifiOrder,
) lifiFillEvidence {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	logs, err := testEnv.client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: opened.openReceiptBlock,
		Addresses: manifest.Adapters,
	})
	if err != nil {
		t.Fatalf("query LI.FI adapter logs: %v", err)
	}
	adapterBinding := adapterbinding.NewLiquidLaneAdapter()
	swapsByTransaction := make(map[common.Hash][]adapterbinding.ILiquidLaneAdapterSwap)
	var transactionOrder []common.Hash
	for index := range logs {
		event, unpackErr := adapterBinding.UnpackDoSwapEvent(&logs[index])
		if unpackErr != nil {
			continue
		}
		if _, exists := swapsByTransaction[logs[index].TxHash]; !exists {
			transactionOrder = append(transactionOrder, logs[index].TxHash)
		}
		swapsByTransaction[logs[index].TxHash] = append(swapsByTransaction[logs[index].TxHash], event.Swap)
	}

	settlerBinding := inputsettler.NewILifiInputSettler()
	for _, hash := range transactionOrder {
		transaction := testEnv.transaction(t, hash)
		if transaction.To() == nil || *transaction.To() != manifest.Executor {
			continue
		}
		values := decodeMethodInput(t, &lifiexecutor.LiquidLaneLifiExecutorMetaData, transaction.Data(), "finaliseWithCurrentTimestamp")
		if len(values) != 3 {
			continue
		}
		order := convertABIValue[lifiexecutor.IInputSettlerStandardOrder](t, values[0])
		direct := convertABIValue[[]lifiexecutor.ILiquidLaneLifiExecutorFillRoute](t, values[1])
		discounts := convertABIValue[[]lifiexecutor.ILiquidLaneLifiExecutorDiscountRoute](t, values[2])
		convertedOrder := convertLifiOrder(order)
		orderID, unpackErr := settlerBinding.UnpackOrderIdentifier(
			testEnv.call(t, manifest.InputSettler, settlerBinding.PackOrderIdentifier(convertedOrder)),
		)
		if unpackErr != nil || orderID != opened.orderID {
			continue
		}
		routes := make([]lifiRedemption, 0, len(direct)+len(discounts))
		for _, route := range direct {
			routes = append(routes, lifiRedemption{adapter: route.Adapter, kind: "direct", amountIn: route.AmountIn})
		}
		for _, route := range discounts {
			terms := route.DiscountSwap.Discount
			routes = append(routes, lifiRedemption{
				adapter:  route.Adapter,
				kind:     "discount",
				amountIn: route.AmountIn,
				discount: &terms,
			})
		}
		swaps := swapsByTransaction[hash]
		if len(swaps) != len(routes) {
			t.Fatalf("LI.FI transaction has %d routes and %d swap events", len(routes), len(swaps))
		}
		for index := range routes {
			routes[index].amountIn = swaps[index].AmountIn
			routes[index].amountOut = swaps[index].AmountOut
		}
		return lifiFillEvidence{transaction: hash, redemptions: routes}
	}
	t.Fatal("LI.FI fill transaction not found")
	return lifiFillEvidence{}
}

func verifyLifiMath(
	t *testing.T,
	testEnv *testEnvironment,
	quote lifiQuote,
	amountIn, amountOut *big.Int,
	redemption lifiRedemption,
) *big.Int {
	t.Helper()
	grossUnit := testEnv.adapterAmountOut(t, redemption.adapter, testEnv.manifest.Lifi.TokenIn, pow10(quote.FromDecimals))
	var discount, advertisedRate *big.Int
	if testEnv.variant == "internal" {
		discount = redemption.discount.Discount
		advertisedRate = advertisedDiscountRate(grossUnit, discount, quote.ToDecimals)
	} else {
		discount = testEnv.adapterMinDiscount(t, redemption.adapter, testEnv.manifest.Lifi.TokenIn)
		advertisedRate = testEnv.adapterMaxRate(t, redemption.adapter, testEnv.manifest.Lifi.TokenIn)
	}
	quoteCeiling := quoteBufferedAmount(
		amountOutForRate(amountIn, advertisedRate, quote.FromDecimals, quote.ToDecimals),
		lifiPriceBufferBPS,
	)
	rangeWire := quote.Ranges[0]
	expectedRange := singleRouteRangeQuote(rangeQuoteInput{
		minimumInput:   parseBig(t, rangeWire.MinAmount),
		maximumInput:   parseBig(t, rangeWire.MaxAmount),
		candidateRate:  advertisedRate,
		inputDecimals:  quote.FromDecimals,
		outputDecimals: quote.ToDecimals,
		priceBufferBPS: lifiPriceBufferBPS,
	})
	if amountIn.Cmp(parseBig(t, rangeWire.MaxAmount)) != 0 || amountOut.Cmp(expectedRange.amountOut) != 0 {
		t.Fatalf("LI.FI range output = %s, want %s", amountOut, expectedRange.amountOut)
	}
	grossOutput := testEnv.adapterAmountOut(t, redemption.adapter, testEnv.manifest.Lifi.TokenIn, amountIn)
	executableOutput := discountedAmountOut(grossOutput, discount)
	expectedRouteOutput := executableOutput
	if testEnv.variant == "external" {
		expectedRouteOutput = fillBufferedAmount(executableOutput, lifiPriceBufferBPS)
	}
	if redemption.amountIn.Cmp(amountIn) != 0 || redemption.amountOut.Cmp(expectedRouteOutput) != 0 {
		t.Fatalf("LI.FI route input/output = %s/%s, want %s/%s", redemption.amountIn, redemption.amountOut, amountIn, expectedRouteOutput)
	}
	if amountOut.Cmp(fillBufferedAmount(executableOutput, lifiPriceBufferBPS)) > 0 {
		t.Fatalf("LI.FI quote %s exceeds fill ceiling for %s", amountOut, executableOutput)
	}
	return new(big.Int).Sub(quoteCeiling, amountOut)
}

func decimalRateAmountOut(t *testing.T, amountIn *big.Int, rate string, inputDecimals, outputDecimals uint8) *big.Int {
	t.Helper()
	parts := strings.Split(rate, ".")
	if len(parts) > 2 {
		t.Fatalf("invalid decimal rate %q", rate)
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	digits := parts[0] + fraction
	parsed := parseBig(t, digits)
	numerator := new(big.Int).Mul(amountIn, parsed)
	numerator.Mul(numerator, pow10(outputDecimals))
	denominator := new(big.Int).Mul(pow10(uint8(len(fraction))), pow10(inputDecimals))
	return numerator.Div(numerator, denominator)
}

func lifiWireOrder(order inputsettler.StandardOrder) map[string]any {
	inputs := make([][2]string, len(order.Inputs))
	for index, input := range order.Inputs {
		inputs[index] = [2]string{input[0].String(), input[1].String()}
	}
	outputs := make([]map[string]any, len(order.Outputs))
	for index, output := range order.Outputs {
		outputs[index] = map[string]any{
			"oracle":       common.Hash(output.Oracle).Hex(),
			"settler":      common.Hash(output.Settler).Hex(),
			"chainId":      output.ChainId.String(),
			"token":        common.Hash(output.Token).Hex(),
			"amount":       output.Amount.String(),
			"recipient":    common.Hash(output.Recipient).Hex(),
			"callbackData": "0x",
			"context":      "0x00",
		}
	}
	return map[string]any{
		"user":          order.User.Hex(),
		"nonce":         order.Nonce.String(),
		"originChainId": order.OriginChainId.String(),
		"expires":       strconv.FormatUint(uint64(order.Expires), 10),
		"fillDeadline":  strconv.FormatUint(uint64(order.FillDeadline), 10),
		"inputOracle":   order.InputOracle.Hex(),
		"inputs":        inputs,
		"outputs":       outputs,
	}
}

func convertLifiOrder(order lifiexecutor.IInputSettlerStandardOrder) inputsettler.StandardOrder {
	outputs := make([]inputsettler.MandateOutput, len(order.Outputs))
	for index, output := range order.Outputs {
		outputs[index] = inputsettler.MandateOutput{
			Oracle:       output.Oracle,
			Settler:      output.Settler,
			ChainId:      output.ChainId,
			Token:        output.Token,
			Amount:       output.Amount,
			Recipient:    output.Recipient,
			CallbackData: output.CallbackData,
			Context:      output.Context,
		}
	}
	return inputsettler.StandardOrder{
		User:          order.User,
		Nonce:         order.Nonce,
		OriginChainId: order.OriginChainId,
		Expires:       order.Expires,
		FillDeadline:  order.FillDeadline,
		InputOracle:   order.InputOracle,
		Inputs:        order.Inputs,
		Outputs:       outputs,
	}
}

func addressID(address common.Address) [32]byte {
	var value [32]byte
	copy(value[12:], address.Bytes())
	return value
}

func containsAddress(values []common.Address, target common.Address) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
