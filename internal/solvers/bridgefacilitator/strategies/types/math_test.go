package types

import (
	"math/big"
	"testing"
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
	big190 := big.NewInt(190)
	cases := []struct {
		name    string
		er      *big.Int
		amount  *big.Int
		minPpm  *big.Int
		maxBps  float64
		wantErr bool
	}{
		{"at floor, under max", big.NewInt(114098544), big.NewInt(amount), big190, 3, false},
		{"one below floor", big.NewInt(114098543), big.NewInt(amount), big190, 3, true},
		{"above max rate", big.NewInt(200000000), big.NewInt(amount), big190, 3, true},       // ~333 ppm > 300
		{"at exactly max rate", big.NewInt(180155594), big.NewInt(amount), big190, 3, false}, // floor(180155594*1e6/amount)=300
		{"nil expectedReturn", nil, big.NewInt(amount), big190, 3, true},
		{"zero expectedReturn", big.NewInt(0), big.NewInt(amount), big190, 3, true},
		{"zero expectedReturn, no floor no max", big.NewInt(0), big.NewInt(amount), big.NewInt(0), 0, true},
		{"nil principal", big.NewInt(1), nil, big190, 3, true},
		{"zero principal", big.NewInt(1), big.NewInt(0), big190, 3, true},
		{"no floor, under max", big.NewInt(1), big.NewInt(amount), big.NewInt(0), 3, false},
		{"no maxRate (unresolved), clears floor", big.NewInt(114098544), big.NewInt(amount), big190, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateYield(c.er, c.amount, c.minPpm, c.maxBps)
			if (err != nil) != c.wantErr {
				t.Fatalf("ValidateYield = %v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}
