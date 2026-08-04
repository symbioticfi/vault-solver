package rfq

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
)

const testSecret = "s3cr3t"

type fakeSwapHandler struct {
	response *swapResponse
	err      error
	calls    int
	phases   []swapPhase
}

func (f *fakeSwapHandler) swap(_ context.Context, request *swapRequest) (*swapResponse, error) {
	f.calls++
	f.phases = append(f.phases, request.Phase)
	return f.response, f.err
}

func testServer() *server {
	execAddr := common.HexToAddress("0x0000000000000000000000000000000000000010")
	clk := func() time.Time { return time.Unix(0, 0) }
	q := &quoteService{
		chainID:  1,
		executor: execAddr,
		reader:   &fakeQuoteCandidateReader{out: map[common.Address]*big.Int{tOut: big.NewInt(1_000000)}},
		strategy: newDefaultTestStrategy(),
		log:      logr.Discard(),
		now:      clk,
	}
	return &server{sharedSecret: testSecret, quotes: q, log: logr.Discard()}
}

func validQuoteBody() quoteRequest {
	return quoteRequest{
		RequestID: "11111111-1111-4111-8111-111111111111", TokenInChainID: 1, TokenOutChainID: 1,
		Swapper: "0x0000000000000000000000000000000000000099",
		TokenIn: tIn.Hex(), TokenOut: tOut.Hex(),
		Amount: "1000000000000000000", Type: "EXACT_INPUT", Protocol: "v1", NumOutputs: 1,
		QuoteID: "22222222-2222-4222-8222-222222222222",
		Adapters: []quoteAdapter{{
			Adapter: vlt.Hex(), Asset: tOut.Hex(), AssetDecimals: 6,
			MaxAssets: "10000000", MaxRate: "1000000000000000000",
		}},
	}
}

func validSwapDiscoveryBody() swapRequest {
	return swapRequest{
		Protocol:        swapProtocolV2,
		Phase:           swapPhaseDiscovery,
		RequestID:       "33333333-3333-4333-8333-333333333333",
		QuoteID:         "44444444-4444-4444-8444-444444444444",
		ChainID:         1,
		Swapper:         "0x0000000000000000000000000000000000000099",
		TokenIn:         tIn.Hex(),
		TokenOut:        tOut.Hex(),
		SampleAmountsIn: []string{"1000000000000000000"},
		Adapters: []quoteAdapter{{
			Adapter: vlt.Hex(), Asset: tOut.Hex(), AssetDecimals: 6,
			MaxAssets: "10000000", MaxRate: "1000000000000000000",
		}},
	}
}

