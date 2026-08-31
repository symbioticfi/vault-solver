package txmanager

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
)

func TestMaxFeePerGasMatchesSendFeePolicy(t *testing.T) {
	b := newMockBackend()
	m := newTestManager(t, b)

	fee, err := m.MaxFeePerGas(context.Background())
	if err != nil {
		t.Fatalf("MaxFeePerGas: %v", err)
	}
	if fee.String() != "46125000000" {
		t.Fatalf("max fee = %s, want one-replacement ceiling 46125000000", fee)
	}
}

func TestTipGweiFloorsNodeSuggestionWithoutBreakingFeeCap(t *testing.T) {
	tests := map[string]struct {
		tip     *big.Int
		tipErr  error
		wantTip int64
	}{
		"low suggestion":         {tip: big.NewInt(1_500), wantTip: 1_000_000_000},
		"higher suggestion":      {tip: big.NewInt(2_000_000_000), wantTip: 2_000_000_000},
		"suggestion above cap":   {tip: big.NewInt(30_000_000_000), wantTip: 20_500_000_000},
		"suggestion unavailable": {tipErr: context.DeadlineExceeded, wantTip: 1_000_000_000},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			b := newMockBackend()
			b.tip, b.tipErr = test.tip, test.tipErr
			m := New(b, mustSigner(t), big.NewInt(11155111), Config{TipGwei: 1}, logr.Discard())
			limit := big.NewInt(40_500_000_000)

			fees, err := m.currentFees(t.Context(), limit)
			if err != nil {
				t.Fatalf("currentFees: %v", err)
			}
			if fees.tip.Cmp(big.NewInt(test.wantTip)) != 0 {
				t.Fatalf("tip = %s, want %d", fees.tip, test.wantTip)
			}
			if fees.maxFee.Cmp(limit) != 0 {
				t.Fatalf("max fee = %s, want hard cap %s", fees.maxFee, limit)
			}
		})
	}
}

func TestTipGweiZeroUsesEtherscanFastFeeHistoryPolicy(t *testing.T) {
	b := newMockBackend()
	b.history = &ethereum.FeeHistory{Reward: [][]*big.Int{
		{big.NewInt(3_000_000_000)},
		{big.NewInt(500_000_000)},
		{big.NewInt(2_000_000_000)},
		{big.NewInt(1_000_000_000)},
		{big.NewInt(1_500_000_000)},
	}}
	b.tip = big.NewInt(1_500)
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{}, logr.Discard())
	limit := big.NewInt(40_500_000_000)

	fees, err := m.currentFees(t.Context(), limit)
	if err != nil {
		t.Fatalf("currentFees: %v", err)
	}
	if want := big.NewInt(500_000_000); fees.tip.Cmp(want) != 0 {
		t.Fatalf("tip = %s, want minimum p25 reward %s", fees.tip, want)
	}
	if b.historyReq.blocks != 5 || b.historyReq.newest != nil ||
		len(b.historyReq.percentiles) != 1 || b.historyReq.percentiles[0] != 25.0 {
		t.Fatalf(
			"fee history request = blocks %d, newest %v, percentiles %v; want 5, latest, [25]",
			b.historyReq.blocks, b.historyReq.newest, b.historyReq.percentiles,
		)
	}
	if fees.maxFee.Cmp(limit) != 0 {
		t.Fatalf("max fee = %s, want hard cap %s", fees.maxFee, limit)
	}
	b.history.Reward[0][0] = new(big.Int)
	fees, err = m.currentFees(t.Context(), limit)
	if err != nil {
		t.Fatalf("currentFees with zero reward: %v", err)
	}
	if fees.tip.Sign() != 0 {
		t.Fatalf("tip = %s, want zero minimum reward", fees.tip)
	}
	b.history = constantFeeHistory(big.NewInt(30_000_000_000))
	fees, err = m.currentFees(t.Context(), limit)
	if err != nil {
		t.Fatalf("currentFees with reward above cap: %v", err)
	}
	if want := big.NewInt(20_500_000_000); fees.tip.Cmp(want) != 0 {
		t.Fatalf("tip = %s, want reward clamped to %s", fees.tip, want)
	}
	b.history.Reward = b.history.Reward[:feeHistoryBlocks-1]
	if _, err := m.currentFees(t.Context(), limit); !errors.Is(err, errFreshFeesUnavailable) {
		t.Fatalf("short fee history error = %v, want fresh-fees error", err)
	}
	b.historyErr = errors.New("fee history unavailable")
	if _, err := m.currentFees(t.Context(), limit); !errors.Is(err, errFreshFeesUnavailable) {
		t.Fatalf("history error = %v, want fresh-fees error", err)
	}
	if b.tipCalls != 0 {
		t.Fatalf("node suggestion called %d times", b.tipCalls)
	}
}

