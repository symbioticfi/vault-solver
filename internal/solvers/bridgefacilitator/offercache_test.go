package bridgefacilitator

import (
	"testing"
	"time"
)

func TestOfferTracker(t *testing.T) {
	tr := newOfferTracker()
	now := time.Unix(1_000_000, 0)

	if tr.hasLive(42, now) {
		t.Fatal("empty tracker should report no live offer")
	}

	tr.record(42, now.Add(30*time.Minute))
	if !tr.hasLive(42, now) {
		t.Fatal("offer should be live before expiry")
	}
	if tr.hasLive(42, now.Add(31*time.Minute)) {
		t.Fatal("offer should be expired after its TTL")
	}
	if tr.hasLive(7, now) {
		t.Fatal("unknown auction should not be live")
	}
}

func TestParseUnixTime(t *testing.T) {
	got, err := parseUnixTime("4102444800")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Unix() != 4_102_444_800 {
		t.Fatalf("got %d", got.Unix())
	}
	if _, err := parseUnixTime("not-a-number"); err == nil {
		t.Fatal("expected error on non-numeric input")
	}
}
