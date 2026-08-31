package defaultstrategy

import (
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// newTestMorphoClient points a morphoClient at the given httptest server.
func newTestMorphoClient(url string) *morphoClient { return newMorphoClient(url) }

// mktA is the market id requested in these tests; mktB is one never requested. The fixtures below mirror
// the LIVE Morpho schema (api.morpho.org/graphql): the output field is `marketId` (not uniqueKey).
var (
	apiMktA = common.HexToHash("0x6209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5")
	apiMktB = common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111")
)

type positionsGraphQLVars struct {
	IDs   []string `json:"ids"`
	First int      `json:"first"`
	Skip  int      `json:"skip"`
}

type positionsGraphQLRequest struct {
	Variables positionsGraphQLVars `json:"variables"`
}

// newJSONServer returns an httptest server replying with a fixed status + JSON body.
func newJSONServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
}

func TestMorphoClientDiscoverMarketData(t *testing.T) {
	loan := common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	coll := common.HexToAddress("0x45804880De22913dAFE09f4980848ECE6EcbAf78")

	t.Run("normal response returns candidate ids", func(t *testing.T) {
		body := `{"data":{"markets":{"items":[
			{"marketId":"` + apiMktA.Hex() + `"},
			{"marketId":"` + apiMktB.Hex() + `"}
		]}}}`
		srv := newJSONServer(t, http.StatusOK, body)
		defer srv.Close()

		got, err := newTestMorphoClient(srv.URL).DiscoverMarketData(context.Background(), 1, []common.Address{loan}, []common.Address{coll})
		if err != nil {
			t.Fatalf("DiscoverMarketData: %v", err)
		}
		if len(got) != 2 || got[0].MarketID != apiMktA || got[1].MarketID != apiMktB {
			t.Fatalf("bad markets: %+v", got)
		}
	})

	t.Run("graphql errors => error", func(t *testing.T) {
		srv := newJSONServer(t, http.StatusOK, `{"errors":[{"message":"bad chain"}]}`)
		defer srv.Close()
		if _, err := newTestMorphoClient(srv.URL).DiscoverMarketData(context.Background(), 1, []common.Address{loan}, []common.Address{coll}); err == nil {
			t.Fatal("expected an error on non-empty graphql errors")
		}
	})

	t.Run("http 500 => error", func(t *testing.T) {
		srv := newJSONServer(t, http.StatusInternalServerError, `{}`)
		defer srv.Close()
		if _, err := newTestMorphoClient(srv.URL).DiscoverMarketData(context.Background(), 1, []common.Address{loan}, []common.Address{coll}); err == nil {
			t.Fatal("expected an error on HTTP 500")
		}
	})

	t.Run("empty/zero marketId skipped, valid kept", func(t *testing.T) {
		body := `{"data":{"markets":{"items":[
			{"marketId":""},
			{"marketId":"0x0000000000000000000000000000000000000000000000000000000000000000"},
			{"marketId":"` + apiMktA.Hex() + `"}
		]}}}`
		srv := newJSONServer(t, http.StatusOK, body)
		defer srv.Close()

		got, err := newTestMorphoClient(srv.URL).DiscoverMarketData(context.Background(), 1, []common.Address{loan}, []common.Address{coll})
		if err != nil {
			t.Fatalf("DiscoverMarketData: %v", err)
		}
		if len(got) != 1 || got[0].MarketID != apiMktA {
			t.Fatalf("want exactly the one valid market, got %+v", got)
		}
	})

	t.Run("request sends lowercased addresses and chainId_in", func(t *testing.T) {
		var gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"markets":{"items":[]}}}`)
		}))
		defer srv.Close()

		if _, err := newTestMorphoClient(srv.URL).DiscoverMarketData(context.Background(), 11155111, []common.Address{loan}, []common.Address{coll}); err != nil {
			t.Fatalf("DiscoverMarketData: %v", err)
		}
		if !strings.Contains(gotBody, strings.ToLower(loan.Hex())) || !strings.Contains(gotBody, strings.ToLower(coll.Hex())) {
			t.Fatalf("request body missing lowercased loan/collateral: %s", gotBody)
		}
		queryBody := strings.NewReplacer(" ", "", "\n", "", "\t", "").Replace(gotBody)
		if !strings.Contains(queryBody, "chainId_in") || !strings.Contains(gotBody, `"chains":[11155111]`) {
			t.Fatalf("request body missing chainId_in scope: %s", gotBody)
		}
	})

	t.Run("empty pair => no call", func(t *testing.T) {
		// A request would dial 127.0.0.1:0 and fail; an empty loan or collateral set must short-circuit.
		api := newMorphoClient("http://127.0.0.1:0")
		if got, err := api.DiscoverMarketData(context.Background(), 1, nil, []common.Address{coll}); err != nil || got != nil {
			t.Fatalf("empty loan: got=%+v err=%v", got, err)
		}
		if got, err := api.DiscoverMarketData(context.Background(), 1, []common.Address{loan}, nil); err != nil || got != nil {
			t.Fatalf("empty collateral: got=%+v err=%v", got, err)
		}
	})
}

func testMorphoPositionsChunkCaps(t *testing.T) {
	var calls []positionsGraphQLVars
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req positionsGraphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		calls = append(calls, req.Variables)
		if len(req.Variables.IDs) > maxPositionMarketIDs || req.Variables.First > maxPositionsPage {
			_, _ = io.WriteString(w, `{"errors":[{"message":"Input validation failed"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"marketPositions":{"items":[]}}}`)
	}))
	defer srv.Close()

	ids := make([]common.Hash, maxPositionMarketIDs+1)
	for i := range ids {
		ids[i] = common.BigToHash(big.NewInt(int64(i + 1)))
	}
	if _, err := newTestMorphoClient(srv.URL).PositionsByMarket(context.Background(), ids, 10_000, nil); err != nil {
		t.Fatalf("PositionsByMarket: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d, want 2 chunks", len(calls))
	}
	if len(calls[0].IDs) != maxPositionMarketIDs || len(calls[1].IDs) != 1 {
		t.Fatalf("bad id chunks: %d/%d", len(calls[0].IDs), len(calls[1].IDs))
	}
	for _, c := range calls {
		if c.First != maxPositionsPage || c.Skip != 0 {
			t.Fatalf("bad page args: %+v", c)
		}
	}
}

func TestMorphoClientPositions(t *testing.T) {
	t.Run("bulk by market", testMorphoPositionsBulkByMarket)
	t.Run("missing state does not panic", testMorphoPositionsMissingState)
	t.Run("bulk chunks live API request caps", testMorphoPositionsChunkCaps)
	t.Run("bulk paginates beyond one API page", testMorphoPositionsPagination)
	t.Run("bulk truncates by global risk after market chunks", testMorphoPositionsGlobalRisk)
}

func morphoPositionFixture() (common.Address, string) {
	borrower := common.HexToAddress("0x629d764eC8563AFA701709B52c1a215e865632dE")
	item := `{
		"user":{"address":"` + borrower.Hex() + `"},
		"market":{"marketId":"` + apiMktA.Hex() + `"},
		"state":{
			"borrowShares":"34",
			"collateral":"56"
		},
		"healthFactor":1.2
	}`
	return borrower, item
}

func testMorphoPositionsBulkByMarket(t *testing.T) {
	borrower, item := morphoPositionFixture()
	srv := newJSONServer(t, http.StatusOK, `{"data":{"marketPositions":{"items":[`+item+`]}}}`)
	defer srv.Close()

	maxHF := 1.3
	got, err := newTestMorphoClient(srv.URL).PositionsByMarket(context.Background(), []common.Hash{apiMktA}, 10, &maxHF)
	if err != nil {
		t.Fatalf("PositionsByMarket: %v", err)
	}
	if len(got) != 1 || got[0].MarketID != apiMktA || got[0].Borrower != borrower {
		t.Fatalf("bad position parse: %+v", got)
	}
	if got[0].HealthFactor == nil || *got[0].HealthFactor != 1.2 ||
		got[0].BorrowShares != "34" || got[0].Collateral != "56" {
		t.Fatalf("bad state parse: %+v", got[0])
	}
}

func testMorphoPositionsMissingState(t *testing.T) {
	borrower, _ := morphoPositionFixture()
	body := `{"data":{"marketPositions":{"items":[{
		"user":{"address":"` + borrower.Hex() + `"},
		"market":{"marketId":"` + apiMktA.Hex() + `"},
		"healthFactor":1.2
	}]}}}`
	srv := newJSONServer(t, http.StatusOK, body)
	defer srv.Close()

	got, err := newTestMorphoClient(srv.URL).PositionsByMarket(context.Background(), []common.Hash{apiMktA}, 10, nil)
	if err != nil {
		t.Fatalf("PositionsByMarket: %v", err)
	}
	if len(got) != 1 || got[0].BorrowShares != "" || got[0].Collateral != "" {
		t.Fatalf("missing state should keep only identity/risk, got %+v", got)
	}
	if _, ok := positionStateFromAPI(got[0]); ok {
		t.Fatal("missing state must fail closed before entering the monitor snapshot")
	}
}

func testMorphoPositionsPagination(t *testing.T) {
	var skips []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req positionsGraphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		skips = append(skips, req.Variables.Skip)
		count := req.Variables.First
		if req.Variables.Skip >= maxPositionsPage {
			count = 1
		}
		items := make([]string, count)
		for i := range items {
			addr := common.BigToAddress(big.NewInt(int64(req.Variables.Skip + i + 1))).Hex()
			items[i] = `{"user":{"address":"` + addr + `"},"market":{"marketId":"` + apiMktA.Hex() + `"},"state":{"borrowShares":"1","collateral":"1"},"healthFactor":1.2}`
		}
		_, _ = io.WriteString(w, `{"data":{"marketPositions":{"items":[`+strings.Join(items, ",")+`]}}}`)
	}))
	defer srv.Close()

	got, err := newTestMorphoClient(srv.URL).PositionsByMarket(context.Background(), []common.Hash{apiMktA}, maxPositionsPage+1, nil)
	if err != nil {
		t.Fatalf("PositionsByMarket: %v", err)
	}
	if len(got) != maxPositionsPage+1 {
		t.Fatalf("positions = %d, want %d", len(got), maxPositionsPage+1)
	}
	if len(skips) != 2 || skips[0] != 0 || skips[1] != maxPositionsPage {
		t.Fatalf("skips = %+v, want [0 %d]", skips, maxPositionsPage)
	}
}

func testMorphoPositionsGlobalRisk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req positionsGraphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		hf := "1.2"
		chunkBorrower := common.HexToAddress("0x0000000000000000000000000000000000000001")
		if len(req.Variables.IDs) == 1 {
			hf = "1.01"
			chunkBorrower = common.HexToAddress("0x0000000000000000000000000000000000000002")
		}
		chunkItem := `{"user":{"address":"` + chunkBorrower.Hex() + `"},"market":{"marketId":"` + apiMktA.Hex() + `"},"state":{"borrowShares":"1","collateral":"1"},"healthFactor":` + hf + `}`
		_, _ = io.WriteString(w, `{"data":{"marketPositions":{"items":[`+chunkItem+`]}}}`)
	}))
	defer srv.Close()

	ids := make([]common.Hash, maxPositionMarketIDs+1)
	for i := range ids {
		ids[i] = common.BigToHash(big.NewInt(int64(i + 1)))
	}
	got, err := newTestMorphoClient(srv.URL).PositionsByMarket(context.Background(), ids, 1, nil)
	if err != nil {
		t.Fatalf("PositionsByMarket: %v", err)
	}
	if len(got) != 1 || got[0].Borrower != common.HexToAddress("0x0000000000000000000000000000000000000002") {
		t.Fatalf("global top risk was not selected after chunk merge: %+v", got)
	}
}
