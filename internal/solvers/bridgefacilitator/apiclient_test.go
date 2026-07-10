package bridgefacilitator

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/httptransport"
)

const generatedExactID = 9_007_199_254_740_993

const generatedAuctionFixture = `{
  "id": 9007199254740993,
  "requestId": "0x0000000000000000000000000000000000000010",
  "amountRequested": "1000000000",
  "solve_start_time": null,
  "maxRate": 50.5,
  "status": "open",
  "asset": null,
  "depositAsset": null,
  "vault": null,
  "settlement": null,
  "direction": null,
  "eip712Domain": {
    "name": "SuperstateRequest",
    "version": "1",
    "chainId": 9007199254740993,
    "salt": "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  }
}`

func TestGeneratedAuctionExactFields(t *testing.T) {
	var auction threef.AuctionDto
	if err := json.Unmarshal([]byte(generatedAuctionFixture), &auction); err != nil {
		t.Fatalf("unmarshal generated auction: %v", err)
	}
	if got := auction.Id; got != generatedExactID {
		t.Errorf("auction id = %d, want %d", got, generatedExactID)
	}
	if got := reflect.TypeOf(auction.Id).Kind(); got != reflect.Int64 {
		t.Errorf("auction id kind = %s, want int64", got)
	}
	domain, ok := auction.GetEip712DomainOk()
	if !ok || domain == nil {
		t.Fatal("generated auction omitted EIP-712 domain")
	}
	if got := domain.GetChainId(); got != generatedExactID {
		t.Errorf("domain chain id = %d, want %d", got, generatedExactID)
	}
	if got := reflect.TypeOf(domain.GetChainId()).Kind(); got != reflect.Int64 {
		t.Errorf("domain chain id kind = %s, want int64", got)
	}
	rate, ok := auction.GetMaxRateOk()
	if !ok || rate == nil {
		t.Fatal("generated auction omitted max rate")
	}
	if got := *rate; got != 50.5 {
		t.Errorf("max rate = %v, want 50.5", got)
	}
	if got := reflect.TypeOf(*rate).Kind(); got != reflect.Float64 {
		t.Errorf("max rate kind = %s, want float64", got)
	}
	raw, err := json.Marshal(domain)
	if err != nil {
		t.Fatalf("marshal generated EIP-712 domain: %v", err)
	}
	const wantSalt = `"salt":"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`
	if !strings.Contains(string(raw), wantSalt) {
		t.Fatalf("generated EIP-712 domain = %s, want retained salt", raw)
	}
}

func TestGeneratedRequestIDs(t *testing.T) {
	type capturedRequest struct {
		path    string
		chainID string
	}
	captured := make(chan capturedRequest, 3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured <- capturedRequest{path: r.URL.Path, chainID: r.URL.Query().Get("chainId")}
		http.Error(w, "request captured", http.StatusTeapot)
	}))
	defer srv.Close()

	cfg := threef.NewConfiguration()
	cfg.Servers = threef.ServerConfigurations{{URL: srv.URL}}
	client := threef.NewAPIClient(cfg)

	_, resp, _ := client.OfferAPI.OfferControllerGetV1(t.Context()).
		Maker("0x0000000000000000000000000000000000000042").
		ChainId(generatedExactID).Execute()
	closeResp(resp)
	_, resp, _ = client.OfferAPI.OfferControllerGetByIdV1(t.Context(), generatedExactID).
		Maker("0x0000000000000000000000000000000000000042").
		ChainId(generatedExactID).Execute()
	closeResp(resp)
	_, resp, _ = client.AuctionAPI.AuctionControllerGetByIdV1(t.Context(), generatedExactID).Execute()
	closeResp(resp)

	want := []capturedRequest{
		{path: "/v1/offer", chainID: "9007199254740993"},
		{path: "/v1/offer/9007199254740993", chainID: "9007199254740993"},
		{path: "/v1/auction/9007199254740993"},
	}
	for i, expected := range want {
		if got := <-captured; got != expected {
			t.Errorf("request %d = %+v, want %+v", i, got, expected)
		}
	}
}

// fakeSigner is a minimal signer.Signer test double that signs nothing meaningful (65 zero bytes).
type fakeSigner struct{}

func (fakeSigner) Address() common.Address { return common.Address{} }
func (fakeSigner) SignHash(_ common.Hash) ([]byte, error) {
	return make([]byte, 65), nil
}
func (fakeSigner) SignTx(tx *types.Transaction, _ *big.Int) (*types.Transaction, error) {
	return tx, nil
}

func TestAPIClient_ListOffers_SignedPerAdapter(t *testing.T) {
	var gotMaker, gotAuth, gotKey, gotDeadline, gotChainID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMaker = r.URL.Query().Get("maker")
		gotDeadline = r.URL.Query().Get("deadline")
		gotChainID = r.URL.Query().Get("chainId")
		gotAuth = r.Header.Get("Authorization")
		gotKey = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	adapter := common.HexToAddress("0x0000000000000000000000000000000000000042")
	ac := newAPIClient(srv.URL, fakeSigner{}, big.NewInt(11155111), 5*time.Second, logr.Discard())
	if _, err := ac.listOffers(context.Background(), adapter); err != nil {
		t.Fatalf("listOffers: %v", err)
	}
	if gotMaker != lowerAddr(adapter) || gotDeadline == "" || gotChainID != "11155111" ||
		!strings.HasPrefix(gotAuth, "Bearer 0x") || gotKey != "" {
		t.Fatalf("maker=%q chainId=%q deadline=%q auth=%q key=%q", gotMaker, gotChainID, gotDeadline, gotAuth, gotKey)
	}
}

func TestAPIClient_ListAuctions_OversizedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte(`[]`))
		_, _ = w.Write([]byte(strings.Repeat(" ", maxGeneratedResponseBytes+1)))
	}))
	defer srv.Close()

	ac := newAPIClient(srv.URL, fakeSigner{}, big.NewInt(1), 5*time.Second, logr.Discard())
	_, err := ac.listAuctions(context.Background())
	if !errors.Is(err, httptransport.ErrResponseTooLarge) {
		t.Fatalf("error = %v, want ErrResponseTooLarge", err)
	}
}
