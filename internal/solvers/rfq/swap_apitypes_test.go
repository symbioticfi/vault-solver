package rfq

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestParseUint256RejectsValuesAboveMaximum(t *testing.T) {
	maximum := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	if parsed, err := parseUint256(maximum.String(), "amount"); err != nil || parsed.Cmp(maximum) != 0 {
		t.Fatalf("maximum uint256 = %v, err %v", parsed, err)
	}
	overflow := new(big.Int).Add(maximum, big.NewInt(1))
	if parsed, err := parseUint256(overflow.String(), "amount"); err == nil {
		t.Fatalf("overflow parsed as %v", parsed)
	}
}

const (
	testDiscoveryRequestID = "8fcc1d0d-246d-4e8e-9620-13c76857b31a"
	testConfirmRequestID   = "2ac09473-0c50-4db0-ad22-9417522f3ca2"
	testBuildRequestID     = "5e56f7c0-3840-4545-a8ca-e942ce3f3d71"
	testQuoteID            = "92b1be9d-25c1-4eca-80d1-fd1338ab57d2"
	testSolverQuoteID      = "ed972bed-60a9-499e-ab25-0d4d09b4aa5a"
	testBuildID            = "7423df2b-957b-47d5-acbc-21c3bd8a614e"
	testSwapper            = "0x7777777777777777777777777777777777777777"
	testTokenIn            = "0x1111111111111111111111111111111111111111"
	testTokenOut           = "0x2222222222222222222222222222222222222222"
	testAdapter            = "0x3333333333333333333333333333333333333333"
	testVault              = "0x6666666666666666666666666666666666666666"
	testRouter             = "0x5555555555555555555555555555555555555555"
)

func TestSwapRequestParseDiscovery(t *testing.T) {
	r := baseSwapRequest(swapPhaseDiscovery, testDiscoveryRequestID)
	r.SampleAmountsIn = []string{"4", "10"}
	r.Adapters = []quoteAdapter{{
		Adapter: testAdapter, Asset: testTokenOut, AssetDecimals: 6, MaxAssets: "200", MaxRate: "1000000000000000000",
	}}

	parsed, err := r.parse(1, common.HexToAddress(testRouter))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Phase != swapPhaseDiscovery || len(parsed.SampleAmountsIn) != 2 || len(parsed.Inventory) != 1 {
		t.Fatalf("parsed discovery = %+v", parsed)
	}
	if parsed.SampleAmountsIn[0].String() != "4" || parsed.SampleAmountsIn[1].String() != "10" {
		t.Fatalf("samples = %v", parsed.SampleAmountsIn)
	}
}

func TestSwapRequestParseRejectsMoreThan64Adapters(t *testing.T) {
	r := baseSwapRequest(swapPhaseDiscovery, testDiscoveryRequestID)
	r.SampleAmountsIn = []string{"10"}
	adapter := quoteAdapter{
		Adapter: testAdapter, Asset: testTokenOut, AssetDecimals: 6, MaxAssets: "200", MaxRate: "1000000000000000000",
	}
	r.Adapters = make([]quoteAdapter, 65)
	for i := range r.Adapters {
		r.Adapters[i] = adapter
	}
	if parsed, err := r.parse(1, common.HexToAddress(testRouter)); err == nil {
		t.Fatalf("65 adapters parsed as %+v", parsed)
	}
}

func TestSwapRequestParseRejectsMoreThan64LiquidityDomains(t *testing.T) {
	r := validBuildRequest()
	r.LiquidityDomains = make([]string, maxSwapCalls+1)
	for i := range r.LiquidityDomains {
		vault := strings.ToLower(common.BigToAddress(big.NewInt(int64(i + 1))).Hex())
		r.LiquidityDomains[i] = "capacity:1:" + vault + ":" + testTokenOut
	}
	if parsed, err := r.parse(1, common.HexToAddress(testRouter)); err == nil {
		t.Fatalf("65 liquidity domains parsed as %+v", parsed)
	}
}

func TestSwapRequestParseConfirm(t *testing.T) {
	r := baseSwapRequest(swapPhaseConfirm, testConfirmRequestID)
	r.DiscoveryRequestID = ptr(testDiscoveryRequestID)
	r.AmountIn = ptr("10")
	r.MinAmountOut = ptr("19")
	r.Deadline = int64Ptr(2_000_000_000)
	r.Adapters = []quoteAdapter{{
		Adapter: testAdapter, Asset: testTokenOut, AssetDecimals: 6, MaxAssets: "200", MaxRate: "1000000000000000000",
	}}

	parsed, err := r.parse(1, common.HexToAddress(testRouter))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.DiscoveryRequestID.String() != testDiscoveryRequestID || parsed.AmountIn.String() != "10" ||
		parsed.MinAmountOut.String() != "19" || !parsed.Deadline.Equal(time.Unix(2_000_000_000, 0)) {
		t.Fatalf("parsed confirm = %+v", parsed)
	}
}

