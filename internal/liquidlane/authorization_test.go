package liquidlane

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestUnauthorizedAdapters(t *testing.T) {
	adapterA := common.HexToAddress("0x000000000000000000000000000000000000000a")
	adapterB := common.HexToAddress("0x000000000000000000000000000000000000000b")
	adapterC := common.HexToAddress("0x000000000000000000000000000000000000000c")
	routes := []Route{
		{Adapter: adapterA},
		{Adapter: adapterB},
		{Adapter: adapterB},
		{Adapter: adapterC},
	}

	got := UnauthorizedAdapters(routes, []Route{{Adapter: adapterB}})
	want := []common.Address{adapterA, adapterC}
	if !slices.Equal(got, want) {
		t.Fatalf("UnauthorizedAdapters() = %v, want %v", got, want)
	}
}

func TestValidateDirectAuthorizationNamesDrops(t *testing.T) {
	adapter := common.HexToAddress("0x000000000000000000000000000000000000000a")
	routes := []Route{{Adapter: adapter}}
	dropping := func(_ context.Context, _ []Route, _ common.Address, trace ...DropTrace) ([]Route, error) {
		for _, tr := range trace {
			tr(Drop{Adapter: adapter, Reason: "owner reverted: InvalidInitialization()"})
		}
		return nil, nil
	}
	err := ValidateDirectAuthorization(t.Context(), dropping, routes, common.Address{1})
	if err == nil || !strings.Contains(err.Error(), "is not authorized as direct filler") || !strings.Contains(err.Error(), "owner reverted: InvalidInitialization()") {
		t.Fatalf("err = %v, want the adapter and its dropped read", err)
	}
	allowed := func(context.Context, []Route, common.Address, ...DropTrace) ([]Route, error) { return routes, nil }
	if err := ValidateDirectAuthorization(t.Context(), allowed, routes, common.Address{1}); err != nil {
		t.Fatalf("authorized err = %v", err)
	}
}
