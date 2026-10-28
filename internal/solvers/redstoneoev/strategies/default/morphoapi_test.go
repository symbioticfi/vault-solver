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
	Chains []int    `json:"chains"`
	IDs    []string `json:"ids"`
	First  int      `json:"first"`
	Skip   int      `json:"skip"`
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
			{"marketId":"` + apiMktA.Hex() + `","oracle":{"address":"0x1234567890123456789012345678901234567890"}},
			{"marketId":"` + apiMktB.Hex() + `","oracle":{"address":"0x1234567890123456789012345678901234567890"}}
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
			{"marketId":"","oracle":{"address":"0x1234567890123456789012345678901234567890"}},
			{"marketId":"0x0000000000000000000000000000000000000000000000000000000000000000","oracle":{"address":"0x1234567890123456789012345678901234567890"}},
			{"marketId":"` + apiMktA.Hex() + `","oracle":{"address":"0x1234567890123456789012345678901234567890"}}
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
		var request struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal([]byte(gotBody), &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		queryBody := strings.NewReplacer(" ", "", "\n", "", "\t", "").Replace(request.Query)
		if !strings.Contains(queryBody, "chainId_in") || !strings.Contains(gotBody, `"chains":[11155111]`) {
			t.Fatalf("request body missing chainId_in scope: %s", gotBody)
		}
		if strings.Contains(queryBody, "oracleAddress") || !strings.Contains(queryBody, "oracle{address}") {
			t.Fatalf("discovery must use supported oracle selection: %s", request.Query)
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

func TestMorphoClientDiscoveryOracle(t *testing.T) {
	// Public USDC/WETH market: the nested oracle must preserve the on-chain market identity.
	marketID := common.HexToHash("0xf9acc677910cc17f650416a22e2a14d5da7ccb9626db18f1bf94efe64f92b372")
	loan := common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	coll := common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")
	oracle := common.HexToAddress("0xdC6fd5831277c693b1054e19E94047cB37c77615")
	for _, tc := range []struct {
		name        string
		oracleField string
		wantMarkets int
	}{
		{"nested oracle", `"oracle":{"address":"` + oracle.Hex() + `"},`, 1},
		{"null oracle", `"oracle":null,`, 0},
		{"missing oracle", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"data":{"markets":{"items":[{
				"marketId":"` + marketID.Hex() + `",` + tc.oracleField + `
				"irmAddress":"0x870aC11D48B15DB9a138Cf899d20F13F79Ba00BC",
				"lltv":"915000000000000000",
				"loanAsset":{"address":"` + loan.Hex() + `"},
				"collateralAsset":{"address":"` + coll.Hex() + `"},
				"state":{"blockNumber":26242359,"timestamp":1792602347,
					"borrowAssets":15164,"borrowShares":16077813792,
					"supplyAssets":122169,"supplyShares":112160714677,
					"price":"2510457309924490465032091175"}
			}]}}}`
			srv := newJSONServer(t, http.StatusOK, body)
			defer srv.Close()

			got, err := newTestMorphoClient(srv.URL).DiscoverMarketData(t.Context(), 1, []common.Address{loan}, []common.Address{coll})
			if err != nil {
				t.Fatalf("DiscoverMarketData: %v", err)
			}
			if len(got) != tc.wantMarkets {
				t.Fatalf("markets = %+v, want %d", got, tc.wantMarkets)
			}
			if tc.wantMarkets == 0 {
				return
			}
			if got[0].MarketID != marketID || got[0].Oracle != oracle {
				t.Fatalf("market identity changed: %+v", got[0])
			}
			view, ok := marketInfoFromAPI(got[0])
			if !ok {
				t.Fatal("nested oracle market must remain usable by the monitor")
			}
			derived, err := deriveMarketID(view.info.Params)
			if err != nil || derived != marketID || view.info.Params.Oracle != oracle {
				t.Fatalf("market ID/oracle parity: derived=%s oracle=%s err=%v", derived, view.info.Params.Oracle, err)
			}
		})
	}
}

func TestMorphoClientPositions(t *testing.T) {
	borrower := common.HexToAddress("0x629d764eC8563AFA701709B52c1a215e865632dE")
	item := `{
		"user":{"address":"` + borrower.Hex() + `"},
		"market":{"marketId":"` + apiMktA.Hex() + `","chain":{"id":1}},
		"state":{
			"borrowShares":"34",
			"collateral":"56"
		},
		"healthFactor":1.2
	}`

	t.Run("bulk by market", func(t *testing.T) {
		srv := newJSONServer(t, http.StatusOK, `{"data":{"marketPositions":{"items":[`+item+`]}}}`)
		defer srv.Close()
		maxHF := 1.3
		got, err := newTestMorphoClient(srv.URL).PositionsByMarket(context.Background(), 1, []common.Hash{apiMktA}, 10, &maxHF)
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
	})

	t.Run("missing state does not panic", func(t *testing.T) {
		body := `{"data":{"marketPositions":{"items":[{
			"user":{"address":"` + borrower.Hex() + `"},
			"market":{"marketId":"` + apiMktA.Hex() + `","chain":{"id":1}},
			"healthFactor":1.2
		}]}}}`
		srv := newJSONServer(t, http.StatusOK, body)
		defer srv.Close()

		got, err := newTestMorphoClient(srv.URL).PositionsByMarket(context.Background(), 1, []common.Hash{apiMktA}, 10, nil)
		if err != nil {
			t.Fatalf("PositionsByMarket: %v", err)
		}
		if len(got) != 1 || got[0].BorrowShares != "" || got[0].Collateral != "" {
			t.Fatalf("missing state should keep only identity/risk, got %+v", got)
		}
		if _, ok := positionStateFromAPI(got[0]); ok {
			t.Fatal("missing state must fail closed before entering the monitor snapshot")
		}
	})

	t.Run("bulk chunks live API request caps", func(t *testing.T) {
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
		if _, err := newTestMorphoClient(srv.URL).PositionsByMarket(context.Background(), 1, ids, 10_000, nil); err != nil {
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
	})

	t.Run("bulk paginates beyond one API page", func(t *testing.T) {
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
				items[i] = `{"user":{"address":"` + addr + `"},"market":{"marketId":"` + apiMktA.Hex() + `","chain":{"id":1}},"state":{"borrowShares":"1","collateral":"1"},"healthFactor":1.2}`
			}
			_, _ = io.WriteString(w, `{"data":{"marketPositions":{"items":[`+strings.Join(items, ",")+`]}}}`)
		}))
		defer srv.Close()

		got, err := newTestMorphoClient(srv.URL).PositionsByMarket(context.Background(), 1, []common.Hash{apiMktA}, maxPositionsPage+1, nil)
		if err != nil {
			t.Fatalf("PositionsByMarket: %v", err)
		}
		if len(got) != maxPositionsPage+1 {
			t.Fatalf("positions = %d, want %d", len(got), maxPositionsPage+1)
		}
		if len(skips) != 2 || skips[0] != 0 || skips[1] != maxPositionsPage {
			t.Fatalf("skips = %+v, want [0 %d]", skips, maxPositionsPage)
		}
	})

	t.Run("bulk truncates by global risk after market chunks", func(t *testing.T) {
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
			chunkItem := `{"user":{"address":"` + chunkBorrower.Hex() + `"},"market":{"marketId":"` + req.Variables.IDs[0] + `","chain":{"id":1}},"state":{"borrowShares":"1","collateral":"1"},"healthFactor":` + hf + `}`
			_, _ = io.WriteString(w, `{"data":{"marketPositions":{"items":[`+chunkItem+`]}}}`)
		}))
		defer srv.Close()

		ids := make([]common.Hash, maxPositionMarketIDs+1)
		for i := range ids {
			ids[i] = common.BigToHash(big.NewInt(int64(i + 1)))
		}
		got, err := newTestMorphoClient(srv.URL).PositionsByMarket(context.Background(), 1, ids, 1, nil)
		if err != nil {
			t.Fatalf("PositionsByMarket: %v", err)
		}
		if len(got) != 1 || got[0].Borrower != common.HexToAddress("0x0000000000000000000000000000000000000002") {
			t.Fatalf("global top risk was not selected after chunk merge: %+v", got)
		}
	})
}

func TestMorphoClientPositionsChainAndMarketIsolation(t *testing.T) {
	borrower := common.HexToAddress("0x629d764eC8563AFA701709B52c1a215e865632dE")
	var request struct {
		Query     string               `json:"query"`
		Variables positionsGraphQLVars `json:"variables"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		items := []string{
			`{"user":{"address":"` + borrower.Hex() + `"},"market":{"marketId":"` + apiMktA.Hex() + `","chain":{"id":10}},"healthFactor":1.01}`,
			`{"user":{"address":"` + borrower.Hex() + `"},"market":{"marketId":"` + apiMktB.Hex() + `","chain":{"id":1}},"healthFactor":1.02}`,
			`{"user":{"address":"` + borrower.Hex() + `"},"market":{"marketId":"` + apiMktA.Hex() + `"},"healthFactor":1.03}`,
			`{"user":{"address":"` + borrower.Hex() + `"},"market":{"marketId":"` + apiMktA.Hex() + `","chain":{"id":1}},"healthFactor":1.2}`,
		}
		_, _ = io.WriteString(w, `{"data":{"marketPositions":{"items":[`+strings.Join(items, ",")+`]}}}`)
	}))
	defer server.Close()
	got, err := newTestMorphoClient(server.URL).PositionsByMarket(t.Context(), 1, []common.Hash{apiMktA}, 10, nil)
	if err != nil || len(got) != 1 || got[0].HealthFactor == nil || *got[0].HealthFactor != 1.2 {
		t.Fatalf("positions = %+v, err %v, want only requested chain/market", got, err)
	}
	if len(request.Variables.Chains) != 1 || request.Variables.Chains[0] != 1 || !strings.Contains(request.Query, "chainId_in") {
		t.Fatalf("borrower request missing chain scope: %+v", request)
	}
}

func TestMorphoClientForeignRowsCannotExtendPageBudget(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls > 1 {
			http.Error(w, "unexpected page beyond raw row budget", http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"marketPositions":{"items":[{
			"user":{"address":"0x629d764eC8563AFA701709B52c1a215e865632dE"},
			"market":{"marketId":"`+apiMktA.Hex()+`","chain":{"id":10}},"healthFactor":1.1
		}]}}}`)
	}))
	defer server.Close()
	got, err := newTestMorphoClient(server.URL).PositionsByMarket(t.Context(), 1, []common.Hash{apiMktA}, 1, nil)
	if err != nil || len(got) != 0 || calls != 1 {
		t.Fatalf("positions = %+v, err %v, calls %d; want empty result in one bounded page", got, err, calls)
	}
}
