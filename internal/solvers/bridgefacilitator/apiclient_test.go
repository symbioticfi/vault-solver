package bridgefacilitator

import (
	"context"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
)

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
	var gotMaker, gotAuth, gotKey, gotDeadline string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMaker = r.URL.Query().Get("maker")
		gotDeadline = r.URL.Query().Get("deadline")
		gotAuth = r.Header.Get("Authorization")
		gotKey = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	adapter := common.HexToAddress("0x0000000000000000000000000000000000000042")
	ac := newAPIClient(srv.URL, fakeSigner{}, 5*time.Second, logr.Discard())
	if _, err := ac.listOffers(context.Background(), adapter); err != nil {
		t.Fatalf("listOffers: %v", err)
	}
	if gotMaker != lowerAddr(adapter) || gotDeadline == "" ||
		!strings.HasPrefix(gotAuth, "Bearer 0x") || gotKey != "" {
		t.Fatalf("maker=%q deadline=%q auth=%q key=%q", gotMaker, gotDeadline, gotAuth, gotKey)
	}
}
