package bridgefacilitator

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr/funcr"

	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/signer"
)

type contractPlanner struct {
	mu     sync.Mutex
	calls  int
	inputs []OfferInput
	output OfferOutput
	err    error
}

func (s *contractPlanner) DecideOffers(_ context.Context, input OfferInput) (OfferOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.inputs = append(s.inputs, input)
	return s.output, s.err
}

func (s *contractPlanner) snapshot() (int, []OfferInput) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls, append([]OfferInput(nil), s.inputs...)
}

type contractTrace struct {
	mu      sync.Mutex
	events  []string
	logs    []string
	posts   []string
	postErr map[int]bool
}

func (t *contractTrace) appendLog(entry string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var normalized map[string]any
	if err := json.Unmarshal([]byte(entry), &normalized); err != nil {
		panic(fmt.Sprintf("normalize 3F-R1 log: %v", err))
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		panic(fmt.Sprintf("marshal 3F-R1 log: %v", err))
	}
	t.logs = append(t.logs, string(encoded))
}

func (t *contractTrace) snapshot() (events, logs, posts []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.events...), append([]string(nil), t.logs...), append([]string(nil), t.posts...)
}

func newContractAPIServer(t *testing.T, auctions []threef.AuctionDto, trace *contractTrace) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/auction":
			trace.mu.Lock()
			trace.events = append(trace.events, "GET /v1/auction?domain="+r.URL.Query().Get("domain"))
			trace.mu.Unlock()
			if err := json.NewEncoder(w).Encode(auctions); err != nil {
				t.Errorf("encode auctions: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/v1/offer":
			if r.URL.Query().Get("deadline") == "" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer 0x") {
				t.Errorf("list offers missing dynamic signed auth: query=%q auth=%q", r.URL.RawQuery, r.Header.Get("Authorization"))
			}
			trace.mu.Lock()
			trace.events = append(trace.events, "GET /v1/offer?chainId="+r.URL.Query().Get("chainId")+
				"&deadline=<dynamic>&maker="+r.URL.Query().Get("maker")+" auth=Bearer <signature>")
			trace.mu.Unlock()
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/offer":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read create offer: %v", err)
				return
			}
			trace.mu.Lock()
			postIndex := len(trace.posts)
			trace.posts = append(trace.posts, string(body))
			trace.events = append(trace.events, "POST /v1/offer "+string(body))
			fail := trace.postErr[postIndex]
			trace.mu.Unlock()
			if fail {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`submit failed`))
				return
			}
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Errorf("unexpected API request: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
}

type contractExecutionHarness struct {
	solver  *Solver
	planner *contractPlanner
	signer  *contractRecordingSigner
	trace   *contractTrace
	stop    func()
}

func newContractExecutionHarness(
	t *testing.T,
	auctions []threef.AuctionDto,
	output OfferOutput,
	laneReady func() bool,
	failSignAt map[int]error,
	postErr map[int]bool,
) *contractExecutionHarness {
	t.Helper()
	baseSigner, err := signer.NewFromHexKey(contractPrivateKey)
	if err != nil {
		t.Fatalf("NewFromHexKey: %v", err)
	}
	recorder := &contractRecordingSigner{Signer: baseSigner, failAt: failSignAt}
	trace := &contractTrace{postErr: postErr}
	server := newContractAPIServer(t, auctions, trace)
	rpc, stopRPC := newMulticallFakeClient(t, abiEncodeAggregate3Results(t,
		abiEncodeUint256(t, 1000),
		abiEncodeUint256(t, 0),
		abiEncodeUint256(t, 0),
		abiEncodeUint256(t, 1000),
		abiEncodeUint256(t, 0),
	))
	adapter := common.HexToAddress("0x00000000000000000000000000000000000000A1")
	collateral := common.HexToAddress("0x3333333333333333333333333333333333333333")
	planner := &contractPlanner{output: output}
	if laneReady == nil {
		laneReady = func() bool { return true }
	}
	s := &Solver{
		cfg:       &Config{OfferExpiryBuffer: 2 * time.Hour},
		api:       newAPIClient(server.URL, recorder, big.NewInt(11155111), time.Second),
		reader:    newReader(rpc, common.Address{}),
		planner:   planner,
		signer:    recorder,
		log:       funcr.NewJSON(trace.appendLog, funcr.Options{Verbosity: 1}),
		laneReady: laneReady,
		nonceSeq:  40,
		offers:    newOfferTracker(),
		targets: []Target{{
			Adapter: adapter, Vault: common.HexToAddress("0x4444444444444444444444444444444444444444"), Collateral: collateral,
		}},
	}
	return &contractExecutionHarness{
		solver: s, planner: planner, signer: recorder, trace: trace,
		stop: func() { stopRPC(); server.Close() },
	}
}

