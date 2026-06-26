package redstoneoev

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/go-errors/errors"
)

// Hand-written WS structs pinned to RedStone's zod schema and live auction frames; there is no upstream
// OpenAPI to generate from.
func opName(raw []byte) (string, error) {
	var head struct {
		Op string `json:"op"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return "", errors.Errorf("ws: decode op: %w", err)
	}
	return head.Op, nil
}

func isFeedAuction(raw []byte) bool {
	var frame struct {
		Payload map[string]json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &frame); err != nil || len(frame.Payload) == 0 {
		return false
	}
	if _, ok := frame.Payload["positions"]; ok {
		return false
	}
	if _, ok := frame.Payload["prices"]; ok {
		return false
	}
	return true
}

type AuctionMessage struct {
	Op        string         `json:"op"`
	ID        string         `json:"id"`
	Timestamp int64          `json:"timestamp"`
	TimeoutMs int            `json:"timeoutMs"`
	Payload   AuctionPayload `json:"payload"`
}

type AuctionPayload struct {
	Prices map[string]string `json:"prices"`
}

// dedupKey returns the key used to suppress a replayed delivery of this auction. The auctioneer's `id` is
// authoritative when present; when it's empty (some frames carry none) we'd otherwise NEVER record the
// frame as seen, so a replay would be processed twice → a second nonce + a double bid. So derive a synthetic
// key from the frame content — a hash over the auctioneer emit timestamp/timeout plus the sorted prices.
// Folding in the emit timestamp is essential: two genuinely-distinct id-less auctions at the SAME price
// (e.g. the same oracle re-auctioned) emit at different times, so without it the second would collide with
// the first and be wrongly dropped as a duplicate. A reconnect REPLAY of one frame carries the same
// timestamp, so it still dedups. Prefixed by source so a content hash can never collide with a real id.
func (a AuctionMessage) dedupKey() string {
	if a.ID != "" {
		return "id:" + a.ID
	}
	prices := make([]string, 0, len(a.Payload.Prices))
	for k, v := range a.Payload.Prices {
		prices = append(prices, k+"="+v)
	}
	sort.Strings(prices)
	h := sha256.New()
	// Emit timestamp + timeout first: distinguishes two same-price auctions emitted at different times,
	// while a replay of the same frame (same timestamp/timeout) still hashes identically.
	fmt.Fprintf(h, "%d|%d\x00", a.Timestamp, a.TimeoutMs)
	h.Write([]byte(strings.Join(prices, ",")))
	return "hash:" + hex.EncodeToString(h.Sum(nil))
}

type AuctionResult struct {
	Op   string            `json:"op"`
	ID   string            `json:"id"`
	Data AuctionResultData `json:"data"`
}

type AuctionResultData struct {
	Bid        string `json:"bid"`
	Liquidator string `json:"liquidator"`
}

type LiquidationResult struct {
	Op   string                `json:"op"`
	ID   string                `json:"id"`
	Data LiquidationResultData `json:"data"`
}

type LiquidationResultData struct {
	Success    bool   `json:"success"`
	TxHash     string `json:"txHash"`
	Liquidator string `json:"liquidator"`
	Error      string `json:"error"`
}

type Blacklisted struct {
	Op   string          `json:"op"`
	ID   string          `json:"id"`
	Data BlacklistedData `json:"data"`
}

type BlacklistedData struct {
	Liquidator string `json:"liquidator"`
	Msg        string `json:"msg"`
}

type SubscribeMessage struct {
	Op    string `json:"op"`
	Topic string `json:"topic"`
}

type SolveMessage struct {
	Op   string    `json:"op"`
	ID   string    `json:"id"`
	Data SolveData `json:"data"`
}

// SolveData carries the bid. `bid` is a decimal ether string of the signed wei bidAmount; `nonce`
// and `maxTxGasPrice` are decimal strings; `operationData`/`liquidationSig` are 0x-hex.
type SolveData struct {
	Bid               string   `json:"bid"`
	Nonce             string   `json:"nonce"`
	OperationCallback string   `json:"operationCallback"`
	OperationData     string   `json:"operationData"`
	LiquidationSig    string   `json:"liquidationSig"`
	MaxTxGasPrice     string   `json:"maxTxGasPrice"`
	Borrowers         []string `json:"borrowers,omitempty"`
}

func marshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil { // unreachable: our outbound shapes are static and marshal-safe
		panic("redstoneoev: marshal: " + err.Error())
	}
	return b
}
