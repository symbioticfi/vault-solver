//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"

	erc4626binding "github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	adapterbinding "github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
)

// These public Anvil fixture keys must never hold assets or permissions outside disposable local chains.
const (
	anvilDeployerKey  = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	anvilOrderUserKey = "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
	anvilSolverKey    = "5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a"
)

func (testEnv *testEnvironment) call(t *testing.T, target common.Address, data []byte) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := testEnv.client.CallContract(ctx, ethereum.CallMsg{To: &target, Data: data}, nil)
	if err != nil {
		t.Fatalf("eth_call %s: %v", target, err)
	}
	return result
}

func (testEnv *testEnvironment) send(t *testing.T, privateKeyHex string, target common.Address, data []byte) *types.Receipt {
	t.Helper()
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		t.Fatalf("decode transaction key: %v", err)
	}
	from := crypto.PubkeyToAddress(privateKey.PublicKey)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	nonce, err := testEnv.client.PendingNonceAt(ctx, from)
	if err != nil {
		t.Fatalf("read nonce for %s: %v", from, err)
	}
	tip, err := testEnv.client.SuggestGasTipCap(ctx)
	if err != nil {
		t.Fatalf("suggest gas tip: %v", err)
	}
	header, err := testEnv.client.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatalf("read latest header: %v", err)
	}
	feeCap := new(big.Int).Set(tip)
	if header.BaseFee != nil {
		feeCap.Add(feeCap, new(big.Int).Mul(header.BaseFee, big.NewInt(2)))
	}
	message := ethereum.CallMsg{
		From:      from,
		To:        &target,
		GasFeeCap: feeCap,
		GasTipCap: tip,
		Data:      data,
	}
	gas, err := testEnv.client.EstimateGas(ctx, message)
	if err != nil {
		t.Fatalf("estimate transaction to %s: %v", target, err)
	}
	gas += gas / 5
	chainID := big.NewInt(testEnv.manifest.Chain.ID)
	transaction := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		GasTipCap: tip,
		GasFeeCap: feeCap,
		Gas:       gas,
		To:        &target,
		Data:      data,
	})
	signed, err := types.SignTx(transaction, types.LatestSignerForChainID(chainID), privateKey)
	if err != nil {
		t.Fatalf("sign transaction: %v", err)
	}
	if err := testEnv.client.SendTransaction(ctx, signed); err != nil {
		t.Fatalf("send transaction %s: %v", signed.Hash(), err)
	}
	return testEnv.waitReceipt(t, signed.Hash(), 30*time.Second)
}

func (testEnv *testEnvironment) waitReceipt(t *testing.T, hash common.Hash, timeout time.Duration) *types.Receipt {
	t.Helper()
	var receipt *types.Receipt
	eventually(t, "transaction receipt "+hash.Hex(), timeout, 200*time.Millisecond, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		current, err := testEnv.client.TransactionReceipt(ctx, hash)
		if err != nil {
			if errors.Is(err, ethereum.NotFound) {
				return errors.New("not mined")
			}
			return errors.Errorf("read receipt: %w", err)
		}
		receipt = current
		return nil
	})
	if receipt.Status != types.ReceiptStatusSuccessful {
		t.Fatalf("transaction %s reverted", hash)
	}
	return receipt
}

func (testEnv *testEnvironment) transaction(t *testing.T, hash common.Hash) *types.Transaction {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	transaction, pending, err := testEnv.client.TransactionByHash(ctx, hash)
	if err != nil {
		t.Fatalf("read transaction %s: %v", hash, err)
	}
	if pending {
		t.Fatalf("transaction %s is still pending", hash)
	}
	return transaction
}

func (testEnv *testEnvironment) balanceOf(t *testing.T, token, account common.Address) *big.Int {
	t.Helper()
	binding := erc4626binding.NewIERC4626()
	balance, err := binding.UnpackBalanceOf(testEnv.call(t, token, binding.PackBalanceOf(account)))
	if err != nil {
		t.Fatalf("decode balanceOf(%s, %s): %v", token, account, err)
	}
	return balance
}

func (testEnv *testEnvironment) adapterIsFiller(
	t *testing.T,
	adapter, marketMaker, filler common.Address,
) bool {
	t.Helper()
	binding := adapterbinding.NewLiquidLaneAdapter()
	allowed, err := binding.UnpackIsFiller(testEnv.call(t, adapter, binding.PackIsFiller(marketMaker, filler)))
	if err != nil {
		t.Fatalf("decode isFiller(%s): %v", adapter, err)
	}
	return allowed
}

func (testEnv *testEnvironment) adapterAmountOut(
	t *testing.T,
	adapter, token common.Address,
	amount *big.Int,
) *big.Int {
	t.Helper()
	binding := adapterbinding.NewLiquidLaneAdapter()
	output, err := binding.UnpackGetAmountOut(testEnv.call(t, adapter, binding.PackGetAmountOut(token, amount)))
	if err != nil {
		t.Fatalf("decode getAmountOut(%s): %v", adapter, err)
	}
	return output
}

func (testEnv *testEnvironment) adapterMinDiscount(t *testing.T, adapter, token common.Address) *big.Int {
	t.Helper()
	binding := adapterbinding.NewLiquidLaneAdapter()
	discount, err := binding.UnpackMinDiscount(testEnv.call(t, adapter, binding.PackMinDiscount(token)))
	if err != nil {
		t.Fatalf("decode minDiscount(%s): %v", adapter, err)
	}
	return discount
}

