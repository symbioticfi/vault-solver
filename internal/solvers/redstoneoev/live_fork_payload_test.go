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
//	# Set strategy.config.testMonitor.markets/positions in $OEV_CONFIG.
//	go test -tags live ./internal/solvers/redstoneoev -run TestLiveSepoliaDumpForkPayload -v
func TestLiveSepoliaDumpForkPayload(t *testing.T) {
	if os.Getenv("ETH_RPC_URL_SEPOLIA") == "" || os.Getenv("OEV_SIGNER_PRIVATE_KEY") == "" {
		t.Skip("set ETH_RPC_URL_SEPOLIA and OEV_SIGNER_PRIVATE_KEY to dump a live Sepolia fork payload")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfgPath := getenvDefault("OEV_CONFIG", "../../../config/redstone-oev.example.yaml")
	cfg, err := appconfig.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Solvers) != 1 || cfg.Solvers[0].Name != Name {
		t.Fatalf("expected single %s solver in %s", Name, cfgPath)
	}
	chainClient, err := chain.Dial(ctx, []string{cfg.Chain.RPCURL}, "", cfg.Chain.MulticallAddress, logr.Discard())
	if err != nil {
		t.Fatalf("dial chain: %v", err)
	}
	defer chainClient.Close()
	sgnr, err := signer.FromConfig(cfg.Signer)
	if err != nil {
		t.Fatalf("load signer: %v", err)
	}
	built, err := factory(cfg.Solvers[0].Config, solver.Deps{Chain: chainClient, Signer: sgnr, Log: logr.Discard()})
	if err != nil {
		t.Fatalf("build solver: %v", err)
	}
	s, ok := built.(*Solver)
	if !ok {
		t.Fatalf("unexpected solver type %T", built)
	}
	if err := s.refreshState(ctx); err != nil {
		t.Fatalf("refresh solver state: %v", err)
	}
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
	if err != nil {
		t.Fatalf("decode operationData: %v", err)
	}
	bid, err := parse.EthToWei(decision.solve.Data.Bid, "solve.bid")
	if err != nil {
		t.Fatalf("parse bid: %v", err)
	}
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
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := os.WriteFile("/tmp/oev-fork-payload.json", raw, 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
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
