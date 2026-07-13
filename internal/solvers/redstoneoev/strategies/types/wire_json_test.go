package types

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestBidInputMarshalJSON(t *testing.T) {
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
			Loan:         common.HexToAddress("0x00000000000000000000000000000000000000ee"),
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
			GasLimit:           2_000_000,
		},
		PendingAuctions: []PendingAuction{{
			ID:        "a0",
			SentAt:    time.Unix(9, 0).UTC(),
			Won:       true,
			ExpiresAt: time.Unix(20, 0).UTC(),
		}},
	}
	b, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
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
		`"price":"123456789"`,
		`"pendingAuctions":`,
		`"won":true`,
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("json %s missing %s", js, want)
		}
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
