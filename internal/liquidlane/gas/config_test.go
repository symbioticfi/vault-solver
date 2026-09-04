package gas

import (
	"testing"
	"time"
)

func TestParseOptionalConfig(t *testing.T) {
	if got, err := ParseOptionalConfig(nil); err != nil || got != nil {
		t.Fatalf("omitted config = %v, %v", got, err)
	}
	got, err := ParseOptionalConfig(&RawConfig{
		NativeUSDFeed: "0x1111111111111111111111111111111111111111",
		NativeMaxAge:  "1m",
		TokenUSDFeeds: []RawTokenFeed{{
			Token: "0x2222222222222222222222222222222222222222",
			Feed:  "0x3333333333333333333333333333333333333333", MaxAge: "2m",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.NativeUSDFeed.MaxAge != time.Minute || len(got.TokenUSDFeeds) != 1 {
		t.Fatalf("parsed config = %+v", got)
	}
}
