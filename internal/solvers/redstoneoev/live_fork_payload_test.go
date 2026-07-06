//go:build live

package redstoneoev

import (
	"context"
	"encoding/json"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
	appconfig "github.com/symbioticfi/vault-solver/internal/config"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/solver"
)

type forkPayload struct {
	Callback          string   `json:"callback"`
	Executor          string   `json:"executor"`
	Signer            string   `json:"signer"`
	AuctionID         string   `json:"auctionId"`
	BidWei            string   `json:"bidWei"`
	Nonce             string   `json:"nonce"`
	MaxTxGasPrice     string   `json:"maxTxGasPrice"`
	OperationData     string   `json:"operationData"`
	LiquidationSig    string   `json:"liquidationSig"`
	LiquidateCalldata string   `json:"liquidateCalldata"`
	PayBidCalldata    string   `json:"payBidCalldata"`
	Borrowers         []string `json:"borrowers"`
}

// TestLiveSepoliaDumpForkPayload writes /tmp/oev-fork-payload.json for an anvil-fork settlement replay.
//
//	set -a; . ./.env.local; set +a
//	OEV_TEST_MONITOR=true OEV_ONCHAIN_PRICE_FOR_TEST=true \
//	OEV_TEST_MARKETS=... OEV_TEST_POSITIONS=... \
//	go test -tags live ./internal/solvers/redstoneoev -run TestLiveSepoliaDumpForkPayload -v
func TestLiveSepoliaDumpForkPayload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfgPath := getenvDefault("OEV_CONFIG", "../../../config/redstone-oev.sepolia.example.yaml")
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
	s.refreshState(ctx)
	s.mon.refresh(ctx)
	snap := s.mon.snapshot()
	if snap == nil || len(snap.prices) == 0 {
		t.Fatalf("empty monitor snapshot")
	}

	prices := make(map[string]string, len(snap.prices))
	for id, price := range snap.prices {
		oracle := snap.markets[id].Params.Oracle
		prices[oracle.Hex()] = price.String()
	}
	auction := AuctionMessage{
		Op:        "auction",
		ID:        "fork-debug-" + time.Now().UTC().Format("20060102T150405Z"),
		Timestamp: int64(snap.blockTime) * 1000,
		Payload:   AuctionPayload{Prices: prices},
	}
	decision := s.buildBid(auction, func() time.Time { return time.Unix(int64(snap.blockTime), 0) })
	if decision.skip != "" {
		t.Fatalf("buildBid skipped: %s gross=%s", decision.skip, decision.gross)
	}

	opData, err := hexutil.Decode(decision.solve.Data.OperationData)
	if err != nil {
		t.Fatalf("decode operationData: %v", err)
	}
	bid := new(big.Int).Set(decision.bidNative)
	out := forkPayload{
		Callback:          s.cfg.Callback.Hex(),
		Executor:          s.cfg.Executor.Hex(),
		Signer:            sgnr.Address().Hex(),
		AuctionID:         auction.ID,
		BidWei:            bid.String(),
		Nonce:             decision.solve.Data.Nonce,
		MaxTxGasPrice:     decision.solve.Data.MaxTxGasPrice,
		OperationData:     decision.solve.Data.OperationData,
		LiquidationSig:    decision.solve.Data.LiquidationSig,
		LiquidateCalldata: hexutil.Encode(callbackB.PackLiquidate(bid, sgnr.Address(), opData)),
		PayBidCalldata:    hexutil.Encode(callbackB.PackPayBid(bid)),
		Borrowers:         decision.solve.Data.Borrowers,
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := os.WriteFile("/tmp/oev-fork-payload.json", raw, 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	t.Logf("wrote /tmp/oev-fork-payload.json: nonce=%s legs=%d maxTxGasPrice=%s", out.Nonce, len(out.Borrowers), out.MaxTxGasPrice)
}

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
