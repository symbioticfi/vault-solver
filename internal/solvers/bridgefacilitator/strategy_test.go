package bridgefacilitator

import (
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/types"
	webhookstrategy "github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/webhook"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

func mustBig(t *testing.T, s string) *big.Int {
	t.Helper()
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("bad big.Int %q", s)
	}
	return n
}

func baseOfferInput(t *testing.T) types.OfferInput {
	t.Helper()
	adapter := common.HexToAddress("0x0000000000000000000000000000000000000001")
	return types.OfferInput{
		Now: time.Unix(0, 0),
		Adapters: []types.AdapterSnapshot{{
			ID:            adapterID(adapter),
			Adapter:       adapter,
			Vault:         common.HexToAddress("0x0000000000000000000000000000000000000002"),
			Collateral:    common.HexToAddress("0x0000000000000000000000000000000000000003"),
			Fundable:      mustBig(t, "1000"),
			MaxAssets:     mustBig(t, "800"),
			MinAssets:     new(big.Int),
			MinYieldBps:   new(big.Int),
			MaxConcurrent: maxRequests,
		}},
		Auctions: []types.AuctionSnapshot{{
			ID:              "10",
			AuctionID:       10,
			OriginalIndex:   0,
			Request:         common.HexToAddress("0x0000000000000000000000000000000000000010"),
			Status:          "open",
			DepositAsset:    common.HexToAddress("0x0000000000000000000000000000000000000003"),
			AmountRequested: mustBig(t, "700"),
			RemainingAmount: mustBig(t, "700"),
			MaxRateBps:      200,
		}},
	}
}

func TestStrategyRegistryUsesBuiltIns(t *testing.T) {
	got, err := newStrategy(StrategyConfig{Name: "default"})
	if err != nil {
		t.Fatalf("newStrategy default: %v", err)
	}
	if got == nil {
		t.Fatal("newStrategy default returned nil")
	}
	names := strategies.Registered()
	if len(names) != 2 || names[0] != "default" || names[1] != "webhook" {
		t.Fatalf("registered strategies = %v, want [default webhook]", names)
	}
}

func TestBuildStrategyInputKeepsFullyCoveredAuctions(t *testing.T) {
	now := time.Unix(100, 0)
	adapter := common.HexToAddress("0x0000000000000000000000000000000000000001")
	collateral := common.HexToAddress("0x0000000000000000000000000000000000000003")
	offers := newOfferTracker()
	offers.record(adapter, 10, testOfferState(now.Add(time.Minute), big.NewInt(100)))

	input := buildStrategyInput(
		[]threef.AuctionDto{testAuctionDto(10, collateral, "100")},
		[]*adapterOffering{{
			target: Target{
				Adapter:    adapter,
				Vault:      common.HexToAddress("0x0000000000000000000000000000000000000002"),
				Collateral: collateral,
			},
			st: exposureState{
				fundable:    big.NewInt(100),
				maxAssets:   big.NewInt(100),
				minAssets:   new(big.Int),
				minYieldBps: new(big.Int),
			},
		}},
		offers,
		now,
	)

	if len(input.Auctions) != 1 {
		t.Fatalf("auctions = %d, want fully covered auction passed to strategy", len(input.Auctions))
	}
	if input.Auctions[0].RemainingAmount.Sign() != 0 {
		t.Fatalf("remaining = %s, want 0", input.Auctions[0].RemainingAmount)
	}
	if len(input.LiveOffers) != 1 ||
		input.LiveOffers[0].AdapterID != adapterID(adapter) || input.LiveOffers[0].AuctionID != 10 {
		t.Fatalf("liveOffers = %+v, want the adapter's live offer on auction 10", input.LiveOffers)
	}
}

func TestWebhookStrategyDecodesLowerCamelResponse(t *testing.T) {
	input := baseOfferInput(t)
	offer := input.Auctions[0]
	maker := input.Adapters[0].Adapter
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		if !strings.Contains(string(body), `"fundable":"1000"`) || strings.Contains(string(body), `"Fundable"`) {
			t.Fatalf("request body does not use decimal-string lower-camel JSON: %s", string(body))
		}
		_, _ = w.Write([]byte(`{
			"offers": [{
				"auctionId": 10,
				"request": "` + offer.Request.Hex() + `",
				"maker": "` + maker.Hex() + `",
				"principal": "700",
				"expectedReturn": "14"
			}]
		}`))
	}))
	defer srv.Close()
	client, err := webhook.NewClient(webhook.Config{URL: srv.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	out, err := webhookstrategy.New(client).DecideOffers(t.Context(), input)
	if err != nil {
		t.Fatalf("DecideOffers: %v", err)
	}
	if len(out.Offers) != 1 || out.Offers[0].Principal.String() != "700" ||
		out.Offers[0].ExpectedReturn.String() != "14" {
		t.Fatalf("unexpected webhook output: %+v", out)
	}
}

func testAuctionDto(id int64, depositAsset common.Address, amountRequested string) threef.AuctionDto {
	maxRate := float32(200)
	request := common.HexToAddress("0x0000000000000000000000000000000000000010")
	return threef.AuctionDto{
		Id:              float32(id),
		RequestId:       request.Hex(),
		AmountRequested: *threef.NewNullableString(&amountRequested),
		MaxRate:         *threef.NewNullableFloat32(&maxRate),
		Status:          "open",
		DepositAsset: *threef.NewNullableAuctionDepositAssetDto(
			threef.NewAuctionDepositAssetDto(depositAsset.Hex(), "USDC", 6),
		),
	}
}
