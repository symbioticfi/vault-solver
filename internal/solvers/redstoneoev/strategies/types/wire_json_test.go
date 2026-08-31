package types

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
)

func TestBidInputMarshalJSON(t *testing.T) {
	loan := common.HexToAddress("0x00000000000000000000000000000000000000ee")
	rate, ok := new(big.Int).SetString("1234567890123456789012345", 10)
	if !ok {
		t.Fatal("parse gas rate")
	}
	input := BidInput{
		Now: time.Unix(10, 0).UTC(),
		Auction: AuctionSnapshot{
			ID:            "a1",
			Timestamp:     123,
			TimeoutMs:     400,
			RawPriceCount: 2,
			Prices: []AuctionPrice{{
				Oracle: common.HexToAddress("0x00000000000000000000000000000000000000aa"),
				Price:  big.NewInt(123456789),
			}},
		},
		Adapter: AdapterSnapshot{
			Address:      common.HexToAddress("0x00000000000000000000000000000000000000cc"),
			Vault:        common.HexToAddress("0x00000000000000000000000000000000000000cd"),
			Loan:         loan,
			LoanDecimals: 6,
			FreeAssets:   big.NewInt(100),
			Withdrawable: big.NewInt(90),
			Redeemable: []RedeemableSnapshot{{
				Asset:          common.HexToAddress("0x00000000000000000000000000000000000000ff"),
				Decimals:       18,
				MaxRate:        big.NewInt(123),
				MaxAssets:      big.NewInt(456),
				AcquireBalance: big.NewInt(7),
			}},
			Filler: true,
		},
		Context: BidContext{
			ChainID:            big.NewInt(11155111),
			Executor:           common.HexToAddress("0x00000000000000000000000000000000000000bb"),
			Callback:           common.HexToAddress("0x00000000000000000000000000000000000000ab"),
			Signer:             common.HexToAddress("0x00000000000000000000000000000000000000dd"),
			ExecutorDeposit:    big.NewInt(1000),
			ExecutorMinDeposit: big.NewInt(100),
			MaxTxGasPrice:      big.NewInt(30),
			GasPrices: liquidlanegas.NewPriceSnapshot(map[common.Address]*big.Int{
				loan: rate,
			}),
			GasLimit: 2_000_000,
		},
		PendingAuctions: []PendingAuction{
			{
				ID:        "alpha",
				SentAt:    time.Date(2030, time.January, 2, 2, 59, 5, 1, time.UTC),
				Won:       false,
				ExpiresAt: time.Date(2030, time.January, 2, 3, 4, 5, 1, time.UTC),
			},
			{
				ID:        "zeta",
				SentAt:    time.Date(2030, time.January, 2, 3, 3, 5, 0, time.UTC),
				Won:       true,
				ExpiresAt: time.Date(2030, time.January, 2, 3, 8, 5, 0, time.UTC),
			},
		},
	}
	b, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var wire struct {
		PendingAuctions json.RawMessage `json:"pendingAuctions"`
	}
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatalf("decode bid input JSON: %v", err)
	}
	const wantPendingAuctions = `[{"id":"alpha","sentAt":"2030-01-02T02:59:05.000000001Z","won":false,"expiresAt":"2030-01-02T03:04:05.000000001Z"},{"id":"zeta","sentAt":"2030-01-02T03:03:05Z","won":true,"expiresAt":"2030-01-02T03:08:05Z"}]`
	if string(wire.PendingAuctions) != wantPendingAuctions {
		t.Fatalf("pendingAuctions JSON = %s, want %s", wire.PendingAuctions, wantPendingAuctions)
	}

	js := string(b)
	for _, want := range []string{
		`"timeoutMs":400`,
		`"rawPriceCount":2`,
		`"adapter":{"address":"0x00000000000000000000000000000000000000cc"`,
		`"vault":"0x00000000000000000000000000000000000000cd"`,
		`"loan":"0x00000000000000000000000000000000000000ee"`,
		`"loanDecimals":6`,
		`"freeAssets":"100"`,
		`"withdrawable":"90"`,
		`"redeemable":[{"asset":"0x00000000000000000000000000000000000000ff"`,
		`"decimals":18`,
		`"maxRate":"123"`,
		`"maxAssets":"456"`,
		`"acquireBalance":"7"`,
		`"filler":true`,
		`"chainId":"11155111"`,
		`"callback":"0x00000000000000000000000000000000000000ab"`,
		`"executorDeposit":"1000"`,
		`"executorMinDeposit":"100"`,
		`"gasPrices":{"tokenOutPerNative":{"0x00000000000000000000000000000000000000ee":"1234567890123456789012345"}}`,
		`"price":"123456789"`,
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("json %s missing %s", js, want)
		}
	}
	input.Context.GasPrices = nil
	disabled, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("MarshalJSON without gas: %v", err)
	}
	if !strings.Contains(string(disabled), `"gasPrices":null`) {
		t.Fatalf("disabled gas JSON = %s, want explicit null gasPrices", disabled)
	}
}

func TestBidOutputUnmarshalJSON(t *testing.T) {
	var out BidOutput
	if err := json.Unmarshal([]byte(`{
		"decision":"bid",
		"bidAmount":"10",
		"operationData":"0x1234"
	}`), &out); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if out.BidAmount.String() != "10" || string(out.OperationData) != "\x12\x34" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestBidOutputJSONRoundTrip(t *testing.T) {
	want := BidOutput{
		Decision:      DecisionBid,
		Reason:        "profitable",
		BidAmount:     big.NewInt(10),
		OperationData: []byte{0x12, 0x34},
	}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if got := string(b); got != `{"decision":"bid","reason":"profitable","bidAmount":"10","operationData":"0x1234"}` {
		t.Fatalf("json = %s", got)
	}
	var got BidOutput
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if got.Decision != want.Decision || got.Reason != want.Reason ||
		got.BidAmount.Cmp(want.BidAmount) != 0 || string(got.OperationData) != string(want.OperationData) {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func TestBidOutputUnmarshalJSONRejectsUnknownField(t *testing.T) {
	var out BidOutput
	err := json.Unmarshal([]byte(`{"decision":"skip","extra":1}`), &out)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v, want unknown field", err)
	}
}

func TestBidOutputUnmarshalJSONRejectsBadDecimal(t *testing.T) {
	var out BidOutput
	err := json.Unmarshal([]byte(`{
		"decision":"bid",
		"bidAmount":"bad",
		"operationData":"0x1234"
	}`), &out)
	if err == nil || !strings.Contains(err.Error(), "invalid decimal") {
		t.Fatalf("error = %v, want invalid decimal", err)
	}
}
