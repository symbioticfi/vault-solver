package threefapi

import (
	"context"
	"encoding/json"
	"errors"
	"math"
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

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("TEST_3F_KEY", "secret-one")
	c, err := New(Config{URL: server.URL, APIKeyEnv: "TEST_3F_KEY", Broker: testBroker}) //nolint:gosec // APIKeyEnv names an environment variable, never a credential.
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
		if err := ValidateID(id); err == nil {
			t.Errorf("invalid ID %v accepted", id)
		}
	}
	if err := ValidateID((1 << 24) - 1); err != nil {
		t.Fatal(err)
	}
}

func TestConfigAndMissingSecret(t *testing.T) {
	for _, tc := range []Config{
		{URL: "https://user:secret@example.org", APIKeyEnv: "KEY", Broker: testBroker}, //nolint:gosec // Rejected fixture credentials test URL validation; no real credential.
		{URL: "https://example.org?secret=value", APIKeyEnv: "KEY", Broker: testBroker},
		{URL: "https://example.org", APIKeyEnv: "", Broker: testBroker},
		{URL: "https://example.org", APIKeyEnv: "KEY"},
	} {
		if err := Validate(tc); err == nil {
			t.Error("invalid config accepted")
		}
	}
	c := newTestClient(t, func(http.ResponseWriter, *http.Request) { t.Error("missing key sent") })
	t.Setenv("TEST_3F_KEY", "")
	if _, err := c.Offers(context.Background()); err == nil {
		t.Fatal("missing secret accepted")
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
			exact, exists := rows[0].AdditionalProperties[ExactMaxRateKey]
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
	for _, body := range []string{
		"null", "[] []",
		"[" + strings.Replace(offerJSON("SUBMITTED"), `"id":12`, `"id":12,"id":13`, 1) + "]",
		"[" + strings.Replace(offerJSON("SUBMITTED"), `"id":12`, `"id":12.00000001`, 1) + "]",
		"[" + strings.Replace(offerJSON("SUBMITTED"), `"id":12`, `"id":16777216`, 1) + "]",
		"[" + strings.Replace(offerJSON("SUBMITTED"), `"id":12`, `"id":"12"`, 1) + "]",
		"[" + strings.Replace(offerJSON("SUBMITTED"), `"pendingAmount":"0",`, "", 1) + "]",
		"[" + strings.Replace(offerJSON("SUBMITTED"), `"mintDeadline":null,`, "", 1) + "]",
		"[" + offerJSON("NEW_STATUS") + "]",
	} {
		t.Run(body, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			})
			if _, err := c.Offers(t.Context()); err == nil {
				t.Fatal("ambiguous or incomplete offers accepted")
			}
		})
	}
	for _, body := range []string{
		"null",
		`[{"id":34,"requestId":"` + testAccount.Hex() + `","amountRequested":"1","status":"ACTIVE"}]`,
		`[{"id":34,"requestId":"` + testAccount.Hex() + `","amountRequested":"1","status":"ACTIVE","maxRate":1.1,"maxRate":1.2}]`,
	} {
		t.Run(body, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			})
			if _, err := c.Auctions(t.Context()); err == nil {
				t.Fatal("ambiguous or incomplete auctions accepted")
			}
		})
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
