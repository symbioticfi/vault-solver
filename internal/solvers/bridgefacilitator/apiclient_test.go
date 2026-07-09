package bridgefacilitator

import (
	"context"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/httptransport"
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