func contractExecution(auctionID int64, request, maker common.Address, principal int64, reason string) OfferExecution {
	return OfferExecution{
		AuctionID: auctionID, Request: request, Maker: maker,
		Principal: big.NewInt(principal), ExpectedReturn: big.NewInt(1), Reason: reason,
	}
}

// Test3FR1AuctionIdentityContract freezes raw DTO lookup independently of eligibility.
func Test3FR1AuctionIdentityContract(t *testing.T) {
	t.Parallel()

	collateral := "0x3333333333333333333333333333333333333333"
	first := contractAuctionDTO(7.25, "0x1111111111111111111111111111111111111111", collateral)
	second := contractAuctionDTO(7.75, "0x2222222222222222222222222222222222222222", collateral)
	second.Status = "closed"
	invalid := contractAuctionDTO(-3.75, "bad", collateral)
	invalid.AmountRequested = threef.NullableString{}
	_, views := buildStrategyInput([]threef.AuctionDto{first, invalid, second}, nil, newOfferTracker(), time.Unix(1_700_000_000, 0).UTC())
	if len(views) != 2 {
		t.Fatalf("views = %d, want every int64 identity with duplicate collapse", len(views))
	}
	if got := views[7].dto; got.Id != 7.75 || got.RequestId != second.RequestId || got.Status != "closed" {
		t.Fatalf("duplicate key 7 = %+v, want last raw DTO", got)
	}
	if got := views[-3].dto; got.Id != -3.75 || got.RequestId != "bad" || got.AmountRequested.IsSet() {
		t.Fatalf("malformed raw DTO missing from key -3: %+v", got)
	}

	first.Status = "open"
	second.Status = "solvable"
	input, _ := buildStrategyInput([]threef.AuctionDto{first, second}, nil, newOfferTracker(), time.Unix(1_700_000_000, 0).UTC())
	if len(input.Auctions) != 2 || input.Auctions[0].AuctionID != 7 || input.Auctions[1].AuctionID != 7 ||
		input.Auctions[0].OriginalIndex != 0 || input.Auctions[1].OriginalIndex != 1 ||
		input.Auctions[0].Request != common.HexToAddress(first.RequestId) || input.Auctions[1].Request != common.HexToAddress(second.RequestId) {
		t.Fatalf("eligible duplicate snapshots changed: %+v", input.Auctions)
	}
}

