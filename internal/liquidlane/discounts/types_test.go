package discounts

import (
	"strings"
	"testing"
)

func TestParseOffer(t *testing.T) {
	offer, err := ParseOffer(ListItem{
		DiscountID:         "0x" + hash64,
		Adapter:            "0x0000000000000000000000000000000000000abc",
		TokenToRedeem:      "0x0000000000000000000000000000000000000def",
		Collateral:         "0x0000000000000000000000000000000000000c01",
		CollateralDecimals: 6,
		Discount:           "100000",
		Deadline:           1_900_000_000,
		MaxRate:            "1000000000000000000",
		MaxAssets:          "5000",
	})
	if err != nil {
		t.Fatalf("ParseOffer: %v", err)
	}
	if offer.MaxAssets.String() != "5000" || offer.Discount.String() != "100000" || offer.CollateralDecimals != 6 {
		t.Fatalf("offer = %+v", offer)
	}
}

func TestParseOfferRejectsInvalidDiscount(t *testing.T) {
	item := ListItem{
		DiscountID:         "0x" + hash64,
		Adapter:            "0x0000000000000000000000000000000000000abc",
		TokenToRedeem:      "0x0000000000000000000000000000000000000def",
		Collateral:         "0x0000000000000000000000000000000000000c01",
		CollateralDecimals: 6,
		Discount:           "1000001",
		Deadline:           1_900_000_000,
		MaxRate:            "1000000000000000000",
		MaxAssets:          "5000",
	}
	if _, err := ParseOffer(item); err == nil {
		t.Fatal("expected out-of-range discount error")
	}
	item.Discount = ""
	if _, err := ParseOffer(item); err == nil {
		t.Fatal("expected missing discount error")
	}
}

func TestParseOfferRejectsMalformedIDAndExpiredShape(t *testing.T) {
	item := ListItem{
		DiscountID:         "0x01",
		Adapter:            "0x0000000000000000000000000000000000000abc",
		TokenToRedeem:      "0x0000000000000000000000000000000000000def",
		Collateral:         "0x0000000000000000000000000000000000000c01",
		CollateralDecimals: 6,
		Discount:           "100000",
		Deadline:           1_900_000_000,
		MaxRate:            "1",
		MaxAssets:          "1",
	}
	if _, err := ParseOffer(item); err == nil {
		t.Fatal("expected malformed id error")
	}
	item.DiscountID = "0x" + hash64
	item.Deadline = 0
	if _, err := ParseOffer(item); err == nil {
		t.Fatal("expected deadline error")
	}
	item.Deadline = 1_900_000_000
	item.DiscountID = "0x" + strings.Repeat("0", 64)
	if _, err := ParseOffer(item); err == nil {
		t.Fatal("expected zero id error")
	}
}

func TestParseSigned(t *testing.T) {
	parsed, err := ParseSigned(&Resolved{
		DiscountID: "0x" + hash64,
		Discount: Terms{
			Adapter:       "0x0000000000000000000000000000000000000abc",
			TokenToRedeem: "0x0000000000000000000000000000000000000def",
			Discount:      "123",
			Signer:        "0x0000000000000000000000000000000000000aaa",
			Protocol:      "0x0000000000000000000000000000000000000bbb",
			Nonce:         "0x2",
			Deadline:      1_900_000_000,
		},
		SignerSignature: "0xdead", ProtocolDeadline: 1_900_000_001, ProtocolSignature: "0xbeef",
	})
	if err != nil {
		t.Fatalf("ParseSigned: %v", err)
	}
	if parsed.Terms.Discount.String() != "123" || parsed.Terms.Nonce.String() != "2" {
		t.Fatalf("parsed = %+v", parsed)
	}
}

func TestParseSignedAcceptsZeroAndRejectsOutOfRangeDiscount(t *testing.T) {
	resolved := &Resolved{
		DiscountID: "0x" + hash64,
		Discount: Terms{
			Adapter:       "0x0000000000000000000000000000000000000abc",
			TokenToRedeem: "0x0000000000000000000000000000000000000def",
			Discount:      "0", Signer: "0x0000000000000000000000000000000000000aaa",
			Protocol: "0x0000000000000000000000000000000000000bbb", Nonce: "0x2", Deadline: 1_900_000_000,
		},
		SignerSignature: "0xdead", ProtocolDeadline: 1_900_000_001, ProtocolSignature: "0xbeef",
	}
	if _, err := ParseSigned(resolved); err != nil {
		t.Fatalf("zero discount: %v", err)
	}
	resolved.Discount.Discount = "1000001"
	if _, err := ParseSigned(resolved); err == nil {
		t.Fatal("expected out-of-range discount error")
	}
}

// The backend's discount nonce is a base-10 uint256 string with leading zeroes accepted. Parsing it
// as hex rejected every such value ("hex number with leading zero digits"), so discounts silently
// stopped resolving in production. 0x-prefixed values stay accepted for a backend on the older
// hex contract; the prefix keeps the two unambiguous.
func TestParseDiscountNonceAcceptsBase10AndLegacyHex(t *testing.T) {
	for _, test := range []struct {
		raw     string
		want    int64
		wantErr bool
	}{
		{raw: "2", want: 2},
		{raw: "0", want: 0},
		{raw: "0123", want: 123},
		{raw: "115792089237316195423570985008687907853269984665640564039457584007913129639935", want: -1},
		{raw: "0x2", want: 2},
		{raw: "0xff", want: 255},
		{raw: "", wantErr: true},
		{raw: "12ab", wantErr: true},
		{raw: "-1", wantErr: true},
	} {
		t.Run(test.raw, func(t *testing.T) {
			got, err := parseDiscountNonce(test.raw)
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseDiscountNonce(%q) = %v; want an error", test.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDiscountNonce(%q): %v", test.raw, err)
			}
			if test.want >= 0 && got.Int64() != test.want {
				t.Fatalf("parseDiscountNonce(%q) = %s; want %d", test.raw, got, test.want)
			}
		})
	}
}
