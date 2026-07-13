package defaultstrategy

import (
	"math/big"
	"testing"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/morpho"
)

func TestComposeLoanPerEth(t *testing.T) {
	cases := []struct {
		name                             string
		ethUsd, loanUsd                  *big.Int
		ethFeedDec, loanFeedDec, loanDec int
		want                             string
	}{
		{"USDC at 2500, 8-dec feeds, 6-dec loan", mustBig("250000000000"), mustBig("100000000"), 8, 8, 6, "2500000000"},
		{"18-dec loan", mustBig("250000000000"), mustBig("100000000"), 8, 8, 18, "2500000000000000000000"},
		{"mixed feed decimals", mustBig("2500000000000000000000"), mustBig("100000000"), 18, 8, 6, "2500000000"},
		{"zero loan price", mustBig("250000000000"), big.NewInt(0), 8, 8, 6, ""},
		{"negative answer", big.NewInt(-1), mustBig("100000000"), 8, 8, 6, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := composeLoanPerEth(c.ethUsd, c.loanUsd, c.ethFeedDec, c.loanFeedDec, c.loanDec)
			if c.want == "" {
				if got != nil {
					t.Fatalf("want nil, got %s", got)
				}
				return
			}
			if got == nil || got.String() != c.want {
				t.Fatalf("got %v, want %s", got, c.want)
			}
		})
	}
}

func TestLegProfitFloorsIncludeFirstSwapOverhead(t *testing.T) {
	rate := new(big.Int).Set(morpho.Wad)
	gasPrice := big.NewInt(1)
	routes := []liquidlanegas.Route{liquidlanegas.RouteAcquire, liquidlanegas.RouteAllocate}
	legs := []selectedLeg{{}, {}}

	got := legsWithProfitFloors(legs, gasPrediction{Routes: routes}, gasPrice, rate)
	if want := liquidlanegas.UnitsForRouteAt(routes[0], true); got[0].MinProfit.Uint64() != want {
		t.Fatalf("first leg floor = %s, want %d", got[0].MinProfit, want)
	}
	if want := liquidlanegas.UnitsForRouteAt(routes[1], false); got[1].MinProfit.Uint64() != want {
		t.Fatalf("additional leg floor = %s, want %d", got[1].MinProfit, want)
	}
}

func TestValidRateAndConversions(t *testing.T) {
	if got := validRate(nil); got != nil {
		t.Fatalf("no cached oracle rate should fail closed, got %v", got)
	}

	rate := mustBig("2500000000")
	if got := validRate(rate); got == nil || got.String() != "2500000000" {
		t.Fatalf("oracle rate present should be used, got %v", got)
	}

	if got := loanToNative(mustBig("2500000000"), mustBig("2500000000")); got.Cmp(morpho.Wad) != 0 {
		t.Fatalf("2500e6 loan at 2500e6/ETH = %s native units, want 1 ETH", got)
	}
	if got := loanToNative(mustBig("1"), nil); got.Sign() != 0 {
		t.Fatalf("nil rate should convert to 0, got %s", got)
	}
	if got := nativeToLoan(morpho.Wad, mustBig("2500000000")); got.String() != "2500000000" {
		t.Fatalf("1 native at 2500e6/native = %s loan units", got)
	}
}