// Test3FR1ExcludedRawIdentityExecutionContract proves that strategy output can name raw rows
// omitted from its input and that the execution lookup still signs/submits them when later checks allow.
func Test3FR1ExcludedRawIdentityExecutionContract(t *testing.T) {
	request := common.HexToAddress("0x2222222222222222222222222222222222222222")
	maker := common.HexToAddress("0x00000000000000000000000000000000000000A1")
	collateral := "0x3333333333333333333333333333333333333333"
	unsetString := threef.NullableString{}
	nullString := *threef.NewNullableString(nil)
	unsetFloat := threef.NullableFloat32{}
	nullFloat := *threef.NewNullableFloat32(nil)
	unsetDeposit := threef.NullableAuctionDepositAssetDto{}
	nullDeposit := *threef.NewNullableAuctionDepositAssetDto(nil)

	tests := []struct {
		name       string
		mutate     func(*threef.AuctionDto)
		wantSubmit bool
	}{
		{name: "closed status", mutate: func(d *threef.AuctionDto) { d.Status = "closed" }, wantSubmit: true},
		{name: "whitespace status", mutate: func(d *threef.AuctionDto) { d.Status = " open " }, wantSubmit: true},
		{name: "invalid request", mutate: func(d *threef.AuctionDto) { d.RequestId = "bad" }, wantSubmit: true},
		{name: "zero request", mutate: func(d *threef.AuctionDto) { d.RequestId = "0x0000000000000000000000000000000000000000" }, wantSubmit: true},
		{name: "amount absent", mutate: func(d *threef.AuctionDto) { d.AmountRequested = unsetString }, wantSubmit: true},
		{name: "amount null", mutate: func(d *threef.AuctionDto) { d.AmountRequested = nullString }, wantSubmit: true},
		{name: "amount invalid", mutate: func(d *threef.AuctionDto) { d.SetAmountRequested("nan") }, wantSubmit: true},
		{name: "amount zero", mutate: func(d *threef.AuctionDto) { d.SetAmountRequested("0") }, wantSubmit: true},
		{name: "amount negative", mutate: func(d *threef.AuctionDto) { d.SetAmountRequested("-1") }, wantSubmit: true},
		{name: "max rate absent", mutate: func(d *threef.AuctionDto) { d.MaxRate = unsetFloat }},
		{name: "max rate null", mutate: func(d *threef.AuctionDto) { d.MaxRate = nullFloat }},
		{name: "deposit absent", mutate: func(d *threef.AuctionDto) { d.DepositAsset = unsetDeposit }, wantSubmit: true},
		{name: "deposit null", mutate: func(d *threef.AuctionDto) { d.DepositAsset = nullDeposit }, wantSubmit: true},
		{name: "deposit missing address", mutate: func(d *threef.AuctionDto) { d.SetDepositAsset(threef.AuctionDepositAssetDto{}) }, wantSubmit: true},
		{name: "deposit invalid", mutate: func(d *threef.AuctionDto) { d.SetDepositAsset(*threef.NewAuctionDepositAssetDto("bad", "BAD", 18)) }, wantSubmit: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			valid := contractAuctionDTO(1, "0x1111111111111111111111111111111111111111", collateral)
			valid.SetSolveStartTime("2100-01-01T00:00:00Z")
			excluded := contractAuctionDTO(2, "0x9999999999999999999999999999999999999999", collateral)
			excluded.SetSolveStartTime("2100-01-01T00:00:00Z")
			test.mutate(&excluded)
			h := newContractExecutionHarness(t, []threef.AuctionDto{valid, excluded}, OfferOutput{Offers: []OfferExecution{
				contractExecution(2, request, maker, 100, "excluded-raw-row"),
			}}, nil, nil, nil)
			defer h.stop()
			before := time.Now()
			h.solver.discoverAndOffer(t.Context())
			after := time.Now()
			calls, inputs := h.planner.snapshot()
			if calls != 1 || len(inputs) != 1 || len(inputs[0].Auctions) != 1 || inputs[0].Auctions[0].AuctionID != 1 {
				t.Fatalf("strategy calls/input = %d %+v, want only valid raw row", calls, inputs)
			}
			if inputs[0].Now.Before(before) || inputs[0].Now.After(after) {
				t.Fatalf("strategy input now = %s, outside call interval [%s,%s]", inputs[0].Now, before, after)
			}
			_, logs, posts := h.trace.snapshot()
			if test.wantSubmit {
				if len(posts) != 1 || !strings.Contains(posts[0], `"auctionId":2`) || !strings.Contains(posts[0], `"maker":"0x00000000000000000000000000000000000000a1"`) ||
					!strings.Contains(posts[0], `"nonce":"41"`) || !strings.Contains(posts[0], `"expiration":"4102452000"`) {
					t.Fatalf("excluded row POST = %v", posts)
				}
				if !strings.Contains(strings.Join(logs, "\n"), `"msg":"offer submitted"`) {
					t.Fatalf("success log missing: %v", logs)
				}
			} else if len(posts) != 0 || !strings.Contains(strings.Join(logs, "\n"), `"msg":"offer: unbiddable auction; skipping"`) {
				t.Fatalf("unresolved maxRate trace posts=%v logs=%v", posts, logs)
			}
		})
	}
}