func (testEnv *testEnvironment) adapterMaxRate(t *testing.T, adapter, token common.Address) *big.Int {
	t.Helper()
	binding := adapterbinding.NewLiquidLaneAdapter()
	rate, err := binding.UnpackGetMaxRate(testEnv.call(t, adapter, binding.PackGetMaxRate(token)))
	if err != nil {
		t.Fatalf("decode getMaxRate(%s): %v", adapter, err)
	}
	return rate
}

func (testEnv *testEnvironment) tokenDecimals(t *testing.T, address common.Address) uint8 {
	t.Helper()
	for _, token := range append(testEnv.manifest.Tokens.Input, testEnv.manifest.Tokens.Output...) {
		if token.Address == address {
			return token.Decimals
		}
	}
	t.Fatalf("manifest has no token metadata for %s", address)
	return 0
}

func (testEnv *testEnvironment) getJSON(t *testing.T, url string, output any) int {
	t.Helper()
	return testEnv.requestJSON(t, http.MethodGet, url, nil, nil, output)
}

func (testEnv *testEnvironment) postJSON(t *testing.T, url string, input, output any) int {
	t.Helper()
	return testEnv.requestJSON(t, http.MethodPost, url, nil, input, output)
}

func (testEnv *testEnvironment) requestJSON(
	t *testing.T,
	method string,
	url string,
	headers map[string]string,
	input any,
	output any,
) int {
	t.Helper()
	var body []byte
	var err error
	if input != nil {
		body, err = json.Marshal(input)
		if err != nil {
			t.Fatalf("encode %s %s: %v", method, url, err)
		}
	}
	request, err := http.NewRequestWithContext(context.Background(), method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build %s %s: %v", method, url, err)
	}
	if input != nil {
		request.Header.Set("content-type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	return testEnv.doJSON(t, request, output)
}

func (testEnv *testEnvironment) doJSON(t *testing.T, request *http.Request, output any) int {
	t.Helper()
	//nolint:gosec // The trusted local harness supplies every E2E endpoint.
	response, err := testEnv.httpClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", request.Method, request.URL, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		t.Fatalf("read %s %s: %v", request.Method, request.URL, err)
	}
	if output != nil && len(body) > 0 {
		if err := json.Unmarshal(body, output); err != nil {
			t.Fatalf("decode %s %s response (%d): %v; body=%s", request.Method, request.URL, response.StatusCode, err, body)
		}
	}
	return response.StatusCode
}

func TestGetJSON(t *testing.T) {
	type requestSnapshot struct {
		method         string
		hasContentType bool
		body           []byte
	}
	received := make(chan requestSnapshot, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		_, hasContentType := request.Header["Content-Type"]
		received <- requestSnapshot{
			method:         request.Method,
			hasContentType: hasContentType,
			body:           body,
		}
		writer.WriteHeader(http.StatusAccepted)
		if _, err := io.WriteString(writer, `{"value":"ok"}`); err != nil {
			return
		}
	}))
	defer server.Close()

	testEnv := testEnvironment{httpClient: server.Client()}
	var output struct {
		Value string `json:"value"`
	}
	status := testEnv.getJSON(t, server.URL, &output)
	request := <-received
	if status != http.StatusAccepted || output.Value != "ok" {
		t.Fatalf("GET response status=%d value=%q", status, output.Value)
	}
	if request.method != http.MethodGet || request.hasContentType || len(request.body) != 0 {
		t.Fatalf("GET request method=%s has-content-type=%t body=%q", request.method, request.hasContentType, request.body)
	}
}

func decodeMethodInput(t *testing.T, metadata *bind.MetaData, data []byte, rawName string) []any {
	t.Helper()
	if len(data) < 4 {
		t.Fatalf("calldata has %d bytes", len(data))
	}
	parsed, err := metadata.ParseABI()
	if err != nil {
		t.Fatalf("parse %s ABI: %v", metadata.ID, err)
	}
	method, err := parsed.MethodById(data[:4])
	if err != nil {
		t.Fatalf("resolve %s selector: %v", metadata.ID, err)
	}
	if method.RawName != rawName {
		t.Fatalf("method = %s, want %s", method.RawName, rawName)
	}
	values, err := method.Inputs.Unpack(data[4:])
	if err != nil {
		t.Fatalf("decode %s.%s calldata: %v", metadata.ID, rawName, err)
	}
	return values
}

func convertABIValue[T any](t *testing.T, value any) T {
	t.Helper()
	converted := abi.ConvertType(value, new(T))
	result, ok := converted.(*T)
	if !ok {
		t.Fatalf("ABI value %T cannot convert to target", value)
	}
	return *result
}

func signTypedData(t *testing.T, privateKeyHex string, digest []byte) string {
	t.Helper()
	key, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		t.Fatalf("decode signing key: %v", err)
	}
	signature, err := crypto.Sign(digest, key)
	if err != nil {
		t.Fatalf("sign typed data: %v", err)
	}
	signature[64] += 27
	return hexutil.Encode(signature)
}

func addressForKey(t *testing.T, privateKeyHex string) common.Address {
	t.Helper()
	key, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		t.Fatalf("decode account key: %v", err)
	}
	return crypto.PubkeyToAddress(key.PublicKey)
}

func eventually(
	t *testing.T,
	label string,
	timeout time.Duration,
	interval time.Duration,
	operation func() error,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		err := operation()
		if err == nil {
			return
		}
		lastErr = err
		time.Sleep(interval)
	}
	t.Fatalf("%s timed out: %v", label, lastErr)
}

func isHTTPSuccess(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices
}

func parseBig(t *testing.T, value string) *big.Int {
	t.Helper()
	parsed, ok := new(big.Int).SetString(value, 10)
	if !ok {
		t.Fatalf("invalid integer %q", value)
	}
	return parsed
}
