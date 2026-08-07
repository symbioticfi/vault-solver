package defaultstrategy

import (
	"math/big"
	"testing"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/morpho"
)

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
