package iris

import (
	"math/big"
	"reflect"
	"testing"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func fails(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
}

func ok(t *testing.T, condition bool, context ...any) {
	t.Helper()
	if !condition {
		t.Fatal(append([]any{"condition failed"}, context...)...)
	}
}

// equal compares big integers by value and everything else deeply.
func equal[T any](t *testing.T, want, got T, context ...any) {
	t.Helper()
	if w, isBig := any(want).(*big.Int); isBig {
		if g := any(got).(*big.Int); (w == nil) != (g == nil) || (w != nil && w.Cmp(g) != 0) {
			t.Fatal(append([]any{"got", g, "want", w}, context...)...)
		}
		return
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatal(append([]any{"got", got, "want", want}, context...)...)
	}
}
