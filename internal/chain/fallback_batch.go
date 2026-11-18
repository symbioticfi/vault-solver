package chain

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"strconv"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
)

// Match the pinned SDK's complete response envelope, independently of the metrics prefix cap.
const maxRPCBatchResponseBytes = 32 << 20

func inspectRPCBatch(body []byte) rpcRequestInfo {
	info := rpcRequestInfo{boundedMethod: "batch", rawMethod: "batch"}
	var members []struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
	}
	if json.Unmarshal(body, &members) != nil || len(members) == 0 || len(members) > protocolio.MaxReadBatch {
		return info
	}
	ids := make(map[string]bool, len(members))
	var sensitive, pending bool
	for _, member := range members {
		id, valid := batchRPCID(member.ID)
		method := boundedRPCMethodName(member.Method)
		if _, duplicate := ids[id]; !valid || duplicate || member.JSONRPC != jsonRPCVersion || method == "other" || method == rpcMethodSendRawTransaction {
			return info
		}
		isSensitive := member.Method == rpcMethodGetTransactionReceipt || member.Method == "eth_getBlockByNumber" || member.Method == "eth_getBlockByHash"
		ids[id] = isSensitive
		sensitive = sensitive || isSensitive
		pending = pending || isPendingLookupMethod(member.Method)
	}
	info.batch, info.nullFallback, info.pendingLookup = ids, sensitive, pending
	return info
}

func batchRPCID(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "", false
	}
	if raw[0] == '"' {
		var id string
		if json.Unmarshal(raw, &id) != nil {
			return "", false
		}
		return strconv.Quote(id), true
	}
	number, ok := new(big.Int).SetString(string(raw), 10)
	if !ok {
		return "", false
	}
	return number.String(), true
}

func inspectNullRPCResult(resp *http.Response, request rpcRequestInfo) (bool, []byte, error) {
	if request.batch == nil {
		return hasNullRPCResult(resp, request.requestID)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRPCBatchResponseBytes+1))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	if err != nil {
		return false, body, errors.Errorf("read RPC batch response: %w", err)
	}
	if len(body) > maxRPCBatchResponseBytes {
		return false, nil, errors.New("read RPC batch response exceeds limit")
	}
	var members []rpcResponseEnvelope
	if valid := json.Unmarshal(body, &members) == nil; !valid || len(members) != len(request.batch) {
		// Leave malformed envelopes unchanged for the SDK's exact response validation.
		return false, body, nil
	}
	seen := make(map[string]bool, len(members))
	for _, member := range members {
		id, valid := batchRPCID(member.ID)
		sensitive, known := request.batch[id]
		if !valid || !known || seen[id] || member.JSONRPC != jsonRPCVersion || len(member.Result) == 0 ||
			(len(member.Error) > 0 && !bytes.Equal(bytes.TrimSpace(member.Error), []byte("null"))) {
			return false, body, nil
		}
		seen[id] = true
		// Any mined attempt or known header is evidence the caller must keep from this endpoint.
		if sensitive && !bytes.Equal(bytes.TrimSpace(member.Result), []byte("null")) {
			return false, body, nil
		}
	}
	return true, body, nil
}