// Test3FR1DuplicateExecutionFactsContract freezes last-wins execution facts and proves that
// mismatched and zero strategy Request values are signed as supplied rather than checked against raw DTOs.
func Test3FR1DuplicateExecutionFactsContract(t *testing.T) {
	t.Parallel()

	collateral := "0x3333333333333333333333333333333333333333"
	first := contractAuctionDTO(7.25, "0x1111111111111111111111111111111111111111", collateral)
	first.SetMaxRate(0.1)
	first.SetSolveStartTime("2099-01-01T00:00:00Z")
	firstDomain := first.GetEip712Domain()
	firstDomain.SetName("first-domain")
	firstDomain.SetVersion("first-version")
	firstDomain.SetChainId(1)
	first.SetEip712Domain(firstDomain)
	last := contractAuctionDTO(7.75, "0x9999999999999999999999999999999999999999", collateral)
	last.SetMaxRate(250.5)
	last.SetSolveStartTime("2100-01-01T00:00:00Z")
	lastDomain := last.GetEip712Domain()
	lastDomain.SetName("last-domain")
	lastDomain.SetVersion("last-version")
	lastDomain.SetChainId(11155111)
	last.SetEip712Domain(lastDomain)
	maker := common.HexToAddress("0x00000000000000000000000000000000000000A1")
	mismatch := common.HexToAddress("0x2222222222222222222222222222222222222222")
	h := newContractExecutionHarness(t, []threef.AuctionDto{first, last}, OfferOutput{Offers: []OfferExecution{
		contractExecution(7, mismatch, maker, 100, "mismatched-request"),
		contractExecution(7, common.Address{}, maker, 100, "zero-request"),
	}}, nil, nil, nil)
	defer h.stop()

	h.solver.discoverAndOffer(t.Context())
	calls, inputs := h.planner.snapshot()
	if calls != 1 || len(inputs) != 1 || len(inputs[0].Auctions) != 2 ||
		inputs[0].Auctions[0].AuctionID != 7 || inputs[0].Auctions[1].AuctionID != 7 ||
		inputs[0].Auctions[0].OriginalIndex != 0 || inputs[0].Auctions[1].OriginalIndex != 1 {
		t.Fatalf("duplicate strategy input changed: calls=%d inputs=%+v", calls, inputs)
	}
	_, _, posts := h.trace.snapshot()
	if len(posts) != 2 {
		t.Fatalf("duplicate output POSTs = %v, want two ordered submissions", posts)
	}
	const wantMismatch = `{"amount":"100","auctionId":7.75,"chainId":11155111,"expectedReturn":"1","expiration":"4102452000","maker":"0x00000000000000000000000000000000000000a1","nonce":"41","signature":"0x3b0ab7e9cf4f05ba6080626bd03d3603658b21217e817d770d8247c1a61139756fecf854afacac1d500d9e1fbcc7dffff41afccf5366d0408cdac98641270f901c","useCallback":true}
`
	const wantZero = `{"amount":"100","auctionId":7.75,"chainId":11155111,"expectedReturn":"1","expiration":"4102452000","maker":"0x00000000000000000000000000000000000000a1","nonce":"42","signature":"0xfe2231c407ab7a02fc59e29414cecaaa370c5e89f27da5d9d2e8963fdc2799f33861a6482ca0b82e9933f7f58dcf50585e53d851176e33492237e2c6f0cf56261c","useCallback":true}
`
	if posts[0] != wantMismatch || posts[1] != wantZero {
		t.Fatalf("last-wins/request digest POST bytes changed:\nfirst=%s\nsecond=%s", posts[0], posts[1])
	}
	if len(h.signer.hashes) != 3 { // one GetOffers auth plus two offer digests
		t.Fatalf("signing inputs = %v, want auth plus two offers", h.signer.hashes)
	}
	const wantMismatchDigest = "0x9644ac7fa73318b42c56c69bc5a0d13cbe85255b1a649f2418d6a3205624e41f"
	const wantZeroDigest = "0x3aabec5a8abc3cb9ae689cde4cb8ff4b6559e9273000fb1eb5fd0b5a0d8ad50b"
	if h.signer.hashes[1].Hex() != wantMismatchDigest || h.signer.hashes[2].Hex() != wantZeroDigest {
		t.Fatalf("mismatched/zero Request digests changed: mismatch=%s zero=%s", h.signer.hashes[1].Hex(), h.signer.hashes[2].Hex())
	}
	if len(h.solver.offers.offers) != 0 {
		t.Fatalf("successful submissions inserted local tracker entries: %+v", h.solver.offers.offers)
	}
}

