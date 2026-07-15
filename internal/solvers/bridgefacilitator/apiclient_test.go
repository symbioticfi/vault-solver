package bridgefacilitator

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/threef"
)

// fakeSigner is a minimal signer.Signer test double that signs nothing meaningful (65 zero bytes).
type fakeSigner struct{ addr common.Address }

func (s fakeSigner) Address() common.Address { return s.addr }
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
	chainID, _ := strconv.ParseFloat(gotChainID, 64) // generated client serializes chainId as a float
	if gotMaker != lowerAddr(adapter) || gotDeadline == "" || chainID != 11155111 ||
		!strings.HasPrefix(gotAuth, "Bearer 0x") || gotKey != "" {
		t.Fatalf("maker=%q chainId=%q deadline=%q auth=%q key=%q", gotMaker, gotChainID, gotDeadline, gotAuth, gotKey)
	}
}

func TestAPIClient_CreateOfferReturnsID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/offer" {
			t.Fatalf("request = %s %s, want POST /v1/offer", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":123}`))
	}))
	defer srv.Close()

	dto := threef.NewCreateOfferDto(42, lowerAddr(common.Address{0x42}), "100", "5", "7", "4102444800", true)
	ac := newAPIClient(srv.URL, fakeSigner{}, big.NewInt(11155111), 5*time.Second, logr.Discard())
	got, err := ac.createOffer(context.Background(), *dto)
	if err != nil {
		t.Fatalf("createOffer: %v", err)
	}
	if got != 123 {
		t.Fatalf("offer id = %d, want 123", got)
	}
}

func TestAPIClient_GetOfferByID_SignedPerAdapter(t *testing.T) {
	adapter := common.HexToAddress("0x0000000000000000000000000000000000000042")
	var gotMaker, gotAuth, gotDeadline, gotChainID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/offer/123" {
			t.Fatalf("request = %s %s, want GET /v1/offer/123", r.Method, r.URL.Path)
		}
		gotMaker = r.URL.Query().Get("maker")
		gotDeadline = r.URL.Query().Get("deadline")
		gotChainID = r.URL.Query().Get("chainId")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":123,
			"auctionId":42,
			"status":"SUBMITTED",
			"maker":"` + lowerAddr(adapter) + `",
			"requestId":"0x0000000000000000000000000000000000000043",
			"asset":null,
			"vault":null,
			"amount":"100",
			"expectedReturn":"5",
			"nonce":"7",
			"expiration":"4102444800",
			"signature":null
		}`))
	}))
	defer srv.Close()

	ac := newAPIClient(srv.URL, fakeSigner{}, big.NewInt(11155111), 5*time.Second, logr.Discard())
	offer, err := ac.getOfferByID(context.Background(), adapter, 123)
	if err != nil {
		t.Fatalf("getOfferByID: %v", err)
	}
	chainID, _ := strconv.ParseFloat(gotChainID, 64)
	if gotMaker != lowerAddr(adapter) || gotDeadline == "" || chainID != 11155111 ||
		!strings.HasPrefix(gotAuth, "Bearer 0x") {
		t.Fatalf("maker=%q chainId=%q deadline=%q auth=%q", gotMaker, gotChainID, gotDeadline, gotAuth)
	}
	if offer == nil || offer.Id != 123 || offer.AuctionId != 42 || offer.Status != offerStatusSubmitted {
		t.Fatalf("offer = %+v", offer)
	}
}

func TestAPIClient_CancelOfferSignsPayload(t *testing.T) {
	adapter := common.HexToAddress("0x0000000000000000000000000000000000000042")
	var gotBody struct {
		OfferID   float32 `json:"offerId"`
		Maker     string  `json:"maker"`
		ChainID   float32 `json:"chainId"`
		Deadline  string  `json:"deadline"`
		Signature string  `json:"signature"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/offer/cancel" {
			t.Fatalf("request = %s %s, want POST /v1/offer/cancel", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":123,"status":"CANCELLED"}`))
	}))
	defer srv.Close()

	ac := newAPIClient(srv.URL, fakeSigner{}, big.NewInt(11155111), 5*time.Second, logr.Discard())
	id, status, err := ac.cancelOffer(context.Background(), adapter, 123)
	if err != nil {
		t.Fatalf("cancelOffer: %v", err)
	}
	if id != 123 || status != "CANCELLED" {
		t.Fatalf("cancel response = %d/%q, want 123/CANCELLED", id, status)
	}
	if gotBody.OfferID != 123 || gotBody.Maker != lowerAddr(adapter) || gotBody.ChainID != 11155111 ||
		gotBody.Deadline == "" || !strings.HasPrefix(gotBody.Signature, "0x") {
		t.Fatalf("cancel body = %+v", gotBody)
	}
}