func TestCurrentFeesRejectConfiguredFloorAboveFeeHeadroom(t *testing.T) {
	b := newMockBackend()
	b.tip = big.NewInt(30_000_000_000)
	m := New(b, mustSigner(t), big.NewInt(11155111), Config{TipGwei: 21}, logr.Discard())

	_, err := m.currentFees(t.Context(), big.NewInt(40_500_000_000))
	if err == nil || !strings.Contains(err.Error(), "priority fee floor") {
		t.Fatalf("currentFees error = %v, want configured-floor error", err)
	}
}

func TestMaxFeeGweiRejectsCurrentBaseFeeAboveCap(t *testing.T) {
	b := newMockBackend()
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{MaxFeeGwei: 10, PollInterval: time.Millisecond},
		logr.Discard(),
	)
	if _, err := m.MaxFeePerGas(t.Context()); err == nil {
		t.Fatal("expected max fee cap below current base fee to fail")
	}
}

func TestValidateFeeHeadroom(t *testing.T) {
	tests := []struct {
		name    string
		maxFee  float64
		tip     float64
		wantErr bool
	}{
		{name: "automatic tip", maxFee: 50},
		{name: "floor one wei below reserved cap", maxFee: 50, tip: 39.506172838},
		{name: "floor equals reserved cap", maxFee: 50, tip: 39.506172839, wantErr: true},
		{name: "floor one wei above reserved cap", maxFee: 50, tip: 39.506172840, wantErr: true},
		{name: "reported invalid configuration", maxFee: 50, tip: 40, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := &Manager{cfg: Config{MaxFeeGwei: test.maxFee, TipGwei: test.tip}}
			err := m.ValidateFeeHeadroom()
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateFeeHeadroom() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestSend_ReservesReplacementHeadroomInsideRequestCap(t *testing.T) {
	b := newMockBackend()
	m := newTestManager(t, b)

	res := m.Send(context.Background(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "capped",
		MaxFeePerGas: big.NewInt(40_000_000_000),
	})
	if res.Err != nil {
		t.Fatalf("send: %v", res.Err)
	}
	tx := b.lastSent()
	if tx == nil {
		t.Fatal("no transaction sent")
	}
	wantInitialCap := reserveFeeBump(big.NewInt(40_000_000_000))
	if tx.GasFeeCap().Cmp(wantInitialCap) != 0 {
		t.Fatalf("gas fee cap = %s, want replacement-reserved cap %s", tx.GasFeeCap(), wantInitialCap)
	}
	if tx.GasTipCap().Cmp(big.NewInt(1_000_000_000)) != 0 {
		t.Fatalf("gas tip cap = %s, want 1000000000", tx.GasTipCap())
	}
}

func TestSend_RejectsRequestCapWithoutReplacementHeadroom(t *testing.T) {
	b := newMockBackend()
	m := newTestManager(t, b)

	res := m.Send(context.Background(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "capped",
		MaxFeePerGas: big.NewInt(20_500_000_000),
	})
	if res.Err == nil {
		t.Fatal("expected request cap without replacement headroom to fail")
	}
	if res.NotAdmitted {
		t.Fatal("fee failure was classified as a manager admission failure")
	}
	if tx := b.lastSent(); tx != nil {
		t.Fatalf("underfunded request sent transaction %s", tx.Hash())
	}
}