// Test3FR1DiscoverAndOfferContinuationTrace pins the complete ordered error/success continuation trace.
func Test3FR1DiscoverAndOfferContinuationTrace(t *testing.T) {
	t.Parallel()

	collateral := "0x3333333333333333333333333333333333333333"
	auction := contractAuctionDTO(10, "0x1111111111111111111111111111111111111111", collateral)
	auction.SetSolveStartTime("2100-01-01T00:00:00Z")
	maker := common.HexToAddress("0x00000000000000000000000000000000000000A1")
	unknownMaker := common.HexToAddress("0x00000000000000000000000000000000000000C3")
	request := common.HexToAddress("0x2222222222222222222222222222222222222222")
	nilPrincipal := contractExecution(10, request, maker, 1, "nil-principal")
	nilPrincipal.Principal = nil
	h := newContractExecutionHarness(t, []threef.AuctionDto{auction}, OfferOutput{Offers: []OfferExecution{
		contractExecution(999, request, maker, 100, "unknown-auction"),
		contractExecution(10, request, unknownMaker, 100, "unknown-maker"),
		nilPrincipal,
		contractExecution(10, request, maker, 100, "sign-failure"),
		contractExecution(10, request, maker, 100, "submit-failure"),
		contractExecution(10, request, maker, 100, "success-after-errors"),
	}}, nil, map[int]error{3: stderrors.New("injected offer signing failure")}, map[int]bool{0: true})
	defer h.stop()
	h.solver.targets = append(h.solver.targets, Target{
		Adapter:    common.HexToAddress("0x00000000000000000000000000000000000000B2"),
		Vault:      common.HexToAddress("0x5555555555555555555555555555555555555555"),
		Collateral: common.HexToAddress(collateral),
	})

	h.solver.discoverAndOffer(t.Context())
	events, logs, posts := h.trace.snapshot()
	calls, inputs := h.planner.snapshot()
	if calls != 1 || len(inputs) != 1 || len(inputs[0].Adapters) != 2 ||
		inputs[0].Adapters[0].ID != "0x00000000000000000000000000000000000000a1" ||
		inputs[0].Adapters[1].ID != "0x00000000000000000000000000000000000000b2" ||
		len(inputs[0].Auctions) != 1 || len(inputs[0].LiveOffers) != 0 {
		t.Fatalf("strategy call trace changed: calls=%d input=%+v", calls, inputs)
	}
	if len(posts) != 2 || !strings.Contains(posts[0], `"nonce":"42"`) || !strings.Contains(posts[1], `"nonce":"43"`) {
		t.Fatalf("error continuation nonce POSTs = %v, want failed signing nonce 41 consumed then 42/43", posts)
	}
	if len(h.signer.hashes) != 5 || h.solver.nonceSeq != 43 {
		t.Fatalf("signing calls=%d nonceSeq=%d, want two reconciliations plus three offer attempts ending at 43", len(h.signer.hashes), h.solver.nonceSeq)
	}
	gotEvents := strings.Join(events, "\n")
	gotLogs := strings.Join(logs, "\n")
	const wantEvents = `GET /v1/auction?domain=true
GET /v1/offer?chainId=1.1155111e+07&deadline=<dynamic>&maker=0x00000000000000000000000000000000000000a1 auth=Bearer <signature>
GET /v1/offer?chainId=1.1155111e+07&deadline=<dynamic>&maker=0x00000000000000000000000000000000000000b2 auth=Bearer <signature>
POST /v1/offer {"amount":"100","auctionId":10,"chainId":11155111,"expectedReturn":"1","expiration":"4102452000","maker":"0x00000000000000000000000000000000000000a1","nonce":"42","signature":"0xc6ef74ff328d42536fd4c77538e5b24333b88c1840313b791ac714fce34a22d918ca8dc74d29167f7a7ae7dfb1cb92657e92a481504b810dcfabea51e9cff58b1c","useCallback":true}

POST /v1/offer {"amount":"100","auctionId":10,"chainId":11155111,"expectedReturn":"1","expiration":"4102452000","maker":"0x00000000000000000000000000000000000000a1","nonce":"43","signature":"0x62e5b903510ae5496544a44bbf0ed550d4ac14cace8f9542bcc893ca96fadde20a79d1a1a62f32cb62ade96fde363da78d1f9f41aaf81d966008e7147d7d21601b","useCallback":true}
`
	const wantLogs = `{"count":1,"level":1,"logger":"","msg":"discovered auctions"}
{"adapter":"0x00000000000000000000000000000000000000A1","fundable":"1000","level":1,"logger":"","maxAssets":"1000","minAssets":"0","minYieldPpm":"0","msg":"adapter liquidity","openRequests":0}
{"adapter":"0x00000000000000000000000000000000000000b2","fundable":"1000","level":1,"logger":"","maxAssets":"1000","minAssets":"0","minYieldPpm":"0","msg":"adapter liquidity","openRequests":0}
{"error":"auction 999 not found","logger":"","msg":"offer: build"}
{"auctionId":10,"error":"offer for adapter 0x00000000000000000000000000000000000000C3 absent from this pass's snapshot","logger":"","msg":"offer: unknown maker; skipping"}
{"adapter":"0x00000000000000000000000000000000000000A1","auctionId":10,"error":"invalid offer amounts (must be positive): principal=\u003cnil\u003e expectedReturn=1","logger":"","msg":"offer: yield out of bounds; skipping"}
{"adapter":"0x00000000000000000000000000000000000000A1","auctionId":10,"error":"sign offer: injected offer signing failure","logger":"","msg":"offer: build"}
{"adapter":"0x00000000000000000000000000000000000000A1","auctionId":10,"error":"3f api: create offer: 500 Internal Server Error: submit failed: 500 Internal Server Error","logger":"","msg":"offer: submit"}
{"adapter":"0x00000000000000000000000000000000000000A1","auctionId":10,"expectedReturn":"1","level":0,"logger":"","msg":"offer submitted","principal":"100","request":"0x2222222222222222222222222222222222222222","strategyReason":"success-after-errors"}`
	if gotEvents != wantEvents || gotLogs != wantLogs {
		t.Fatalf("discoverAndOffer continuation trace changed:\n-EVENTS-\n%s\n-LOGS-\n%s", gotEvents, gotLogs)
	}
	if len(h.solver.offers.offers) != 0 {
		t.Fatalf("successful submission inserted local tracker entries: %+v", h.solver.offers.offers)
	}
}

