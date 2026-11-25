package iris

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func validRequest() quoteRequest {
	return quoteRequest{ //nolint:gosec // G101 false positive: public mainnet token addresses, not credentials.
		RequestID: "0199a3f1-8c00-7000-8000-000000000000", QuoteID: "0199a3f1-8c00-7000-8000-000000000001", ChainID: 1,
		Borrower: "0xaaccE4EC50328120eF3899a978E87FcA6B8bBE84", Receiver: "0xaaccE4EC50328120eF3899a978E87FcA6B8bBE84",
		CollateralToken: "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", Collateral: "1000000000000000000",
		DebtToken: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Debt: "1000000000", MaxFixedRate: "81000000000000000",
		Duration: "2592000", VenueBitmap: "3",
	}
}

func TestParseRequest(t *testing.T) {
	r, err := parseRequest(validRequest(), big.NewInt(1), false)
	must(t, err)
	equal(t, common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"), r.DebtToken)
	equal(t, big.NewInt(3), r.VenueBitmap)
	equal(t, common.HexToAddress("0xaaccE4EC50328120eF3899a978E87FcA6B8bBE84"), r.Borrower)
	equal(t, r.Borrower, r.Receiver)

	tests := []struct {
		name   string
		mutate func(*quoteRequest)
	}{
		{"other chain", func(q *quoteRequest) { q.ChainID = 5 }},
		{"no request id", func(q *quoteRequest) { q.RequestID = "" }},
		{"no borrower", func(q *quoteRequest) { q.Borrower = "" }},
		{"no receiver", func(q *quoteRequest) { q.Receiver = "" }},
		{"zero debt", func(q *quoteRequest) { q.Debt = "0" }},
		{"bad bitmap", func(q *quoteRequest) { q.VenueBitmap = "x" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := validRequest()
			tt.mutate(&body)
			_, err := parseRequest(body, big.NewInt(1), false)
			fails(t, err)
		})
	}
	indicative := validRequest()
	indicative.Borrower, indicative.Receiver = "", ""
	_, err = parseRequest(indicative, big.NewInt(1), true)
	must(t, err)
}

func TestQuoteResponseShapes(t *testing.T) {
	m := Market{Account: common.HexToAddress("0xACC"), Blm: common.HexToAddress("0x702"), OverdueRate: big.NewInt(1), OverduePeriod: big.NewInt(2), BondLltv: big.NewInt(3), VenueID: 1, Data: []byte{1}}
	q := testQuote(m)
	r := request{RequestID: "r", QuoteID: "q"}

	firm := newQuoteResponse(r, m, q, false)
	equal(t, q.Deadline.String(), firm.Deadline)
	equal(t, 2+2*placeholderSignatureBytes, len(firm.Signature))
	ok(t, !firm.Indicative)

	indicative := newQuoteResponse(r, m, q, true)
	encoded, err := json.Marshal(indicative)
	must(t, err)
	for _, field := range []string{"deadline", "nonce", "signature"} {
		ok(t, !strings.Contains(string(encoded), `"`+field+`"`))
	}
	ok(t, strings.Contains(string(encoded), `"indicative":true`), string(encoded))
}

func TestHandlerAuthAndDeclines(t *testing.T) {
	s := &Solver{cfg: Config{Server: ServerConfig{AuthHeader: "x-api-key"}, Timing: Timing{ResponseBudget: defaultResponseBudget}}, authValue: "secret", chainID: big.NewInt(1)}
	body, err := json.Marshal(validRequest())
	must(t, err)
	post := func(key string, payload []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quote", bytes.NewReader(payload))
		if key != "" {
			req.Header.Set("x-api-key", key)
		}
		rec := httptest.NewRecorder()
		s.handle(false)(rec, req)
		return rec
	}

	equal(t, http.StatusUnauthorized, post("", body).Code)
	equal(t, http.StatusUnauthorized, post("wrong", body).Code)
	equal(t, http.StatusBadRequest, post("secret", []byte("{")).Code)

	// No markets discovered yet: a reasoned decline, never a quote.
	rec := post("secret", body)
	equal(t, http.StatusNotFound, rec.Code)
	ok(t, strings.Contains(rec.Body.String(), `"reason":"markets_unavailable"`), rec.Body.String())

	index := newMarketIndex(nil)
	s.markets.Store(&index)
	rec = post("secret", body)
	equal(t, http.StatusNotFound, rec.Code)
	ok(t, strings.Contains(rec.Body.String(), `"reason":"no_market"`), rec.Body.String())
}
