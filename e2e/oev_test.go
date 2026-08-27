//go:build e2e

package e2e

import (
	"context"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"

	erc20binding "github.com/symbioticfi/vault-solver/api/bindings/erc20"
	adapterbinding "github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	callbackbinding "github.com/symbioticfi/vault-solver/api/bindings/oev/callback"
	morphobinding "github.com/symbioticfi/vault-solver/api/bindings/oev/morpho"
)

type oevScenario struct {
	MarketID           common.Hash    `json:"marketId"`
	Borrower           common.Address `json:"borrower"`
	Collateral         string         `json:"collateral"`
	BorrowShares       string         `json:"borrowShares"`
	HealthyBorrowLimit string         `json:"healthyBorrowLimit"`
	AuctionBorrowLimit string         `json:"auctionBorrowLimit"`
	AuctionPrice       string         `json:"auctionPrice"`
}

type oevSolve struct {
	Data oevSolveData `json:"data"`
}

type oevSolveData struct {
	Bid               string         `json:"bid"`
	OperationCallback common.Address `json:"operationCallback"`
	OperationData     string         `json:"operationData"`
}

type oevAuction struct {
	ID     string   `json:"id"`
	Status string   `json:"status"`
	TxHash string   `json:"txHash"`
	Solve  oevSolve `json:"solve"`
	Error  string   `json:"error"`
}

type oevOperationAuth struct {
	AuctionKey      [32]byte
	BidAmount       *big.Int
	MinBundleProfit *big.Int
	Deadline        *big.Int
}

type oevOperationLeg struct {
	MarketId       [32]byte
	Borrower       common.Address
	MaxSeizeAssets *big.Int
	MinProfit      *big.Int
}

type oevOperationData struct {
	Auth    oevOperationAuth
	Legs    []oevOperationLeg
	AuthSig []byte
}

type oevSizingSnapshot struct {
	market             morphobinding.MarketOutput
	position           morphobinding.PositionOutput
	params             morphobinding.IdToMarketParamsOutput
	marketConfig       oevMarket
	maxAssets          *big.Int
	maxRate            *big.Int
	collateralDecimals uint8
	loanDecimals       uint8
}

func testOEV(t *testing.T, testEnv *testEnvironment) {
	t.Helper()
	if testEnv.variant != "protocol" {
		t.Fatalf("redstoneoev variant = %q, want protocol", testEnv.variant)
	}
	manifest := testEnv.manifest.OEV
	if manifest.Executor == (common.Address{}) || manifest.Callback == (common.Address{}) || len(manifest.Markets) == 0 {
		t.Fatal("OEV deployment manifest is incomplete")
	}

	var health struct {
		Clients int `json:"clients"`
	}
	eventually(t, "OEV solver websocket", 90*time.Second, 2*time.Second, func() error {
		status := testEnv.getJSON(t, testEnv.fixtureURL+"/health", &health)
		if status != http.StatusOK || health.Clients <= 0 {
			return errors.Errorf("health status=%d clients=%d", status, health.Clients)
		}
		return nil
	})
	verifyOEVGraphQL(t, testEnv, len(manifest.Markets))
	metricsBefore := getMetrics(t, testEnv)

	var scenarioResponse struct {
		Scenario oevScenario `json:"scenario"`
	}
	status := testEnv.postJSON(t, testEnv.fixtureURL+"/scenario/prepare-liquidatable", map[string]string{
		"marketId": manifest.Markets[0].ID.Hex(),
	}, &scenarioResponse)
	if status != http.StatusOK {
		t.Fatalf("prepare OEV scenario status = %d", status)
	}
	scenario := scenarioResponse.Scenario
	borrowShares := parseBig(t, scenario.BorrowShares)
	healthyLimit := parseBig(t, scenario.HealthyBorrowLimit)
	auctionLimit := parseBig(t, scenario.AuctionBorrowLimit)
	if borrowShares.Cmp(auctionLimit) <= 0 || borrowShares.Cmp(healthyLimit) >= 0 {
		t.Fatalf("OEV scenario borrow=%s auctionLimit=%s healthyLimit=%s", borrowShares, auctionLimit, healthyLimit)
	}
	time.Sleep(12 * time.Second)

	sizing := captureOEVSizing(t, testEnv, scenario)
	var created oevCreatedResponse
	status = testEnv.postJSON(t, testEnv.fixtureURL+"/auction", map[string]any{
		"marketIds": []string{scenario.MarketID.Hex()},
	}, &created)
	if status != http.StatusOK || created.Auction.ID == "" {
		t.Fatalf("create OEV auction status = %d, id = %q", status, created.Auction.ID)
	}

	var settled oevAuction
	eventually(t, "settled OEV auction", 90*time.Second, 2*time.Second, func() error {
		var state struct {
			Auctions []oevAuction `json:"auctions"`
		}
		stateStatus := testEnv.getJSON(t, testEnv.fixtureURL+"/state", &state)
		if stateStatus != http.StatusOK {
			return errors.Errorf("state status %d", stateStatus)
		}
		for _, auction := range state.Auctions {
			if auction.ID != created.Auction.ID {
				continue
			}
			if auction.Status == "failed" {
				return errors.Errorf("auction failed: %s", auction.Error)
			}
			if auction.Status != "settled" || auction.TxHash == "" || common.HexToHash(auction.TxHash) == (common.Hash{}) {
				return errors.Errorf("auction status %q tx %q", auction.Status, auction.TxHash)
			}
			settled = auction
			return nil
		}
		return errors.New("auction missing from state")
	})

	mathResult := verifyOEVSettlement(t, testEnv, scenario, sizing, settled)
	verifyOEVMetrics(t, testEnv, metricsBefore)
	t.Logf(
		"OEV flow auction=%s tx=%s bid=%s seized=%s repaid=%s profit=%s",
		settled.ID,
		settled.TxHash,
		mathResult.bid,
		mathResult.seized,
		mathResult.repaid,
		mathResult.profit,
	)
}

