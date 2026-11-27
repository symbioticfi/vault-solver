package threef

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/api/threef"
)

var (
	testBroker  = common.HexToAddress("0x1111111111111111111111111111111111111111")
	testAccount = common.HexToAddress("0x2222222222222222222222222222222222222222")
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *apiClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("TEST_3F_KEY", "secret-one")
	c, err := newClient(apiConfig{URL: server.URL, APIKeyEnv: "TEST_3F_KEY", Broker: testBroker}) //nolint:gosec // APIKeyEnv names an environment variable, never a credential.
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func offerJSON(status string) string {
	return `{"id":12,"auctionId":34,"status":"` + status + `","executionMode":"PUSH","mintDeadline":null,"maker":"` + testAccount.Hex() + `","requestId":"0x3333333333333333333333333333333333333333","amount":"100","expectedReturn":"9","filledAmount":"0","filledExpectedReturn":"0","pendingAmount":"0","pendingExpectedReturn":"0","nonce":"1","expiration":"4102444800","signature":null}`
}

func TestIdentity(t *testing.T) {
	for _, tc := range []struct {
		name            string
		broker, account string
		enabled         bool
		wantErr         bool
	}{
		{"matching", testBroker.Hex(), testAccount.Hex(), true, false},
		{"wrong broker", testAccount.Hex(), testAccount.Hex(), true, true},
		{"wrong default", testBroker.Hex(), testBroker.Hex(), true, true},
		{"disabled", testBroker.Hex(), testAccount.Hex(), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-api-key") != "secret-one" {
					t.Error("missing API key")
				}
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v1/facilitator/offer-address" {
					_ = json.NewEncoder(w).Encode(map[string]any{"facilitator": tc.broker, "offerAddress": tc.account})
					return
				}
				_ = json.NewEncoder(w).Encode([]map[string]any{{"maker": testAccount.Hex(), "submissionsEnabled": tc.enabled, "isDefault": true}})
			})
			if err := c.Identity(t.Context(), testAccount); (err != nil) != tc.wantErr {
				t.Fatalf("Identity error: %v", err)
			}
		})
	}
}

func TestIdentitySendsSingleAPIKeyHeader(t *testing.T) {
	var calls int
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		// The generated identity methods supply an empty lowercase header.
		values := r.Header.Values("x-api-key")
		if len(values) != 1 || values[0] != "secret-one" {
			t.Error("identity request must send exactly one API credential")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/facilitator/offer-address" {
			_ = json.NewEncoder(w).Encode(map[string]any{"facilitator": testBroker.Hex(), "offerAddress": testAccount.Hex()})
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"maker": testAccount.Hex(), "submissionsEnabled": true, "isDefault": true}})
	})
	if err := c.Identity(t.Context(), testAccount); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("identity requests: %d", calls)
	}
}

func TestUnsignedCreateWireAndPointOfUseKey(t *testing.T) {
	var calls int
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/offer" || r.Header.Get("x-api-key") != "secret-two" {
			t.Error("wrong create request")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		callback, isBool := body["useCallback"].(bool)
		if body["maker"] != strings.ToLower(testBroker.Hex()) || !isBool || callback {
			t.Error("wrong broker/mode")
		}
		for _, name := range []string{"chainId", "signature", "executionMode", "pushAuthorizationSignature", "authorizationDeadline"} {
			if _, ok := body[name]; ok {
				t.Errorf("unexpected signed field %s", name)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":12}`))
	})
	t.Setenv("TEST_3F_KEY", "secret-two")
	dto := *threef.NewCreateOfferDto(34, testBroker.Hex(), "100", "9", "1", "4102444800", false)
	created, err := c.Create(t.Context(), dto)
	if err != nil || created.Id != 12 || calls != 1 {
		t.Fatalf("create: %+v %v calls=%d", created, err, calls)
	}
	dto.ChainId = new(float32(1))
	if _, err = c.Create(t.Context(), dto); err == nil || calls != 1 {
		t.Fatal("signed request reached network")
	}
}

func TestCreateErrorOmitsResponseBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"upstream refused secret-one\nplease retry","credentials":"hidden"}`))
	})
	dto := *threef.NewCreateOfferDto(34, testBroker.Hex(), "100", "9", "1", "4102444800", false)
	_, err := c.Create(t.Context(), dto)
	if err == nil || !strings.Contains(err.Error(), "HTTP status 500") {
		t.Fatalf("missing HTTP status: %v", err)
	}
	if strings.Contains(err.Error(), "upstream refused") || strings.Contains(err.Error(), "secret-one") || strings.Contains(err.Error(), "hidden") {
		t.Fatal("error exposed the response body")
	}
}

func TestCreateRejectsNonceOutsideStorageRange(t *testing.T) {
	for _, tc := range []struct {
		nonce string
		valid bool
	}{
		{"0", true},
		{"1", true},
		{"9223372036854775807", true},
		{"9223372036854775808", false},
		{"18446744073709551616", false},
		{"-1", false},
	} {
		t.Run(tc.nonce, func(t *testing.T) {
			calls := 0
			client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":12}`))
			})
			dto := *threef.NewCreateOfferDto(34, testBroker.Hex(), "100", "9", tc.nonce, "4102444800", false)
			_, err := client.Create(t.Context(), dto)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, got error %v", tc.valid, err)
			}
			if !tc.valid && calls != 0 {
				t.Fatal("unsupported nonce reached the API")
			}
		})
	}
}

func TestOffersAndCancelWire(t *testing.T) {
	var posted bool
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "secret-one" {
			t.Error("missing key")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			if r.URL.Query().Get("maker") != strings.ToLower(testBroker.Hex()) || len(r.URL.Query()) != 1 {
				t.Error("incorrect list auth query")
			}
			if r.URL.Path == "/v1/offer" {
				_, _ = w.Write([]byte("[" + offerJSON("SUBMITTED") + "]"))
			} else {
				_, _ = w.Write([]byte(offerJSON("SUBMITTED")))
			}
			return
		}
		posted = true
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 2 || body["maker"] != strings.ToLower(testBroker.Hex()) || body["offerId"] != float64(12) {
			t.Error("incorrect cancellation")
		}
		_, _ = w.Write([]byte(`{"id":12,"status":"CANCELLED"}`))
	})
	rows, err := c.Offers(t.Context())
	if err != nil || len(rows) != 1 || rows[0].PendingExpectedReturn != "0" {
		t.Fatalf("offers: %+v %v", rows, err)
	}
	if _, err = c.Cancel(t.Context(), 12); err != nil || !posted {
		t.Fatalf("cancel: %v posted=%v", err, posted)
	}
}

func TestCancelAcceptedDoesNotPost(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("accepted cancellation posted")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(offerJSON("AWAITING_MINT")))
	})
	if _, err := c.Cancel(t.Context(), 12); err == nil {
		t.Fatal("accepted cancellation allowed")
	}
}

func TestTransportBoundsAndRedaction(t *testing.T) {
	var redirected bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected = true }))
	t.Cleanup(target.Close)
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"redirect", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
		}},
		{"upstream error", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("secret-one"))
		}},
		{"large body", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("x", int(maxResponseBytes)+1)))
		}},
		{"invalid json", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"secret-one":`)) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, tc.handler)
			_, err := c.Offers(t.Context())
			if err == nil || strings.Contains(err.Error(), "secret-one") || strings.Contains(err.Error(), "http://") {
				t.Fatalf("unsafe error %v", err)
			}
		})
	}
	if redirected {
		t.Fatal("credential redirect followed")
	}
}

func TestExactIDValidation(t *testing.T) {
	for _, id := range []float32{0, -1, 1.5, 1 << 24, float32(math.Inf(1)), float32(math.NaN())} {
		if err := validateID(id); err == nil {
			t.Errorf("invalid ID %v accepted", id)
		}
	}
	if err := validateID((1 << 24) - 1); err != nil {
		t.Fatal(err)
	}
}

func TestConfigAndMissingSecret(t *testing.T) {
	for _, tc := range []apiConfig{
		{URL: "https://user:secret@example.org", APIKeyEnv: "KEY", Broker: testBroker}, //nolint:gosec // Rejected fixture credentials test URL validation; no real credential.
		{URL: "https://example.org?secret=value", APIKeyEnv: "KEY", Broker: testBroker},
		{URL: "https://example.org", APIKeyEnv: "", Broker: testBroker},
		{URL: "https://example.org", APIKeyEnv: "KEY"},
	} {
		if err := validateAPIConfig(tc); err == nil {
			t.Error("invalid config accepted")
		}
	}
	c := newTestClient(t, func(http.ResponseWriter, *http.Request) { t.Error("missing key sent") })
	t.Setenv("TEST_3F_KEY", "")
	if _, err := c.Offers(context.Background()); err == nil || !strings.Contains(err.Error(), "TEST_3F_KEY is missing") {
		t.Fatalf("missing secret accepted or cause dropped: %v", err)
	}
	if _, err := newClient(apiConfig{URL: "https://example.org", APIKeyEnv: "TEST_3F_KEY", Broker: testBroker}); err == nil { //nolint:gosec // APIKeyEnv names an environment variable, never a credential.
		t.Fatal("client started without its API key")
	}
}

func TestAuctionsExactRate(t *testing.T) {
	for _, rate := range []string{"123.1", "0.000000000001", "1.23456789e2", "null"} {
		t.Run(rate, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/auction" || r.URL.Query().Get("scope") != "all" {
					t.Error("incorrect auction request")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[{"id":34,"requestId":"` + testAccount.Hex() + `","amountRequested":"100000000000000000000001","status":"ACTIVE","maxRate":` + rate + `,"exactMaxRate":"forged"}]`))
			})
			rows, err := c.Auctions(t.Context())
			if err != nil || len(rows) != 1 {
				t.Fatalf("auctions: %+v %v", rows, err)
			}
			exact, exists := rows[0].AdditionalProperties[exactMaxRateKey]
			if (rate == "null" && exists) || (rate != "null" && exact != rate) {
				t.Fatalf("incorrect exact rate: %v", exact)
			}
			if amount := rows[0].AmountRequested.Get(); amount == nil || *amount != "100000000000000000000001" {
				t.Fatal("principal lost precision")
			}
		})
	}
}

