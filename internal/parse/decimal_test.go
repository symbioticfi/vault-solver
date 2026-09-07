package parse

import (
	"math/big"
	"testing"
)

func TestNonnegativeDecimal(t *testing.T) {
	for _, test := range []struct {
		raw             string
		optional, valid bool
		want            string
	}{
		{"", true, true, ""}, {"", false, false, ""}, {"0", false, true, "0"},
		{"-1", true, false, ""}, {"bad", false, false, ""},
		{"123456789012345678901234567890", false, true, "123456789012345678901234567890"},
	} {
		value, err := NonnegativeDecimal(test.raw, "amount", test.optional)
		if (err == nil) != test.valid {
			t.Fatalf("raw=%q optional=%v: error=%v", test.raw, test.optional, err)
		}
		if err == nil && DecimalText(value, "") != test.want {
			t.Fatalf("raw=%q: got=%v", test.raw, value)
		}
	}
	if DecimalText(nil, "0") != "0" || DecimalText(big.NewInt(5), "0") != "5" {
		t.Fatal("decimal wire representation changed")
	}
}
