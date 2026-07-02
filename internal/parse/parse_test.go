package parse

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestAddress(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    common.Address
		wantErr bool
	}{
		{name: "valid", in: "0x1111111111111111111111111111111111111111", want: common.HexToAddress("0x1111111111111111111111111111111111111111")},
		{name: "zero", in: "0x0000000000000000000000000000000000000000", want: common.Address{}},
		{name: "no prefix", in: "1111111111111111111111111111111111111111", want: common.HexToAddress("0x1111111111111111111111111111111111111111")},
		{name: "too short", in: "0x1234", wantErr: true},
		{name: "empty", in: "", wantErr: true},
		{name: "not hex", in: "0xZZZZ111111111111111111111111111111111111", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Address(tt.in, "field")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Address(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("Address(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestNonZeroAddress(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    common.Address
		wantErr bool
	}{
		{name: "valid", in: "0x2222222222222222222222222222222222222222", want: common.HexToAddress("0x2222222222222222222222222222222222222222")},
		{name: "zero address", in: "0x0000000000000000000000000000000000000000", wantErr: true},
		{name: "invalid", in: "0xnope", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NonZeroAddress(tt.in, "field")
			if (err != nil) != tt.wantErr {
				t.Fatalf("NonZeroAddress(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("NonZeroAddress(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestHash(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "valid", in: "0x000000000000000000000000000000000000000000000000000000000000beef"},
		{name: "no prefix", in: "000000000000000000000000000000000000000000000000000000000000beef", wantErr: true},
		{name: "too short", in: "0xbeef", wantErr: true},
		{name: "too long", in: "0x00000000000000000000000000000000000000000000000000000000000000beef", wantErr: true},
		{name: "empty", in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Hash(tt.in, "field")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Hash(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err == nil && got != common.HexToHash(tt.in) {
				t.Fatalf("Hash(%q) = %v, want %v", tt.in, got, common.HexToHash(tt.in))
			}
		})
	}
}

func TestBig(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    *big.Int
		wantErr bool
	}{
		{name: "positive", in: "12345", want: big.NewInt(12345)},
		{name: "zero", in: "0", want: big.NewInt(0)},
		{name: "negative", in: "-7", want: big.NewInt(-7)},
		{name: "not a number", in: "abc", wantErr: true},
		{name: "empty", in: "", wantErr: true},
		{name: "hex rejected", in: "0x10", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Big(tt.in, "field")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Big(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err == nil && got.Cmp(tt.want) != 0 {
				t.Fatalf("Big(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestEthToWei(t *testing.T) {
	mustBig := func(s string) *big.Int {
		n, _ := new(big.Int).SetString(s, 10)
		return n
	}
	tests := []struct {
		name    string
		in      string
		want    *big.Int
		wantErr bool
	}{
		{name: "fractional", in: "0.0005", want: mustBig("500000000000000")},
		{name: "whole", in: "1", want: mustBig("1000000000000000000")},
		{name: "zero", in: "0", want: big.NewInt(0)},
		{name: "trailing dot", in: "5.", want: mustBig("5000000000000000000")},
		{name: "leading dot", in: ".5", want: mustBig("500000000000000000")},
		{name: "18 decimals", in: "0.000000000000000001", want: big.NewInt(1)},
		{name: "more than 18 decimals", in: "0.0000000000000000001", wantErr: true},
		{name: "negative", in: "-1", wantErr: true},
		{name: "garbage", in: "abc", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EthToWei(tt.in, "field")
			if (err != nil) != tt.wantErr {
				t.Fatalf("EthToWei(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err == nil && got.Cmp(tt.want) != 0 {
				t.Fatalf("EthToWei(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestOrDefault(t *testing.T) {
	if got := OrDefault("", "fallback"); got != "fallback" {
		t.Fatalf("OrDefault empty string = %q, want fallback", got)
	}
	if got := OrDefault("set", "fallback"); got != "set" {
		t.Fatalf("OrDefault non-empty string = %q, want set", got)
	}
	if got := OrDefault(0, 42); got != 42 {
		t.Fatalf("OrDefault zero int = %d, want 42", got)
	}
	if got := OrDefault(7, 42); got != 7 {
		t.Fatalf("OrDefault non-zero int = %d, want 7", got)
	}
}

func TestMsDuration(t *testing.T) {
	fallback := 3 * time.Second
	ptr := func(i int) *int { return &i }
	tests := []struct {
		name    string
		in      *int
		want    time.Duration
		wantErr bool
	}{
		{name: "nil uses fallback", in: nil, want: fallback},
		{name: "positive", in: ptr(1500), want: 1500 * time.Millisecond},
		{name: "zero rejected", in: ptr(0), wantErr: true},
		{name: "negative rejected", in: ptr(-5), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MsDuration(tt.in, fallback, "field")
			if (err != nil) != tt.wantErr {
				t.Fatalf("MsDuration err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("MsDuration = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDuration(t *testing.T) {
	fallback := 10 * time.Second
	tests := []struct {
		name    string
		in      string
		want    time.Duration
		wantErr bool
	}{
		{name: "empty uses fallback", in: "", want: fallback},
		{name: "valid", in: "2m", want: 2 * time.Minute},
		{name: "invalid", in: "notaduration", wantErr: true},
		{name: "zero rejected", in: "0s", wantErr: true},
		{name: "negative rejected", in: "-1s", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Duration(tt.in, fallback, "field")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Duration(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("Duration(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
