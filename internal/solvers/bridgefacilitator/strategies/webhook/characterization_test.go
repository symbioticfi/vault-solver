package webhookstrategy

import (
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

// Test3FR1WebhookRequestCharacterization is the immutable 3F-R1 webhook wire baseline.
func Test3FR1WebhookRequestCharacterization(t *testing.T) {
	t.Parallel()

	const wantBody = `{"now":"2023-11-14T22:13:20Z","adapters":[{"id":"adapter-1","adapter":"0x0000000000000000000000000000000000000001","vault":"0x0000000000000000000000000000000000000002","collateral":"0x0000000000000000000000000000000000000003","fundable":"1000","openCount":2,"maxAssets":"800","minAssets":"100","minYieldPpm":"190","maxConcurrent":50}],"auctions":[{"id":"10","auctionId":10,"originalIndex":4,"request":"0x0000000000000000000000000000000000000010","status":"SoLvAbLe","depositAsset":"0x0000000000000000000000000000000000000003","amountRequested":"900","remainingAmount":"700","maxRateBps":250.5}],"liveOffers":[{"adapterId":"adapter-1","auctionId":10}]}`

	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("request = %s content-type=%q, want POST application/json", r.Method, r.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"offers":[]}`))
	}))
	defer server.Close()

	client, err := webhook.NewClient(webhook.Config{URL: server.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	strategy := New(client)
	input := strategytypes.OfferInput{
		Now: time.Unix(1_700_000_000, 0).UTC(),
		Adapters: []strategytypes.AdapterSnapshot{{
			ID: "adapter-1", Adapter: common.HexToAddress("0x1"), Vault: common.HexToAddress("0x2"),
			Collateral: common.HexToAddress("0x3"), Fundable: big.NewInt(1000), OpenCount: 2,
			MaxAssets: big.NewInt(800), MinAssets: big.NewInt(100), MinYieldPpm: big.NewInt(190), MaxConcurrent: 50,
		}},
		Auctions: []strategytypes.AuctionSnapshot{{
			ID: "10", AuctionID: 10, OriginalIndex: 4, Request: common.HexToAddress("0x10"), Status: "SoLvAbLe",
			DepositAsset: common.HexToAddress("0x3"), AmountRequested: big.NewInt(900),
			RemainingAmount: big.NewInt(700), MaxRateBps: 250.5,
		}},
		LiveOffers: []strategytypes.LiveOffer{{AdapterID: "adapter-1", AuctionID: 10}},
	}
	out, err := strategy.DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(out.Offers) != 0 {
		t.Fatalf("output = %+v, want no offers", out)
	}
	if gotBody != wantBody {
		t.Fatalf("webhook request bytes changed:\n got  %s\n want %s", gotBody, wantBody)
	}
}
