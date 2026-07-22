package types

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func mustBig(t *testing.T, s string) *big.Int {
	t.Helper()
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("bad big.Int %q", s)
	}
	return n
}

func TestOfferInputMarshalJSONWireShape(t *testing.T) {
	input := OfferInput{
		Now: time.Unix(1, 0).UTC(),
		Adapters: []AdapterSnapshot{{
			ID:            "adapter-1",
			Adapter:       common.HexToAddress("0x0000000000000000000000000000000000000001"),
			Vault:         common.HexToAddress("0x0000000000000000000000000000000000000002"),
			Collateral:    common.HexToAddress("0x0000000000000000000000000000000000000003"),
			Fundable:      mustBig(t, "1000"),
			OpenCount:     1,
			MaxAssets:     mustBig(t, "500"),
			MinAssets:     mustBig(t, "100"),
			MinYieldPpm:   mustBig(t, "190"),
			MaxConcurrent: 3,
		}},
		Auctions: []AuctionSnapshot{{
			ID:              "10",
			AuctionID:       10,
			OriginalIndex:   0,
			Request:         common.HexToAddress("0x0000000000000000000000000000000000000010"),
			Status:          "open",
			DepositAsset:    common.HexToAddress("0x0000000000000000000000000000000000000003"),
			AmountRequested: mustBig(t, "900"),
			RemainingAmount: mustBig(t, "700"),
			MaxRateBps:      200,
		}},
		LiveOffers: []LiveOffer{{AdapterID: "adapter-1", AuctionID: 10}},
	}

	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(body), "Fundable") || !strings.Contains(string(body), `"fundable":"1000"`) {
		t.Fatalf("JSON does not use lower-camel decimal-string amounts: %s", body)
	}
	if !strings.Contains(string(body), `"minYieldPpm":"190"`) {
		t.Fatalf("wire must carry the exact minYieldPpm floor: %s", body)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("Unmarshal raw: %v", err)
	}
	adapters := raw["adapters"].([]any)
	adapter := adapters[0].(map[string]any)
	if adapter["maxAssets"] != "500" || adapter["minAssets"] != "100" {
		t.Fatalf("adapter amounts not decimal strings: %#v", adapter)
	}
	liveOffers := raw["liveOffers"].([]any)
	liveOffer := liveOffers[0].(map[string]any)
	if liveOffer["adapterId"] != "adapter-1" || liveOffer["auctionId"].(float64) != 10 {
		t.Fatalf("liveOffer wire shape: %#v", liveOffer)
	}
}

func TestOfferOutputUnmarshalJSONWireShape(t *testing.T) {
	var out OfferOutput
	if err := json.Unmarshal([]byte(`{
		"offers": [{
			"auctionId": 10,
			"request": "0x0000000000000000000000000000000000000010",
			"maker": "0x0000000000000000000000000000000000000001",
			"principal": "500",
			"expectedReturn": "10",
			"reason": "largest"
		}]
	}`), &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(out.Offers) != 1 ||
		out.Offers[0].AuctionID != 10 ||
		out.Offers[0].Principal.String() != "500" ||
		out.Offers[0].ExpectedReturn.String() != "10" ||
		out.Offers[0].Reason != "largest" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestOfferOutputUnmarshalJSONRejectsUnknownFields(t *testing.T) {
	var out OfferOutput
	err := json.Unmarshal([]byte(`{"offers":[],"extra":1}`), &out)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Unmarshal error = %v, want unknown field rejection", err)
	}
}

func TestOfferOutputUnmarshalJSONRejectsInvalidDecimal(t *testing.T) {
	var out OfferOutput
	err := json.Unmarshal([]byte(`{"offers":[{"auctionId":10,"principal":"nan","expectedReturn":"1"}]}`), &out)
	if err == nil || !strings.Contains(err.Error(), "principal") {
		t.Fatalf("Unmarshal error = %v, want principal decimal rejection", err)
	}
}
