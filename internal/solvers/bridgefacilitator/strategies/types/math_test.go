package types

import (
	"math/big"
	"testing"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

func TestExpectedReturn(t *testing.T) {
	// 100,000 USDC (6 decimals) at 200 bps (2%) => 2,000 USDC.
	principal := new(big.Int).SetUint64(100_000_000_000)
	got := ExpectedReturn(principal, 200)
	want := new(big.Int).SetUint64(2_000_000_000)
	if got.Cmp(want) != 0 {
		t.Fatalf("expected %s, got %s", want, got)
	}

	// Exactness at large principals where a big.Float path drifts by 1 wei (18-decimal assets, big amounts).
	exact := []struct {
		principal string
		rateBps   float64
		want      string
	}{
		{"999999999999999999", 1.91, "190999999999999"},             // float path gives 191000000000000
		{"141970357433434898528749", 200, "2839407148668697970574"}, // float path gives ...575
		{"1000000000000000000000000", 3, "300000000000000000000"},   // 1M of an 18-dp token at 3 bps
	}
	for _, c := range exact {
		p, _ := new(big.Int).SetString(c.principal, 10)
		if g := ExpectedReturn(p, c.rateBps); g.String() != c.want {
			t.Fatalf("ExpectedReturn(%s, %g) = %s, want %s", c.principal, c.rateBps, g, c.want)
		}
	}
}

func TestMeetsMinYield(t *testing.T) {
	cases := []struct {
		name      string
		er        int64
		principal int64
		minPpm    int64
		want      bool
	}{
		{"no floor (zero)", 1, 1000, 0, true},
		{"exactly at floor", 190, 1_000_000, 190, true},
		{"above floor", 300, 1_000_000, 190, true},
		{"one wei below floor", 189, 1_000_000, 190, false},
		// Real case: floor(600518648976*1.9/1e4)=114098543 yields 189.9999 ppm, just under 190.
		{"truncated maxRate under floor", 114098543, 600518648976, 190, false},
		{"bumped clears floor", 114098544, 600518648976, 190, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := MeetsMinYield(big.NewInt(c.er), big.NewInt(c.principal), big.NewInt(c.minPpm))
			if got != c.want {
				t.Fatalf("MeetsMinYield(%d, %d, %d) = %v, want %v", c.er, c.principal, c.minPpm, got, c.want)
			}
		})
	}
}

func TestMinYieldReturn(t *testing.T) {
	cases := []struct {
		name      string
		principal int64
		minPpm    int64
		want      int64
	}{
		{"no floor", 1_000_000, 0, 0},
		{"exact multiple", 1_000_000, 190, 190},
		{"rounds up", 600518648976, 190, 114098544}, // ceil(114098543.305)
		{"rounds up 191", 1_000_003, 191, 192},      // ceil(191.000573)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := MinYieldReturn(big.NewInt(c.principal), big.NewInt(c.minPpm))
			if got.Cmp(big.NewInt(c.want)) != 0 {
				t.Fatalf("MinYieldReturn(%d, %d) = %s, want %d", c.principal, c.minPpm, got, c.want)
			}
			// The result must clear the floor it was derived from.
			if c.minPpm > 0 && !MeetsMinYield(got, big.NewInt(c.principal), big.NewInt(c.minPpm)) {
				t.Fatalf("MinYieldReturn(%d, %d) = %s does not clear its own floor", c.principal, c.minPpm, got)
			}
		})
	}
}

