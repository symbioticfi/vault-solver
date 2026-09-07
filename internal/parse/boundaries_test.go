package parse

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestRejectAmbiguousDecimal(t *testing.T) {
	for _, value := range []string{"", ".", "-0.1", "+1", "1.2.3", "1e3", " 1"} {
		if _, err := EthToWei(value, "amount"); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}

func TestYAMLRejectsTrailingDocument(t *testing.T) {
	var out struct {
		Value int `yaml:"value"`
	}
	if err := YAML(strings.NewReader("value: 1\n---\nvalue: 2\n"), &out); err == nil {
		t.Fatal("accepted a second configuration document")
	}
}

func TestMillisecondOverflow(t *testing.T) {
	value := int(math.MaxInt64/int64(time.Millisecond)) + 1
	if _, err := MsDuration(&value, time.Second, "interval"); err == nil {
		t.Fatal("accepted overflowing duration")
	}
}

func TestUintBoundsAndGrammar(t *testing.T) {
	for _, tc := range []struct {
		input string
		bits  int
		valid bool
	}{
		{"0", 256, true}, {"255", 8, true}, {"256", 8, false},
		{"", 256, false}, {"+1", 256, false}, {"-0", 256, false},
		{" 1", 256, false}, {"1e2", 256, false}, {"0x1", 256, false},
	} {
		t.Run(tc.input, func(t *testing.T) {
			_, err := Uint(tc.input, "amount", tc.bits)
			if (err == nil) != tc.valid {
				t.Fatalf("Uint(%q, %d) = %v", tc.input, tc.bits, err)
			}
		})
	}
}

func TestAddressesRejectCaseAlias(t *testing.T) {
	_, err := Addresses([]string{"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}, "adapters")
	if err == nil {
		t.Fatal("expected duplicate canonical address rejection")
	}
}

func TestHarnessBool(t *testing.T) {
	for _, raw := range []string{"true", "1", " TRUE "} {
		if value, err := Bool(raw, "test"); err != nil || !value {
			t.Fatalf("%q: %v, %v", raw, value, err)
		}
	}
	for _, raw := range []string{"", "false", "0", " FALSE "} {
		if value, err := Bool(raw, "test"); err != nil || value {
			t.Fatalf("%q: %v, %v", raw, value, err)
		}
	}
	if _, err := Bool("ture", "test"); err == nil {
		t.Fatal("accepted misspelled harness flag")
	}
}
