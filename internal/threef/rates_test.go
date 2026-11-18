package threef

import (
	"math/big"
	"testing"
)

func TestReturnBounds(t *testing.T) {
	for _, tc := range []struct {
		name             string
		principal, rate  string
		basis            int64
		minimum, maximum string
	}{
		{"annual", "100000000", "100", YearSeconds, "250000", "1000000"},
		{"daily", "365000000", "100", 86400, "2500", "10000"},
		{"omitted basis", "100000000", "100", 0, "250000", "1000000"},
		{"rounding", "100001", "100", YearSeconds, "251", "1000"},
		{"decimal APR", "100000000000000000000001", "1.9", YearSeconds, "4750000000000000001", "19000000000000000000"},
		{"dust cannot fit", "1", "100", YearSeconds, "1", "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			principal, _ := new(big.Int).SetString(tc.principal, 10)
			rate, _ := new(big.Rat).SetString(tc.rate)
			originalRate := new(big.Rat).Set(rate)
			minimum, maximum, err := ReturnBounds(principal, rate, tc.basis)
			if err != nil || minimum.String() != tc.minimum || maximum.String() != tc.maximum {
				t.Fatalf("bounds = %v, %v, %v; want %s, %s", minimum, maximum, err, tc.minimum, tc.maximum)
			}
			if principal.String() != tc.principal || rate.Cmp(originalRate) != 0 {
				t.Fatal("mutated input")
			}
		})
	}
}

func TestReturnBoundsRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name      string
		principal *big.Int
		rate      *big.Rat
		basis     int64
	}{
		{"missing principal", nil, big.NewRat(100, 1), YearSeconds},
		{"zero principal", new(big.Int), big.NewRat(100, 1), YearSeconds},
		{"missing rate", big.NewInt(100), nil, YearSeconds},
		{"zero rate", big.NewInt(100), new(big.Rat), YearSeconds},
		{"negative rate", big.NewInt(100), big.NewRat(-100, 1), YearSeconds},
		{"negative basis", big.NewInt(100), big.NewRat(100, 1), -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := ReturnBounds(tc.principal, tc.rate, tc.basis); err == nil {
				t.Fatal("invalid inputs accepted")
			}
		})
	}
}