type oevCreatedResponse struct {
	Auction oevCreatedAuction `json:"auction"`
}

type oevCreatedAuction struct {
	ID string `json:"id"`
}

type oevGraphQLResponse struct {
	Data oevGraphQLData `json:"data"`
}

type oevGraphQLData struct {
	Markets oevGraphQLMarkets `json:"markets"`
}

type oevGraphQLMarkets struct {
	Items []oevGraphQLMarket `json:"items"`
}

type oevGraphQLMarket struct {
	MarketID string `json:"marketId"`
}

func verifyOEVGraphQL(t *testing.T, testEnv *testEnvironment, marketCount int) {
	t.Helper()
	var response oevGraphQLResponse
	status := testEnv.postJSON(t, testEnv.fixtureURL+"/graphql", map[string]any{
		"operationName": "MorphoDiscoverMarkets",
		"variables": map[string]any{
			"loan":   []string{testEnv.manifest.OEV.LoanToken.Hex()},
			"coll":   collateralAddresses(testEnv.manifest.OEV.Markets),
			"chains": []int64{testEnv.manifest.Chain.ID},
			"first":  100,
		},
		"query": "query MorphoDiscoverMarkets { markets { items { marketId } } }",
	}, &response)
	if status != http.StatusOK || len(response.Data.Markets.Items) < marketCount {
		t.Fatalf("OEV GraphQL status=%d markets=%d, want at least %d", status, len(response.Data.Markets.Items), marketCount)
	}
}

