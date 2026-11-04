package uniswapx

import (
	"context"
	"math/big"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"

	dexbinding "github.com/symbioticfi/vault-solver/api/bindings/uniswapx/dex"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type dexExecutionCaller struct {
	data []byte
	gas  uint64
}

func (c *dexExecutionCaller) CallContract(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	c.data = append([]byte(nil), call.Data...)
	return nil, nil
}

func (c *dexExecutionCaller) EstimateGas(context.Context, ethereum.CallMsg) (uint64, error) {
	return c.gas, nil
}

type dexExecutionTxManager struct{ *executionTestTxManager }

func (*dexExecutionTxManager) MaxFeePerGas(context.Context) (*big.Int, error) {
	return big.NewInt(1_000_000_000), nil
}

func TestDexStartFillReservesBridgeAndSubmitsCompositeCalldata(t *testing.T) {
	for _, mode := range []string{"success", "gas exceeds budget"} {
		t.Run(mode, func(t *testing.T) {
			pricing, input, market := dexPricingFixture(t, 6)
			f := newDirectExecutionFixture(t)
			s := f.solver
			s.cfg.Dex, s.cfg.Gas, s.cfg.Strategy = pricing.cfg.Dex, &liquidlanegas.OracleConfig{}, pricing.cfg.Strategy
			s.strategy, s.dex = pricing.strategy, market
			s.txm = &dexExecutionTxManager{f.txm}
			caller := &dexExecutionCaller{gas: 300_000}
			if mode == "gas exceeds budget" {
				caller.gas = 500_001
			}
			s.chain = caller
			route := input.Inventory[0].Route
			payout := liquidlane.AmountOutForRate(big.NewInt(10_000_000), input.Inventory[0].Price, 6, 6)
			physical := liquidlane.FillQuote{Inventory: input.Inventory[0], AmountIn: big.NewInt(10_000_000),
				GrossAmountOut: payout, MaxAmountOut: payout, MinDiscount: new(big.Int)}
			s.reader = &executionTestReader{now: f.now, snapshot: fillSnapshot{Direct: []liquidlane.FillQuote{physical},
				Physical: []liquidlane.FillQuote{physical}, GasPrices: input.GasPrices}}
			f.order.TokenIn, f.order.TokenOut = input.TokenIn, input.TokenOut
			f.order.AmountIn, f.order.AmountOut = big.NewInt(10_000_000), big.NewInt(5_000_000)
			pending, err := s.startFill(t.Context(), []liquidlane.Route{route}, f.order, f.now, f.now)
			if mode != "success" {
				if err == nil || pending != nil || s.capacity.Len() != 0 || len(f.txm.reqs) != 0 {
					t.Fatalf("bad gas submitted or retained capacity: pending=%v err=%v reservations=%v", pending, err, s.capacity.Snapshot())
				}
				return
			}
			if err != nil || pending == nil || len(f.txm.reqs) != 1 {
				t.Fatalf("pending=%v err=%v requests=%d", pending, err, len(f.txm.reqs))
			}
			if got := s.capacity.Snapshot()[route.CapacityID]; got == nil || got.Cmp(payout) != 0 {
				t.Fatalf("bridge reservation=%v want=%s", got, payout)
			}
			parsed, err := dexbinding.UniswapXDexMetaData.ParseABI()
			if err != nil {
				t.Fatal(err)
			}
			method := parsed.Methods["executeWithDex"]
			values, err := method.Inputs.Unpack(caller.data[4:])
			if err != nil {
				t.Fatal(err)
			}
			call := *abi.ConvertType(values[1], new(dexbinding.ILiquidLaneUniswapXExecutorFillCall)).(*dexbinding.ILiquidLaneUniswapXExecutorFillCall)
			limits := *abi.ConvertType(values[2], new(dexbinding.LiquidLaneUniswapXDexExecutorDexLimits)).(*dexbinding.LiquidLaneUniswapXDexExecutorDexLimits)
			if len(call.Routes) != 1 || limits.MaxBridgePayout.Cmp(payout) != 0 ||
				call.Routes[0].AmountOut.Cmp(new(big.Int).Add(limits.MaxDexInput, limits.MinBridgeResidual)) != 0 {
				t.Fatalf("wrong bridge units/caps: call=%+v limits=%+v", call, limits)
			}
			if f.txm.reqs[0].MaxFeePerGas == nil || f.txm.reqs[0].Deadline.After(time.Now().Add(31*time.Second)) {
				t.Fatal("composite did not retain its fee cap and market expiry")
			}
			f.txm.complete(txmanager.Result{Outcome: txmanager.OutcomeConfirmed})
			s.completePendingFill(t.Context(), pending, <-pending.result)
			if s.capacity.Len() != 0 {
				t.Fatal("composite reservation not released")
			}
		})
	}
}