func TestSwapRequestParseBuild(t *testing.T) {
	r := baseSwapRequest(swapPhaseBuild, testBuildRequestID)
	r.SolverQuoteID = ptr(testSolverQuoteID)
	r.BuildID = ptr(testBuildID)
	r.AmountIn = ptr("10")
	r.MinAmountOut = ptr("19")
	r.Deadline = int64Ptr(2_000_000_000)
	r.LiquidityDomains = []string{"capacity:1:" + testVault + ":" + testTokenOut}
	r.Router = ptr(strings.ToUpper(testRouter[:2]) + testRouter[2:])

	parsed, err := r.parse(1, common.HexToAddress(testRouter))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.SolverQuoteID.String() != testSolverQuoteID || parsed.BuildID.String() != testBuildID ||
		parsed.Router != common.HexToAddress(testRouter) || len(parsed.LiquidityDomains) != 1 {
		t.Fatalf("parsed build = %+v", parsed)
	}
}

func TestSwapRequestParseRejectsPhaseMismatches(t *testing.T) {
	cases := map[string]swapRequest{
		"discovery with amount": func() swapRequest {
			r := baseSwapRequest(swapPhaseDiscovery, testDiscoveryRequestID)
			r.SampleAmountsIn, r.AmountIn = []string{"1"}, ptr("1")
			return r
		}(),
		"discovery with empty domains": func() swapRequest {
			r := baseSwapRequest(swapPhaseDiscovery, testDiscoveryRequestID)
			r.SampleAmountsIn = []string{"1"}
			r.LiquidityDomains = []string{}
			return r
		}(),
		"confirm without discovery": func() swapRequest {
			r := baseSwapRequest(swapPhaseConfirm, testConfirmRequestID)
			r.AmountIn, r.MinAmountOut, r.Deadline = ptr("1"), ptr("1"), int64Ptr(2_000_000_000)
			return r
		}(),
		"confirm with domains": func() swapRequest {
			r := validConfirmRequest()
			r.LiquidityDomains = []string{"capacity:1:" + testVault + ":" + testTokenOut}
			return r
		}(),
		"confirm with empty samples": func() swapRequest {
			r := validConfirmRequest()
			r.SampleAmountsIn = []string{}
			return r
		}(),
		"build with adapters": func() swapRequest {
			r := validBuildRequest()
			r.Adapters = []quoteAdapter{{Adapter: testAdapter}}
			return r
		}(),
		"build with samples": func() swapRequest {
			r := validBuildRequest()
			r.SampleAmountsIn = []string{"1"}
			return r
		}(),
		"build with empty adapters": func() swapRequest {
			r := validBuildRequest()
			r.Adapters = []quoteAdapter{}
			return r
		}(),
	}
	for name, request := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := request.parse(1, common.HexToAddress(testRouter)); err == nil {
				t.Fatal("expected phase mismatch")
			}
		})
	}
}

