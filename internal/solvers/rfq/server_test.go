package rfq

import (
	"bytes"
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

func testServer() *server {
	execAddr := common.HexToAddress("0x0000000000000000000000000000000000000010")
	clk := func() time.Time { return time.Unix(0, 0) }
	q := &quoteService{
		chainID:       1,
		executor:      execAddr,
		laneAvailable: func() bool { return true },
		reader:        &fakeQuoteCandidateReader{out: map[common.Address]*big.Int{tOut: big.NewInt(1_000000)}},
		strategy:      newDefaultTestStrategy(),
		log:           logr.Discard(),
		now:           clk,
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