func captureOEVSizing(t *testing.T, testEnv *testEnvironment, scenario oevScenario) oevSizingSnapshot {
	t.Helper()
	var marketConfig oevMarket
	found := false
	for _, candidate := range testEnv.manifest.OEV.Markets {
		if candidate.ID == scenario.MarketID {
			marketConfig, found = candidate, true
			break
		}
	}
	if !found {
		t.Fatalf("OEV scenario market %s is absent from manifest", scenario.MarketID)
	}
	morpho := morphobinding.NewMorpho()
	market, err := morpho.UnpackMarket(testEnv.call(t, testEnv.manifest.OEV.Morpho, morpho.PackMarket(scenario.MarketID)))
	if err != nil {
		t.Fatalf("decode Morpho market: %v", err)
	}
	position, err := morpho.UnpackPosition(
		testEnv.call(t, testEnv.manifest.OEV.Morpho, morpho.PackPosition(scenario.MarketID, scenario.Borrower)),
	)
	if err != nil {
		t.Fatalf("decode Morpho position: %v", err)
	}
	params, err := morpho.UnpackIdToMarketParams(
		testEnv.call(t, testEnv.manifest.OEV.Morpho, morpho.PackIdToMarketParams(scenario.MarketID)),
	)
	if err != nil {
		t.Fatalf("decode Morpho market params: %v", err)
	}
	if params.CollateralToken != marketConfig.CollateralToken {
		t.Fatalf("OEV collateral = %s, want %s", params.CollateralToken, marketConfig.CollateralToken)
	}
	adapter := adapterbinding.NewLiquidLaneAdapter()
	maxAssets, err := adapter.UnpackGetMaxAssets(
		testEnv.call(t, testEnv.manifest.OEV.Adapter, adapter.PackGetMaxAssets(marketConfig.CollateralToken)),
	)
	if err != nil {
		t.Fatalf("decode OEV max assets: %v", err)
	}
	maxRate := testEnv.adapterMaxRate(t, testEnv.manifest.OEV.Adapter, marketConfig.CollateralToken)
	token := erc20binding.NewERC20()
	collateralDecimals, err := token.UnpackDecimals(
		testEnv.call(t, marketConfig.CollateralToken, token.PackDecimals()),
	)
	if err != nil {
		t.Fatalf("decode collateral decimals: %v", err)
	}
	loanDecimals, err := token.UnpackDecimals(
		testEnv.call(t, testEnv.manifest.OEV.LoanToken, token.PackDecimals()),
	)
	if err != nil {
		t.Fatalf("decode loan decimals: %v", err)
	}
	return oevSizingSnapshot{
		market:             market,
		position:           position,
		params:             params,
		marketConfig:       marketConfig,
		maxAssets:          maxAssets,
		maxRate:            maxRate,
		collateralDecimals: collateralDecimals,
		loanDecimals:       loanDecimals,
	}
}

type oevMathResult struct {
	bid    *big.Int
	seized *big.Int
	repaid *big.Int
	profit *big.Int
}