func TestValidateYield(t *testing.T) {
	const amount = 600518648976 // ~600.5k USDC; floor 190 ppm → 114098544; maxRate 3 bps = 300 ppm
	principal := big.NewInt(amount)
	big190 := big.NewInt(190)
	partialMinimum := big.NewInt(1_000_000)
	partialSafe := PartialSafeMinYieldReturn(principal, partialMinimum, big190)
	cases := []struct {
		name         string
		er           *big.Int
		amount       *big.Int
		minPrincipal *big.Int
		minPpm       *big.Int
		maxBps       float64
		wantErr      bool
	}{
		{"at floor, full only", big.NewInt(114098544), principal, principal, big190, 3, false},
		{"partial-safe margin", partialSafe, principal, partialMinimum, big190, 3, false},
		{"full yield safe but partial unsafe", big.NewInt(114098544), principal, partialMinimum, big190, 3, true},
		{"principal below minimum", principal, principal, new(big.Int).Add(principal, big.NewInt(1)), big190, 0, true},
		{"one below floor", big.NewInt(114098543), principal, principal, big190, 3, true},
		{"above max rate", big.NewInt(200000000), principal, principal, big190, 3, true},
		{"at exactly max rate", big.NewInt(180155594), principal, principal, big190, 3, false},
		{"nil expectedReturn", nil, principal, principal, big190, 3, true},
		{"zero expectedReturn", big.NewInt(0), principal, principal, big190, 3, true},
		{"zero expectedReturn, no floor no max", big.NewInt(0), principal, principal, big.NewInt(0), 0, true},
		{"nil principal", big.NewInt(1), nil, nil, big190, 3, true},
		{"zero principal", big.NewInt(1), big.NewInt(0), nil, big190, 3, true},
		{"no floor, under max", big.NewInt(1), principal, big.NewInt(1), big.NewInt(0), 3, false},
		{"no maxRate, full only", big.NewInt(114098544), principal, principal, big190, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateYield(c.er, c.amount, c.minPrincipal, c.minPpm, c.maxBps)
			if (err != nil) != c.wantErr {
				t.Fatalf("ValidateYield = %v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}

// prorated is the Request contract's partial-consumption yield: floor(expectedReturn * pt / principal).
func prorated(expectedReturn, principal, pt *big.Int) *big.Int {
	num := new(big.Int).Mul(expectedReturn, pt)
	return num.Quo(num, principal)
}

// requiredYield is the contract's per-consumption floor: ceil(pt * minYieldPpm / 1e6).
func requiredYield(pt, minYieldPpm *big.Int) *big.Int {
	num := new(big.Int).Mul(pt, minYieldPpm)
	num.Add(num, big.NewInt(yieldPpmScale-1))
	return num.Quo(num, big.NewInt(yieldPpmScale))
}

// TestPartialSafeMinYieldReturnMainnetRegression reproduces mainnet tx
// 0xc6374447679d3e4c3cb6df5e680f42d28d0142677a63001ce20a111c85398386: a 30,000.035000 USDC offer priced
// exactly at the 190 ppm floor (5,700,007) was consumed at 29,946.365238 and pro-rated to 5,689,809 —
// one base unit below the required 5,689,810 — reverting TooLowYield.
func TestPartialSafeMinYieldReturnMainnetRegression(t *testing.T) {
	principal := big.NewInt(30_000_035_000)
	consumed := big.NewInt(29_946_365_238)
	floor := big.NewInt(190)

	exact := MinYieldReturn(principal, floor)
	if exact.Int64() != 5_700_007 {
		t.Fatalf("MinYieldReturn = %s, want the on-chain offer's 5700007", exact)
	}
	if got, want := prorated(exact, principal, consumed), requiredYield(consumed, floor); got.Cmp(want) >= 0 {
		t.Fatalf("floor-exact pricing pro-rated to %s >= required %s; the mainnet failure should reproduce", got, want)
	}

	minimum := big.NewInt(1_000_000)
	safe := PartialSafeMinYieldReturn(principal, minimum, floor)
	if want := int64(5_700_007 + 30_001); safe.Int64() != want { // margin = ceil(principal/minimum)
		t.Fatalf("PartialSafeMinYieldReturn = %s, want %d", safe, want)
	}
	if got, want := prorated(safe, principal, consumed), requiredYield(consumed, floor); got.Cmp(want) < 0 {
		t.Fatalf("margined pricing pro-rated to %s < required %s", got, want)
	}
}

func TestPartialSafeMinYieldReturnAnyScaleAnyPpm(t *testing.T) {
	principals := []*big.Int{
		big.NewInt(5),
		big.NewInt(1_000_003),
		big.NewInt(2_000_000),
		big.NewInt(30_000_035_000),
		new(big.Int).Add(chain.Exp10(22), big.NewInt(7)),
	}
	minimums := []*big.Int{
		big.NewInt(1),
		big.NewInt(1_000_000),
		big.NewInt(1_000_000),
		big.NewInt(1_000_000),
		chain.Exp10(18),
	}
	ppms := []*big.Int{big.NewInt(1), big.NewInt(190), big.NewInt(999), big.NewInt(250_000)}

	for i, principal := range principals {
		minimum := minimums[i]
		if minimum.Cmp(principal) > 0 {
			minimum = principal
		}
		for _, ppm := range ppms {
			safe := PartialSafeMinYieldReturn(principal, minimum, ppm)
			if safe.Sign() <= 0 {
				t.Fatalf("P=%s min=%s ppm=%s: no priced return", principal, minimum, ppm)
			}
			samples := []*big.Int{
				new(big.Int).Set(minimum),
				new(big.Int).Add(minimum, big.NewInt(1)),
				new(big.Int).Sub(principal, big.NewInt(1)),
				new(big.Int).Set(principal),
			}
			// Exercise consumptions where pt*ppm is barely above an integer multiple of the ppm scale,
			// making the adapter's rounded-up requirement hardest to satisfy.
			scale := big.NewInt(yieldPpmScale)
			gcd := new(big.Int).GCD(nil, nil, ppm, scale)
			modulus := new(big.Int).Quo(scale, gcd)
			if inverse := new(big.Int).ModInverse(new(big.Int).Quo(ppm, gcd), modulus); inverse != nil {
				first := new(big.Int).Mod(inverse, modulus)
				if first.Cmp(minimum) < 0 {
					steps := new(big.Int).Sub(minimum, first)
					steps.Add(steps, new(big.Int).Sub(modulus, big.NewInt(1))).Quo(steps, modulus)
					first.Add(first, new(big.Int).Mul(steps, modulus))
				}
				samples = append(samples, first, new(big.Int).Add(first, modulus))
			}
			for _, pt := range samples {
				if pt.Sign() <= 0 || pt.Cmp(principal) > 0 {
					continue
				}
				got, want := prorated(safe, principal, pt), requiredYield(pt, ppm)
				if got.Cmp(want) < 0 {
					t.Fatalf("P=%s min=%s ppm=%s pt=%s: pro-rated %s < required %s", principal, minimum, ppm, pt, got, want)
				}
			}
		}
	}

	if got := PartialSafeMinYieldReturn(big.NewInt(1_000_000), big.NewInt(1), nil); got.Sign() != 0 {
		t.Fatalf("no-floor pricing = %s, want 0 (fall back to the auction max rate)", got)
	}
}
