//go:build live

package redstoneoev

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-logr/logr"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"

	callbackbinding "github.com/symbioticfi/vault-solver/api/bindings/oev/callback"
	"github.com/symbioticfi/vault-solver/internal/chain"

	appconfig "github.com/symbioticfi/vault-solver/internal/config"
	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/solver"

	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/default"
)

type forkPayload struct {
	Callback          string `json:"callback"`
	Executor          string `json:"executor"`
	Signer            string `json:"signer"`
	AuctionID         string `json:"auctionId"`
	BidWei            string `json:"bidWei"`
	Nonce             string `json:"nonce"`
	MaxTxGasPrice     string `json:"maxTxGasPrice"`
	OperationData     string `json:"operationData"`
	LiquidationSig    string `json:"liquidationSig"`
	LiquidateCalldata string `json:"liquidateCalldata"`
	PayBidCalldata    string `json:"payBidCalldata"`
}

// TestLiveSepoliaDumpForkPayload writes /tmp/oev-fork-payload.json for an anvil-fork settlement replay.
//
//	set -a; . ./.env.local; set +a
//	OEV_TEST_MONITOR=true OEV_TEST_MARKETS=... OEV_TEST_POSITIONS=... \
//	go test -tags live ./internal/solvers/redstoneoev -run TestLiveSepoliaDumpForkPayload -v
func TestLiveSepoliaDumpForkPayload(t *testing.T) {
	if os.Getenv("ETH_RPC_URL_SEPOLIA") == "" || os.Getenv("OEV_SIGNER_PRIVATE_KEY") == "" {
		t.Skip("set ETH_RPC_URL_SEPOLIA and OEV_SIGNER_PRIVATE_KEY to dump a live Sepolia fork payload")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfgPath := getenvDefault("OEV_CONFIG", "../../../config/redstone-oev.example.yaml")
	cfg, err := appconfig.Load(cfgPath)
	testcheck.NoError(t, err, "load config: %v")
	if len(cfg.Solvers) != 1 || cfg.Solvers[0].Name != Name {
		t.Fatalf("expected single %s solver in %s", Name, cfgPath)
	}
	chainClient, err := chain.Dial(ctx, []string{cfg.Chain.RPCURL}, "", cfg.Chain.MulticallAddress, logr.Discard())
	testcheck.NoError(t, err, "dial chain: %v")
	defer chainClient.Close()
	sgnr, err := signer.FromConfig(cfg.Signer)
	testcheck.NoError(t, err, "load signer: %v")
	built, err := New(cfg.Solvers[0].Config, solver.Deps{Chain: chainClient, Signer: sgnr, Log: logr.Discard()})
	testcheck.NoError(t, err, "build solver: %v")
	s, ok := built.(*Solver)
	if !ok {
		t.Fatalf("unexpected solver type %T", built)
	}
	testcheck.NoError(t, s.refreshState(ctx), "refresh solver state: %v")
	strategy := defaultStrategyOf(t, s)
	go strategy.Run(ctx)
	snap := waitForStrategySnapshot(t, ctx, strategy)

	prices := make(map[string]string, len(snap.Prices))
	for id, price := range snap.Prices {
		oracle := snap.Markets[id].Params.Oracle
		prices[oracle.Hex()] = price.String()
	}
	auction := AuctionMessage{
		Op:        "auction",
		ID:        "fork-debug-" + time.Now().UTC().Format("20060102T150405Z"),
		Timestamp: int64(snap.BlockTime) * 1000,
		Payload:   AuctionPayload{Prices: prices},
	}
	decision := s.buildBid(t.Context(), auction, func() time.Time { return time.Unix(int64(snap.BlockTime), 0) })
	if decision.skip != "" {
		t.Fatalf("buildBid skipped: %s", decision.skip)
	}

	opData, err := hexutil.Decode(decision.solve.Data.OperationData)
	testcheck.NoError(t, err, "decode operationData: %v")
	bid, err := parse.EthToWei(decision.solve.Data.Bid, "solve.bid")
	testcheck.NoError(t, err, "parse bid: %v")
	callbackAddr := common.HexToAddress(decision.solve.Data.OperationCallback)
	callbackABI := callbackbinding.NewSymbioticOevSolver()
	out := forkPayload{
		Callback:          callbackAddr.Hex(),
		Executor:          s.cfg.Executor.Hex(),
		Signer:            sgnr.Address().Hex(),
		AuctionID:         auction.ID,
		BidWei:            bid.String(),
		Nonce:             decision.solve.Data.Nonce,
		MaxTxGasPrice:     decision.solve.Data.MaxTxGasPrice,
		OperationData:     decision.solve.Data.OperationData,
		LiquidationSig:    decision.solve.Data.LiquidationSig,
		LiquidateCalldata: hexutil.Encode(callbackABI.PackLiquidate(bid, sgnr.Address(), opData)),
		PayBidCalldata:    hexutil.Encode(callbackABI.PackPayBid(bid)),
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	testcheck.NoError(t, err, "marshal payload: %v")
	testcheck.NoError(t, os.WriteFile("/tmp/oev-fork-payload.json", raw, 0o600), "write payload: %v")
	t.Logf("wrote /tmp/oev-fork-payload.json: nonce=%s maxTxGasPrice=%s", out.Nonce, out.MaxTxGasPrice)
}

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func waitForStrategySnapshot(t *testing.T, ctx context.Context, strategy *defaultstrategy.Strategy) defaultstrategy.SnapshotSeed {
	t.Helper()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		snap := strategy.SnapshotForTest()
		if len(snap.Prices) > 0 {
			return snap
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait strategy snapshot: %v", ctx.Err())
		case <-tick.C:
		}
	}
}