func verifyOEVSettlement(
	t *testing.T,
	testEnv *testEnvironment,
	scenario oevScenario,
	sizing oevSizingSnapshot,
	auction oevAuction,
) oevMathResult {
	t.Helper()
	operationBytes, err := hexutil.Decode(auction.Solve.Data.OperationData)
	if err != nil || len(operationBytes) == 0 {
		t.Fatalf("decode OEV operation data: %v", err)
	}
	operation := decodeOEVOperation(t, operationBytes)
	expectedAuctionKey := crypto.Keccak256Hash([]byte("id:" + auction.ID))
	bid, ok := parseFixed(auction.Solve.Data.Bid, 18)
	if !ok {
		t.Fatalf("invalid OEV bid %q", auction.Solve.Data.Bid)
	}
	configuredBid, ok := parseFixed(testEnv.manifest.OEV.Bid.BidETH, 18)
	if !ok || bid.Cmp(configuredBid) != 0 || operation.Auth.BidAmount.Cmp(configuredBid) != 0 {
		t.Fatalf("OEV bid wire=%s signed=%s configured=%s", bid, operation.Auth.BidAmount, configuredBid)
	}
	if common.Hash(operation.Auth.AuctionKey) != expectedAuctionKey || auction.Solve.Data.OperationCallback != testEnv.manifest.OEV.Callback {
		t.Fatalf("OEV operation envelope key=%s callback=%s", common.Hash(operation.Auth.AuctionKey), auction.Solve.Data.OperationCallback)
	}
	if len(operation.Legs) != 1 {
		t.Fatalf("OEV operation has %d legs, want 1", len(operation.Legs))
	}
	leg := operation.Legs[0]
	if common.Hash(leg.MarketId) != scenario.MarketID || leg.Borrower != scenario.Borrower {
		t.Fatalf("OEV leg targets market=%s borrower=%s", common.Hash(leg.MarketId), leg.Borrower)
	}

	price := parseBig(t, scenario.AuctionPrice)
	debtClamp := maxSeizeForFullDebt(
		sizing.position.BorrowShares,
		price,
		sizing.params.Lltv,
		sizing.market.TotalBorrowAssets,
		sizing.market.TotalBorrowShares,
	)
	liquidityClamp := collateralForBudget(
		sizing.maxAssets,
		sizing.maxRate,
		sizing.collateralDecimals,
		sizing.loanDecimals,
		testEnv.manifest.OEV.Sizing.SwapHaircutBPS,
	)
	expectedMaxSeize := minBigInt(sizing.position.Collateral, minBigInt(debtClamp, liquidityClamp))
	if leg.MaxSeizeAssets.Cmp(expectedMaxSeize) != 0 {
		t.Fatalf("OEV max seize = %s, want %s (debt=%s liquidity=%s)", leg.MaxSeizeAssets, expectedMaxSeize, debtClamp, liquidityClamp)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	receipt, err := testEnv.client.TransactionReceipt(ctx, common.HexToHash(auction.TxHash))
	if err != nil {
		t.Fatalf("read OEV receipt: %v", err)
	}
	legResult, bundleResult, payBidResult := decodeOEVEvents(t, testEnv.manifest.OEV.Callback, receipt.Logs)
	status := new(big.Int).And(new(big.Int).Rsh(new(big.Int).Set(legResult.Code), 8), big.NewInt(0xff)).Int64()
	reason := new(big.Int).And(new(big.Int).Set(legResult.Code), big.NewInt(0xff)).Int64()
	if status != 1 || reason != 0 || legResult.SeizedAssets.Cmp(expectedMaxSeize) != 0 {
		t.Fatalf("OEV leg result status=%d reason=%d seized=%s want=%s", status, reason, legResult.SeizedAssets, expectedMaxSeize)
	}
	expectedRepaid := repaidAssetsForSeize(
		legResult.SeizedAssets,
		price,
		sizing.params.Lltv,
		sizing.market.TotalBorrowAssets,
		sizing.market.TotalBorrowShares,
	)
	if legResult.RepaidAssets.Cmp(expectedRepaid) != 0 {
		t.Fatalf("OEV repaid assets = %s, want %s", legResult.RepaidAssets, expectedRepaid)
	}
	grossOutput := testEnv.adapterAmountOut(t, testEnv.manifest.OEV.Adapter, sizing.marketConfig.CollateralToken, legResult.SeizedAssets)
	discount := testEnv.adapterMinDiscount(t, testEnv.manifest.OEV.Adapter, sizing.marketConfig.CollateralToken)
	adapterOutput := discountedAmountOut(grossOutput, discount)
	expectedProfit := new(big.Int).Sub(adapterOutput, legResult.RepaidAssets)
	if legResult.ProfitLoan.Cmp(expectedProfit) != 0 || legResult.ProfitLoan.Cmp(leg.MinProfit) < 0 {
		t.Fatalf("OEV profit = %s, want %s and at least %s", legResult.ProfitLoan, expectedProfit, leg.MinProfit)
	}
	if common.Hash(bundleResult.AuctionKey) != expectedAuctionKey ||
		bundleResult.TotalProfitLoan.Cmp(legResult.ProfitLoan) != 0 ||
		bundleResult.MinProfitLoan.Cmp(operation.Auth.MinBundleProfit) != 0 ||
		bundleResult.TotalProfitLoan.Cmp(bundleResult.MinProfitLoan) < 0 || !bundleResult.BidAuthorized {
		t.Fatalf("invalid OEV bundle result: %+v", bundleResult)
	}
	if common.Hash(payBidResult.AuctionKey) != expectedAuctionKey || payBidResult.BidAmount.Cmp(bid) != 0 || !payBidResult.Paid {
		t.Fatalf("invalid OEV bid payment: %+v", payBidResult)
	}
	return oevMathResult{bid: bid, seized: legResult.SeizedAssets, repaid: legResult.RepaidAssets, profit: legResult.ProfitLoan}
}

func decodeOEVOperation(t *testing.T, data []byte) oevOperationData {
	t.Helper()
	tuple, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "auth", Type: "tuple", Components: []abi.ArgumentMarshaling{
			{Name: "auctionKey", Type: "bytes32"},
			{Name: "bidAmount", Type: "uint256"},
			{Name: "minBundleProfit", Type: "uint256"},
			{Name: "deadline", Type: "uint256"},
		}},
		{Name: "legs", Type: "tuple[]", Components: []abi.ArgumentMarshaling{
			{Name: "marketId", Type: "bytes32"},
			{Name: "borrower", Type: "address"},
			{Name: "maxSeizeAssets", Type: "uint256"},
			{Name: "minProfit", Type: "uint256"},
		}},
		{Name: "authSig", Type: "bytes"},
	})
	if err != nil {
		t.Fatalf("construct OEV operation ABI: %v", err)
	}
	values, err := (abi.Arguments{{Type: tuple}}).Unpack(data)
	if err != nil || len(values) != 1 {
		t.Fatalf("decode OEV operation data: %v", err)
	}
	return convertABIValue[oevOperationData](t, values[0])
}

