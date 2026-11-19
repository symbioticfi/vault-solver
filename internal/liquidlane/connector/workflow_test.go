package connector

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector/connectortest"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

var outerSelector = [4]byte{0xaa, 0xbb, 0xcc, 0xdd}

func testWorkflow(t *testing.T) (*Workflow, connectortest.Fixture) {
	t.Helper()
	terms, err := ParseDiscount(connectortest.DiscountFixture(), *connectortest.DiscountID(), vlt, tIn)
	if err != nil {
		t.Fatal(err)
	}
	filler := common.HexToAddress("0x10")
	f := connectortest.NewFixture(t, terms.Swap, terms.Signature, common.HexToAddress("0x43"), filler, outerSelector)
	return &Workflow{Chain: f.Chain, ChainID: big.NewInt(1), Source: f.Permissions, Filler: filler, Signer: f.Chain.Signer}, f
}

func testRequest(amountOut *big.Int) Request {
	return Request{
		Venue: vlt, Executor: common.HexToAddress("0x43"), DiscountID: *connectortest.DiscountID(), MaxAssets: big.NewInt(900),
		TokenIn: tIn, AmountIn: big.NewInt(123), TokenOut: tOut, AmountOut: amountOut,
	}
}

func resolveFixture(context.Context, string) (*discounts.Resolved, error) {
	return connectortest.DiscountFixture(), nil
}

func recipeInputs(t *testing.T, calldata []byte) []*big.Int {
	t.Helper()
	values, err := recipeBinding.GetABI().Methods["execute"].Inputs.Unpack(calldata[4:])
	if err != nil {
		t.Fatal(err)
	}
	return values[2].([]*big.Int)
}

func TestPrepareTakesConnectorPayment(t *testing.T) {
	for _, tc := range []struct {
		name      string
		gross     *big.Int
		remaining int64
		maxAssets int64
		want      int64
		wantErr   string
	}{
		// The fixture discount is 2%: 500 * (1e8 - 2e6) / 1e8.
		{name: "discounted oracle amount", gross: big.NewInt(500), remaining: 900, maxAssets: 900, want: 490},
		{name: "floored like the contract", gross: big.NewInt(101), remaining: 900, maxAssets: 900, want: 98},
		{name: "oracle unreadable", remaining: 900, maxAssets: 900, wantErr: "getAmountOut"},
		{name: "pays nothing", gross: big.NewInt(1), remaining: 900, maxAssets: 900, wantErr: "pays nothing"},
		{name: "above policy budget", gross: big.NewInt(500), remaining: 489, maxAssets: 900, wantErr: "whole award"},
		{name: "above advertised capacity", gross: big.NewInt(500), remaining: 900, maxAssets: 489, wantErr: "whole award"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, f := testWorkflow(t)
			f.Chain.AmountOut, f.Chain.Remaining = tc.gross, big.NewInt(tc.remaining)
			request := testRequest(nil)
			request.MaxAssets = big.NewInt(tc.maxAssets)
			legs, err := w.PrepareLegs(t.Context(), []Request{request}, resolveFixture)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %s", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if inputs := recipeInputs(t, legs[0].Call.Data); legs[0].AmountOut.Int64() != tc.want || inputs[1].Int64() != tc.want {
				t.Fatalf("payment = %s, recipe input = %s, want %d", legs[0].AmountOut, inputs[1], tc.want)
			}
		})
	}
}