func do(t *testing.T, h http.Handler, method, path, secret string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, r)
	if secret != "" {
		req.Header.Set(sharedSecretHeader, secret)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func doRaw(t *testing.T, h http.Handler, method, path, secret, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	if secret != "" {
		req.Header.Set(sharedSecretHeader, secret)
	}
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestServer_Health(t *testing.T) {
	rr := do(t, testServer().handler(), http.MethodGet, "/health", "", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("health = %d, want 200", rr.Code)
	}
}

func TestServer_QuoteAuth(t *testing.T) {
	h := testServer().handler()
	if rr := do(t, h, http.MethodPost, "/quote", "", validQuoteBody()); rr.Code != http.StatusForbidden {
		t.Fatalf("no secret = %d, want 403", rr.Code)
	}
	if rr := do(t, h, http.MethodPost, "/quote", "wrong", validQuoteBody()); rr.Code != http.StatusForbidden {
		t.Fatalf("wrong secret = %d, want 403", rr.Code)
	}
}

func TestServer_QuoteOK(t *testing.T) {
	rr := do(t, testServer().handler(), http.MethodPost, "/quote", testSecret, validQuoteBody())
	if rr.Code != http.StatusOK {
		t.Fatalf("quote = %d, want 200 (body %s)", rr.Code, rr.Body.String())
	}
	var resp quoteResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AmountOut != "1000000" { // 1.0 USDC oracle, no quote discount
		t.Fatalf("amountOut = %s, want 1000000", resp.AmountOut)
	}
	if resp.Filler != "0x0000000000000000000000000000000000000010" {
		t.Fatalf("filler = %s, want lowercased executor", resp.Filler)
	}
}

func TestServer_QuoteSchemaValidation(t *testing.T) {
	// Huma validates the request against the struct tags and returns 422 (RFC 9457) on a schema violation.
	h := testServer().handler()
	cases := map[string]func(*quoteRequest){
		"bad protocol":    func(q *quoteRequest) { q.Protocol = "v2" },
		"bad type":        func(q *quoteRequest) { q.Type = "EXACT_OUTPUT" },
		"zero outputs":    func(q *quoteRequest) { q.NumOutputs = 0 },
		"bad swapper":     func(q *quoteRequest) { q.Swapper = "nope" },
		"bad amount":      func(q *quoteRequest) { q.Amount = "abc" },
		"bad adapter dec": func(q *quoteRequest) { q.Adapters[0].AssetDecimals = 999 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			body := validQuoteBody()
			mutate(&body)
			if rr := do(t, h, http.MethodPost, "/quote", testSecret, body); rr.Code != http.StatusUnprocessableEntity {
				t.Fatalf("%s: code = %d, want 422", name, rr.Code)
			}
		})
	}
}

func TestServer_ServesOpenAPISpec(t *testing.T) {
	rr := do(t, testServer().handler(), http.MethodGet, "/openapi.json", "", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("/openapi.json = %d, want 200", rr.Code)
	}
	if body := rr.Body.String(); !strings.Contains(body, "\"openapi\"") || !strings.Contains(body, "/quote") {
		t.Fatalf("/openapi.json missing expected content")
	}
}

func TestServer_SwapRouteIsConditional(t *testing.T) {
	disabled := testServer()
	if rr := do(t, disabled.handler(), http.MethodPost, "/swap", testSecret, validSwapDiscoveryBody()); rr.Code != http.StatusNotFound {
		t.Fatalf("disabled /swap = %d, want 404", rr.Code)
	}
	if rr := do(t, disabled.handler(), http.MethodGet, "/openapi.json", "", nil); strings.Contains(rr.Body.String(), `"/swap"`) {
		t.Fatal("disabled OpenAPI unexpectedly advertises /swap")
	}

	enabled := testServer()
	enabled.swaps = &fakeSwapHandler{response: &swapResponse{
		Protocol: swapProtocolV2, Phase: swapPhaseDiscovery,
		RequestID: "33333333-3333-4333-8333-333333333333",
		QuoteID:   "44444444-4444-4444-8444-444444444444",
		ChainID:   1,
		Swapper:   "0x0000000000000000000000000000000000000099",
		TokenIn:   tIn.Hex(), TokenOut: tOut.Hex(),
		Points: &[]swapPointResponse{},
	}}
	if rr := do(t, enabled.handler(), http.MethodGet, "/openapi.json", "", nil); !strings.Contains(rr.Body.String(), `"/swap"`) {
		t.Fatal("enabled OpenAPI does not advertise /swap")
	}
}

func TestServer_SwapAuthAndSuccess(t *testing.T) {
	srv := testServer()
	handler := &fakeSwapHandler{response: &swapResponse{
		Protocol: swapProtocolV2, Phase: swapPhaseDiscovery,
		RequestID: "33333333-3333-4333-8333-333333333333",
		QuoteID:   "44444444-4444-4444-8444-444444444444",
		ChainID:   1,
		Swapper:   "0x0000000000000000000000000000000000000099",
		TokenIn:   tIn.Hex(), TokenOut: tOut.Hex(),
		Points: &[]swapPointResponse{},
	}}
	srv.swaps = handler
	h := srv.handler()

	for name, secret := range map[string]string{"missing": "", "wrong": "wrong"} {
		t.Run(name, func(t *testing.T) {
			if rr := do(t, h, http.MethodPost, "/swap", secret, validSwapDiscoveryBody()); rr.Code != http.StatusForbidden {
				t.Fatalf("/swap = %d, want 403 (body %s)", rr.Code, rr.Body.String())
			}
		})
	}
	if handler.calls != 0 {
		t.Fatalf("unauthorized requests reached swap service %d times", handler.calls)
	}

	rr := do(t, h, http.MethodPost, "/swap", testSecret, validSwapDiscoveryBody())
	if rr.Code != http.StatusOK {
		t.Fatalf("/swap = %d, want 200 (body %s)", rr.Code, rr.Body.String())
	}
	var response swapResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode /swap response: %v", err)
	}
	if response.Points == nil || len(*response.Points) != 0 {
		t.Fatalf("points = %#v, want explicit empty array", response.Points)
	}
	if handler.calls != 1 {
		t.Fatalf("authorized calls = %d, want 1", handler.calls)
	}
}