func TestRejectAmbiguousOrIncompleteJSON(t *testing.T) {
	offer := offerJSON("SUBMITTED")
	for _, tc := range []struct{ body, want string }{
		{"null", "3f offers response is empty"},
		{"[] []", "trailing JSON response data at byte 4"},
		{"[" + strings.Replace(offer, `"id":12`, `"id":12,"id":13`, 1) + "]", `duplicate JSON property "id"`},
		{"[" + strings.Replace(offer, `"id":12`, `"id":12.00000001`, 1) + "]", "JSON id 12.00000001 is not an exact positive integer"},
		{"[" + strings.Replace(offer, `"id":12`, `"id":16777216`, 1) + "]", "JSON id 16777216 is not an exact positive integer"},
		{"[" + strings.Replace(offer, `"id":12`, `"id":"12"`, 1) + "]", "JSON id is a string, not a number"},
		{"[" + strings.Replace(offer, `"pendingAmount":"0",`, "", 1) + "]", `row 0: 3f offer 12 pendingAmount: "" is not a canonical uint256 decimal`},
		{"[" + strings.Replace(offer, `"mintDeadline":null,`, "", 1) + "]", "3f offer 12 has no mintDeadline field"},
		{"[" + offerJSON("NEW_STATUS") + "]", `3f offer 12 has unknown status "NEW_STATUS"`},
		{"[" + strings.Replace(offer, `"amount":"100"`, `"amount":"0100"`, 1) + "]", `3f offer 12 amount: "0100" is not a canonical uint256 decimal`},
	} {
		t.Run(tc.body, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})
			if _, err := c.Offers(t.Context()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Offers err = %v, want %q", err, tc.want)
			}
		})
	}
	for _, tc := range []struct{ body, want string }{
		{"null", "3f auctions response is empty"},
		{`[{"id":34,"requestId":"` + testAccount.Hex() + `","amountRequested":"1","status":"ACTIVE"}]`, "3f auction 34 has no maxRate"},
		{`[{"id":34,"requestId":"` + testAccount.Hex() + `","amountRequested":"1","status":"ACTIVE","maxRate":1.1,"maxRate":1.2}]`, `duplicate JSON property "maxRate"`},
		{`[{"id":34,"requestId":"0x0","amountRequested":"1","status":"ACTIVE","maxRate":1}]`, `3f auction 34: requestId "0x0" is not a nonzero address`},
		{`[{"id":34,"requestId":"` + testAccount.Hex() + `","amountRequested":"1.5","status":"ACTIVE","maxRate":1}]`, `3f auction 34 amountRequested: "1.5" is not a canonical uint256 decimal`},
	} {
		t.Run(tc.body, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})
			if _, err := c.Auctions(t.Context()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Auctions err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidateURLNamesCheckWithoutURL(t *testing.T) {
	for _, tc := range []struct{ url, want string }{
		{"ftp://api.example", "scheme is not http or https"},
		{"https://", "has no host"},
		{"https://user:secret@api.example", "contains credentials"},
		{"https://api.example?key=secret", "contains a query"},
		{"https://api.example#secret", "contains a fragment"},
		{"https://api.example/secret", "has a path"},
	} {
		err := validateURL(tc.url)
		if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "secret") {
			t.Errorf("validateURL(%q) = %v, want %q without the URL", tc.url, err, tc.want)
		}
	}
}

