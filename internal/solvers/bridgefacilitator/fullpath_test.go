package bridgefacilitator

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	adapterbinding "github.com/symbioticfi/vault-solver/api/bindings/3f/adapter"
	vaultcontrollerbinding "github.com/symbioticfi/vault-solver/api/bindings/3f/vaultcontroller"
	erc4626binding "github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	multicallbinding "github.com/symbioticfi/vault-solver/api/bindings/multicall3"
	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type multicallResponder func([]multicallbinding.Multicall3Call3) ([]multicallbinding.Multicall3Result, error)

type decodedMulticallRPC struct {
	server *httptest.Server

	mu  sync.Mutex
	err error
}

type jsonRPCErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type jsonRPCResponse struct {
	JSONRPC string            `json:"jsonrpc"`
	ID      json.RawMessage   `json:"id"`
	Result  string            `json:"result,omitempty"`
	Error   *jsonRPCErrorBody `json:"error,omitempty"`
}

func newDecodedMulticallClient(
	t *testing.T,
	multicallAddr common.Address,
	respond multicallResponder,
) (*chain.Client, *decodedMulticallRPC) {
	t.Helper()

	multicallABI, err := multicallbinding.Multicall3MetaData.ParseABI()
	if err != nil {
		t.Fatalf("parse Multicall3 ABI: %v", err)
	}
	rpcServer := &decodedMulticallRPC{}
	rpcServer.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			JSONRPC string            `json:"jsonrpc"`
			ID      json.RawMessage   `json:"id"`
			Method  string            `json:"method"`
			Params  []json.RawMessage `json:"params"`
		}
		if decodeErr := json.NewDecoder(r.Body).Decode(&request); decodeErr != nil {
			rpcServer.fail(w, nil, errors.Errorf("decode JSON-RPC request: %w", decodeErr))
			return
		}
		switch request.Method {
		case "eth_chainId":
			writeJSONRPCResult(w, request.ID, "0xaa36a7")
		case "eth_call":
			result, callErr := decodeAndAnswerMulticall(request.Params, multicallAddr, multicallABI, respond)
			if callErr != nil {
				rpcServer.fail(w, request.ID, callErr)
				return
			}
			writeJSONRPCResult(w, request.ID, hexutil.Encode(result))
		default:
			rpcServer.fail(w, request.ID, errors.Errorf("unexpected JSON-RPC method %q", request.Method))
		}
	}))

	client, err := chain.Dial(t.Context(), []string{rpcServer.server.URL}, "", multicallAddr.Hex(), 11155111, logr.Discard())
	if err != nil {
		rpcServer.server.Close()
		t.Fatalf("dial decoded Multicall server: %v", err)
	}
	t.Cleanup(func() {
		client.Close()
		rpcServer.server.Close()
	})
	return client, rpcServer
}

func decodeAndAnswerMulticall(
	params []json.RawMessage,
	multicallAddr common.Address,
	multicallABI *abi.ABI,
	respond multicallResponder,
) ([]byte, error) {
	if len(params) == 0 {
		return nil, errors.New("eth_call omitted call object")
	}
	var call struct {
		To    common.Address `json:"to"`
		Data  hexutil.Bytes  `json:"data"`
		Input hexutil.Bytes  `json:"input"`
	}
	if err := json.Unmarshal(params[0], &call); err != nil {
		return nil, errors.Errorf("decode eth_call object: %w", err)
	}
	if call.To != multicallAddr {
		return nil, errors.Errorf("eth_call target = %s, want Multicall3 %s", call.To.Hex(), multicallAddr.Hex())
	}
	data := []byte(call.Data)
	if len(data) == 0 {
		data = call.Input
	}
	method := multicallABI.Methods["aggregate3"]
	if len(data) < 4 || !bytes.Equal(data[:4], method.ID) {
		return nil, errors.Errorf("eth_call does not contain aggregate3 calldata")
	}
	values, err := method.Inputs.Unpack(data[4:])
	if err != nil {
		return nil, errors.Errorf("decode aggregate3 calls: %w", err)
	}
	calls := *abi.ConvertType(values[0], new([]multicallbinding.Multicall3Call3)).(*[]multicallbinding.Multicall3Call3)
	results, err := respond(calls)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, errors.Errorf("Multicall responder returned %d results for %d calls", len(results), len(calls))
	}
	encoded, err := method.Outputs.Pack(results)
	if err != nil {
		return nil, errors.Errorf("encode aggregate3 results: %w", err)
	}
	return encoded, nil
}