func TestServer_SwapRoutesAllThreePhases(t *testing.T) {
	srv := testServer()
	handler := &fakeSwapHandler{response: &swapResponse{
		Protocol: swapProtocolV2, RequestID: "33333333-3333-4333-8333-333333333333",
		QuoteID: "44444444-4444-4444-8444-444444444444", ChainID: 1,
		Swapper: "0x0000000000000000000000000000000000000099", TokenIn: tIn.Hex(), TokenOut: tOut.Hex(),
	}}
	srv.swaps = handler
	h := srv.handler()

	discovery := validSwapDiscoveryBody()
	discoveryID := discovery.RequestID
	amountIn, minAmountOut, deadline := discovery.SampleAmountsIn[0], "1000000", int64(2_000_000_000)
	confirm := discovery
	confirm.Phase = swapPhaseConfirm
	confirm.DiscoveryRequestID = &discoveryID
	confirm.SampleAmountsIn = nil
	confirm.AmountIn = &amountIn
	confirm.MinAmountOut = &minAmountOut
	confirm.Deadline = &deadline

	solverQuoteID := "55555555-5555-4555-8555-555555555555"
	buildID := "66666666-6666-4666-8666-666666666666"
	router := "0x0000000000000000000000000000000000000077"
	build := confirm
	build.Phase = swapPhaseBuild
	build.DiscoveryRequestID = nil
	build.SolverQuoteID = &solverQuoteID
	build.BuildID = &buildID
	build.Adapters = nil
	build.Router = &router
	build.LiquidityDomains = []string{"capacity:1:0x0000000000000000000000000000000000000042:0x0000000000000000000000000000000000000043"}

	for _, body := range []swapRequest{discovery, confirm, build} {
		if rr := do(t, h, http.MethodPost, "/swap", testSecret, body); rr.Code != http.StatusOK {
			t.Fatalf("%s /swap = %d, want 200 (body %s)", body.Phase, rr.Code, rr.Body.String())
		}
	}
	want := []swapPhase{swapPhaseDiscovery, swapPhaseConfirm, swapPhaseBuild}
	if len(handler.phases) != len(want) {
		t.Fatalf("routed phases = %v, want %v", handler.phases, want)
	}
	for i := range want {
		if handler.phases[i] != want[i] {
			t.Fatalf("routed phases = %v, want %v", handler.phases, want)
		}
	}
}

func TestServer_SwapServiceOutcomes(t *testing.T) {
	for name, tc := range map[string]struct {
		err      error
		wantCode int
	}{
		"no content":  {err: errSwapNoContent, wantCode: http.StatusNoContent},
		"bad request": {err: &swapServiceError{status: http.StatusBadRequest, message: "invalid swap request"}, wantCode: http.StatusBadRequest},
		"not found":   {err: &swapServiceError{status: http.StatusNotFound, message: "swap record not found"}, wantCode: http.StatusNotFound},
		"gone":        {err: &swapServiceError{status: http.StatusGone, message: "swap quote expired"}, wantCode: http.StatusGone},
		"conflict":    {err: &swapServiceError{status: http.StatusConflict, message: "swap state changed"}, wantCode: http.StatusConflict},
		"store full":  {err: &swapServiceError{status: http.StatusTooManyRequests, message: "swap record store is full"}, wantCode: http.StatusTooManyRequests},
		"dependency":  {err: &swapServiceError{status: http.StatusBadGateway, message: "swap build failed"}, wantCode: http.StatusBadGateway},
	} {
		t.Run(name, func(t *testing.T) {
			srv := testServer()
			srv.swaps = &fakeSwapHandler{err: tc.err}
			rr := do(t, srv.handler(), http.MethodPost, "/swap", testSecret, validSwapDiscoveryBody())
			if rr.Code != tc.wantCode {
				t.Fatalf("/swap = %d, want %d (body %s)", rr.Code, tc.wantCode, rr.Body.String())
			}
			if tc.wantCode == http.StatusNoContent && rr.Body.Len() != 0 {
				t.Fatalf("204 body = %q, want empty", rr.Body.String())
			}
		})
	}
}

func TestServer_SwapRejectsOversizeAndMalformedBodies(t *testing.T) {
	srv := testServer()
	srv.swaps = &fakeSwapHandler{}
	h := srv.handler()

	if rr := doRaw(t, h, http.MethodPost, "/swap", testSecret, `{`); rr.Code != http.StatusBadRequest {
		t.Fatalf("malformed /swap = %d, want 400 (body %s)", rr.Code, rr.Body.String())
	}
	oversize := `{"protocol":"v2","phase":"DISCOVERY","padding":"` + strings.Repeat("x", maxRequestBytes) + `"}`
	if rr := doRaw(t, h, http.MethodPost, "/swap", testSecret, oversize); rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize /swap = %d, want 413 (body %s)", rr.Code, rr.Body.String())
	}
}