func TestSwapRequestParseRejectsInvalidUUIDAmountDeadlineAndDomain(t *testing.T) {
	cases := map[string]swapRequest{
		"non-canonical uuid": func() swapRequest {
			r := baseSwapRequest(swapPhaseDiscovery, strings.ToUpper(testDiscoveryRequestID))
			r.SampleAmountsIn = []string{"1"}
			return r
		}(),
		"duplicate sample": func() swapRequest {
			r := baseSwapRequest(swapPhaseDiscovery, testDiscoveryRequestID)
			r.SampleAmountsIn = []string{"1", "1"}
			return r
		}(),
		"descending sample": func() swapRequest {
			r := baseSwapRequest(swapPhaseDiscovery, testDiscoveryRequestID)
			r.SampleAmountsIn = []string{"2", "1"}
			return r
		}(),
		"zero sample": func() swapRequest {
			r := baseSwapRequest(swapPhaseDiscovery, testDiscoveryRequestID)
			r.SampleAmountsIn = []string{"0"}
			return r
		}(),
		"zero exact amount": func() swapRequest {
			r := validConfirmRequest()
			r.AmountIn = ptr("0")
			return r
		}(),
		"invalid deadline": func() swapRequest {
			r := validConfirmRequest()
			r.Deadline = int64Ptr(1 << 48)
			return r
		}(),
		"wrong domain chain": func() swapRequest {
			r := validBuildRequest()
			r.LiquidityDomains = []string{"capacity:2:" + testVault + ":" + testTokenOut}
			return r
		}(),
		"uppercase domain": func() swapRequest {
			r := validBuildRequest()
			r.LiquidityDomains = []string{strings.ToUpper("capacity:1:" + testVault + ":" + testTokenOut)}
			return r
		}(),
		"duplicate domain": func() swapRequest {
			r := validBuildRequest()
			d := "capacity:1:" + testVault + ":" + testTokenOut
			r.LiquidityDomains = []string{d, d}
			return r
		}(),
		"wrong router": func() swapRequest {
			r := validBuildRequest()
			r.Router = ptr(testAdapter)
			return r
		}(),
	}
	for name, request := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := request.parse(1, common.HexToAddress(testRouter)); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestSwapResponseJSONPinsV2FieldNamesAndLowercaseValues(t *testing.T) {
	points := []swapPointResponse{{
		AmountIn: "10", AmountOut: "19",
		LiquidityDomains: []string{"capacity:1:" + testVault + ":" + testTokenOut},
	}}
	response := swapResponse{
		Protocol: swapProtocolV2, Phase: swapPhaseDiscovery, RequestID: testDiscoveryRequestID,
		QuoteID: testQuoteID, ChainID: 1, Swapper: testSwapper, TokenIn: testTokenIn, TokenOut: testTokenOut,
		Points: &points,
	}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(body)
	for _, field := range []string{`"protocol":"v2"`, `"phase":"DISCOVERY"`, `"requestId"`, `"quoteId"`,
		`"chainId"`, `"swapper"`, `"tokenIn"`, `"tokenOut"`, `"points"`, `"amountIn"`, `"amountOut"`,
		`"liquidityDomains"`} {
		if !strings.Contains(got, field) {
			t.Errorf("JSON missing %s: %s", field, got)
		}
	}
	for _, absent := range []string{`"solverQuoteId"`, `"buildId"`, `"router"`, `"calls"`, `"validUntil"`} {
		if strings.Contains(got, absent) {
			t.Errorf("discovery JSON contains %s: %s", absent, got)
		}
	}
	for _, value := range []string{testSwapper, testTokenIn, testTokenOut, testVault} {
		if !strings.Contains(got, value) {
			t.Errorf("JSON missing normalized value %s", value)
		}
	}
}

func TestSwapCallResponseJSONOmitsRouterAuthorizationFields(t *testing.T) {
	call := swapCallResponse{
		To: testAdapter, Data: "0x9a4568b6", AmountIn: "10", AmountOut: "19", TokenOut: testTokenOut,
		LiquidityDomain: "capacity:1:" + testVault + ":" + testTokenOut, ValidUntil: 2_000_000_000,
	}
	body, err := json.Marshal(call)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"to"`, `"data"`, `"amountIn"`, `"amountOut"`, `"tokenOut"`,
		`"liquidityDomain"`, `"validUntil"`} {
		if !strings.Contains(string(body), field) {
			t.Fatalf("call JSON missing %s: %s", field, body)
		}
	}
	for _, field := range []string{`"authSigner"`, `"authDeadline"`, `"authSignature"`} {
		if strings.Contains(string(body), field) {
			t.Fatalf("call JSON contains removed Router authorization field %s: %s", field, body)
		}
	}
}

func baseSwapRequest(phase swapPhase, requestID string) swapRequest {
	return swapRequest{
		Protocol: swapProtocolV2, Phase: phase, RequestID: requestID, QuoteID: testQuoteID, ChainID: 1,
		Swapper: testSwapper, TokenIn: testTokenIn, TokenOut: testTokenOut,
	}
}

func validConfirmRequest() swapRequest {
	r := baseSwapRequest(swapPhaseConfirm, testConfirmRequestID)
	r.DiscoveryRequestID, r.AmountIn, r.MinAmountOut = ptr(testDiscoveryRequestID), ptr("10"), ptr("19")
	r.Deadline = int64Ptr(2_000_000_000)
	return r
}

func validBuildRequest() swapRequest {
	r := baseSwapRequest(swapPhaseBuild, testBuildRequestID)
	r.SolverQuoteID, r.BuildID = ptr(testSolverQuoteID), ptr(testBuildID)
	r.AmountIn, r.MinAmountOut, r.Deadline = ptr("10"), ptr("19"), int64Ptr(2_000_000_000)
	r.LiquidityDomains = []string{"capacity:1:" + testVault + ":" + testTokenOut}
	r.Router = ptr(testRouter)
	return r
}

func ptr(value string) *string { return &value }

func int64Ptr(value int64) *int64 { return &value }