func (s *decodedMulticallRPC) fail(w http.ResponseWriter, id json.RawMessage, err error) {
	s.mu.Lock()
	if s.err == nil {
		s.err = err
	}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if encodeErr := json.NewEncoder(w).Encode(jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &jsonRPCErrorBody{Code: -32000, Message: err.Error()},
	}); encodeErr != nil {
		return
	}
}

func (s *decodedMulticallRPC) assertClean(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		t.Fatalf("decoded Multicall server: %v", s.err)
	}
}

func writeJSONRPCResult(w http.ResponseWriter, id json.RawMessage, result string) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}); err != nil {
		return
	}
}

type offerPathAPI struct {
	server *httptest.Server

	mu        sync.Mutex
	auction   string
	wantMaker string
	posts     []threef.CreateOfferDto
	err       error
}

func newOfferPathAPI(t *testing.T, auction, wantMaker string) *offerPathAPI {
	t.Helper()
	api := &offerPathAPI{auction: auction, wantMaker: wantMaker}
	api.server = httptest.NewServer(http.HandlerFunc(api.serveHTTP))
	t.Cleanup(api.server.Close)
	return api
}

func (a *offerPathAPI) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/auction":
		if r.URL.Query().Get("domain") != "true" {
			a.recordError(errors.New("auction list did not request EIP-712 domains"))
			http.Error(w, "domain is required", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(a.auction))
	case r.Method == http.MethodPost && r.URL.Path == "/v1/offer":
		var dto threef.CreateOfferDto
		if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
			a.recordError(errors.Errorf("decode create offer: %w", err))
			http.Error(w, "invalid offer", http.StatusBadRequest)
			return
		}
		if dto.Maker != strings.ToLower(dto.Maker) || dto.Maker != a.wantMaker {
			a.recordError(errors.Errorf("offer maker = %q, want lowercase %q", dto.Maker, a.wantMaker))
			http.Error(w, "maker must be lowercase", http.StatusBadRequest)
			return
		}
		a.mu.Lock()
		a.posts = append(a.posts, dto)
		postNumber := len(a.posts)
		a.mu.Unlock()
		if postNumber == 1 {
			http.Error(w, "forced failure", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1}`))
	default:
		a.recordError(errors.Errorf("unexpected API request %s %s", r.Method, r.URL.RequestURI()))
		http.NotFound(w, r)
	}
}

func (a *offerPathAPI) recordError(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err == nil {
		a.err = err
	}
}

func (a *offerPathAPI) snapshot(t *testing.T) []threef.CreateOfferDto {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		t.Fatalf("offer API server: %v", a.err)
	}
	return append([]threef.CreateOfferDto(nil), a.posts...)
}

func newOfferPathResponder(
	adapter, vault, collateral, signerAddress common.Address,
) (multicallResponder, error) {
	adapterABI, err := adapterbinding.ThreeFAdapterMetaData.ParseABI()
	if err != nil {
		return nil, errors.Errorf("parse ThreeFAdapter ABI: %w", err)
	}
	vaultABI, err := erc4626binding.IERC4626MetaData.ParseABI()
	if err != nil {
		return nil, errors.Errorf("parse IERC4626 ABI: %w", err)
	}
	outputs := map[string]any{
		"vault":               vault,
		"offerSigner":         signerAddress,
		"asset":               collateral,
		"getMaxAssets":        big.NewInt(1_000_000_000),
		"minYieldPerRequest":  new(big.Int),
		"minAssetsPerRequest": new(big.Int),
		"maxAssetsPerRequest": big.NewInt(1_000_000_000),
		"requestsLength":      new(big.Int),
	}
	return func(calls []multicallbinding.Multicall3Call3) ([]multicallbinding.Multicall3Result, error) {
		if len(calls) != 1 && len(calls) != 2 && len(calls) != 5 {
			return nil, errors.Errorf("offer-path Multicall has unexpected %d-call shape", len(calls))
		}
		results := make([]multicallbinding.Multicall3Result, len(calls))
		for i, call := range calls {
			contractABI := adapterABI
			if call.Target == vault {
				contractABI = vaultABI
			} else if call.Target != adapter {
				return nil, errors.Errorf("offer-path call %d has unexpected target %s", i, call.Target.Hex())
			}
			if len(call.CallData) < 4 {
				return nil, errors.Errorf("offer-path call %d is shorter than a selector", i)
			}
			method, methodErr := contractABI.MethodById(call.CallData[:4])
			if methodErr != nil {
				return nil, errors.Errorf("offer-path call %d selector: %w", i, methodErr)
			}
			value, ok := outputs[method.Name]
			if !ok {
				return nil, errors.Errorf("unexpected offer-path method %q", method.Name)
			}
			resolutionCall := method.Name == "vault" || method.Name == "offerSigner" || method.Name == "asset"
			if call.AllowFailure != resolutionCall {
				return nil, errors.Errorf("offer-path %s allowFailure = %t, want %t", method.Name, call.AllowFailure, resolutionCall)
			}
			encoded, packErr := method.Outputs.Pack(value)
			if packErr != nil {
				return nil, errors.Errorf("encode %s output: %w", method.Name, packErr)
			}
			results[i] = multicallbinding.Multicall3Result{Success: true, ReturnData: encoded}
		}
		return results, nil
	}, nil
}

func TestFullOfferPath(t *testing.T) {
	const (
		auctionID = int64(42)
		chainID   = int64(11155111)
	)
	adapter := common.HexToAddress("0x00000000000000000000000000000000000000A1")
	vault := common.HexToAddress("0x00000000000000000000000000000000000000B1")
	collateral := common.HexToAddress("0x00000000000000000000000000000000000000C1")
	request := common.HexToAddress("0x00000000000000000000000000000000000000D1")
	multicallAddr := common.HexToAddress("0x00000000000000000000000000000000000000E1")
	const saltHex = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	localSigner, err := signer.NewFromHexKey(strings.Repeat("0", 63) + "1")
	if err != nil {
		t.Fatalf("build local signer: %v", err)
	}
	respond, err := newOfferPathResponder(adapter, vault, collateral, localSigner.Address())
	if err != nil {
		t.Fatalf("build offer-path responder: %v", err)
	}
	chainClient, rpcServer := newDecodedMulticallClient(t, multicallAddr, respond)

	auctionJSON := `[{` +
		`"id":42,` +
		`"requestId":"` + lowerAddr(request) + `",` +
		`"amountRequested":"1000000000",` +
		`"solve_start_time":null,` +
		`"maxRate":50.5,` +
		`"status":"open",` +
		`"asset":null,` +
		`"depositAsset":{"address":"` + lowerAddr(collateral) + `","symbol":"USDC","decimals":6},` +
		`"vault":null,` +
		`"settlement":null,` +
		`"direction":null,` +
		`"eip712Domain":{"name":"SuperstateRequest","version":"1","chainId":11155111,"salt":"` + saltHex + `"}` +
		`}]`
	apiServer := newOfferPathAPI(t, auctionJSON, lowerAddr(adapter))

	strategy, err := newStrategy(StrategyConfig{Name: defaultStrategyName})
	if err != nil {
		t.Fatalf("build default strategy: %v", err)
	}
	fixedNow := time.Unix(1_800_000_000, 0)
	cfg := &Config{
		APIBaseURL:      apiServer.server.URL,
		RedeemBatchSize: 2,
		HTTPTimeout:     2 * time.Second,
		Targets:         []Target{{Adapter: adapter}},
		Intervals:       Intervals{Discover: 20 * time.Minute, OfferTTL: 45 * time.Minute},
	}
	s := &Solver{
		cfg: cfg,
		deps: solver.Deps{
			Chain: chainClient, Signer: localSigner, Log: logr.Discard(),
		},
		api:                newAPIClient(apiServer.server.URL, localSigner, big.NewInt(chainID), cfg.HTTPTimeout, logr.Discard()),
		reader:             newReader(chainClient),
		strategy:           strategy,
		log:                logr.Discard(),
		signerAddr:         localSigner.Address(),
		now:                func() time.Time { return fixedNow },
		offers:             newOfferTracker(),
		pendingRedemptions: make(map[redeemKey]struct{}),
	}
	s.nonceSeq.Store(100)
	if err := s.resolveTargets(t.Context()); err != nil {
		t.Fatalf("resolve production target: %v", err)
	}
	if len(s.cfg.Targets) != 1 || s.cfg.Targets[0].Vault != vault || s.cfg.Targets[0].Collateral != collateral {
		t.Fatalf("resolved target = %+v, want vault %s collateral %s", s.cfg.Targets, vault.Hex(), collateral.Hex())
	}

	s.discoverAndOffer(t.Context())
	if got := len(s.offers.offers); got != 0 {
		t.Fatalf("offer tracker after failed POST has %d entries, want 0", got)
	}
	if got := len(apiServer.snapshot(t)); got != 1 {
		t.Fatalf("POST count after forced failure = %d, want 1", got)
	}

	s.discoverAndOffer(t.Context())
	posts := apiServer.snapshot(t)
	if len(posts) != 2 {
		t.Fatalf("POST count after success = %d, want 2", len(posts))
	}
	got := posts[1]
	if got.Amount != "1000000000" || got.ExpectedReturn != "5050000" {
		t.Fatalf("offer amounts = %s/%s, want 1000000000/5050000", got.Amount, got.ExpectedReturn)
	}
	wantExpiration := strconv.FormatInt(fixedNow.Add(cfg.Intervals.OfferTTL).Unix(), 10)
	if got.Expiration != wantExpiration {
		t.Fatalf("expiration = %s, want %s", got.Expiration, wantExpiration)
	}
	if got.AuctionId != auctionID || got.GetChainId() != chainID || !got.UseCallback {
		t.Fatalf("offer identity = auction %d chain %d callback %t", got.AuctionId, got.GetChainId(), got.UseCallback)
	}
	assertOfferSignature(t, got, request, localSigner.Address(), saltHex)
	if len(s.offers.offers) != 1 {
		t.Fatalf("offer tracker after successful POST has %d entries, want 1", len(s.offers.offers))
	}
	state, ok := s.offers.offers[offerKey{adapter: adapter, auction: auctionID}]
	if !ok || !state.expiry.Equal(fixedNow.Add(cfg.Intervals.OfferTTL)) || state.principal.Cmp(big.NewInt(1_000_000_000)) != 0 {
		t.Fatalf("tracked offer = %+v, want exact successful offer", state)
	}
	rpcServer.assertClean(t)
}

func assertOfferSignature(
	t *testing.T,
	dto threef.CreateOfferDto,
	request common.Address,
	wantSigner common.Address,
	saltHex string,
) {
	t.Helper()
	parseUint := func(field, value string) *big.Int {
		n, ok := new(big.Int).SetString(value, 10)
		if !ok {
			t.Fatalf("%s = %q is not an integer", field, value)
		}
		return n
	}
	sigField, ok := dto.GetSignatureOk()
	if !ok || sigField == nil {
		t.Fatal("offer omitted signature")
	}
	sig, err := hexutil.Decode(*sigField)
	if err != nil || len(sig) != crypto.SignatureLength {
		t.Fatalf("decode signature = %x, %v", sig, err)
	}
	if sig[64] != 27 && sig[64] != 28 {
		t.Fatalf("signature recovery id = %d, want 27 or 28", sig[64])
	}
	sig[64] -= 27
	digest := OfferDigest(Offer{
		Maker:          common.HexToAddress(dto.Maker),
		Amount:         parseUint("amount", dto.Amount),
		ExpectedReturn: parseUint("expectedReturn", dto.ExpectedReturn),
		Nonce:          parseUint("nonce", dto.Nonce),
		Expiration:     parseUint("expiration", dto.Expiration),
		UseCallback:    dto.UseCallback,
	}, OfferDomain{
		Name:              "SuperstateRequest",
		Version:           "1",
		ChainID:           big.NewInt(dto.GetChainId()),
		VerifyingContract: request,
		Salt:              hashPointer(common.HexToHash(saltHex)),
	})
	publicKey, err := crypto.SigToPub(digest.Bytes(), sig)
	if err != nil {
		t.Fatalf("recover offer signature: %v", err)
	}
	if got := crypto.PubkeyToAddress(*publicKey); got != wantSigner {
		t.Fatalf("recovered signer = %s, want %s", got.Hex(), wantSigner.Hex())
	}
}

func hashPointer(hash common.Hash) *common.Hash { return &hash }

type redemptionState struct {
	mu sync.RWMutex

	reportedLength int64
	requests       []common.Address
	ready          map[common.Address]bool
}

func (s *redemptionState) set(reportedLength int64, requests []common.Address, ready ...common.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reportedLength = reportedLength
	s.requests = append([]common.Address(nil), requests...)
	s.ready = make(map[common.Address]bool, len(ready))
	for _, request := range ready {
		s.ready[request] = true
	}
}

func (s *redemptionState) snapshot() (int64, []common.Address, map[common.Address]bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ready := make(map[common.Address]bool, len(s.ready))
	for request, ok := range s.ready {
		ready[request] = ok
	}
	return s.reportedLength, append([]common.Address(nil), s.requests...), ready
}

func newRedemptionResponder(
	adapter common.Address,
	state *redemptionState,
) (multicallResponder, error) {
	adapterABI, err := adapterbinding.ThreeFAdapterMetaData.ParseABI()
	if err != nil {
		return nil, errors.Errorf("parse ThreeFAdapter ABI: %w", err)
	}
	controllerABI, err := vcABI()
	if err != nil {
		return nil, err
	}
	return func(calls []multicallbinding.Multicall3Call3) ([]multicallbinding.Multicall3Result, error) {
		reportedLength, requests, ready := state.snapshot()
		results := make([]multicallbinding.Multicall3Result, len(calls))
		for i, call := range calls {
			result, answerErr := answerRedemptionCall(adapterABI, controllerABI, adapter, call, reportedLength, requests, ready)
			if answerErr != nil {
				return nil, errors.Errorf("redemption call %d: %w", i, answerErr)
			}
			results[i] = result
		}
		return results, nil
	}, nil
}

func vcABI() (*abi.ABI, error) {
	parsed, err := vaultcontrollerbinding.IVaultControllerMetaData.ParseABI()
	if err != nil {
		return nil, errors.Errorf("parse IVaultController ABI: %w", err)
	}
	return parsed, nil
}

func answerRedemptionCall(
	adapterABI, controllerABI *abi.ABI,
	adapter common.Address,
	call multicallbinding.Multicall3Call3,
	reportedLength int64,
	requests []common.Address,
	ready map[common.Address]bool,
) (multicallbinding.Multicall3Result, error) {
	if len(call.CallData) < 4 {
		return multicallbinding.Multicall3Result{}, errors.New("calldata is shorter than a selector")
	}
	if call.Target == adapter {
		method, err := adapterABI.MethodById(call.CallData[:4])
		if err != nil {
			return multicallbinding.Multicall3Result{}, errors.Errorf("adapter selector: %w", err)
		}
		switch method.Name {
		case "requestsLength":
			if call.AllowFailure {
				return multicallbinding.Multicall3Result{}, errors.New("requestsLength unexpectedly allows failure")
			}
			return encodeCallResult(method, big.NewInt(reportedLength))
		case "requests":
			if !call.AllowFailure {
				return multicallbinding.Multicall3Result{}, errors.New("requests slot must allow failure")
			}
			values, unpackErr := method.Inputs.Unpack(call.CallData[4:])
			if unpackErr != nil {
				return multicallbinding.Multicall3Result{}, errors.Errorf("decode requests index: %w", unpackErr)
			}
			index := values[0].(*big.Int)
			if !index.IsInt64() || index.Sign() < 0 || index.Int64() >= int64(len(requests)) {
				return multicallbinding.Multicall3Result{Success: false}, nil
			}
			return encodeCallResult(method, requests[index.Int64()])
		default:
			return multicallbinding.Multicall3Result{}, errors.Errorf("unexpected adapter method %q", method.Name)
		}
	}

	method, err := controllerABI.MethodById(call.CallData[:4])
	if err != nil || method.Name != "canWithdraw" {
		return multicallbinding.Multicall3Result{}, errors.Errorf("unexpected request selector for %s", call.Target.Hex())
	}
	if !call.AllowFailure {
		return multicallbinding.Multicall3Result{}, errors.New("canWithdraw must allow failure")
	}
	return encodeCallResult(method, ready[call.Target])
}

func encodeCallResult(method *abi.Method, values ...any) (multicallbinding.Multicall3Result, error) {
	encoded, err := method.Outputs.Pack(values...)
	if err != nil {
		return multicallbinding.Multicall3Result{}, errors.Errorf("encode %s output: %w", method.Name, err)
	}
	return multicallbinding.Multicall3Result{Success: true, ReturnData: encoded}, nil
}

type ambiguousTransactionBackend struct {
	mu   sync.Mutex
	sent []*gethtypes.Transaction
}

func (b *ambiguousTransactionBackend) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	return 0, nil
}

func (b *ambiguousTransactionBackend) SuggestGasTipCap(context.Context) (*big.Int, error) {
	return big.NewInt(1), nil
}

func (b *ambiguousTransactionBackend) HeaderByNumber(context.Context, *big.Int) (*gethtypes.Header, error) {
	return &gethtypes.Header{Number: big.NewInt(100), BaseFee: big.NewInt(1)}, nil
}

func (b *ambiguousTransactionBackend) EstimateGas(context.Context, ethereum.CallMsg) (uint64, error) {
	return 100_000, nil
}

func (b *ambiguousTransactionBackend) SendTransaction(_ context.Context, tx *gethtypes.Transaction) error {
	b.mu.Lock()
	b.sent = append(b.sent, tx)
	b.mu.Unlock()
	return errors.New("temporary broadcast timeout")
}

func (b *ambiguousTransactionBackend) TransactionReceipt(context.Context, common.Hash) (*gethtypes.Receipt, error) {
	return nil, ethereum.NotFound
}

func (b *ambiguousTransactionBackend) BlockNumber(context.Context) (uint64, error) { return 100, nil }

func (b *ambiguousTransactionBackend) sentTransactions() []*gethtypes.Transaction {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*gethtypes.Transaction(nil), b.sent...)
}

func TestRedeemFullBoundary(t *testing.T) {
	adapter := common.HexToAddress("0x00000000000000000000000000000000000000A2")
	multicallAddr := common.HexToAddress("0x00000000000000000000000000000000000000E2")
	requests := []common.Address{
		common.HexToAddress("0x0000000000000000000000000000000000000011"),
		common.HexToAddress("0x0000000000000000000000000000000000000012"),
		common.HexToAddress("0x0000000000000000000000000000000000000013"),
		common.HexToAddress("0x0000000000000000000000000000000000000014"),
	}
	state := &redemptionState{}
	state.set(5, requests, requests...)
	respond, err := newRedemptionResponder(adapter, state)
	if err != nil {
		t.Fatalf("build redemption responder: %v", err)
	}
	chainClient, rpcServer := newDecodedMulticallClient(t, multicallAddr, respond)

	ready, err := newReader(chainClient).readyToRedeem(t.Context(), adapter)
	if err != nil {
		t.Fatalf("read ready prefix: %v", err)
	}
	assertAddresses(t, ready, requests)

	localSigner, err := signer.NewFromHexKey(strings.Repeat("0", 63) + "2")
	if err != nil {
		t.Fatalf("build local signer: %v", err)
	}
	backend := &ambiguousTransactionBackend{}
	manager := txmanager.New(backend, localSigner, big.NewInt(11155111), txmanager.Config{
		PollInterval:    time.Millisecond,
		PendingInterval: 5 * time.Millisecond,
		MaxReplacements: 1,
	}, logr.Discard())
	managerCtx, cancelManager := context.WithCancel(t.Context())
	managerDone := make(chan error, 1)
	go func() { managerDone <- manager.Start(managerCtx) }()
	t.Cleanup(func() {
		cancelManager()
		if startErr := <-managerDone; startErr != nil {
			t.Errorf("txmanager.Start: %v", startErr)
		}
	})

	s := &Solver{
		cfg: &Config{
			RedeemBatchSize: 2,
			Targets:         []Target{{Adapter: adapter}},
		},
		deps: solver.Deps{
			Chain: chainClient, TxManager: manager, Signer: localSigner, Log: logr.Discard(),
		},
		reader:             newReader(chainClient),
		log:                logr.Discard(),
		pendingRedemptions: make(map[redeemKey]struct{}),
	}

	s.redeemReady(t.Context(), s.cfg.Targets[0])
	sent := backend.sentTransactions()
	if len(sent) == 0 {
		t.Fatal("txmanager backend captured no redemption attempt")
	}
	if sent[0].To() == nil || *sent[0].To() != adapter {
		t.Fatalf("redemption target = %v, want %s", sent[0].To(), adapter.Hex())
	}
	decodedFinalizeCalls := decodeFinalizeRequests(t, sent[0].Data())
	if len(decodedFinalizeCalls) != s.cfg.RedeemBatchSize {
		t.Fatalf("finalize calls = %d, want %d", len(decodedFinalizeCalls), s.cfg.RedeemBatchSize)
	}
	assertAddresses(t, decodedFinalizeCalls, requests[:s.cfg.RedeemBatchSize])

	physicalAttempts := len(sent)
	state.set(2, requests[:2], requests[:2]...)
	s.redeemReady(t.Context(), s.cfg.Targets[0])
	if got := len(backend.sentTransactions()); got != physicalAttempts {
		t.Fatalf("unresolved requests were resubmitted: physical attempts = %d, want %d", got, physicalAttempts)
	}

	state.set(0, nil)
	s.redeemReady(t.Context(), s.cfg.Targets[0])
	if len(s.pendingRedemptions) != 0 {
		t.Fatalf("authoritative absence left %d pending redemption keys", len(s.pendingRedemptions))
	}

	state.set(1, requests[:1], requests[0])
	s.redeemReady(t.Context(), s.cfg.Targets[0])
	if got := len(backend.sentTransactions()); got <= physicalAttempts {
		t.Fatalf("request did not become eligible after authoritative absence: attempts = %d, want > %d", got, physicalAttempts)
	}
	rpcServer.assertClean(t)
}

func decodeFinalizeRequests(t *testing.T, data []byte) []common.Address {
	t.Helper()
	adapterABI, err := adapterbinding.ThreeFAdapterMetaData.ParseABI()
	if err != nil {
		t.Fatalf("parse ThreeFAdapter ABI: %v", err)
	}
	multicallMethod := adapterABI.Methods["multicall"]
	if len(data) < 4 || !bytes.Equal(data[:4], multicallMethod.ID) {
		t.Fatal("redemption transaction does not call adapter.multicall")
	}
	values, err := multicallMethod.Inputs.Unpack(data[4:])
	if err != nil {
		t.Fatalf("decode adapter.multicall: %v", err)
	}
	calls := *abi.ConvertType(values[0], new([][]byte)).(*[][]byte)
	requests := make([]common.Address, len(calls))
	for i, call := range calls {
		method, methodErr := adapterABI.MethodById(call[:4])
		if methodErr != nil || method.Name != "finalizeRequest" {
			t.Fatalf("adapter.multicall item %d is not finalizeRequest: %v", i, methodErr)
		}
		arguments, unpackErr := method.Inputs.Unpack(call[4:])
		if unpackErr != nil {
			t.Fatalf("decode finalizeRequest %d: %v", i, unpackErr)
		}
		requests[i] = arguments[0].(common.Address)
	}
	return requests
}

func assertAddresses(t *testing.T, got, want []common.Address) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("addresses = %v (len %d), want %v (len %d)", got, len(got), want, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("address %d = %s, want %s", i, got[i].Hex(), want[i].Hex())
		}
	}
}