func decodeOEVEvents(
	t *testing.T,
	callback common.Address,
	logs []*types.Log,
) (*callbackbinding.SymbioticOevSolverLegResult, *callbackbinding.SymbioticOevSolverBundleResult, *callbackbinding.SymbioticOevSolverPayBidResult) {
	t.Helper()
	binding := callbackbinding.NewSymbioticOevSolver()
	var leg *callbackbinding.SymbioticOevSolverLegResult
	var bundle *callbackbinding.SymbioticOevSolverBundleResult
	var bid *callbackbinding.SymbioticOevSolverPayBidResult
	for _, entry := range logs {
		if entry.Address != callback || len(entry.Topics) == 0 {
			continue
		}
		if event, err := binding.UnpackLegResultEvent(entry); err == nil {
			if leg != nil {
				t.Fatal("multiple OEV LegResult events")
			}
			leg = event
			continue
		}
		if event, err := binding.UnpackBundleResultEvent(entry); err == nil {
			if bundle != nil {
				t.Fatal("multiple OEV BundleResult events")
			}
			bundle = event
			continue
		}
		if event, err := binding.UnpackPayBidResultEvent(entry); err == nil {
			if bid != nil {
				t.Fatal("multiple OEV PayBidResult events")
			}
			bid = event
		}
	}
	if leg == nil || bundle == nil || bid == nil {
		t.Fatalf("missing OEV events: leg=%t bundle=%t bid=%t", leg != nil, bundle != nil, bid != nil)
	}
	return leg, bundle, bid
}

func verifyOEVMetrics(t *testing.T, testEnv *testEnvironment, baseline string) {
	t.Helper()
	for _, name := range []string{"oev_auctions_total", "oev_bids_total", "oev_wins_total", "oev_hotpath_seconds_count"} {
		initial := metricValue(baseline, name, map[string]string{"strategy": "default"})
		eventually(t, name+" metric", 90*time.Second, 2*time.Second, func() error {
			value := metricValue(getMetrics(t, testEnv), name, map[string]string{"strategy": "default"})
			if value-initial < 1 {
				return errors.Errorf("metric delta is %v", value-initial)
			}
			return nil
		})
	}
	metrics := getMetrics(t, testEnv)
	if metricValue(metrics, "oev_deposit_wei", map[string]string{"strategy": "default"}) <= 0 ||
		metricValue(metrics, "oev_deposit_below_floor", map[string]string{"strategy": "default"}) != 0 {
		t.Fatal("OEV deposit metrics are unhealthy")
	}
}

func getMetrics(t *testing.T, testEnv *testEnvironment) string {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testEnv.metricsURL+"/metrics", nil)
	if err != nil {
		t.Fatalf("build metrics request: %v", err)
	}
	response, err := testEnv.httpClient.Do(request)
	if err != nil {
		t.Fatalf("get metrics: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("read metrics status=%d: %v", response.StatusCode, err)
	}
	return string(body)
}

func metricValue(text, name string, labels map[string]string) float64 {
	var total float64
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, name) || len(line) <= len(name) || (line[len(name)] != '{' && line[len(name)] != ' ') {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || !metricLabelsMatch(fields[0], labels) {
			continue
		}
		value, err := strconv.ParseFloat(fields[1], 64)
		if err == nil {
			total += value
		}
	}
	return total
}

func metricLabelsMatch(series string, wanted map[string]string) bool {
	start := strings.IndexByte(series, '{')
	if start < 0 {
		return len(wanted) == 0
	}
	labelText := strings.TrimSuffix(series[start+1:], "}")
	found := make(map[string]string)
	for _, pair := range strings.Split(labelText, ",") {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			value, err := strconv.Unquote(parts[1])
			if err == nil {
				found[parts[0]] = value
			}
		}
	}
	for key, value := range wanted {
		if found[key] != value {
			return false
		}
	}
	return true
}

func collateralAddresses(markets []oevMarket) []string {
	addresses := make([]string, len(markets))
	for index, market := range markets {
		addresses[index] = market.CollateralToken.Hex()
	}
	return addresses
}
