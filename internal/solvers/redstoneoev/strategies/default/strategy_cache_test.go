package defaultstrategy

import (
	"math/big"
	"testing"
)

func TestDecisionStateCacheClonesCallbackNative(t *testing.T) {
	var cache decisionStateCache
	callbackNative := big.NewInt(100)

	cache.store(decisionState{CallbackNative: callbackNative})
	callbackNative.SetInt64(1)

	got, ok := cache.load()
	if !ok {
		t.Fatal("expected cached state")
	}
	if got.CallbackNative.String() != "100" {
		t.Fatalf("callbackNative = %s, want 100", got.CallbackNative)
	}

	got.CallbackNative.SetInt64(2)
	again, ok := cache.load()
	if !ok {
		t.Fatal("expected cached state")
	}
	if again.CallbackNative.String() != "100" {
		t.Fatalf("callbackNative after load mutation = %s, want 100", again.CallbackNative)
	}
}