// Test3FR1DiscoverAndOfferLaneStopTrace pins the final readiness recheck and abandonment of later output.
func Test3FR1DiscoverAndOfferLaneStopTrace(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	readiness := []bool{true, true, false}
	laneReady := func() bool {
		mu.Lock()
		defer mu.Unlock()
		if len(readiness) == 0 {
			t.Error("unexpected extra lane readiness check")
			return false
		}
		ready := readiness[0]
		readiness = readiness[1:]
		return ready
	}
	auction := contractAuctionDTO(20, "0x1111111111111111111111111111111111111111", "0x3333333333333333333333333333333333333333")
	auction.SetSolveStartTime("2100-01-01T00:00:00Z")
	maker := common.HexToAddress("0x00000000000000000000000000000000000000A1")
	request := common.HexToAddress("0x2222222222222222222222222222222222222222")
	h := newContractExecutionHarness(t, []threef.AuctionDto{auction}, OfferOutput{Offers: []OfferExecution{
		contractExecution(20, request, maker, 100, "before-pause"),
		contractExecution(20, request, maker, 100, "after-pause"),
		contractExecution(20, request, maker, 100, "never-built"),
	}}, laneReady, nil, nil)
	defer h.stop()

	h.solver.discoverAndOffer(t.Context())
	_, logs, posts := h.trace.snapshot()
	if len(posts) != 1 || !strings.Contains(posts[0], `"nonce":"41"`) || h.solver.nonceSeq != 42 {
		t.Fatalf("lane-stop posts=%v nonceSeq=%d, want first submitted and second built before stop", posts, h.solver.nonceSeq)
	}
	mu.Lock()
	remainingChecks := len(readiness)
	mu.Unlock()
	if remainingChecks != 0 {
		t.Fatalf("unused lane readiness results = %d", remainingChecks)
	}
	const wantTail = `{"level":1,"logger":"","msg":"stopping offer submission: transaction lane no longer ready"}`
	if len(logs) == 0 || logs[len(logs)-1] != wantTail {
		t.Fatalf("lane-stop log tail = %v, want %s", logs, wantTail)
	}
	if len(h.solver.offers.offers) != 0 {
		t.Fatalf("successful submission inserted local tracker entries: %+v", h.solver.offers.offers)
	}
}

var _ Planner = (*contractPlanner)(nil)
