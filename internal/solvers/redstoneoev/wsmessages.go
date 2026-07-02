package redstoneoev

import (
	"encoding/json"

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

// dedupKey returns the key used to suppress a replayed delivery of this auction. RedStone's auction id is
// the only valid identity; empty-id frames are dropped before bidding.
func (a AuctionMessage) dedupKey() string {
	if a.ID == "" {
		return ""
	}
	return "id:" + a.ID
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
