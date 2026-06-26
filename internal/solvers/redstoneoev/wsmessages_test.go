package redstoneoev

import (
	"encoding/json"
	"testing"
)

// capturedAuction is a real `oev/liquidations` frame shape (docs/OEV-PLAN.md §6.1): note `timeoutMs`
// (not the docs example's `durationMs`) and the liquidations payload nested under `payload`.
const capturedAuction = `{
  "op":"auction","id":"6382e936-c915-496a-bb3e-fa3b4ccc3a8d","timestamp":1781243340988,"timeoutMs":500,
  "payload":{
    "positions":[
      {"market_unique_key":"0x6209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5",
       "borrower_address":"0x629d764ec8563afa701709b52c1a215e865632de","current_ltv":108.83,
       "oracle_address":"0xfED5bC312C7139743bc3ab21Ef92f5AeB353339D","lltv":"860000000000000000",
       "collateral_decimals":18,"loan_decimals":6,
       "collateral_address":"0x17e892d4E802B01d7DA49Ca3542560f6851AA4D3",
       "loan_address":"0x468BB3245BF520a0CD030BDE029c98aCEAF84C9d",
       "collateral_assets":"1000000000000000000","borrow_assets":"1685600048","borrow_shares":"1685600000000000"},
      {"market_unique_key":"0x6209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5",
       "borrower_address":"0x378a49c640fd9eea888a6a553caae441e2fdebc6","current_ltv":102.17,
       "oracle_address":"0xfED5bC312C7139743bc3ab21Ef92f5AeB353339D","lltv":"860000000000000000",
       "collateral_decimals":18,"loan_decimals":6,
       "collateral_address":"0x17e892d4E802B01d7DA49Ca3542560f6851AA4D3",
       "loan_address":"0x468BB3245BF520a0CD030BDE029c98aCEAF84C9d",
       "collateral_assets":"1000000000000000000","borrow_assets":"1582400019","borrow_shares":"1582399974653062"}
    ],
    "prices":{"0xfED5bC312C7139743bc3ab21Ef92f5AeB353339D":"1800943620100000000000000000"}
  }
}`

func TestDecodeAuctionFrame(t *testing.T) {
	if op, err := opName([]byte(capturedAuction)); err != nil || op != "auction" {
		t.Fatalf("opName = %q, %v; want auction", op, err)
	}
	var a AuctionMessage
	if err := json.Unmarshal([]byte(capturedAuction), &a); err != nil {
		t.Fatal(err)
	}
	if a.ID != "6382e936-c915-496a-bb3e-fa3b4ccc3a8d" {
		t.Fatalf("id = %q", a.ID)
	}
	if a.TimeoutMs != 500 {
		t.Fatalf("timeoutMs = %d, want 500", a.TimeoutMs)
	}
	if got := a.Payload.Prices["0xfED5bC312C7139743bc3ab21Ef92f5AeB353339D"]; got != "1800943620100000000000000000" {
		t.Fatalf("price = %q", got)
	}
}

func TestDetectFeedAuctionFrame(t *testing.T) {
	feed := []byte(`{
	  "op":"auction","id":"e9803b9f-4318-4dc0-811d-23f2f0b938f2",
	  "timestamp":1726058300000,"durationMs":400,
	  "payload":{"ETH":"250000000000","BTC":"6000000000000","USDC":"99878787"}
	}`)
	if !isFeedAuction(feed) {
		t.Fatal("flat feed auction must be detected")
	}
	if isFeedAuction([]byte(capturedAuction)) {
		t.Fatal("liquidation auction must not be detected as a feed auction")
	}
}

// TestDedupKeyTimestamp pins that an id-less frame folds the auctioneer emit timestamp into its dedup key.
// Distinct same-price re-auctions get distinct keys, while reconnect replay of one frame still dedups.
func TestDedupKeyTimestamp(t *testing.T) {
	mk := func(ts int64, timeout int) AuctionMessage {
		return AuctionMessage{
			Timestamp: ts, TimeoutMs: timeout,
			Payload: AuctionPayload{Prices: map[string]string{"0xoracleA": "1800000000000000000000000000"}},
		}
	}
	a := mk(1781243340988, 500)
	replay := mk(1781243340988, 500) // identical frame redelivered on reconnect
	later := mk(1781243341488, 500)  // same price, emitted 500ms later → a distinct auction

	if a.dedupKey() != replay.dedupKey() {
		t.Fatal("identical id-less frames (same timestamp) must dedup to the same key")
	}
	if a.dedupKey() == later.dedupKey() {
		t.Fatal("two same-price id-less frames at different timestamps must NOT collide")
	}
	// A different timeout (same price + timestamp) is also a distinct frame.
	if a.dedupKey() == mk(1781243340988, 400).dedupKey() {
		t.Fatal("differing timeoutMs must yield a distinct dedup key")
	}
}

func TestMarshalSolve(t *testing.T) {
	msg := SolveMessage{Op: "solve", ID: "abc", Data: SolveData{
		Bid: "0.0005", Nonce: "3", OperationCallback: "0x7Aa3", OperationData: "0x1234",
		LiquidationSig: "0xdead", MaxTxGasPrice: "60000000000", Borrowers: []string{"0x629d"},
	}}
	var back map[string]any
	if err := json.Unmarshal(marshal(msg), &back); err != nil {
		t.Fatal(err)
	}
	if back["op"] != "solve" || back["id"] != "abc" {
		t.Fatalf("solve top-level wrong: %v", back)
	}
	data, _ := back["data"].(map[string]any)
	for _, k := range []string{"bid", "nonce", "operationCallback", "operationData", "liquidationSig", "maxTxGasPrice", "borrowers"} {
		if _, ok := data[k]; !ok {
			t.Fatalf("solve.data missing %q", k)
		}
	}
}