func TestServer_SwapOpenAPIPinsWireNames(t *testing.T) {
	srv := testServer()
	srv.swaps = &fakeSwapHandler{}
	rr := do(t, srv.handler(), http.MethodGet, "/openapi.json", "", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("/openapi.json = %d, want 200", rr.Code)
	}
	spec := rr.Body.String()
	for _, field := range []string{
		"protocol", "phase", "requestId", "discoveryRequestId", "quoteId", "solverQuoteId", "buildId",
		"chainId", "swapper", "router", "tokenIn", "tokenOut", "sampleAmountsIn", "adapters", "points",
		"amountIn", "minAmountOut", "amountOut", "liquidityDomains", "validUntil", "calls", "liquidityDomain",
	} {
		if !strings.Contains(spec, `"`+field+`"`) {
			t.Fatalf("OpenAPI missing wire field %q", field)
		}
	}
	for _, forbidden := range []string{
		`"validity"`, `"nativeValue"`, `"authSigner"`, `"authDeadline"`, `"authSignature"`,
	} {
		if strings.Contains(spec, forbidden) {
			t.Fatalf("OpenAPI contains forbidden wire field %s", forbidden)
		}
	}
}

func TestServer_QuoteWrongChainNoContent(t *testing.T) {
	body := validQuoteBody()
	body.TokenInChainID = 2 // not our chain
	rr := do(t, testServer().handler(), http.MethodPost, "/quote", testSecret, body)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("wrong-chain quote = %d, want 204", rr.Code)
	}
}

func TestServer_QuoteBelowMinAmountNoContent(t *testing.T) {
	srv := testServer()
	body := validQuoteBody() // 1e18 of tIn
	srv.quotes.minAmountsIn = map[common.Address]*big.Int{
		tIn: mustBig(t, "2000000000000000000"),
	}
	if rr := do(t, srv.handler(), http.MethodPost, "/quote", testSecret, body); rr.Code != http.StatusNoContent {
		t.Fatalf("below-minimum quote = %d, want 204 (body %s)", rr.Code, rr.Body.String())
	}

	// The same request at exactly the minimum is still quoted.
	srv.quotes.minAmountsIn = map[common.Address]*big.Int{tIn: mustBig(t, body.Amount)}
	if rr := do(t, srv.handler(), http.MethodPost, "/quote", testSecret, body); rr.Code != http.StatusOK {
		t.Fatalf("at-minimum quote = %d, want 200 (body %s)", rr.Code, rr.Body.String())
	}
}

func TestServer_QuoteWhitelist(t *testing.T) {
	rogue := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	rogueAdapter := quoteAdapter{
		Adapter: rogue.Hex(), Asset: tOut.Hex(), AssetDecimals: 6,
		MaxAssets: "10000000", MaxRate: "2000000000000000000",
	}
	cases := map[string]struct {
		whitelist adapterWhitelist
		adapters  []quoteAdapter // nil keeps validQuoteBody's single vlt adapter
		wantCode  int
	}{
		"drops non-whitelisted adapters": {
			whitelist: buildAdapterWhitelist(true, []recoveryVault{{Adapter: vlt}}),
			adapters:  append(validQuoteBody().Adapters, rogueAdapter),
			wantCode:  http.StatusOK,
		},
		"no whitelisted adapter declines": {
			whitelist: buildAdapterWhitelist(true, []recoveryVault{{Adapter: rogue}}),
			wantCode:  http.StatusNoContent,
		},
		"enabled with no vaults declines everything": { // fail closed
			whitelist: buildAdapterWhitelist(true, nil),
			wantCode:  http.StatusNoContent,
		},
		"disabled keeps all adapters": {
			whitelist: buildAdapterWhitelist(false, []recoveryVault{{Adapter: vlt}}),
			adapters:  []quoteAdapter{rogueAdapter}, // only a non-configured adapter: still quoted
			wantCode:  http.StatusOK,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := testServer()
			srv.quotes.whitelist = tc.whitelist
			body := validQuoteBody()
			if tc.adapters != nil {
				body.Adapters = tc.adapters
			}

			rr := do(t, srv.handler(), http.MethodPost, "/quote", testSecret, body)
			if rr.Code != tc.wantCode {
				t.Fatalf("quote = %d, want %d (body %s)", rr.Code, tc.wantCode, rr.Body.String())
			}

			if tc.wantCode == http.StatusNoContent {
				return
			}
			var resp quoteResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode quote response: %v", err)
			}
			if resp.AmountOut == "" {
				t.Fatalf("quote response missing amountOut: %+v", resp)
			}
		})
	}
}