func TestUnsignedOfferProblem(t *testing.T) {
	valid := threef.CreateOfferDto{AuctionId: 34, Maker: strings.ToLower(testBroker.Hex()), Amount: "100", ExpectedReturn: "9", Nonce: "1", Expiration: "4102444800"}
	if problem := unsignedOfferProblem(valid, testBroker); problem != "" {
		t.Fatalf("valid offer rejected: %s", problem)
	}
	for _, tc := range []struct {
		edit func(*threef.CreateOfferDto)
		want string
	}{
		{func(d *threef.CreateOfferDto) { d.Nonce = "-1" }, `nonce "-1" is not a nonnegative int64`},
		{func(d *threef.CreateOfferDto) { d.AuctionId = 0 }, "3f ID 0 must be an exact positive integer below 16777216"},
		{func(d *threef.CreateOfferDto) { d.Maker = "0x1" }, `maker "0x1" is not broker ` + testBroker.Hex()},
		{func(d *threef.CreateOfferDto) { d.Amount = "0" }, `amount "0" is not a positive uint256`},
		{func(d *threef.CreateOfferDto) { d.UseCallback = true }, "useCallback is set"},
	} {
		dto := valid
		tc.edit(&dto)
		if got := unsignedOfferProblem(dto, testBroker); got != tc.want {
			t.Errorf("unsignedOfferProblem = %q, want %q", got, tc.want)
		}
	}
}

func TestEmptyListsAndCancelledContext(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	})
	if rows, err := c.Offers(t.Context()); err != nil || len(rows) != 0 {
		t.Fatalf("empty offers: %v %v", rows, err)
	}
	if rows, err := c.Auctions(t.Context()); err != nil || len(rows) != 0 {
		t.Fatalf("empty auctions: %v %v", rows, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.Offers(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request: %v", err)
	}
}

func TestAPIRejectsOversizedInventory(t *testing.T) {
	for _, route := range []string{"/v1/offer", "/v1/auction"} {
		t.Run(route, func(t *testing.T) {
			row := offerJSON("SUBMITTED")
			if route == "/v1/auction" {
				row = `{"id":34,"requestId":"` + testAccount.Hex() + `","amountRequested":"1","status":"OPEN","maxRate":null}`
			}
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != route {
					t.Error("unexpected inventory route", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte("[" + strings.Repeat(row+",", 4096) + row + "]"))
			})
			var err error
			if route == "/v1/offer" {
				_, err = c.Offers(t.Context())
			} else {
				_, err = c.Auctions(t.Context())
			}
			if err == nil {
				t.Fatal("oversized inventory accepted")
			}
		})
	}
}

func TestAuctionRateBasisUsesExactJSONBeforeFloatRounding(t *testing.T) {
	for _, tc := range []struct {
		basis               string
		yield               int64
		wantFits, wantError bool
	}{
		{"16777217", 16777217, true, false}, {"9223372036854775807", 9223372036854775807, true, false},
		{"3.1536e7", 31536000, true, false},
		{"0", 31536000, true, false}, {"null", 31536000, true, false},
	} {
		t.Run(tc.basis, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[{"id":34,"requestId":"` + testAccount.Hex() + `","amountRequested":"31536000","status":"OPEN","maxRate":10000,"rateBasisSeconds":` + tc.basis + `}]`))
			})
			rows, err := c.Auctions(t.Context())
			if err != nil || len(rows) != 1 {
				t.Fatalf("auctions: %v", err)
			}
			m := testMarket()
			m.MaxPrincipal, m.MinimumYield1e8 = big.NewInt(31536000), new(big.Int)
			mismatch, err := termsMismatch(m, m.MaxPrincipal, big.NewInt(tc.yield), rows[0])
			if fits := mismatch == "" && err == nil; fits != tc.wantFits || (err != nil) != tc.wantError {
				t.Fatalf("fits=%v err=%v", fits, err)
			}
		})
	}
}
