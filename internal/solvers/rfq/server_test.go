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

// fakeReader stands in for the on-chain pricing reads in HTTP tests.
type fakeReader struct {
	decimals int
	oracle   map[common.Address]*big.Int
}

func (f fakeReader) tokenDecimals(context.Context, common.Address) (int, error) {
	return f.decimals, nil
}

func (f fakeReader) amountsOut(
	_ context.Context, _ common.Address, assets []common.Address, _ *big.Int,
) (map[common.Address]*big.Int, error) {
	out := make(map[common.Address]*big.Int)
	for _, a := range assets {
		if v, ok := f.oracle[a]; ok {
			out[a] = v
		}
	}
	return out, nil
}

const testSecret = "s3cr3t"

func testServer() *server {
	execAddr := common.HexToAddress("0x0000000000000000000000000000000000000010")
	clk := func() time.Time { return time.Unix(0, 0) }
	st := newStore(clk)
	q := &quoteService{
		chainID:     1,
		executor:    execAddr,
		discountBps: 1000,
		reader:      fakeReader{decimals: 18, oracle: map[common.Address]*big.Int{tOut: big.NewInt(1_000000)}},
		store:       st,
		log:         logr.Discard(),
		now:         clk,
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
	if resp.AmountOut != "900000" { // 1.0 USDC oracle − 10% discount
		t.Fatalf("amountOut = %s, want 900000", resp.AmountOut)
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
