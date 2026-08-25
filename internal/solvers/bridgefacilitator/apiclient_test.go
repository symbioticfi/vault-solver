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
func (fakeSigner) SignTx(
	_ context.Context,
	tx *types.Transaction,
	_ *big.Int,
) (*types.Transaction, error) {
	return tx, nil
}

func TestAPIClientListAuctions(t *testing.T) {
	want := testAuctionDto(7, common.Address{0xaa})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode([]threef.AuctionDto{want}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	client := newAPIClient(srv.URL, fakeSigner{}, big.NewInt(11155111), time.Second, logr.Discard())
	got, err := client.listAuctions(t.Context())
	if err != nil {
		t.Fatalf("listAuctions: %v", err)
	}
	if len(got) != 1 || got[0].Id != want.Id || got[0].RequestId != want.RequestId {
		t.Fatalf("auctions = %+v, want auction %v", got, want.Id)
	}
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
