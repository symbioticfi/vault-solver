package discounts

import (
	"math/big"
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
			Nonce:         "2",
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

func TestParseSignedAcceptsDecimalUint256Nonce(t *testing.T) {
	for _, nonce := range []string{
		"0002",
		new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1)).String(),
	} {
		t.Run(nonce, func(t *testing.T) {
			parsed, err := ParseSigned(&Resolved{
				DiscountID: "0x" + hash64,
				Discount: Terms{
					Adapter:       "0x0000000000000000000000000000000000000abc",
					TokenToRedeem: "0x0000000000000000000000000000000000000def",
					Discount:      "123",
					Signer:        "0x0000000000000000000000000000000000000aaa",
					Protocol:      "0x0000000000000000000000000000000000000bbb",
					Nonce:         nonce,
					Deadline:      1_900_000_000,
				},
				SignerSignature: "0xdead", ProtocolDeadline: 1_900_000_001, ProtocolSignature: "0xbeef",
			})
			if err != nil {
				t.Fatalf("ParseSigned nonce %q: %v", nonce, err)
			}
			want, ok := new(big.Int).SetString(nonce, 10)
			if !ok || parsed.Terms.Nonce.Cmp(want) != 0 {
				t.Fatalf("nonce = %s, want %s", parsed.Terms.Nonce, nonce)
			}
		})
	}
}

func TestParseUint256DecimalRejectsInvalidValues(t *testing.T) {
	overflow := new(big.Int).Lsh(big.NewInt(1), 256).String()
	for _, nonce := range []string{"0x2", "+2", "-2", "2.0", overflow} {
		if _, err := parseUint256Decimal(nonce, "nonce"); err == nil {
			t.Fatalf("parseUint256Decimal(%q) succeeded, want error", nonce)
		}
	}
}

func TestParseSignedAcceptsZeroAndRejectsOutOfRangeDiscount(t *testing.T) {
	resolved := &Resolved{
		DiscountID: "0x" + hash64,
		Discount: Terms{
			Adapter:       "0x0000000000000000000000000000000000000abc",
			TokenToRedeem: "0x0000000000000000000000000000000000000def",
			Discount:      "0", Signer: "0x0000000000000000000000000000000000000aaa",
			Protocol: "0x0000000000000000000000000000000000000bbb", Nonce: "2", Deadline: 1_900_000_000,
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
