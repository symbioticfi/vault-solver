package discounts

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientResolveSingle(t *testing.T) {
	var gotPath, gotMethod, gotID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		gotID, _ = body["discountId"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"requestId":"00000000-0000-0000-0000-000000000000",` +
			`"discountId":"0x` + hash64 + `",` +
			`"discount":{"adapter":"0x0000000000000000000000000000000000000abc",` +
			`"tokenToRedeem":"0x0000000000000000000000000000000000000def",` +
			`"discount":"123","signer":"0x0000000000000000000000000000000000000aaa",` +
			`"protocol":"0x0000000000000000000000000000000000000bbb","nonce":"2","deadline":1900000000},` +
			`"signerSignature":"0xdead","protocolDeadline":1900000001,"protocolSignature":"0xbeef"}`))
	}))
	defer srv.Close()

	id := "0x" + hash64
	res, err := NewClient(srv.URL).Resolve(context.Background(), id)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if gotPath != "/api-internal/v1/discounts" || gotMethod != http.MethodPost || gotID != id {
		t.Fatalf("request = path %q method %q id %q", gotPath, gotMethod, gotID)
	}
	if res.Discount.Adapter != "0x0000000000000000000000000000000000000abc" ||
		res.Discount.Discount != "123" || res.Discount.Nonce != "2" ||
		res.Discount.Deadline != 1900000000 {
		t.Fatalf("discount terms = %+v", res.Discount)
	}
	if res.SignerSignature != "0xdead" || res.ProtocolSignature != "0xbeef" || res.ProtocolDeadline != 1900000001 {
		t.Fatalf("resolved = %+v", res)
	}
}

func TestClientResolveBatchSingleEntryAccepted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"requestId":"00000000-0000-0000-0000-000000000000",` +
			`"discounts":[{"discountId":"0x` + hash64 + `",` +
			`"discount":{"adapter":"0x0000000000000000000000000000000000000abc",` +
			`"tokenToRedeem":"0x0000000000000000000000000000000000000def",` +
			`"discount":"123","signer":"0x0000000000000000000000000000000000000aaa",` +
			`"protocol":"0x0000000000000000000000000000000000000bbb","nonce":"2","deadline":1900000000},` +
			`"signerSignature":"0xdead","protocolDeadline":1900000001,"protocolSignature":"0xbeef"}]}`))
	}))
	defer srv.Close()

	res, err := NewClient(srv.URL).Resolve(context.Background(), "0x"+hash64)
	if err != nil {
		t.Fatalf("Resolve batch: %v", err)
	}
	if res.Discount.Adapter != "0x0000000000000000000000000000000000000abc" || res.SignerSignature != "0xdead" {
		t.Fatalf("resolved from batch = %+v", res)
	}
}

func TestClientResolveBatchMultipleRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entry := `{"discountId":"0x` + hash64 + `",` +
			`"discount":{"adapter":"0x0000000000000000000000000000000000000abc",` +
			`"tokenToRedeem":"0x0000000000000000000000000000000000000def",` +
			`"discount":"1","signer":"0x0000000000000000000000000000000000000aaa",` +
			`"protocol":"0x0000000000000000000000000000000000000bbb","nonce":"2","deadline":1900000000},` +
			`"signerSignature":"0xdead","protocolDeadline":1900000001,"protocolSignature":"0xbeef"}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"requestId":"00000000-0000-0000-0000-000000000000","discounts":[` +
			entry + `,` + entry + `]}`))
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL).Resolve(context.Background(), "0x"+hash64); err == nil {
		t.Fatalf("expected an error when the backend resolves more than one discount")
	}
}

func TestClientListDiscounts(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"requestId":"00000000-0000-0000-0000-000000000000",` +
			`"protocol":"0x0000000000000000000000000000000000000bbb","discounts":[` +
			`{"discountId":"0x` + hash64 + `","adapter":"0x0000000000000000000000000000000000000abc",` +
			`"tokenToRedeem":"0x0000000000000000000000000000000000000def",` +
			`"collateral":"0x0000000000000000000000000000000000000c01","collateralDecimals":6,` +
			`"discount":"10","signer":"0x0000000000000000000000000000000000000aaa","deadline":1900000000,` +
			`"maxRate":"1000000","maxAssets":"5000"}]}`))
	}))
	defer srv.Close()

	resp, err := NewClient(srv.URL).ListDiscounts(context.Background())
	if err != nil {
		t.Fatalf("ListDiscounts: %v", err)
	}
	if gotPath != "/api-internal/v1/discounts" {
		t.Fatalf("path = %q", gotPath)
	}
	if len(resp.Discounts) != 1 || resp.Discounts[0].CollateralDecimals != 6 ||
		resp.Discounts[0].MaxAssets != "5000" || resp.Discounts[0].Deadline != 1900000000 {
		t.Fatalf("discounts = %+v", resp.Discounts)
	}
}

func TestClientPreservesBaseURLPathPrefix(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(
			`{"requestId":"00000000-0000-0000-0000-000000000000",` +
				`"protocol":"0x0000000000000000000000000000000000000001","discounts":[]}`,
		))
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL + "/backend").ListDiscounts(t.Context()); err != nil {
		t.Fatalf("ListDiscounts: %v", err)
	}
	if gotPath != "/backend/api-internal/v1/discounts" {
		t.Fatalf("path = %q, want prefixed internal path", gotPath)
	}
}

const hash64 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
